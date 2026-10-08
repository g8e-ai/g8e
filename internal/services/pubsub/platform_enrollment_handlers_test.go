// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

const (
	enrollTestOwnerID  = "user-owner"
	enrollTestOrgID    = "org-1"
	enrollTestRequest  = "req-1"
	enrollTestEnvelope = "env-1"
	enrollTestCSR      = "-----BEGIN CERTIFICATE REQUEST-----\nsecret-csr-body\n-----END CERTIFICATE REQUEST-----"
)

var errEnrollTestBoom = errors.New("boom")

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// enrollTestDocStore is an in-memory PlatformEnrollmentDocStore that evaluates
// equality filters and conditional updates the way the real document store
// does, so handler state transitions are asserted against observable state
// rather than against recorded calls.
type enrollTestDocStore struct {
	mu   sync.Mutex
	docs map[string]map[string]*models.Document

	getErr   map[string]error // keyed by collection
	setErr   error
	queryErr error
	condErr  error
	updErr   error
	delErr   error

	// condApplied forces DocConditionalUpdate to report (not-)applied,
	// simulating a concurrent writer that wins the compare-and-set.
	condApplied *bool

	condCalls []enrollTestCondCall
	deletes   []string
	enrolled  []string
}

type enrollTestCondCall struct {
	Collection string
	ID         string
	Field      string
	Value      interface{}
	Set        map[string]json.RawMessage
}

func newEnrollTestDocStore() *enrollTestDocStore {
	return &enrollTestDocStore{
		docs:   map[string]map[string]*models.Document{},
		getErr: map[string]error{},
	}
}

func (s *enrollTestDocStore) seed(t *testing.T, collection, id string, v interface{}) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	s.put(collection, id, fields)
}

func (s *enrollTestDocStore) put(collection, id string, fields map[string]json.RawMessage) {
	if s.docs[collection] == nil {
		s.docs[collection] = map[string]*models.Document{}
	}
	s.docs[collection][id] = &models.Document{ID: id, Collection: collection, Data: fields}
}

func (s *enrollTestDocStore) field(t *testing.T, collection, id, name string) string {
	t.Helper()
	doc := s.docs[collection][id]
	require.NotNil(t, doc, "document %s/%s must exist", collection, id)
	raw, ok := doc.Data[name]
	require.Truef(t, ok, "document %s/%s must have field %q", collection, id, name)
	var out string
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func (s *enrollTestDocStore) DocSet(_ context.Context, collection, id string, data json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setErr != nil {
		return s.setErr
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	s.put(collection, id, fields)
	return nil
}

func (s *enrollTestDocStore) DocGet(_ context.Context, collection, id string) (*models.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.getErr[collection]; err != nil {
		return nil, err
	}
	return s.docs[collection][id], nil
}

func (s *enrollTestDocStore) DocQuery(collection string, filters []models.DocFilter, _ string, _ int) ([]*models.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	var out []*models.Document
	for _, doc := range s.docs[collection] {
		match := true
		for _, f := range filters {
			actual, present := doc.Data[f.Field]
			equal := present && string(actual) == string(f.Value)
			switch f.Op {
			case "==":
				match = match && equal
			case "!=":
				match = match && !equal
			default:
				match = false
			}
		}
		if match {
			out = append(out, doc)
		}
	}
	return out, nil
}

func (s *enrollTestDocStore) FindOperatorLeases(ownerID, systemFingerprint string) ([]*models.Document, error) {
	return s.DocQuery(marshaler.CollectionName(constants.CollectionOperators), []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", ownerID))},
		{Field: "system_fingerprint", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", systemFingerprint))},
		{Field: "operator_type", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorTypeRemote))},
		{Field: "status", Op: "!=", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorStatusTerminated))},
	}, "", 0)
}

func (s *enrollTestDocStore) DocConditionalUpdate(_ context.Context, collection, id string, setFields json.RawMessage, conditionField string, conditionValue interface{}) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var set map[string]json.RawMessage
	if err := json.Unmarshal(setFields, &set); err != nil {
		return false, err
	}
	s.condCalls = append(s.condCalls, enrollTestCondCall{
		Collection: collection, ID: id, Field: conditionField, Value: conditionValue, Set: set,
	})
	if s.condErr != nil {
		return false, s.condErr
	}
	doc := s.docs[collection][id]
	if doc == nil {
		return false, nil
	}
	applied := false
	if raw, ok := doc.Data[conditionField]; ok {
		want, err := json.Marshal(conditionValue)
		if err != nil {
			return false, err
		}
		applied = string(raw) == string(want)
	}
	if s.condApplied != nil {
		applied = *s.condApplied
	}
	if applied {
		for k, v := range set {
			doc.Data[k] = v
		}
	}
	return applied, nil
}

func (s *enrollTestDocStore) DocUpdate(_ context.Context, collection, id string, data json.RawMessage) (*models.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.updErr != nil {
		return nil, s.updErr
	}
	doc := s.docs[collection][id]
	if doc == nil {
		return nil, constants.ErrNotFound
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(data, &patch); err != nil {
		return nil, err
	}
	for k, v := range patch {
		doc.Data[k] = v
	}
	return doc, nil
}

func (s *enrollTestDocStore) DocDelete(_ context.Context, collection, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.delErr != nil {
		return s.delErr
	}
	s.deletes = append(s.deletes, collection+"/"+id)
	delete(s.docs[collection], id)
	return nil
}

func (s *enrollTestDocStore) NotifyOperatorEnrolled(_ context.Context, operatorID, userID, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enrolled = append(s.enrolled, operatorID+"/"+userID+"/"+name)
}

type enrollTestPKI struct {
	trustBundle    []byte
	trustErr       error
	appCertPEM     string
	appChainPEM    string
	appErr         error
	operatorCert   string
	cliCert        string
	signErrForLeaf map[string]error
	revokeErr      error
	revokeErrFor   map[string]error // keyed by serial

	appCalls    []enrollTestAppSign
	signCalls   []enrollTestCSRSign
	revocations []enrollTestRevocation
}

type enrollTestAppSign struct{ CSR, AppName, UserID string }

type enrollTestCSRSign struct {
	CSR, LeafType, OrganizationID, OperatorID, UserID, SessionID, GatewayID string
}

type enrollTestRevocation struct{ Serial, Reason string }

func (p *enrollTestPKI) SignPlatformAppCSR(csrPEM, appName, userID string) (string, string, error) {
	p.appCalls = append(p.appCalls, enrollTestAppSign{CSR: csrPEM, AppName: appName, UserID: userID})
	if p.appErr != nil {
		return "", "", p.appErr
	}
	return p.appCertPEM, p.appChainPEM, nil
}

func (p *enrollTestPKI) SignCSR(csrPEM, leafType, organizationID, operatorID, userID, sessionID, gatewayID string) (string, string, error) {
	p.signCalls = append(p.signCalls, enrollTestCSRSign{
		CSR: csrPEM, LeafType: leafType, OrganizationID: organizationID,
		OperatorID: operatorID, UserID: userID, SessionID: sessionID, GatewayID: gatewayID,
	})
	if err := p.signErrForLeaf[leafType]; err != nil {
		return "", "", err
	}
	if leafType == string(constants.LeafTypeOperator) {
		return p.operatorCert, "operator-chain", nil
	}
	return p.cliCert, "cli-chain", nil
}

func (p *enrollTestPKI) GatewayTrustBundle() ([]byte, error) {
	if p.trustErr != nil {
		return nil, p.trustErr
	}
	return p.trustBundle, nil
}

func (p *enrollTestPKI) RevokeCertificate(_ context.Context, serial, reason string) error {
	p.revocations = append(p.revocations, enrollTestRevocation{Serial: serial, Reason: reason})
	if err := p.revokeErrFor[serial]; err != nil {
		return err
	}
	return p.revokeErr
}

type enrollTestCLISessions struct {
	persistErr    error
	deactivateErr error
	persisted     []enrollTestCLISession
	deactivated   []string
}

type enrollTestCLISession struct {
	CLISessionID, OperatorSessionID, UserID, SystemFingerprint, CertFingerprint, CertSerial, LoginMethod string
}

func (c *enrollTestCLISessions) PersistCLISession(_ context.Context, cliSessionID, operatorSessionID, userID, systemFingerprint, certFingerprint, certSerial, loginMethod string) error {
	if c.persistErr != nil {
		return c.persistErr
	}
	c.persisted = append(c.persisted, enrollTestCLISession{
		CLISessionID: cliSessionID, OperatorSessionID: operatorSessionID, UserID: userID,
		SystemFingerprint: systemFingerprint, CertFingerprint: certFingerprint,
		CertSerial: certSerial, LoginMethod: loginMethod,
	})
	return nil
}

func (c *enrollTestCLISessions) DeactivateCLISession(_ context.Context, sessionID string) error {
	c.deactivated = append(c.deactivated, sessionID)
	return c.deactivateErr
}

type enrollTestOperatorSessions struct {
	persistErr    error
	deactivateErr error
	persisted     []enrollTestOperatorSession
	deactivated   []string
}

type enrollTestOperatorSession struct {
	OperatorSessionID, UserID, OrgID, OperatorID, LoginMethod string
}

func (o *enrollTestOperatorSessions) PersistOperatorSession(_ context.Context, operatorSessionID, userID, orgID, operatorID, loginMethod string) error {
	if o.persistErr != nil {
		return o.persistErr
	}
	o.persisted = append(o.persisted, enrollTestOperatorSession{
		OperatorSessionID: operatorSessionID, UserID: userID, OrgID: orgID,
		OperatorID: operatorID, LoginMethod: loginMethod,
	})
	return nil
}

func (o *enrollTestOperatorSessions) DeactivateOperatorSession(_ context.Context, operatorSessionID string) error {
	o.deactivated = append(o.deactivated, operatorSessionID)
	return o.deactivateErr
}

type enrollTestConnections struct{ disconnected []string }

func (c *enrollTestConnections) DisconnectIdentity(spiffeID string) int {
	c.disconnected = append(c.disconnected, spiffeID)
	return 1
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

type enrollTestEnv struct {
	store     *enrollTestDocStore
	pki       *enrollTestPKI
	cli       *enrollTestCLISessions
	opSession *enrollTestOperatorSessions
	conns     *enrollTestConnections
	handler   *PlatformEnrollmentHandler
}

// newEnrollTestEnv wires a handler to in-memory fakes and seeds the approving
// owner and their organization, which every issuance path requires.
func newEnrollTestEnv(t *testing.T) *enrollTestEnv {
	t.Helper()
	env := &enrollTestEnv{
		store: newEnrollTestDocStore(),
		pki: &enrollTestPKI{
			trustBundle:    []byte("trust-bundle-pem"),
			appCertPEM:     enrollTestCertPEM(t, 4242, "spiffe://g8e.test/app/dashboard", time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)),
			appChainPEM:    "app-chain",
			operatorCert:   enrollTestCertPEM(t, 1001, "spiffe://g8e.test/operator/op", time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)),
			cliCert:        enrollTestCertPEM(t, 1002, "spiffe://g8e.test/cli/owner", time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)),
			signErrForLeaf: map[string]error{},
			revokeErrFor:   map[string]error{},
		},
		cli:       &enrollTestCLISessions{},
		opSession: &enrollTestOperatorSessions{},
		conns:     &enrollTestConnections{},
	}
	env.store.seed(t, marshaler.CollectionName(constants.CollectionUsers), enrollTestOwnerID,
		models.User{OrganizationID: enrollTestOrgID})
	env.store.seed(t, marshaler.CollectionName(constants.CollectionOrganizations), enrollTestOrgID,
		models.Organization{OwnerUserID: enrollTestOwnerID})
	env.handler = newPlatformEnrollmentHandler(PlatformEnrollmentDeps{
		DocStore:         env.store,
		PKI:              env.pki,
		CLISessions:      env.cli,
		OperatorSessions: env.opSession,
		Connections:      env.conns,
		Posture:          "consensus",
	}, slog.New(slog.DiscardHandler))
	return env
}

func (e *enrollTestEnv) seedRequest(t *testing.T, req models.PlatformEnrollmentRequest) {
	t.Helper()
	if req.ID == "" {
		req.ID = enrollTestRequest
	}
	e.store.seed(t, platformEnrollmentCollection(), req.ID, req)
}

func (e *enrollTestEnv) requestState(t *testing.T) models.PlatformEnrollmentState {
	t.Helper()
	return models.PlatformEnrollmentState(e.store.field(t, platformEnrollmentCollection(), enrollTestRequest, "state"))
}

// enrollTestCertPEM mints a self-signed certificate with a deterministic
// serial, expiry, and (optionally) a SPIFFE URI SAN.
func enrollTestCertPEM(t *testing.T, serial int64, spiffeID string, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "enroll-test"},
		NotBefore:    notAfter.Add(-24 * time.Hour),
		NotAfter:     notAfter,
	}
	if spiffeID != "" {
		u, err := url.Parse(spiffeID)
		require.NoError(t, err)
		tmpl.URIs = []*url.URL{u}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func enrollTestMessage(t *testing.T, event constants.EventType, p *commonv1.PlatformEnrollmentGovernancePayload) *PubSubCommandMessage {
	t.Helper()
	raw, err := proto.Marshal(p)
	require.NoError(t, err)
	return &PubSubCommandMessage{ID: enrollTestEnvelope, EventType: event, Payload: raw}
}

func enrollTestAppRequest(state models.PlatformEnrollmentState) models.PlatformEnrollmentRequest {
	return models.PlatformEnrollmentRequest{
		ID:            enrollTestRequest,
		ComponentKind: models.PlatformComponentDashboard,
		ComponentName: "dashboard-app",
		InstanceID:    "inst-1",
		State:         state,
		App:           &models.PlatformAppCSRPayload{CSRPEM: enrollTestCSR},
	}
}

func enrollTestOperatorRequest(state models.PlatformEnrollmentState) models.PlatformEnrollmentRequest {
	return models.PlatformEnrollmentRequest{
		ID:                enrollTestRequest,
		ComponentKind:     models.PlatformComponentOperator,
		ComponentName:     "operator",
		InstanceID:        "inst-2",
		Hostname:          "host-a",
		SystemFingerprint: "fp-host-a",
		State:             state,
		Operator: &models.PlatformOperatorCSRPayload{
			OperatorCSRPEM: "operator-csr",
			CLICSRPEM:      "cli-csr",
		},
	}
}

// ---------------------------------------------------------------------------
// Pure helpers
// ---------------------------------------------------------------------------

func TestProtoComponentKind_MapsEveryKnownKindAndRejectsOthers(t *testing.T) {
	tests := []struct {
		name    string
		in      commonv1.PlatformComponentKind
		want    models.PlatformComponentKind
		wantErr error
	}{
		{"dashboard", commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD, models.PlatformComponentDashboard, nil},
		{"ensemble", commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_ENSEMBLE, models.PlatformComponentEnsemble, nil},
		{"operator", commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR, models.PlatformComponentOperator, nil},
		{"application", commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_APPLICATION, models.PlatformComponentApplication, nil},
		{"unspecified is rejected", commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_UNSPECIFIED, "", constants.ErrPlatformEnrollmentInvalidComponent},
		{"unknown numeric value is rejected", commonv1.PlatformComponentKind(99), "", constants.ErrPlatformEnrollmentInvalidComponent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := protoComponentKind(tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProtoDecision_MapsApproveAndDenyAndRejectsOthers(t *testing.T) {
	tests := []struct {
		name    string
		in      commonv1.PlatformEnrollmentDecision
		want    models.PlatformEnrollmentDecision
		wantErr error
	}{
		{"approve", commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_APPROVE, models.PlatformEnrollmentDecisionApprove, nil},
		{"deny", commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_DENY, models.PlatformEnrollmentDecisionDeny, nil},
		{"unspecified is rejected", commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_UNSPECIFIED, "", constants.ErrPlatformEnrollmentInvalidDecision},
		{"unknown numeric value is rejected", commonv1.PlatformEnrollmentDecision(42), "", constants.ErrPlatformEnrollmentInvalidDecision},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := protoDecision(tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFingerprintsFromPayload_NilYieldsZeroValueAndPopulatedCopiesAllFields(t *testing.T) {
	assert.Equal(t, models.PlatformEnrollmentCSRFingerprints{}, fingerprintsFromPayload(nil))

	got := fingerprintsFromPayload(&commonv1.PlatformEnrollmentFingerprints{App: "a", Operator: "o", Cli: "c"})
	assert.Equal(t, models.PlatformEnrollmentCSRFingerprints{App: "a", Operator: "o", CLI: "c"}, got)
}

func TestFingerprintsSummary_OmitsEmptyFieldsAndKeepsStableOrder(t *testing.T) {
	tests := []struct {
		name string
		in   models.PlatformEnrollmentCSRFingerprints
		want string
	}{
		{"none", models.PlatformEnrollmentCSRFingerprints{}, "[]"},
		{"app only", models.PlatformEnrollmentCSRFingerprints{App: "a1"}, "[app=a1]"},
		{"operator and cli", models.PlatformEnrollmentCSRFingerprints{Operator: "o1", CLI: "c1"}, "[operator=o1,cli=c1]"},
		{"all three", models.PlatformEnrollmentCSRFingerprints{App: "a1", Operator: "o1", CLI: "c1"}, "[app=a1,operator=o1,cli=c1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fingerprintsSummary(tt.in))
		})
	}
}

func TestCertificatePEMHelpers_ExtractSerialFingerprintExpiryAndSPIFFEID(t *testing.T) {
	notAfter := time.Date(2032, 5, 6, 7, 8, 9, 0, time.UTC)
	certPEM := enrollTestCertPEM(t, 987654321, "spiffe://g8e.test/operator/abc", notAfter)
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	digest := sha256.Sum256(block.Bytes)

	assert.Equal(t, "987654321", serialFromPEM(certPEM))
	assert.Equal(t, hex.EncodeToString(digest[:]), fingerprintFromPEM(certPEM))
	assert.True(t, notAfter.Equal(expiryFromPEM(certPEM)), "expiry must be the certificate NotAfter")

	id, err := spiffeIDFromCertificate(certPEM)
	require.NoError(t, err)
	assert.Equal(t, "spiffe://g8e.test/operator/abc", id)
}

func TestCertificatePEMHelpers_MalformedInputDegradesPerContract(t *testing.T) {
	notPEM := "definitely not pem"
	badDER := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not a certificate")}))
	noURI := enrollTestCertPEM(t, 7, "", time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC))

	for name, input := range map[string]string{"not pem": notPEM, "pem with invalid DER": badDER} {
		t.Run(name, func(t *testing.T) {
			assert.Empty(t, serialFromPEM(input))
			assert.Empty(t, fingerprintFromPEM(input))
			assert.True(t, expiryFromPEM(input).IsZero())
			_, err := spiffeIDFromCertificate(input)
			require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidPayload)
		})
	}

	t.Run("certificate without a URI SAN has no SPIFFE ID", func(t *testing.T) {
		_, err := spiffeIDFromCertificate(noURI)
		require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidPayload)
	})
}

// ---------------------------------------------------------------------------
// decodePayload
// ---------------------------------------------------------------------------

func TestDecodePayload_RejectsMalformedProtobuf(t *testing.T) {
	env := newEnrollTestEnv(t)

	_, err := env.handler.decodePayload(&PubSubCommandMessage{
		EventType: constants.EventPlatformEnrollmentCreateRequested,
		Payload:   []byte{0xff, 0xff, 0xff},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "platform enrollment: decode payload")
}

func TestDecodePayload_RejectsEventTypeWithDifferentPayloadType(t *testing.T) {
	env := newEnrollTestEnv(t)

	// An empty payload decodes fine as any message; the event type decides
	// the concrete type, so a non-enrollment event must not be accepted.
	_, err := env.handler.decodePayload(&PubSubCommandMessage{
		EventType: constants.Event.Operator.HeartbeatRequested,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxPayloadActionMismatch)
}

func TestDecodePayload_RejectsUnknownEventType(t *testing.T) {
	env := newEnrollTestEnv(t)

	_, err := env.handler.decodePayload(&PubSubCommandMessage{EventType: "not.a.real.event"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}

// ---------------------------------------------------------------------------
// HandleCreate
// ---------------------------------------------------------------------------

func TestHandleCreate_AuditsRequestWithoutWritingToStore(t *testing.T) {
	env := newEnrollTestEnv(t)
	msg := enrollTestMessage(t, constants.EventPlatformEnrollmentCreateRequested, &commonv1.PlatformEnrollmentGovernancePayload{
		RequestId:     enrollTestRequest,
		ComponentKind: commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_ENSEMBLE,
		InstanceId:    "inst-9",
		Fingerprints:  &commonv1.PlatformEnrollmentFingerprints{App: "fp-app"},
	})

	summary, err := env.handler.HandleCreate(t.Context(), msg)

	require.NoError(t, err)
	assert.Equal(t, "platform enrollment create request_id=req-1 component=ensemble instance=inst-9 fingerprints=[app=fp-app]", summary)
	assert.Empty(t, env.store.condCalls, "CREATE is audit-only")
	assert.Empty(t, env.store.docs[platformEnrollmentCollection()], "CREATE must not write the request document")
}

func TestHandleCreate_FailsClosedOnBadInput(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	valid := &commonv1.PlatformEnrollmentGovernancePayload{
		RequestId:     enrollTestRequest,
		ComponentKind: commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD,
	}

	tests := []struct {
		name    string
		ctx     context.Context
		msg     func(t *testing.T) *PubSubCommandMessage
		wantErr error
		wantMsg string
	}{
		{
			name: "cancelled context",
			ctx:  cancelled,
			msg: func(t *testing.T) *PubSubCommandMessage {
				return enrollTestMessage(t, constants.EventPlatformEnrollmentCreateRequested, valid)
			},
			wantErr: context.Canceled,
		},
		{
			name: "undecodable payload",
			ctx:  t.Context(),
			msg: func(*testing.T) *PubSubCommandMessage {
				return &PubSubCommandMessage{EventType: constants.EventPlatformEnrollmentCreateRequested, Payload: []byte{0xff}}
			},
			wantMsg: "decode payload",
		},
		{
			name: "unspecified component kind",
			ctx:  t.Context(),
			msg: func(t *testing.T) *PubSubCommandMessage {
				return enrollTestMessage(t, constants.EventPlatformEnrollmentCreateRequested, &commonv1.PlatformEnrollmentGovernancePayload{RequestId: enrollTestRequest})
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidComponent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnrollTestEnv(t)

			summary, err := env.handler.HandleCreate(tt.ctx, tt.msg(t))

			require.Error(t, err)
			assert.Empty(t, summary)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HandleDecide
// ---------------------------------------------------------------------------

// Decision state, authority, and rollback assertions live beside the real
// SQLite transaction in gateway/platform_enrollment_store_test.go.
func (s *enrollTestDocStore) DecidePlatformEnrollments(ctx context.Context, actor string, req models.PlatformEnrollmentBatchDecisionRequest, envelopeID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := req.Validate(); err != nil {
		return err
	}
	return s.condErr
}

func TestHandleDecide_PropagatesAtomicDecisionFailure(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.store.condErr = constants.ErrPlatformEnrollmentAlreadyDecided
	_, err := env.handler.HandleDecide(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentDecideRequested, &commonv1.PlatformEnrollmentGovernancePayload{
		ActorUserId:     enrollTestOwnerID,
		Decision:        commonv1.PlatformEnrollmentDecision_PLATFORM_ENROLLMENT_DECISION_APPROVE,
		DecisionTargets: []*commonv1.PlatformEnrollmentDecisionTarget{{RequestId: enrollTestRequest}},
	}))
	require.ErrorIs(t, err, constants.ErrPlatformEnrollmentAlreadyDecided)
}

// ---------------------------------------------------------------------------
// HandleIssue — application components
// ---------------------------------------------------------------------------

func issuePayload(kind commonv1.PlatformComponentKind) *commonv1.PlatformEnrollmentGovernancePayload {
	return &commonv1.PlatformEnrollmentGovernancePayload{
		RequestId:     enrollTestRequest,
		ComponentKind: kind,
		ActorUserId:   enrollTestOwnerID,
	}
}

func TestHandleIssue_AppComponentSignsCSRAndCompletesRequest(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, enrollTestAppRequest(models.PlatformEnrollmentStateIssuing))
	msg := enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
		issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD))

	summary, err := env.handler.HandleIssue(t.Context(), msg)

	require.NoError(t, err)
	wantFP := fingerprintFromPEM(env.pki.appCertPEM)
	assert.Equal(t, "platform enrollment issue request_id=req-1 component=dashboard cert_fingerprint="+wantFP, summary)
	assert.NotContains(t, summary, "operator_id=", "app issuance has no operator id")
	assert.NotContains(t, summary, "secret-csr-body", "receipt summaries must never carry CSR material")

	require.Len(t, env.pki.appCalls, 1)
	assert.Equal(t, enrollTestAppSign{CSR: enrollTestCSR, AppName: "dashboard-app", UserID: enrollTestOwnerID}, env.pki.appCalls[0])
	assert.Empty(t, env.pki.signCalls, "app issuance must not use the operator/CLI signer")

	assert.Equal(t, models.PlatformEnrollmentStateCompleted, env.requestState(t))
	coll := platformEnrollmentCollection()
	assert.Equal(t, enrollTestEnvelope, env.store.field(t, coll, enrollTestRequest, "issuance_envelope_id"))
	assert.Equal(t, enrollTestEnvelope, env.store.field(t, coll, enrollTestRequest, "issuance_receipt_id"))
	assert.Equal(t, "4242", env.store.field(t, coll, enrollTestRequest, "certificate_serial"))
	assert.Equal(t, wantFP, env.store.field(t, coll, enrollTestRequest, "certificate_fingerprint"))

	req, err := loadPlatformEnrollmentRequest(t.Context(), env.handler.deps, enrollTestRequest)
	require.NoError(t, err)
	require.NotNil(t, req.Issued)
	require.NotNil(t, req.Issued.App)
	assert.Equal(t, "dashboard-app", req.Issued.App.AppID)
	assert.Equal(t, env.pki.appCertPEM, req.Issued.App.AppCert)
	assert.Equal(t, "app-chain", req.Issued.App.CertChain)
	assert.Equal(t, "trust-bundle-pem", req.Issued.App.TrustBundle)
	assert.True(t, time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC).Equal(req.Issued.App.ExpiresAt))
	assert.Nil(t, req.Issued.Operator)

	require.Len(t, env.store.condCalls, 1)
	assert.Equal(t, string(models.PlatformEnrollmentStateIssuing), env.store.condCalls[0].Value,
		"completion must be conditional on the issuing lease still being held")
}

func TestHandleIssue_AppKindsAllUseTheAppSigner(t *testing.T) {
	for _, kind := range []models.PlatformComponentKind{
		models.PlatformComponentDashboard, models.PlatformComponentEnsemble, models.PlatformComponentApplication,
	} {
		t.Run(string(kind), func(t *testing.T) {
			env := newEnrollTestEnv(t)
			req := enrollTestAppRequest(models.PlatformEnrollmentStateIssuing)
			req.ComponentKind = kind
			env.seedRequest(t, req)
			protoKind := map[models.PlatformComponentKind]commonv1.PlatformComponentKind{
				models.PlatformComponentDashboard:   commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD,
				models.PlatformComponentEnsemble:    commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_ENSEMBLE,
				models.PlatformComponentApplication: commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_APPLICATION,
			}[kind]

			_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested, issuePayload(protoKind)))

			require.NoError(t, err)
			assert.Len(t, env.pki.appCalls, 1)
			assert.Equal(t, models.PlatformEnrollmentStateCompleted, env.requestState(t))
		})
	}
}

func TestHandleIssue_RejectsInvalidRequests(t *testing.T) {
	dashboard := commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD

	tests := []struct {
		name    string
		seed    func(t *testing.T, e *enrollTestEnv)
		payload *commonv1.PlatformEnrollmentGovernancePayload
		wantErr error
	}{
		{
			name:    "unspecified component kind",
			payload: issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_UNSPECIFIED),
			wantErr: constants.ErrPlatformEnrollmentInvalidComponent,
		},
		{
			name:    "missing request id",
			payload: &commonv1.PlatformEnrollmentGovernancePayload{ComponentKind: dashboard, ActorUserId: enrollTestOwnerID},
			wantErr: constants.ErrPlatformEnrollmentRequestIDRequired,
		},
		{
			name:    "missing actor",
			payload: &commonv1.PlatformEnrollmentGovernancePayload{RequestId: enrollTestRequest, ComponentKind: dashboard},
			wantErr: constants.ErrPlatformEnrollmentInvalidDecision,
		},
		{
			name:    "request does not exist",
			payload: issuePayload(dashboard),
			wantErr: constants.ErrPlatformEnrollmentRequestNotFound,
		},
		{
			name: "approved request without an issuance lease",
			seed: func(t *testing.T, e *enrollTestEnv) {
				e.seedRequest(t, enrollTestAppRequest(models.PlatformEnrollmentStateApproved))
			},
			payload: issuePayload(dashboard),
			wantErr: constants.ErrPlatformEnrollmentIssuanceInProgress,
		},
		{
			name: "pending request is never issued",
			seed: func(t *testing.T, e *enrollTestEnv) {
				e.seedRequest(t, enrollTestAppRequest(models.PlatformEnrollmentStatePending))
			},
			payload: issuePayload(dashboard),
			wantErr: constants.ErrPlatformEnrollmentIssuanceInProgress,
		},
		{
			name: "already completed request is not reissued",
			seed: func(t *testing.T, e *enrollTestEnv) {
				e.seedRequest(t, enrollTestAppRequest(models.PlatformEnrollmentStateCompleted))
			},
			payload: issuePayload(dashboard),
			wantErr: constants.ErrPlatformEnrollmentIssuanceInProgress,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnrollTestEnv(t)
			if tt.seed != nil {
				tt.seed(t, env)
			}

			summary, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested, tt.payload))

			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, summary)
			assert.Empty(t, env.pki.appCalls, "no certificate may be signed for a rejected request")
			assert.Empty(t, env.pki.signCalls, "no certificate may be signed for a rejected request")
		})
	}
}

func TestHandleIssue_SigningFailureRollsLeaseBackToApproved(t *testing.T) {
	tests := []struct {
		name  string
		setup func(e *enrollTestEnv)
		req   models.PlatformEnrollmentRequest
		kind  commonv1.PlatformComponentKind
		want  string
	}{
		{
			name:  "trust bundle unavailable",
			setup: func(e *enrollTestEnv) { e.pki.trustErr = errEnrollTestBoom },
			req:   enrollTestAppRequest(models.PlatformEnrollmentStateIssuing),
			kind:  commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD,
			want:  "trust bundle",
		},
		{
			name:  "app CSR rejected by the CA",
			setup: func(e *enrollTestEnv) { e.pki.appErr = errEnrollTestBoom },
			req:   enrollTestAppRequest(models.PlatformEnrollmentStateIssuing),
			kind:  commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD,
			want:  "sign app csr",
		},
		{
			name:  "operator CSR rejected by the CA",
			setup: func(e *enrollTestEnv) { e.pki.signErrForLeaf[string(constants.LeafTypeOperator)] = errEnrollTestBoom },
			req:   enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing),
			kind:  commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR,
			want:  "sign operator csr",
		},
		{
			name:  "CLI CSR rejected by the CA",
			setup: func(e *enrollTestEnv) { e.pki.signErrForLeaf[string(constants.LeafTypeCLI)] = errEnrollTestBoom },
			req:   enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing),
			kind:  commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR,
			want:  "sign cli csr",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnrollTestEnv(t)
			env.seedRequest(t, tt.req)
			tt.setup(env)

			summary, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested, issuePayload(tt.kind)))

			require.ErrorIs(t, err, errEnrollTestBoom)
			assert.Contains(t, err.Error(), tt.want)
			assert.Empty(t, summary)
			assert.Equal(t, models.PlatformEnrollmentStateApproved, env.requestState(t),
				"a failed signing must return the request to approved so a retry can re-acquire the lease")
			require.Len(t, env.store.condCalls, 1)
			assert.Equal(t, string(models.PlatformEnrollmentStateIssuing), env.store.condCalls[0].Value)
		})
	}
}

func TestHandleIssue_RollbackFailureDoesNotMaskTheSigningError(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, enrollTestAppRequest(models.PlatformEnrollmentStateIssuing))
	env.pki.appErr = errEnrollTestBoom
	env.store.condErr = errors.New("rollback store down")

	_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
		issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD)))

	require.ErrorIs(t, err, errEnrollTestBoom, "the caller must see the signing failure, not the rollback failure")
}

func TestHandleIssue_MissingCSRPayloadIsInvalid(t *testing.T) {
	t.Run("app request without app CSR", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		req := enrollTestAppRequest(models.PlatformEnrollmentStateIssuing)
		req.App = nil
		env.seedRequest(t, req)

		_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
			issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD)))

		require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidPayload)
		assert.Equal(t, models.PlatformEnrollmentStateApproved, env.requestState(t))
	})

	t.Run("operator request without operator CSRs", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		req := enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing)
		req.Operator = nil
		env.seedRequest(t, req)

		_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
			issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR)))

		require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidPayload)
		assert.Empty(t, env.pki.signCalls)
	})
}

func TestHandleIssue_CompletionConflictAndStoreFailure(t *testing.T) {
	t.Run("lease lost before completion", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.seedRequest(t, enrollTestAppRequest(models.PlatformEnrollmentStateIssuing))
		lost := false
		env.store.condApplied = &lost

		summary, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
			issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD)))

		require.ErrorIs(t, err, constants.ErrPlatformEnrollmentIssuanceInProgress)
		assert.Empty(t, summary)
		assert.Equal(t, models.PlatformEnrollmentStateIssuing, env.requestState(t))
	})

	t.Run("completion write fails", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.seedRequest(t, enrollTestAppRequest(models.PlatformEnrollmentStateIssuing))
		env.store.condErr = errEnrollTestBoom

		_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
			issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_DASHBOARD)))

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "complete issuance req-1")
	})
}

// ---------------------------------------------------------------------------
// HandleIssue — operator components
// ---------------------------------------------------------------------------

func TestHandleIssue_OperatorSignsBothCSRsAndPersistsOwnedOperator(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing))

	summary, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
		issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR)))

	require.NoError(t, err)
	require.Len(t, env.pki.signCalls, 2)
	opSign, cliSign := env.pki.signCalls[0], env.pki.signCalls[1]

	assert.Equal(t, "operator-csr", opSign.CSR)
	assert.Equal(t, string(constants.LeafTypeOperator), opSign.LeafType)
	assert.Equal(t, enrollTestOrgID, opSign.OrganizationID)
	assert.NotEmpty(t, opSign.OperatorID)
	assert.NotEmpty(t, opSign.SessionID)
	assert.Empty(t, opSign.UserID, "the operator leaf is bound to the organization, not the user")

	assert.Equal(t, "cli-csr", cliSign.CSR)
	assert.Equal(t, string(constants.LeafTypeCLI), cliSign.LeafType)
	assert.Equal(t, enrollTestOwnerID, cliSign.UserID, "the CLI leaf is bound to the approving owner")
	assert.Empty(t, cliSign.OrganizationID)
	assert.Empty(t, cliSign.OperatorID)
	assert.NotEmpty(t, cliSign.SessionID)
	assert.NotEqual(t, opSign.SessionID, cliSign.SessionID, "operator and CLI sessions must be distinct")

	operatorID := opSign.OperatorID
	cliFP := fingerprintFromPEM(env.pki.cliCert)
	assert.Equal(t, "platform enrollment issue request_id=req-1 component=operator cert_fingerprint="+cliFP+" operator_id="+operatorID, summary)

	// Operator document is persisted user-owned, claimed, active, remote, and not a slot.
	opColl := marshaler.CollectionName(constants.CollectionOperators)
	doc := env.store.docs[opColl][operatorID]
	require.NotNil(t, doc)
	opDoc, err := models.OperatorDocumentFromStore(doc)
	require.NoError(t, err)
	assert.Equal(t, enrollTestOwnerID, opDoc.UserId)
	assert.Equal(t, enrollTestOrgID, opDoc.OrganizationId)
	assert.Equal(t, "host-a", opDoc.Name)
	assert.Equal(t, "fp-host-a", opDoc.SystemFingerprint)
	assert.Equal(t, string(constants.OperatorStatusActive), opDoc.Status)
	assert.Equal(t, string(constants.OperatorTypeRemote), opDoc.OperatorType)
	assert.Equal(t, string(constants.ComponentNameG8EO), opDoc.Component)
	assert.Equal(t, opSign.SessionID, opDoc.OperatorSessionId)
	assert.True(t, opDoc.Claimed)
	assert.False(t, opDoc.IsSlot)
	require.NotNil(t, opDoc.ClaimedAt)

	// Completion metadata records the CLI certificate and generated identities.
	coll := platformEnrollmentCollection()
	assert.Equal(t, models.PlatformEnrollmentStateCompleted, env.requestState(t))
	assert.Equal(t, "1002", env.store.field(t, coll, enrollTestRequest, "certificate_serial"))
	assert.Equal(t, cliFP, env.store.field(t, coll, enrollTestRequest, "certificate_fingerprint"))
	assert.Equal(t, operatorID, env.store.field(t, coll, enrollTestRequest, "operator_id"))
	assert.Equal(t, opSign.SessionID, env.store.field(t, coll, enrollTestRequest, "operator_session_id"))
	assert.Equal(t, cliSign.SessionID, env.store.field(t, coll, enrollTestRequest, "cli_session_id"))

	req, err := loadPlatformEnrollmentRequest(t.Context(), env.handler.deps, enrollTestRequest)
	require.NoError(t, err)
	require.NotNil(t, req.Issued)
	require.NotNil(t, req.Issued.Operator)
	creds := req.Issued.Operator
	assert.Equal(t, env.pki.operatorCert, creds.OperatorCert)
	assert.Equal(t, env.pki.cliCert, creds.CLICert)
	assert.Equal(t, "operator-chain", creds.OperatorCertChain)
	assert.Equal(t, "cli-chain", creds.CLICertChain)
	assert.Equal(t, "trust-bundle-pem", creds.HubTrustBundle)
	assert.Equal(t, "consensus", creds.Posture)
	assert.Equal(t, operatorID, creds.OperatorID)
	assert.Equal(t, opSign.SessionID, creds.OperatorSessionID)
	assert.Equal(t, cliSign.SessionID, creds.CLISessionID)
	assert.Nil(t, req.Issued.App)
}

func TestHandleIssue_OperatorIssuanceFailsClosedOnOwnerLookupProblems(t *testing.T) {
	usersColl := marshaler.CollectionName(constants.CollectionUsers)
	orgsColl := marshaler.CollectionName(constants.CollectionOrganizations)

	tests := []struct {
		name    string
		mutate  func(t *testing.T, e *enrollTestEnv)
		wantErr error
	}{
		{
			name:    "owner user missing",
			mutate:  func(_ *testing.T, e *enrollTestEnv) { delete(e.store.docs[usersColl], enrollTestOwnerID) },
			wantErr: constants.ErrUserNotFound,
		},
		{
			name: "owner has no organization",
			mutate: func(t *testing.T, e *enrollTestEnv) {
				e.store.seed(t, usersColl, enrollTestOwnerID, models.User{})
			},
			wantErr: constants.ErrOrganizationIDRequired,
		},
		{
			name:    "organization missing",
			mutate:  func(_ *testing.T, e *enrollTestEnv) { delete(e.store.docs[orgsColl], enrollTestOrgID) },
			wantErr: constants.ErrOrganizationNotFound,
		},
		{
			name: "owner is not a member of the organization",
			mutate: func(t *testing.T, e *enrollTestEnv) {
				e.store.seed(t, orgsColl, enrollTestOrgID, models.Organization{OwnerUserID: "someone-else"})
			},
			wantErr: constants.ErrOrganizationMembershipInvalid,
		},
		{
			name:    "user lookup fails",
			mutate:  func(_ *testing.T, e *enrollTestEnv) { e.store.getErr[usersColl] = errEnrollTestBoom },
			wantErr: errEnrollTestBoom,
		},
		{
			name:    "organization lookup fails",
			mutate:  func(_ *testing.T, e *enrollTestEnv) { e.store.getErr[orgsColl] = errEnrollTestBoom },
			wantErr: errEnrollTestBoom,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnrollTestEnv(t)
			env.seedRequest(t, enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing))
			tt.mutate(t, env)

			_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
				issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR)))

			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, env.pki.signCalls, "no certificate may be signed before the owner is verified")
			assert.Equal(t, models.PlatformEnrollmentStateApproved, env.requestState(t))
		})
	}
}

func TestLoadPlatformEnrollmentOrganization_AcceptsListedMemberWhoIsNotTheOwner(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.store.seed(t, marshaler.CollectionName(constants.CollectionUsers), "member-1", models.User{OrganizationID: enrollTestOrgID})
	env.store.seed(t, marshaler.CollectionName(constants.CollectionOrganizations), enrollTestOrgID,
		models.Organization{OwnerUserID: enrollTestOwnerID, MemberUserIDs: []string{"member-1"}})

	user, org, err := loadPlatformEnrollmentOrganization(context.Background(), env.handler.deps, "member-1")

	require.NoError(t, err)
	assert.Equal(t, "member-1", user.ID)
	assert.Equal(t, enrollTestOrgID, org.ID)
}

func TestLoadPlatformEnrollmentRequest_EmptyIDAndMissingDocumentYieldNil(t *testing.T) {
	env := newEnrollTestEnv(t)

	req, err := loadPlatformEnrollmentRequest(t.Context(), env.handler.deps, "")
	require.NoError(t, err)
	assert.Nil(t, req)

	req, err = loadPlatformEnrollmentRequest(t.Context(), env.handler.deps, "absent")
	require.NoError(t, err)
	assert.Nil(t, req)
}

func TestLoadPlatformEnrollmentRequest_RejectsDocumentThatDoesNotDecode(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.store.put(platformEnrollmentCollection(), "corrupt", map[string]json.RawMessage{
		"attempt_count": json.RawMessage(`"not-a-number"`),
	})

	_, err := loadPlatformEnrollmentRequest(t.Context(), env.handler.deps, "corrupt")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode request corrupt")
}

func TestHandleIssue_OperatorPersistenceFailuresAbortAndRollBack(t *testing.T) {
	t.Run("operator document write fails", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.seedRequest(t, enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing))
		env.store.setErr = errEnrollTestBoom

		_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
			issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR)))

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "persist operator doc")
		assert.Equal(t, models.PlatformEnrollmentStateApproved, env.requestState(t))
	})

	t.Run("superseding prior leases fails", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.seedRequest(t, enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing))
		env.store.queryErr = errEnrollTestBoom

		_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
			issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR)))

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "supersede prior operator leases")
		assert.Equal(t, models.PlatformEnrollmentStateApproved, env.requestState(t))
	})
}

// ---------------------------------------------------------------------------
// supersedeOperatorLeases
// ---------------------------------------------------------------------------

func seedEnrollOperator(t *testing.T, env *enrollTestEnv, id, owner, fingerprint string, status constants.OperatorStatus, sessionID string) {
	t.Helper()
	env.store.seed(t, marshaler.CollectionName(constants.CollectionOperators), id, map[string]interface{}{
		"id":                  id,
		"user_id":             owner,
		"system_fingerprint":  fingerprint,
		"operator_type":       string(constants.OperatorTypeRemote),
		"status":              string(status),
		"operator_session_id": sessionID,
	})
}

func TestHandleIssue_ReEnrollmentTerminatesPriorLeaseOfTheSameIdentity(t *testing.T) {
	env := newEnrollTestEnv(t)
	seedEnrollOperator(t, env, "old-op", enrollTestOwnerID, "fp-host-a", constants.OperatorStatusActive, "old-session")
	seedEnrollOperator(t, env, "other-host", enrollTestOwnerID, "fp-host-b", constants.OperatorStatusActive, "other-session")
	seedEnrollOperator(t, env, "other-owner", "user-2", "fp-host-a", constants.OperatorStatusActive, "owner2-session")
	seedEnrollOperator(t, env, "already-gone", enrollTestOwnerID, "fp-host-a", constants.OperatorStatusTerminated, "gone-session")
	env.seedRequest(t, enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing))

	_, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
		issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR)))

	require.NoError(t, err)
	opColl := marshaler.CollectionName(constants.CollectionOperators)
	assert.Equal(t, string(constants.OperatorStatusTerminated), env.store.field(t, opColl, "old-op", "status"))
	assert.Contains(t, env.store.field(t, opColl, "old-op", "termination_reason"), "superseded by re-enrollment as operator ")
	assert.Equal(t, []string{"old-session"}, env.opSession.deactivated,
		"only the superseded lease's session may be deactivated")
	assert.Equal(t, string(constants.OperatorStatusActive), env.store.field(t, opColl, "other-host", "status"), "a different host is a different operator")
	assert.Equal(t, string(constants.OperatorStatusActive), env.store.field(t, opColl, "other-owner", "status"), "another owner's operator must not be touched")
	assert.Equal(t, string(constants.OperatorStatusTerminated), env.store.field(t, opColl, "already-gone", "status"))
}

func TestSupersedeOperatorLeases_BehaviorMatrix(t *testing.T) {
	opColl := marshaler.CollectionName(constants.CollectionOperators)

	t.Run("empty fingerprint is a no-op and never queries", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.store.queryErr = errEnrollTestBoom // would surface if a query were issued
		seedEnrollOperator(t, env, "op-a", enrollTestOwnerID, "", constants.OperatorStatusActive, "s")

		require.NoError(t, env.handler.supersedeOperatorLeases(context.Background(), enrollTestOwnerID, "", "new-op"))
		assert.Equal(t, string(constants.OperatorStatusActive), env.store.field(t, opColl, "op-a", "status"))
	})

	t.Run("the replacement itself is never terminated", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		seedEnrollOperator(t, env, "new-op", enrollTestOwnerID, "fp", constants.OperatorStatusActive, "new-session")

		require.NoError(t, env.handler.supersedeOperatorLeases(context.Background(), enrollTestOwnerID, "fp", "new-op"))
		assert.Equal(t, string(constants.OperatorStatusActive), env.store.field(t, opColl, "new-op", "status"))
		assert.Empty(t, env.opSession.deactivated)
	})

	t.Run("lease without a session is terminated without session deactivation", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		seedEnrollOperator(t, env, "old", enrollTestOwnerID, "fp", constants.OperatorStatusActive, "")

		require.NoError(t, env.handler.supersedeOperatorLeases(context.Background(), enrollTestOwnerID, "fp", "new-op"))
		assert.Equal(t, string(constants.OperatorStatusTerminated), env.store.field(t, opColl, "old", "status"))
		assert.Empty(t, env.opSession.deactivated)
	})

	t.Run("an already-invalid operator session is tolerated", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		seedEnrollOperator(t, env, "old", enrollTestOwnerID, "fp", constants.OperatorStatusActive, "stale")
		env.opSession.deactivateErr = constants.ErrGatewayOperatorSessionInvalid

		require.NoError(t, env.handler.supersedeOperatorLeases(context.Background(), enrollTestOwnerID, "fp", "new-op"))
		assert.Equal(t, string(constants.OperatorStatusTerminated), env.store.field(t, opColl, "old", "status"))
	})

	t.Run("a real session deactivation failure aborts before termination", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		seedEnrollOperator(t, env, "old", enrollTestOwnerID, "fp", constants.OperatorStatusActive, "live")
		env.opSession.deactivateErr = errEnrollTestBoom

		err := env.handler.supersedeOperatorLeases(context.Background(), enrollTestOwnerID, "fp", "new-op")

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "deactivate operator session of old")
		assert.Equal(t, string(constants.OperatorStatusActive), env.store.field(t, opColl, "old", "status"),
			"a lease whose session could not be deactivated must stay visible rather than be half-terminated")
	})

	t.Run("termination write failure is reported", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		seedEnrollOperator(t, env, "old", enrollTestOwnerID, "fp", constants.OperatorStatusActive, "s")
		env.store.updErr = errEnrollTestBoom

		err := env.handler.supersedeOperatorLeases(context.Background(), enrollTestOwnerID, "fp", "new-op")

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "terminate superseded operator old")
	})

	t.Run("corrupt operator_session_id field is reported", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.store.put(opColl, "old", map[string]json.RawMessage{
			"user_id":             json.RawMessage(`"` + enrollTestOwnerID + `"`),
			"system_fingerprint":  json.RawMessage(`"fp"`),
			"operator_type":       json.RawMessage(`"` + string(constants.OperatorTypeRemote) + `"`),
			"status":              json.RawMessage(`"` + string(constants.OperatorStatusActive) + `"`),
			"operator_session_id": json.RawMessage(`123`),
		})

		err := env.handler.supersedeOperatorLeases(context.Background(), enrollTestOwnerID, "fp", "new-op")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode operator session of old")
	})
}

// ---------------------------------------------------------------------------
// HandlePersistPolicy
// ---------------------------------------------------------------------------

func persistPolicyPayload() *commonv1.PlatformEnrollmentGovernancePayload {
	return &commonv1.PlatformEnrollmentGovernancePayload{
		RequestId:              enrollTestRequest,
		ActorUserId:            enrollTestOwnerID,
		OwnerUserId:            enrollTestOwnerID,
		TargetCollection:       marshaler.CollectionName(constants.CollectionAppPolicies),
		TargetDocumentId:       "dashboard-app",
		PolicyId:               "policy-1",
		CertificateSerial:      "4242",
		CertificateFingerprint: "fp-cert",
	}
}

func TestHandlePersistPolicy_WritesLeastPrivilegePolicyDocument(t *testing.T) {
	env := newEnrollTestEnv(t)

	summary, err := env.handler.HandlePersistPolicy(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentPersistPolicyRequested, persistPolicyPayload()))

	require.NoError(t, err)
	assert.Equal(t, "platform enrollment persist_policy request_id=req-1 policy_id=policy-1 document=dashboard-app", summary)

	policyColl := marshaler.CollectionName(constants.CollectionAppPolicies)
	doc := env.store.docs[policyColl]["dashboard-app"]
	require.NotNil(t, doc)
	raw, err := json.Marshal(doc.Data)
	require.NoError(t, err)
	var policy models.AppPolicy
	require.NoError(t, json.Unmarshal(raw, &policy))
	assert.Equal(t, "dashboard-app", policy.AppID)
	assert.Equal(t, enrollTestOwnerID, policy.OwnerUserID)
	assert.Equal(t, enrollTestOwnerID, policy.ApprovedByUserID)
	assert.Equal(t, enrollTestRequest, policy.EnrollmentRequestID)
	assert.Equal(t, "4242", policy.CertificateSerial)
	assert.Equal(t, "fp-cert", policy.CertificateFingerprint)
	assert.Empty(t, policy.AllowedCollections, "a freshly enrolled app starts with no collection grants")
	assert.Zero(t, policy.RateLimitRPS)
	assert.Zero(t, policy.MaxPayloadBytes)
	assert.False(t, policy.RequireL3Approval)
	assert.False(t, policy.CreatedAt.IsZero())
}

func TestHandlePersistPolicy_RetainsExistingPolicyInsteadOfOverwriting(t *testing.T) {
	env := newEnrollTestEnv(t)
	policyColl := marshaler.CollectionName(constants.CollectionAppPolicies)
	env.store.seed(t, policyColl, "dashboard-app", models.AppPolicy{AppID: "dashboard-app", RateLimitRPS: 50, AllowedCollections: []string{"events"}})

	summary, err := env.handler.HandlePersistPolicy(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentPersistPolicyRequested, persistPolicyPayload()))

	require.NoError(t, err)
	assert.Equal(t, "platform enrollment persist_policy retained request_id=req-1 policy_id=policy-1 document=dashboard-app", summary)
	var rps int
	require.NoError(t, json.Unmarshal(env.store.docs[policyColl]["dashboard-app"].Data["rate_limit_rps"], &rps))
	assert.Equal(t, 50, rps, "operator-tuned policy must survive re-enrollment")
}

func TestHandlePersistPolicy_RejectsIncompletePayloads(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(p *commonv1.PlatformEnrollmentGovernancePayload)
		wantErr error
	}{
		{"missing request id", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.RequestId = "" }, constants.ErrPlatformEnrollmentRequestIDRequired},
		{"missing target collection", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.TargetCollection = "" }, constants.ErrPlatformEnrollmentInvalidPayload},
		{"missing target document", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.TargetDocumentId = "" }, constants.ErrPlatformEnrollmentInvalidPayload},
		{"missing owner", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.OwnerUserId = "" }, constants.ErrPlatformEnrollmentInvalidPayload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnrollTestEnv(t)
			p := persistPolicyPayload()
			tt.mutate(p)

			summary, err := env.handler.HandlePersistPolicy(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentPersistPolicyRequested, p))

			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, summary)
			assert.Empty(t, env.store.docs[marshaler.CollectionName(constants.CollectionAppPolicies)])
		})
	}
}

func TestHandlePersistPolicy_PropagatesStoreAndContextErrors(t *testing.T) {
	msgFor := func(t *testing.T) *PubSubCommandMessage {
		return enrollTestMessage(t, constants.EventPlatformEnrollmentPersistPolicyRequested, persistPolicyPayload())
	}

	t.Run("existence check fails", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.store.getErr[marshaler.CollectionName(constants.CollectionAppPolicies)] = errEnrollTestBoom

		_, err := env.handler.HandlePersistPolicy(t.Context(), msgFor(t))

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "check existing policy")
	})

	t.Run("write fails", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.store.setErr = errEnrollTestBoom

		_, err := env.handler.HandlePersistPolicy(t.Context(), msgFor(t))

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "persist policy")
	})

	t.Run("cancelled context", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := env.handler.HandlePersistPolicy(ctx, msgFor(t))

		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("undecodable payload", func(t *testing.T) {
		env := newEnrollTestEnv(t)

		_, err := env.handler.HandlePersistPolicy(t.Context(), &PubSubCommandMessage{EventType: constants.EventPlatformEnrollmentPersistPolicyRequested, Payload: []byte{0xff}})

		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// HandleCreateSession
// ---------------------------------------------------------------------------

func createSessionPayload() *commonv1.PlatformEnrollmentGovernancePayload {
	return &commonv1.PlatformEnrollmentGovernancePayload{
		RequestId:              enrollTestRequest,
		ActorUserId:            enrollTestOwnerID,
		OperatorId:             "op-1",
		OperatorSessionId:      "op-session-1",
		CliSessionId:           "cli-session-1",
		CertificateFingerprint: "fp-cli",
		CertificateSerial:      "1002",
	}
}

func TestHandleCreateSession_PersistsBothSessionsBoundToTheApprovingOwner(t *testing.T) {
	env := newEnrollTestEnv(t)

	summary, err := env.handler.HandleCreateSession(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentCreateSessionRequested, createSessionPayload()))

	require.NoError(t, err)
	assert.Equal(t, "platform enrollment create_session request_id=req-1 operator_id=op-1 operator_session=op-session-1 cli_session=cli-session-1 actor=user-owner", summary)
	bootstrap := string(constants.HeartbeatTypeBootstrap)
	assert.Equal(t, []enrollTestCLISession{{
		CLISessionID: "cli-session-1", OperatorSessionID: "op-session-1", UserID: enrollTestOwnerID,
		CertFingerprint: "fp-cli", CertSerial: "1002", LoginMethod: bootstrap,
	}}, env.cli.persisted)
	assert.Equal(t, []enrollTestOperatorSession{{
		OperatorSessionID: "op-session-1", UserID: enrollTestOwnerID, OrgID: enrollTestOrgID,
		OperatorID: "op-1", LoginMethod: bootstrap,
	}}, env.opSession.persisted)
}

func TestHandleCreateSession_RejectsIncompletePayloadsBeforePersisting(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(p *commonv1.PlatformEnrollmentGovernancePayload)
		wantErr error
	}{
		{"missing request id", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.RequestId = "" }, constants.ErrPlatformEnrollmentRequestIDRequired},
		{"missing operator id", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.OperatorId = "" }, constants.ErrPlatformEnrollmentInvalidPayload},
		{"missing operator session", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.OperatorSessionId = "" }, constants.ErrPlatformEnrollmentInvalidPayload},
		{"missing cli session", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.CliSessionId = "" }, constants.ErrPlatformEnrollmentInvalidPayload},
		{"missing actor", func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.ActorUserId = "" }, constants.ErrPlatformEnrollmentInvalidDecision},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnrollTestEnv(t)
			p := createSessionPayload()
			tt.mutate(p)

			_, err := env.handler.HandleCreateSession(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentCreateSessionRequested, p))

			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, env.cli.persisted)
			assert.Empty(t, env.opSession.persisted)
		})
	}
}

func TestHandleCreateSession_PropagatesDependencyFailures(t *testing.T) {
	msgFor := func(t *testing.T) *PubSubCommandMessage {
		return enrollTestMessage(t, constants.EventPlatformEnrollmentCreateSessionRequested, createSessionPayload())
	}

	t.Run("unknown owner", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		delete(env.store.docs[marshaler.CollectionName(constants.CollectionUsers)], enrollTestOwnerID)

		_, err := env.handler.HandleCreateSession(t.Context(), msgFor(t))

		require.ErrorIs(t, err, constants.ErrUserNotFound)
		assert.Empty(t, env.cli.persisted)
	})

	t.Run("cli session persistence fails before the operator session is written", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.cli.persistErr = errEnrollTestBoom

		_, err := env.handler.HandleCreateSession(t.Context(), msgFor(t))

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "persist cli session")
		assert.Empty(t, env.opSession.persisted)
	})

	t.Run("operator session persistence fails", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.opSession.persistErr = errEnrollTestBoom

		_, err := env.handler.HandleCreateSession(t.Context(), msgFor(t))

		require.ErrorIs(t, err, errEnrollTestBoom)
		assert.Contains(t, err.Error(), "persist operator session")
	})

	t.Run("cancelled context", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := env.handler.HandleCreateSession(ctx, msgFor(t))

		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("undecodable payload", func(t *testing.T) {
		env := newEnrollTestEnv(t)

		_, err := env.handler.HandleCreateSession(t.Context(), &PubSubCommandMessage{EventType: constants.EventPlatformEnrollmentCreateSessionRequested, Payload: []byte{0xff}})

		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// HandleRevoke
// ---------------------------------------------------------------------------

func revokePayload() *commonv1.PlatformEnrollmentGovernancePayload {
	return &commonv1.PlatformEnrollmentGovernancePayload{
		RequestId:        enrollTestRequest,
		ActorUserId:      enrollTestOwnerID,
		TargetDocumentId: "dashboard-app",
		Reason:           "  key compromise  ",
	}
}

func completedAppRequest() models.PlatformEnrollmentRequest {
	req := enrollTestAppRequest(models.PlatformEnrollmentStateCompleted)
	req.CertificateSerial = "4242"
	return req
}

func completedOperatorRequest(t *testing.T, env *enrollTestEnv) models.PlatformEnrollmentRequest {
	t.Helper()
	req := enrollTestOperatorRequest(models.PlatformEnrollmentStateCompleted)
	req.CertificateSerial = "1002"
	req.OperatorID = "op-1"
	req.OperatorSessionID = "op-session-1"
	req.CLISessionID = "cli-session-1"
	req.Issued = &models.PlatformEnrollmentCompleteResponse{
		RequestID:     enrollTestRequest,
		ComponentKind: models.PlatformComponentOperator,
		Operator: &models.PlatformEnrollmentOperatorCredentials{
			OperatorCert: env.pki.operatorCert,
			CLICert:      env.pki.cliCert,
		},
	}
	return req
}

func TestHandleRevoke_AppComponentRevokesCertificateDeletesPolicyAndDisconnects(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, completedAppRequest())
	policyColl := marshaler.CollectionName(constants.CollectionAppPolicies)
	env.store.seed(t, policyColl, "dashboard-app", models.AppPolicy{AppID: "dashboard-app"})

	summary, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, revokePayload()))

	require.NoError(t, err)
	assert.Equal(t, "platform enrollment revoke request_id=req-1 component=dashboard", summary)
	assert.Equal(t, []enrollTestRevocation{{Serial: "4242", Reason: "key compromise"}}, env.pki.revocations,
		"the reason must be trimmed before it reaches the CA")
	assert.Equal(t, []string{policyColl + "/dashboard-app"}, env.store.deletes)
	assert.Equal(t, []string{"dashboard-app"}, env.conns.disconnected, "live connections for the revoked app must be dropped")

	coll := platformEnrollmentCollection()
	assert.Equal(t, models.PlatformEnrollmentStateRevoked, env.requestState(t))
	assert.Equal(t, enrollTestOwnerID, env.store.field(t, coll, enrollTestRequest, "revoked_by_user_id"))
	assert.Equal(t, "key compromise", env.store.field(t, coll, enrollTestRequest, "revocation_reason"))
	assert.Equal(t, enrollTestEnvelope, env.store.field(t, coll, enrollTestRequest, "revocation_envelope_id"))
	assert.Equal(t, enrollTestEnvelope, env.store.field(t, coll, enrollTestRequest, "revocation_receipt_id"))
	assert.Equal(t, string(models.PlatformEnrollmentStateCompleted), env.store.condCalls[len(env.store.condCalls)-1].Value)
}

func TestHandleRevoke_BlankReasonDefaultsToTheRevokeIntent(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, completedAppRequest())
	p := revokePayload()
	p.Reason = "   "

	_, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, p))

	require.NoError(t, err)
	require.Len(t, env.pki.revocations, 1)
	assert.Equal(t, string(constants.PlatformEnrollmentIntentRevoke), env.pki.revocations[0].Reason)
}

func TestHandleRevoke_RequestWithoutCertificateSerialSkipsCARevocation(t *testing.T) {
	env := newEnrollTestEnv(t)
	req := completedAppRequest()
	req.CertificateSerial = ""
	env.seedRequest(t, req)

	_, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, revokePayload()))

	require.NoError(t, err)
	assert.Empty(t, env.pki.revocations)
	assert.Equal(t, models.PlatformEnrollmentStateRevoked, env.requestState(t))
}

func TestHandleRevoke_OperatorRevokesBothCertsTerminatesOperatorAndDropsConnections(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, completedOperatorRequest(t, env))
	seedEnrollOperator(t, env, "op-1", enrollTestOwnerID, "fp-host-a", constants.OperatorStatusActive, "op-session-1")
	p := revokePayload()
	p.TargetDocumentId = ""

	summary, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, p))

	require.NoError(t, err)
	assert.Equal(t, "platform enrollment revoke request_id=req-1 component=operator", summary)
	assert.Equal(t, []enrollTestRevocation{
		{Serial: "1002", Reason: "key compromise"},
		{Serial: "1001", Reason: "key compromise"},
	}, env.pki.revocations, "both the recorded CLI serial and the operator certificate serial must be revoked")
	assert.Equal(t, []string{"cli-session-1"}, env.cli.deactivated)
	assert.Equal(t, []string{"op-session-1"}, env.opSession.deactivated)
	assert.Equal(t, []string{"spiffe://g8e.test/operator/op", "spiffe://g8e.test/cli/owner"}, env.conns.disconnected)

	opColl := marshaler.CollectionName(constants.CollectionOperators)
	assert.Equal(t, string(constants.OperatorStatusTerminated), env.store.field(t, opColl, "op-1", "status"))
	assert.Equal(t, "key compromise", env.store.field(t, opColl, "op-1", "termination_reason"))
	assert.Equal(t, models.PlatformEnrollmentStateRevoked, env.requestState(t))
}

func TestHandleRevoke_OperatorToleratesSessionsThatAreAlreadyGone(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, completedOperatorRequest(t, env))
	seedEnrollOperator(t, env, "op-1", enrollTestOwnerID, "fp-host-a", constants.OperatorStatusActive, "op-session-1")
	env.cli.deactivateErr = constants.ErrCLISessionAlreadyDeactivated
	env.opSession.deactivateErr = constants.ErrGatewayOperatorSessionInvalid

	_, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, revokePayload()))

	require.NoError(t, err, "revoking must be idempotent with respect to already-deactivated sessions")
	assert.Equal(t, models.PlatformEnrollmentStateRevoked, env.requestState(t))

	env2 := newEnrollTestEnv(t)
	env2.seedRequest(t, completedOperatorRequest(t, env2))
	seedEnrollOperator(t, env2, "op-1", enrollTestOwnerID, "fp-host-a", constants.OperatorStatusActive, "op-session-1")
	env2.cli.deactivateErr = constants.ErrCLISessionNotFound
	_, err = env2.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, revokePayload()))
	require.NoError(t, err, "a CLI session that no longer exists is not a revocation failure")
}

func TestHandleRevoke_RejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name    string
		seed    func(t *testing.T, e *enrollTestEnv)
		mutate  func(p *commonv1.PlatformEnrollmentGovernancePayload)
		wantErr error
	}{
		{
			name:    "missing request id",
			mutate:  func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.RequestId = "" },
			wantErr: constants.ErrPlatformEnrollmentRequestIDRequired,
		},
		{
			name:    "request does not exist",
			wantErr: constants.ErrPlatformEnrollmentRequestNotFound,
		},
		{
			name: "pending request cannot be revoked",
			seed: func(t *testing.T, e *enrollTestEnv) {
				e.seedRequest(t, enrollTestAppRequest(models.PlatformEnrollmentStatePending))
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidState,
		},
		{
			name: "already revoked request cannot be revoked twice",
			seed: func(t *testing.T, e *enrollTestEnv) {
				r := completedAppRequest()
				r.State = models.PlatformEnrollmentStateRevoked
				e.seedRequest(t, r)
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidState,
		},
		{
			name:    "app revocation requires the target policy document",
			seed:    func(t *testing.T, e *enrollTestEnv) { e.seedRequest(t, completedAppRequest()) },
			mutate:  func(p *commonv1.PlatformEnrollmentGovernancePayload) { p.TargetDocumentId = "" },
			wantErr: constants.ErrPlatformEnrollmentInvalidPayload,
		},
		{
			name: "operator revocation requires recorded issued credentials",
			seed: func(t *testing.T, e *enrollTestEnv) {
				r := completedOperatorRequest(t, e)
				r.Issued = nil
				e.seedRequest(t, r)
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidPayload,
		},
		{
			name: "operator certificate with no serial is rejected",
			seed: func(t *testing.T, e *enrollTestEnv) {
				r := completedOperatorRequest(t, e)
				r.Issued.Operator.OperatorCert = "garbage"
				e.seedRequest(t, r)
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidPayload,
		},
		{
			name: "operator certificate without a SPIFFE ID is rejected",
			seed: func(t *testing.T, e *enrollTestEnv) {
				r := completedOperatorRequest(t, e)
				r.Issued.Operator.OperatorCert = enrollTestCertPEM(t, 55, "", time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC))
				e.seedRequest(t, r)
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidPayload,
		},
		{
			name: "cli certificate without a SPIFFE ID is rejected",
			seed: func(t *testing.T, e *enrollTestEnv) {
				r := completedOperatorRequest(t, e)
				r.Issued.Operator.CLICert = "garbage"
				e.seedRequest(t, r)
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidPayload,
		},
		{
			name: "unknown component kind is rejected",
			seed: func(t *testing.T, e *enrollTestEnv) {
				r := completedAppRequest()
				r.ComponentKind = "mystery"
				e.seedRequest(t, r)
			},
			wantErr: constants.ErrPlatformEnrollmentInvalidComponent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnrollTestEnv(t)
			if tt.seed != nil {
				tt.seed(t, env)
			}
			p := revokePayload()
			if tt.mutate != nil {
				tt.mutate(p)
			}
			stateBefore := ""
			if doc, ok := env.store.docs[platformEnrollmentCollection()][enrollTestRequest]; ok {
				require.NoError(t, json.Unmarshal(doc.Data["state"], &stateBefore))
			}

			summary, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, p))

			require.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, summary)
			if doc, ok := env.store.docs[platformEnrollmentCollection()][enrollTestRequest]; ok {
				var stateAfter string
				require.NoError(t, json.Unmarshal(doc.Data["state"], &stateAfter))
				assert.Equal(t, stateBefore, stateAfter, "a rejected revocation must leave the request state untouched")
			}
		})
	}
}

func TestHandleRevoke_RequiresConnectionDrainer(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, completedAppRequest())
	env.handler.deps.Connections = nil

	_, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, revokePayload()))

	require.ErrorIs(t, err, constants.ErrPlatformEnrollmentDepsRequired)
	assert.Empty(t, env.pki.revocations, "the certificate must not be revoked when connections cannot be drained")
}

func TestHandleRevoke_PropagatesDependencyFailures(t *testing.T) {
	tests := []struct {
		name   string
		seed   func(t *testing.T, e *enrollTestEnv)
		setup  func(e *enrollTestEnv)
		want   string
		wantIs error
	}{
		{
			name:   "CA revocation fails",
			seed:   func(t *testing.T, e *enrollTestEnv) { e.seedRequest(t, completedAppRequest()) },
			setup:  func(e *enrollTestEnv) { e.pki.revokeErr = errEnrollTestBoom },
			want:   "revoke certificate",
			wantIs: errEnrollTestBoom,
		},
		{
			name:   "app policy deletion fails",
			seed:   func(t *testing.T, e *enrollTestEnv) { e.seedRequest(t, completedAppRequest()) },
			setup:  func(e *enrollTestEnv) { e.store.delErr = errEnrollTestBoom },
			want:   "revoke app policy",
			wantIs: errEnrollTestBoom,
		},
		{
			name: "operator certificate revocation fails",
			seed: func(t *testing.T, e *enrollTestEnv) { e.seedRequest(t, completedOperatorRequest(t, e)) },
			setup: func(e *enrollTestEnv) {
				e.pki.revokeErrFor["1001"] = errEnrollTestBoom
			},
			want:   "revoke operator certificate",
			wantIs: errEnrollTestBoom,
		},
		{
			name: "CLI session deactivation fails",
			seed: func(t *testing.T, e *enrollTestEnv) { e.seedRequest(t, completedOperatorRequest(t, e)) },
			setup: func(e *enrollTestEnv) {
				e.cli.deactivateErr = errEnrollTestBoom
			},
			want:   "deactivate CLI session",
			wantIs: errEnrollTestBoom,
		},
		{
			name: "operator session deactivation fails",
			seed: func(t *testing.T, e *enrollTestEnv) { e.seedRequest(t, completedOperatorRequest(t, e)) },
			setup: func(e *enrollTestEnv) {
				e.opSession.deactivateErr = errEnrollTestBoom
			},
			want:   "deactivate operator session",
			wantIs: errEnrollTestBoom,
		},
		{
			name: "operator termination write fails",
			seed: func(t *testing.T, e *enrollTestEnv) {
				e.seedRequest(t, completedOperatorRequest(t, e))
				seedEnrollOperator(t, e, "op-1", enrollTestOwnerID, "fp-host-a", constants.OperatorStatusActive, "op-session-1")
			},
			setup: func(e *enrollTestEnv) {
				e.store.updErr = errEnrollTestBoom
			},
			want:   "terminate operator",
			wantIs: errEnrollTestBoom,
		},
		{
			name: "marking the request revoked fails",
			seed: func(t *testing.T, e *enrollTestEnv) { e.seedRequest(t, completedAppRequest()) },
			setup: func(e *enrollTestEnv) {
				e.store.condErr = errEnrollTestBoom
			},
			want:   "mark revoked",
			wantIs: errEnrollTestBoom,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newEnrollTestEnv(t)
			tt.seed(t, env)
			tt.setup(env)

			_, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, revokePayload()))

			require.ErrorIs(t, err, tt.wantIs)
			assert.Contains(t, err.Error(), tt.want)
		})
	}

	t.Run("lost compare-and-set reports invalid state", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		env.seedRequest(t, completedAppRequest())
		lost := false
		env.store.condApplied = &lost

		_, err := env.handler.HandleRevoke(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, revokePayload()))

		require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidState)
	})

	t.Run("cancelled context", func(t *testing.T) {
		env := newEnrollTestEnv(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := env.handler.HandleRevoke(ctx, enrollTestMessage(t, constants.EventPlatformEnrollmentRevokeRequested, revokePayload()))

		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("undecodable payload", func(t *testing.T) {
		env := newEnrollTestEnv(t)

		_, err := env.handler.HandleRevoke(t.Context(), &PubSubCommandMessage{EventType: constants.EventPlatformEnrollmentRevokeRequested, Payload: []byte{0xff}})

		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Cross-cutting
// ---------------------------------------------------------------------------

func TestReceiptSummaries_NeverContainCredentialMaterial(t *testing.T) {
	env := newEnrollTestEnv(t)
	env.seedRequest(t, enrollTestOperatorRequest(models.PlatformEnrollmentStateIssuing))

	summary, err := env.handler.HandleIssue(t.Context(), enrollTestMessage(t, constants.EventPlatformEnrollmentIssueRequested,
		issuePayload(commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR)))

	require.NoError(t, err)
	for _, secret := range []string{"operator-csr", "cli-csr", "BEGIN CERTIFICATE", "trust-bundle-pem", "operator-chain", "cli-chain"} {
		assert.NotContainsf(t, summary, secret, "receipt summary must not contain %q", secret)
	}
	assert.False(t, strings.Contains(summary, "\n"), "summary must be a single audit line")
}
