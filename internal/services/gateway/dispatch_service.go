// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	govpkg "github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// DispatchTimeout is the maximum time to wait for an operator result after
// publishing a command. The round-trip includes in-process publish, WS delivery,
// operator L4/L5 verification and execution, WS publish back, and in-process
// handler delivery.
const DispatchTimeout = 30 * time.Second

// InferenceProgressResultBuffer is the in-process results-channel capacity
// while streaming inference progress. Overflow is a typed backpressure
// failure, not a dropped event.
const InferenceProgressResultBuffer = 64

// EnvelopeExpiry is the lifetime of a dispatched GovernanceEnvelope from
// construction to operator expiry rejection.
const EnvelopeExpiry = 5 * time.Minute

// BuildEnvelopeParams is the input to BuildGovernanceEnvelope. The gateway
// owns the state Merkle root and governance posture; callers supply the
// command intent fields and the resolved operator/session identifiers.
// Application context fields (CaseID, InvestigationID, TaskID,
// WebSessionID, CliSessionID) are propagated from the command intent on
// the pubsub path and from the HTTP request on the dispatch path,
// ensuring uniform context propagation across transports.
//
// Doctrine is the L1 validator the builder runs against the decoded typed
// payload before setting L1.Validated. A nil doctrine fails closed: the
// builder cannot assert L1.Validated=true without screening.
type BuildEnvelopeParams struct {
	OperatorID        string
	OperatorSessionID string
	EventType         string
	Payload           []byte
	TargetResource    string
	RequestorUserID   string
	ActingAppID       string
	StateMerkleRoot   string
	CaseID            string
	InvestigationID   string
	TaskID            string
	WebSessionID      string
	CliSessionID      string
	Posture           string
	Doctrine          *governance.L1Doctrine
}

// BuildGovernanceEnvelope constructs a canonical GovernanceEnvelope with
// timestamp, expiry, nonce, L1 doctrine screening, and the supplied
// identity and state-root fields. It computes the transaction hash via
// govpkg.GenerateMessageID and sets both Id and TransactionHash to that
// value. The returned envelope is ready for protojson marshaling and
// fan-out to an operator's cmd channel. This is the single envelope
// construction authority for both the HTTP DispatchService and the
// WebSocket PubSub relay.
//
// L1 screening: the builder decodes the typed payload via the shared
// governance.DecodePayloadForAction, runs Doctrine.ValidatePayload, and
// sets L1.Validated=true only after a clean pass. It fails closed on nil
// doctrine (ErrTxDoctrineMissing), decode failure
// (ErrTxPayloadDecodeFailed), and L1 forbidden-pattern violations
// (ErrTxL1ValidationFailed). Governed request events without a typed proto
// decode case fail closed with ErrTxPayloadDecoderMissing.
//
// L3 gating: the gateway dispatch path cannot mint L3 human proofs. When
// the posture requires L3 proof (ratify, notary) and the action is a
// mutation, the builder rejects the envelope early with
// ErrTxL3ProofUnmintable rather than publishing a mutation that cannot
// satisfy L3 at the operator.
func BuildGovernanceEnvelope(params BuildEnvelopeParams) (*commonv1.GovernanceEnvelope, error) {
	if params.Posture == "" {
		return nil, fmt.Errorf("gateway: build envelope: %w", constants.ErrEnvelopePostureMissing)
	}
	if params.Doctrine == nil {
		return nil, fmt.Errorf("gateway: build envelope: %w", constants.ErrTxDoctrineMissing)
	}

	eventType := constants.EventType(params.EventType)
	actionType, err := constants.ValidateGovernedRequest(eventType)
	if err != nil {
		return nil, fmt.Errorf("gateway: build envelope: %w", err)
	}

	// L3 gate: the gateway dispatch path cannot mint L3 human proofs.
	// Reject mutation-classified actions under postures that require L3
	// before constructing an envelope that cannot satisfy L3 at the
	// operator.
	posture, err := governance.ParseGovernancePosture(params.Posture)
	if err != nil {
		return nil, fmt.Errorf("gateway: build envelope: %w", err)
	}
	if posture.RequiresL3Proof() && actionType.IsMutation() {
		return nil, fmt.Errorf("gateway: build envelope: %w", constants.ErrTxL3ProofUnmintable)
	}

	// L1 screening: decode the typed payload and run doctrine validation.
	l1Validated := false
	decoded, err := governance.DecodePayloadForAction(actionType, params.Payload)
	if err != nil {
		return nil, fmt.Errorf("gateway: build envelope: %w", constants.ErrTxPayloadDecodeFailed)
	}
	if decoded == nil {
		return nil, fmt.Errorf("gateway: build envelope: %w", constants.ErrTxPayloadDecoderMissing)
	}
	if violations := params.Doctrine.ValidatePayload(decoded); len(violations) > 0 {
		return nil, fmt.Errorf("gateway: build envelope: %w: %s", constants.ErrTxL1ValidationFailed, strings.Join(violations, ", "))
	}
	l1Validated = true

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("gateway: build envelope: generate nonce: %w", err)
	}

	env := &commonv1.GovernanceEnvelope{
		ProtocolVersion:   "1.0",
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(EnvelopeExpiry)),
		SourceComponent:   commonv1.Component_COMPONENT_CLIENT,
		OperatorId:        params.OperatorID,
		OperatorSessionId: params.OperatorSessionID,
		ActionType:        string(actionType),
		TargetResource:    params.TargetResource,
		EventType:         string(eventType),
		Payload:           params.Payload,
		StateMerkleRoot:   params.StateMerkleRoot,
		Nonce:             hex.EncodeToString(nonce),
		RequestorUserId:   params.RequestorUserID,
		ActingAppId:       params.ActingAppID,
		CaseId:            params.CaseID,
		InvestigationId:   params.InvestigationID,
		TaskId:            params.TaskID,
		WebSessionId:      params.WebSessionID,
		CliSessionId:      params.CliSessionID,
		Posture:           params.Posture,
		Governance: &commonv1.GovernanceMetadata{
			L1: &commonv1.L1Metadata{Validated: l1Validated},
		},
	}

	txHash, err := govpkg.GenerateMessageID(env)
	if err != nil {
		return nil, fmt.Errorf("gateway: build envelope: generate message ID: %w", err)
	}
	env.Id = txHash
	env.TransactionHash = txHash

	return env, nil
}

// DispatchRequest is the input to the command dispatch service.
type DispatchRequest struct {
	TargetOperatorSessionID string
	EventType               string
	Payload                 []byte
	TargetResource          string
	RequestorUserID         string
	ActingAppID             string
	CaseID                  string
	InvestigationID         string
	TaskID                  string
	WebSessionID            string
	CliSessionID            string

	// Timeout is the explicit request deadline for this dispatch. Zero
	// applies DispatchTimeout. Inference dispatches set the provider-aware
	// RequestDeadline from the inference dispatch package so the wait
	// outlives an in-flight provider call.
	Timeout time.Duration

	// OnInferenceProgress receives bounded provider progress telemetry while
	// waiting for the authoritative terminal completion.
	OnInferenceProgress func(*operatorv1.InferenceProgressEvent) error
}

// DispatchResult is the output of a successful command dispatch. For
// inference dispatches, Receipt carries the verified final signed
// ActionReceipt and InferenceResult carries the verified complete result;
// the digest, receipt signature, and receipt persistence attestation were
// verified before Dispatch returned.
type DispatchResult struct {
	TransactionID   string
	ResultEnvelope  *commonv1.GovernanceEnvelope
	Receipt         *operatorv1.ActionReceipt
	InferenceResult *operatorv1.InferenceResult
}

// operatorSessionValidator resolves an operator session ID to the operator
// document. AuthService implements this; the interface makes the dispatch
// service's dependency on auth explicit and testable.
type operatorSessionValidator interface {
	ValidateOperatorSession(operatorSessionID string) (*models.OperatorDocumentGo, error)
}

// L2ConsensusDeliberator sends an envelope to an L2 consensus service for
// deliberation and returns the envelope bytes with L2 votes populated. The
// gateway dispatch path calls this after envelope construction under
// postures that require L2 signatures (consensus, notary). When the
// deliberator is nil or the posture does not require L2, deliberation is
// skipped and the envelope proceeds without L2 votes (failing closed at L4
// verification under a posture that requires them).
type L2ConsensusDeliberator interface {
	Deliberate(ctx context.Context, envelopeBytes []byte) ([]byte, error)
}

// DispatchService constructs a GovernanceEnvelope, publishes it to an operator's
// cmd channel via the in-process WS broker, and correlates the operator's result
// published on the results channel back to the originating request.
//
// The gateway owns the state Merkle root and the governance posture. The
// dispatch service sets StateMerkleRoot to the gateway's current state root.
// Under DoctrinePosture (the docker-compose default), no L2 votes or L3 proofs
// are required for read-only commands. Under postures that require L2
// signatures (consensus, notary), the dispatch service deliberates the
// constructed envelope through l2Deliberator before publish. Mutations under
// postures that require L3 proof (ratify, notary) are rejected at envelope
// construction because the gateway dispatch path cannot mint human proofs.
type DispatchService struct {
	logger            *slog.Logger
	pubsub            *GatewayWebSocketHandler
	stateRootProvider governance.StateRootProvider
	auth              operatorSessionValidator
	posture           string
	doctrine          *governance.L1Doctrine
	l2Deliberator     L2ConsensusDeliberator
	signerStore       governance.SignerStore
}

// NewDispatchService creates a DispatchService wired to the gateway's in-process
// pub/sub broker, state root provider, auth service, L1 doctrine, L2
// consensus deliberator, and receipt signer store. The posture is the
// gateway's governance posture, injected into every envelope built by this
// service so the operator reads it per-transaction at L4 verification time.
// The doctrine is the L1 validator the builder runs against the decoded
// typed payload. The l2Deliberator is invoked after envelope construction
// under postures that require L2 signatures; a nil deliberator skips
// deliberation (the envelope fails closed at L4 under such postures). The
// signerStore resolves the operator's actuator public key for inference
// completion receipt verification; a nil store fails inference dispatch
// closed.
func NewDispatchService(logger *slog.Logger, pubsubHandler *GatewayWebSocketHandler, stateRootProvider governance.StateRootProvider, auth operatorSessionValidator, posture string, doctrine *governance.L1Doctrine, l2Deliberator L2ConsensusDeliberator, signerStore governance.SignerStore) *DispatchService {
	return &DispatchService{
		logger:            logger,
		pubsub:            pubsubHandler,
		stateRootProvider: stateRootProvider,
		auth:              auth,
		posture:           posture,
		doctrine:          doctrine,
		l2Deliberator:     l2Deliberator,
		signerStore:       signerStore,
	}
}

// Dispatch sends a signed command to the target operator and waits for the result.
// The envelope is constructed with the gateway's current state root, a unique
// nonce, and a near-future expiry. The result is correlated by transaction ID.
// Returns an error if the operator session is invalid, envelope construction
// fails, L2 deliberation fails, or the result does not arrive within DispatchTimeout.
func (d *DispatchService) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResult, error) {
	// 1. Resolve the target operator session.
	op, err := d.auth.ValidateOperatorSession(req.TargetOperatorSessionID)
	if err != nil {
		return nil, fmt.Errorf("dispatch: validate operator session: %w", err)
	}

	operatorID := op.ID
	operatorSessionID := op.OperatorSessionID

	actionType, err := constants.ValidateGovernedRequest(constants.EventType(req.EventType))
	if err != nil {
		return nil, fmt.Errorf("dispatch: %w", err)
	}

	if err := validateWitnessCommandDispatch(op, string(actionType), req.Payload); err != nil {
		return nil, err
	}

	// 2. Fetch the gateway's current state root.
	stateRoot, err := d.stateRootProvider.GetCurrentStateRoot()
	if err != nil {
		return nil, fmt.Errorf("dispatch: get state root: %w", err)
	}

	// 3. Build the GovernanceEnvelope via the shared construction helper.
	// BuildGovernanceEnvelope runs L1 screening and rejects mutations under
	// L3-requiring postures (the gateway dispatch path cannot mint L3 proofs).
	env, err := BuildGovernanceEnvelope(BuildEnvelopeParams{
		OperatorID:        operatorID,
		OperatorSessionID: operatorSessionID,
		EventType:         req.EventType,
		Payload:           req.Payload,
		TargetResource:    req.TargetResource,
		RequestorUserID:   req.RequestorUserID,
		ActingAppID:       req.ActingAppID,
		StateMerkleRoot:   stateRoot,
		CaseID:            req.CaseID,
		InvestigationID:   req.InvestigationID,
		TaskID:            req.TaskID,
		WebSessionID:      req.WebSessionID,
		CliSessionID:      req.CliSessionID,
		Posture:           d.posture,
		Doctrine:          d.doctrine,
	})
	if err != nil {
		return nil, fmt.Errorf("dispatch: %w", err)
	}
	txHash := env.Id

	// 4. Marshal as protojson (the canonical wire format).
	wire, err := protojson.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("dispatch: marshal envelope: %w", err)
	}

	// 5. Under postures that require L2 signatures (consensus, notary),
	//    deliberate the envelope through the L2 consensus service before
	//    publish. The deliberator collects signed votes and returns the
	//    envelope with L2 metadata populated. A nil deliberator skips
	//    deliberation; the envelope fails closed at L4 verification under
	//    such postures. A deliberation error fails closed before publish.
	posture, perr := governance.ParseGovernancePosture(d.posture)
	if perr != nil {
		return nil, fmt.Errorf("dispatch: %w", perr)
	}
	if posture.RequiresL2Signature() && d.l2Deliberator != nil {
		deliberated, derr := d.l2Deliberator.Deliberate(ctx, wire)
		if derr != nil {
			return nil, fmt.Errorf("dispatch: l2 deliberation: %w", derr)
		}
		wire = deliberated
	}

	// 6. Register an in-process handler on the operator's results channel to
	//    correlate the result by transaction ID.
	resultsChannel := pubsub.ResultsChannel(operatorID, operatorSessionID)
	resultBuffer := 1
	if req.OnInferenceProgress != nil {
		resultBuffer = InferenceProgressResultBuffer
	}
	resultCh := make(chan *commonv1.GovernanceEnvelope, resultBuffer)
	overflow := make(chan error, 1)

	handler := func(channel string, data []byte) {
		resultEnv := &commonv1.GovernanceEnvelope{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, resultEnv); err != nil {
			d.logger.Warn("dispatch: failed to unmarshal result envelope", "error", err)
			return
		}
		if resultEnv.Id == txHash {
			if isShellCommandDispatch(req.EventType) && !isOperatorCommandTerminalResult(resultEnv) {
				return
			}
			if err := constants.ValidateGovernedResultEnvelope(
				constants.EventType(req.EventType),
				constants.EventType(resultEnv.GetEventType()),
				constants.ActionType(resultEnv.GetActionType()),
			); err != nil {
				d.logger.Warn("dispatch: reject result envelope", "error", err, "transaction_id", txHash)
				return
			}
			select {
			case resultCh <- resultEnv:
			default:
				if req.OnInferenceProgress != nil {
					select {
					case overflow <- constants.ErrInferenceProgressBackpressure:
					default:
					}
					d.logger.Warn("dispatch: inference progress channel full, failing closed",
						"transaction_id", txHash,
						"event_type", resultEnv.GetEventType())
					return
				}
				d.logger.Warn("dispatch: result channel full, dropping event",
					"transaction_id", txHash,
					"event_type", resultEnv.GetEventType())
			}
		}
	}
	unregister := d.pubsub.RegisterHandler(resultsChannel, handler)
	defer unregister()

	// 7. Publish the envelope to the operator's cmd channel. Zero delivery
	//    is a terminal transport failure: no operator received the command,
	//    so no result can ever arrive.
	cmdChannel := pubsub.CmdChannel(operatorID, operatorSessionID)
	delivered := d.pubsub.Publish(cmdChannel, wire)
	d.logger.Info("dispatch: published command",
		"transaction_id", txHash,
		"cmd_channel", cmdChannel,
		"results_channel", resultsChannel,
		"delivered", delivered)
	if delivered == 0 {
		return nil, fmt.Errorf("dispatch: %w", constants.ErrDispatchNoDelivery)
	}

	// 8. Wait for the result with the request deadline. The caller sets
	//    Timeout explicitly (inference sets its provider-aware deadline);
	//    zero applies DispatchTimeout.
	deadline := req.Timeout
	if deadline <= 0 {
		deadline = DispatchTimeout
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	var progressEvents []*operatorv1.InferenceProgressEvent
	for {
		select {
		case overflowErr := <-overflow:
			return nil, fmt.Errorf("dispatch: %w", overflowErr)
		case resultEnv := <-resultCh:
			if actionType == constants.ActionTypeInference {
				if progress, ok := decodeInferenceProgressEnvelope(resultEnv); ok {
					if req.OnInferenceProgress != nil {
						if err := req.OnInferenceProgress(progress); err != nil {
							return nil, fmt.Errorf("dispatch: %w", err)
						}
						progressEvents = append(progressEvents, progress)
					}
					continue
				}
				result, err := d.verifyInferenceCompletion(env, resultEnv)
				if err != nil {
					return nil, err
				}
				if req.OnInferenceProgress != nil {
					if err := models.ReconcileInferenceProgress(progressEvents, result.InferenceResult); err != nil {
						return nil, fmt.Errorf("dispatch: %w", err)
					}
				}
				return result, nil
			}
			if isShellCommandDispatch(req.EventType) {
				if payload := operatorCommandResultPayload(resultEnv); len(payload) > 0 {
					resultEnv.Payload = payload
				}
			}
			return &DispatchResult{
				TransactionID:  txHash,
				ResultEnvelope: resultEnv,
			}, nil
		case <-timeoutCtx.Done():
			if err := ctx.Err(); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil, fmt.Errorf("dispatch: %w: %w", constants.ErrInferenceCanceled, err)
				}
				return nil, fmt.Errorf("dispatch: %w", err)
			}
			return nil, fmt.Errorf("dispatch: %w after %s (transaction %s)", constants.ErrDispatchResultTimeout, deadline, txHash)
		}
	}
}

func decodeInferenceProgressEnvelope(env *commonv1.GovernanceEnvelope) (*operatorv1.InferenceProgressEvent, bool) {
	if env == nil || env.GetEventType() != string(constants.Event.Operator.Inference.ProgressUpdated) {
		return nil, false
	}
	progress := &operatorv1.InferenceProgressEvent{}
	if err := proto.Unmarshal(env.GetPayload(), progress); err != nil {
		return nil, false
	}
	return progress, true
}

// verifyInferenceCompletion decodes the protocol-owned InferenceCompletion
// carried in the result envelope's payload and verifies that the returned
// result and its final signed receipt are one outcome: the receipt's
// transaction identity must match the dispatched envelope, its signature
// and persistence attestation must verify against the operator's actuator
// key, a FAILED receipt terminates the wait as a typed failure, and the
// recomputed result digest must equal both the receipt's result_summary and
// the result's own result_digest. Fail-closed on every check.
func (d *DispatchService) verifyInferenceCompletion(cmdEnv, resultEnv *commonv1.GovernanceEnvelope) (*DispatchResult, error) {
	completion := &operatorv1.InferenceCompletion{}
	if err := proto.Unmarshal(resultEnv.Payload, completion); err != nil {
		return nil, fmt.Errorf("dispatch: %w: %v", constants.ErrInferenceResultDecode, err)
	}

	receipt := completion.GetReceipt()
	if receipt == nil {
		return nil, fmt.Errorf("dispatch: %w", constants.ErrInferenceCompletionNoReceipt)
	}
	if receipt.TransactionId != cmdEnv.Id || receipt.TransactionHash != cmdEnv.TransactionHash {
		return nil, fmt.Errorf("dispatch: %w: receipt transaction identity does not match dispatched envelope", constants.ErrInferenceReceiptVerify)
	}

	if d.signerStore == nil {
		return nil, fmt.Errorf("dispatch: %w: receipt signer store not configured", constants.ErrInferenceReceiptVerify)
	}
	pubKey, err := d.signerStore.GetTrustedSigner(receipt.SignerKeyId)
	if err != nil {
		return nil, fmt.Errorf("dispatch: %w: %v", constants.ErrInferenceReceiptVerify, err)
	}
	if pubKey == nil {
		return nil, fmt.Errorf("dispatch: %w: unknown signer key id %q", constants.ErrInferenceReceiptVerify, receipt.SignerKeyId)
	}
	if err := governance.VerifyActionReceiptSignature(receipt, pubKey); err != nil {
		return nil, fmt.Errorf("dispatch: %w: %v", constants.ErrInferenceReceiptVerify, err)
	}
	if err := governance.VerifyReceiptPersistenceAttestation(receipt, pubKey); err != nil {
		return nil, fmt.Errorf("dispatch: %w: %v", constants.ErrInferenceReceiptVerify, err)
	}

	if receipt.Status != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		return nil, fmt.Errorf("dispatch: %w: %s", inferenceReceiptFailureError(receipt), receipt.ResultSummary)
	}

	result := completion.GetResult()
	if result == nil {
		return nil, fmt.Errorf("dispatch: %w", constants.ErrInferenceCompletionNoResult)
	}
	digest, err := models.ComputeInferenceResultDigest(result)
	if err != nil {
		return nil, fmt.Errorf("dispatch: %w", err)
	}
	if result.ResultDigest != digest || receipt.ResultSummary != digest {
		return nil, fmt.Errorf("dispatch: %w", constants.ErrInferenceResultDigestMismatch)
	}
	request := &operatorv1.InferenceRequested{}
	if err := proto.Unmarshal(cmdEnv.Payload, request); err != nil {
		return nil, fmt.Errorf("dispatch: %w: %v", constants.ErrInferenceResultDecode, err)
	}
	if result.GetProviderAttemptId() != request.GetProviderAttemptId() ||
		(request.GetModel() != "" && result.GetRequestedModel() != request.GetModel()) ||
		result.GetRequestedModelDigest() != request.GetModelDigest() ||
		result.GetCampaignId() != request.GetCampaignId() ||
		result.GetRunId() != request.GetRunId() ||
		result.GetAssignmentId() != request.GetAssignmentId() ||
		result.GetEvaluationAttemptId() != request.GetEvaluationAttemptId() ||
		result.GetScenarioId() != request.GetScenarioId() ||
		result.GetModelRegistryDigest() != request.GetModelRegistryDigest() ||
		result.GetRetryCount() != request.GetRetryCount() ||
		result.GetRetryClassification() != models.ClassifyRetry(request.GetRetryCount()) ||
		result.GetLoadState() != models.ClassifyLoadState(result.LoadDurationNs) {
		return nil, fmt.Errorf("dispatch: %w", constants.ErrInferenceIdentityMismatch)
	}
	if request.GetProviderAttemptId() == "" || !models.IsSHA256Hex(result.GetNormalizedRequestHash()) || !models.IsSHA256Hex(result.GetOutputHash()) {
		return nil, fmt.Errorf("dispatch: %w", constants.ErrInferenceEvidenceHashInvalid)
	}
	if request.GetModelDigest() != "" && result.GetServedModelDigest() != request.GetModelDigest() {
		return nil, fmt.Errorf("dispatch: %w", constants.ErrInferenceModelDigestMismatch)
	}

	return &DispatchResult{
		TransactionID:   cmdEnv.Id,
		ResultEnvelope:  resultEnv,
		Receipt:         receipt,
		InferenceResult: result,
	}, nil
}

// inferenceReceiptFailureError maps a signed FAILED receipt's typed
// failure_code back to the corresponding sentinel so callers can classify
// governance rejections, client faults, and provider failures via errors.Is
// without parsing the result_summary text. The receipt signature is verified
// before this runs, so the code is authentic. An unspecified or unknown code
// falls back to the generic execution-failure sentinel.
func inferenceReceiptFailureError(receipt *operatorv1.ActionReceipt) error {
	switch receipt.FailureCode {
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_GOVERNANCE_REJECTED:
		return constants.ErrInferenceGovernanceRejected
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_OVERRIDE_DENIED:
		return constants.ErrInferenceModelOverrideDenied
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_ROLE_INVALID:
		return constants.ErrInferenceRoleInvalid
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_REF_INVALID:
		return constants.ErrInferenceModelRefInvalid
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_BACKEND_UNAVAILABLE:
		return constants.ErrInferenceBackendUnavailable
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_BACKEND_TIMEOUT:
		return constants.ErrInferenceBackendTimeout
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_GENERATE_FAILED:
		return constants.ErrInferenceGenerateFailed
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_NOT_FOUND:
		return constants.ErrInferenceModelNotFound
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_PROVIDER_RESPONSE_INVALID:
		return constants.ErrInferenceProviderResponseInvalid
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_GENERATION_OPTIONS_INVALID:
		return constants.ErrInferenceGenerationOptionsInvalid
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_CAPABILITY_UNSUPPORTED:
		return constants.ErrInferenceCapabilityUnsupported
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_PROVIDER_ATTEMPT_REQUIRED:
		return constants.ErrInferenceProviderAttemptRequired
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_IDENTITY_MISMATCH:
		return constants.ErrInferenceIdentityMismatch
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_DIGEST_MISMATCH:
		return constants.ErrInferenceModelDigestMismatch
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_EVIDENCE_HASH_INVALID:
		return constants.ErrInferenceEvidenceHashInvalid
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_MODEL_REGISTRY_INVALID:
		return constants.ErrInferenceModelRegistryInvalid
	case operatorv1.ReceiptFailureCode_RECEIPT_FAILURE_CODE_CAMPAIGN_BINDING_INVALID:
		return constants.ErrInferenceCampaignBindingInvalid
	default:
		return constants.ErrInferenceReceiptFailed
	}
}

// DispatchResponse is the typed JSON response for POST /api/v1/operators/commands.
// ResultPayload carries the operator's proto-marshaled result payload (e.g.
// FsReadResult) so the caller can inspect the execution outcome. It is
// base64-encoded in JSON per protojson/encoding/json conventions for []byte.
type DispatchResponse struct {
	Success       bool   `json:"success"`
	TransactionID string `json:"transaction_id"`
	EventType     string `json:"event_type,omitempty"`
	ActionType    string `json:"action_type,omitempty"`
	ResultPayload []byte `json:"result_payload,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ToResponse converts a DispatchResult to a DispatchResponse.
func (r *DispatchResult) ToResponse() DispatchResponse {
	resp := DispatchResponse{
		Success:       true,
		TransactionID: r.TransactionID,
	}
	if r.ResultEnvelope != nil {
		resp.EventType = r.ResultEnvelope.EventType
		resp.ActionType = r.ResultEnvelope.ActionType
		if isOperatorCommandTerminalResult(r.ResultEnvelope) {
			resp.ResultPayload = operatorCommandResultPayload(r.ResultEnvelope)
		} else {
			resp.ResultPayload = r.ResultEnvelope.Payload
		}
	}
	return resp
}

func isShellCommandDispatch(eventType string) bool {
	return eventType == string(constants.Event.Operator.Command.Requested)
}

func isOperatorCommandTerminalResult(env *commonv1.GovernanceEnvelope) bool {
	if env == nil {
		return false
	}
	switch env.GetEventType() {
	case string(constants.Event.Operator.Command.Completed),
		string(constants.Event.Operator.Command.Failed),
		string(constants.Event.Operator.Command.Cancelled):
		return true
	default:
		return false
	}
}

func operatorCommandResultPayload(env *commonv1.GovernanceEnvelope) []byte {
	if env == nil {
		return nil
	}
	if len(env.GetPayload()) > 0 {
		return env.GetPayload()
	}
	if env.GetIntentData() == nil {
		return nil
	}
	wire, err := protojson.Marshal(env.GetIntentData())
	if err != nil {
		return nil
	}
	result := &operatorv1.CommandResult{}
	if err := protojson.Unmarshal(wire, result); err != nil {
		return nil
	}
	out, err := proto.Marshal(result)
	if err != nil {
		return nil
	}
	return out
}

// OperatorCommandRequest is the typed JSON request for POST /api/v1/operators/commands.
type OperatorCommandRequest struct {
	TargetOperatorSessionID string `json:"target_operator_session_id"`
	EventType               string `json:"event_type"`
	Payload                 []byte `json:"payload"`
	TargetResource          string `json:"target_resource,omitempty"`
	CaseID                  string `json:"case_id,omitempty"`
	InvestigationID         string `json:"investigation_id,omitempty"`
	TaskID                  string `json:"task_id,omitempty"`
	WebSessionID            string `json:"web_session_id,omitempty"`
	CliSessionID            string `json:"cli_session_id,omitempty"`
}

// Validate returns an error if the request is missing required fields.
func (r *OperatorCommandRequest) Validate() error {
	if r.TargetOperatorSessionID == "" {
		return constants.ErrGatewayOperatorSessionIDRequired
	}
	if r.EventType == "" {
		return constants.ErrTxUnknownEventType
	}
	if len(r.Payload) == 0 {
		return constants.ErrTxPayloadMissing
	}
	if _, err := constants.ValidateGovernedRequest(constants.EventType(r.EventType)); err != nil {
		return err
	}
	return nil
}

// DispatchControllerDeps groups all dependencies for DispatchController.
type DispatchControllerDeps struct {
	DispatchSvc *DispatchService
	Responder   *response.Writer
	Logger      *slog.Logger
}

// DispatchController handles POST /api/v1/operators/commands, the mTLS-protected
// entry point for dispatching signed commands to operators.
type DispatchController struct {
	dispatchSvc *DispatchService
	responder   *response.Writer
	logger      *slog.Logger
}

// newDispatchController creates a DispatchController from its deps.
func newDispatchController(d DispatchControllerDeps) *DispatchController {
	return &DispatchController{
		dispatchSvc: d.DispatchSvc,
		responder:   d.Responder,
		logger:      d.Logger,
	}
}

// HandleDispatch is the HTTP handler for POST /api/v1/operators/commands.
func (c *DispatchController) HandleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		c.responder.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req OperatorCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.responder.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := req.Validate(); err != nil {
		c.responder.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	// Extract the requestor's user ID from the mTLS identity context.
	requestorUserID, _ := r.Context().Value(constants.ContextKeyUserID).(string)
	actingAppID, _ := r.Context().Value(constants.ContextKeyAppID).(string)

	result, err := c.dispatchSvc.Dispatch(r.Context(), DispatchRequest{
		TargetOperatorSessionID: req.TargetOperatorSessionID,
		EventType:               req.EventType,
		Payload:                 req.Payload,
		TargetResource:          req.TargetResource,
		RequestorUserID:         requestorUserID,
		ActingAppID:             actingAppID,
		CaseID:                  req.CaseID,
		InvestigationID:         req.InvestigationID,
		TaskID:                  req.TaskID,
		WebSessionID:            req.WebSessionID,
		CliSessionID:            req.CliSessionID,
	})
	if err != nil {
		c.logger.Error("dispatch: command dispatch failed", "error", err)
		status := http.StatusInternalServerError
		if errors.Is(err, constants.ErrWitnessCommandNotCapable) {
			status = http.StatusUnprocessableEntity
		}
		c.responder.Error(w, status, err.Error())
		return
	}

	c.responder.JSON(w, http.StatusOK, result.ToResponse())
}
