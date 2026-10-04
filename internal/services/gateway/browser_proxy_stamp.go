// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// BrowserProxyStamp is the identity assertion the Gateway signs for one
// proxied browser request. g8ee rebuilds the same value from the request it
// actually received, so a stamp is only valid for that exact method, target,
// body, identity, and moment. The canonical encoding is shared with g8ee and
// pinned by protocol/conformance/browser_proxy_stamp_vectors.json.
type BrowserProxyStamp struct {
	Method         string
	RequestURI     string // upstream path plus raw query, as sent on the wire
	BodySHA256     string // lowercase hex SHA-256 of the forwarded body
	UserID         string
	UserEmail      string
	WebSessionID   string
	OrganizationID string
	CLISessionID   string
	IssuedAtUnix   int64
	Nonce          string
}

// CanonicalBytes returns the bytes that are signed: the domain tag followed by
// every field, one per line. A field containing a line break could shift
// values between positions, so such a stamp is refused rather than escaped.
func (s BrowserProxyStamp) CanonicalBytes() ([]byte, error) {
	fields := []string{
		constants.BrowserProxyStampDomain,
		s.Method,
		s.RequestURI,
		s.BodySHA256,
		s.UserID,
		s.UserEmail,
		s.WebSessionID,
		s.OrganizationID,
		s.CLISessionID,
		strconv.FormatInt(s.IssuedAtUnix, 10),
		s.Nonce,
	}
	for _, f := range fields {
		if strings.ContainsAny(f, "\r\n") {
			return nil, constants.ErrBrowserProxyStampFieldInvalid
		}
	}
	return []byte(strings.Join(fields, "\n")), nil
}

// BrowserProxyIdentity is the identity the Gateway vouches for on a proxied
// request. Organization is never vouched for; it is signed as empty so g8ee
// cannot be handed unsigned values for it.
type BrowserProxyIdentity struct {
	UserID       string
	UserEmail    string
	WebSessionID string
	CLISessionID string
}

// BrowserProxySigner signs browser-proxy stamps with the Gateway's Actuator key.
// The key is the one g8ee already learns the public half of from the Gateway;
// the stamp domain tag keeps these signatures distinct from receipt signatures.
type BrowserProxySigner struct {
	priv  ed25519.PrivateKey
	keyID string
	now   func() time.Time
	rand  io.Reader
}

// NewBrowserProxySigner returns a signer, or an error if the key or key ID is
// unusable. Callers treat an error as fatal: there is no unsigned mode.
func NewBrowserProxySigner(priv ed25519.PrivateKey, keyID string) (*BrowserProxySigner, error) {
	if len(priv) != ed25519.PrivateKeySize || strings.TrimSpace(keyID) == "" {
		return nil, constants.ErrBrowserProxySignerUnavailable
	}
	return &BrowserProxySigner{priv: priv, keyID: keyID, now: time.Now, rand: rand.Reader}, nil
}

// KeyID returns the identifier g8ee uses to select the verification key.
func (s *BrowserProxySigner) KeyID() string { return s.keyID }

// PublicKey returns the verification key.
func (s *BrowserProxySigner) PublicKey() ed25519.PublicKey {
	return s.priv.Public().(ed25519.PublicKey)
}

func (s *BrowserProxySigner) sign(stamp BrowserProxyStamp) (string, error) {
	canonical, err := stamp.CanonicalBytes()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(ed25519.Sign(s.priv, canonical)), nil
}

// Apply signs req as it will be sent and sets the identity and signature
// headers. body must be the exact bytes req carries. Identity headers are
// rewritten from id alone: anything already on req for them is discarded.
func (s *BrowserProxySigner) Apply(req *http.Request, body []byte, id BrowserProxyIdentity) error {
	nonce := make([]byte, 16)
	if _, err := io.ReadFull(s.rand, nonce); err != nil {
		return fmt.Errorf("%w: nonce: %w", constants.ErrBrowserProxySignerUnavailable, err)
	}
	sum := sha256.Sum256(body)
	stamp := BrowserProxyStamp{
		Method:       req.Method,
		RequestURI:   req.URL.RequestURI(),
		BodySHA256:   hex.EncodeToString(sum[:]),
		UserID:       id.UserID,
		UserEmail:    id.UserEmail,
		WebSessionID: id.WebSessionID,
		CLISessionID: id.CLISessionID,
		IssuedAtUnix: s.now().Unix(),
		Nonce:        hex.EncodeToString(nonce),
	}
	signature, err := s.sign(stamp)
	if err != nil {
		return err
	}

	req.Header.Set(constants.HeaderProxyUserID, stamp.UserID)
	req.Header.Set(constants.HeaderProxyUserEmail, stamp.UserEmail)
	if stamp.WebSessionID != "" {
		req.Header.Set(constants.HeaderProxyWebSessionID, stamp.WebSessionID)
	} else {
		req.Header.Del(constants.HeaderProxyWebSessionID)
	}
	if stamp.CLISessionID != "" {
		req.Header.Set(constants.HeaderProxyCLISessionID, stamp.CLISessionID)
	} else {
		req.Header.Del(constants.HeaderProxyCLISessionID)
	}
	req.Header.Del(constants.HeaderProxyOrganizationID)
	req.Header.Set(constants.HeaderProxyKeyID, s.keyID)
	req.Header.Set(constants.HeaderProxyIssuedAt, strconv.FormatInt(stamp.IssuedAtUnix, 10))
	req.Header.Set(constants.HeaderProxyNonce, stamp.Nonce)
	req.Header.Set(constants.HeaderProxySignature, signature)
	return nil
}
