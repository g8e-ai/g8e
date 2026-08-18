// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/hkdf"

	"github.com/g8e-ai/g8e/internal/constants"
)

const (
	KeySize              = 32
	NonceSize            = 12
	KeyFingerprintSize   = 16
	HKDFInfo             = "g8e-lfaa-kek-v1"
	KeyFingerprintPepper = "g8e-vault-fingerprint-v1"

	aesKWDefaultIVHigh = 0xA6A6A6A6
	aesKWDefaultIVLow  = 0xA6A6A6A6
)

// DeriveKEK derives a Key Encryption Key from private key bytes using HKDF-SHA256.
func DeriveKEK(privateKey []byte) ([]byte, error) {
	if len(privateKey) == 0 {
		return nil, constants.ErrVaultPrivateKeyEmpty
	}

	reader := hkdf.New(sha256.New, privateKey, nil, []byte(HKDFInfo))

	kek := make([]byte, KeySize)
	if _, err := io.ReadFull(reader, kek); err != nil {
		return nil, fmt.Errorf("HKDF expansion failed: %w", err)
	}

	return kek, nil
}

// GenerateDEK generates a cryptographically secure random Data Encryption Key.
func GenerateDEK() ([]byte, error) {
	dek := make([]byte, KeySize)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("failed to generate random DEK: %w", err)
	}
	return dek, nil
}

// GenerateNonce generates a cryptographically secure random nonce for AES-GCM.
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate random nonce: %w", err)
	}
	return nonce, nil
}

// KeyFingerprint returns the first KeyFingerprintSize bytes of a SHA-256 hash
// of the key combined with a domain-separation pepper. Fingerprints are
// identifiers for key comparison, not secrets — the key material itself
// (256-bit ed25519) provides the entropy, so a fast hash is appropriate.
func KeyFingerprint(key []byte) []byte {
	h := sha256.New()
	h.Write([]byte(KeyFingerprintPepper))
	h.Write(key)
	return h.Sum(nil)[:KeyFingerprintSize]
}

// PrivateKeyFingerprint returns the fingerprint of a private key.
func PrivateKeyFingerprint(privateKey []byte) []byte {
	return KeyFingerprint(privateKey)
}

// AESKeyWrap wraps a plaintext key using AES Key Wrap (RFC 3394).
// The KEK must be 16, 24, or 32 bytes (AES-128, AES-192, or AES-256).
// The plaintext must be a multiple of 8 bytes and at least 16 bytes.
//
// RFC 3394 algorithm:
//   - For j = 0 to 5:
//   - For i = 1 to n:
//   - B = AES(K, A | R[i])
//   - A = MSB(64, B) ^ t where t = (n*j)+i
//   - R[i] = LSB(64, B)
func AESKeyWrap(kek, plaintext []byte) ([]byte, error) {
	if len(kek) != 16 && len(kek) != 24 && len(kek) != 32 {
		return nil, constants.ErrVaultInvalidKeySize
	}

	n := len(plaintext)
	if n < 16 || n%8 != 0 {
		return nil, constants.ErrVaultInvalidPlaintextKey
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	numBlocks := n / 8

	a := make([]byte, 8)
	binary.BigEndian.PutUint32(a[0:4], aesKWDefaultIVHigh)
	binary.BigEndian.PutUint32(a[4:8], aesKWDefaultIVLow)

	r := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		r[i] = make([]byte, 8)
		copy(r[i], plaintext[i*8:(i+1)*8])
	}

	buf := make([]byte, 16)

	for j := 0; j <= 5; j++ {
		for i := 1; i <= numBlocks; i++ {
			copy(buf[0:8], a)
			copy(buf[8:16], r[i-1])
			block.Encrypt(buf, buf)

			t := uint64(numBlocks*j + i)
			copy(a, buf[0:8])
			for k := 7; k >= 0 && t > 0; k-- {
				a[k] ^= byte(t & 0xFF)
				t >>= 8
			}

			copy(r[i-1], buf[8:16])
		}
	}

	ciphertext := make([]byte, 8+n)
	copy(ciphertext[0:8], a)
	for i := 0; i < numBlocks; i++ {
		copy(ciphertext[8+i*8:8+(i+1)*8], r[i])
	}

	return ciphertext, nil
}

// AESKeyUnwrap unwraps a ciphertext using AES Key Unwrap (RFC 3394).
// Returns the original plaintext key if the integrity check passes.
//
// RFC 3394 algorithm:
//   - For j = 5 to 0:
//   - For i = n to 1:
//   - B = AES^-1(K, (A ^ t) | R[i])
//   - A = MSB(64, B)
//   - R[i] = LSB(64, B)
func AESKeyUnwrap(kek, ciphertext []byte) ([]byte, error) {
	if len(kek) != 16 && len(kek) != 24 && len(kek) != 32 {
		return nil, constants.ErrVaultInvalidKeySize
	}

	n := len(ciphertext)
	if n < 24 || n%8 != 0 {
		return nil, constants.ErrVaultInvalidWrappedKey
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	numBlocks := (n / 8) - 1

	a := make([]byte, 8)
	copy(a, ciphertext[0:8])

	r := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		r[i] = make([]byte, 8)
		copy(r[i], ciphertext[8+i*8:8+(i+1)*8])
	}

	buf := make([]byte, 16)

	for j := 5; j >= 0; j-- {
		for i := numBlocks; i >= 1; i-- {
			t := uint64(numBlocks*j + i)
			aCopy := make([]byte, 8)
			copy(aCopy, a)
			for k := 7; k >= 0 && t > 0; k-- {
				aCopy[k] ^= byte(t & 0xFF)
				t >>= 8
			}

			copy(buf[0:8], aCopy)
			copy(buf[8:16], r[i-1])
			block.Decrypt(buf, buf)

			copy(a, buf[0:8])

			copy(r[i-1], buf[8:16])
		}
	}

	expectedA := make([]byte, 8)
	binary.BigEndian.PutUint32(expectedA[0:4], aesKWDefaultIVHigh)
	binary.BigEndian.PutUint32(expectedA[4:8], aesKWDefaultIVLow)

	if subtle.ConstantTimeCompare(a, expectedA) != 1 {
		return nil, constants.ErrVaultKeyUnwrapFailed
	}

	plaintext := make([]byte, numBlocks*8)
	for i := 0; i < numBlocks; i++ {
		copy(plaintext[i*8:(i+1)*8], r[i])
	}

	return plaintext, nil
}

// EncryptAESGCM encrypts plaintext using AES-256-GCM with the provided key and nonce.
func EncryptAESGCM(key, nonce, plaintext, additionalData []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, constants.ErrVaultInvalidKeySize
	}
	if len(nonce) != NonceSize {
		return nil, constants.ErrVaultInvalidNonceSize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, additionalData)
	return ciphertext, nil
}

// DecryptAESGCM decrypts ciphertext using AES-256-GCM with the provided key and nonce.
func DecryptAESGCM(key, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, constants.ErrVaultInvalidKeySize
	}
	if len(nonce) != NonceSize {
		return nil, constants.ErrVaultInvalidNonceSize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, constants.ErrVaultDecryptionFailed
	}

	return plaintext, nil
}

// SecureZero zeros out a byte slice to prevent key material from lingering in memory.
func SecureZero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ReadVaultKey reads and decodes the vault private key from a file.
// The key file should contain a hex-encoded private key.
func ReadVaultKey(keyPath string) ([]byte, error) {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	keyHex := strings.TrimSpace(string(data))
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid key size: expected %d bytes, got %d", KeySize, len(key))
	}
	return key, nil
}
