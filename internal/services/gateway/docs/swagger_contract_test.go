package docs

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type swaggerSchemaReference struct {
	Ref string `json:"$ref"`
}

type swaggerResponse struct {
	Schema swaggerSchemaReference `json:"schema"`
}

type swaggerOperation struct {
	Parameters []struct {
		Name   string                 `json:"name"`
		Schema swaggerSchemaReference `json:"schema"`
	} `json:"parameters"`
	Responses struct {
		OK swaggerResponse `json:"200"`
	} `json:"responses"`
}

type swaggerContract struct {
	Definitions struct {
		ActionReceipt struct {
			Properties struct {
				DeterministicStageEvidence struct {
					Items swaggerSchemaReference `json:"items"`
				} `json:"deterministic_stage_evidence"`
				FinalPersistenceAttestation swaggerSchemaReference `json:"final_persistence_attestation"`
			} `json:"properties"`
		} `json:"operatorv1.ActionReceipt"`
		ActionReceiptRecord struct {
			Properties struct {
				ActionReceipt swaggerSchemaReference `json:"action_receipt"`
			} `json:"properties"`
		} `json:"models.ActionReceiptRecord"`
	} `json:"definitions"`
	Paths struct {
		AuditReceipts struct {
			Get swaggerOperation `json:"get"`
		} `json:"/api/v1/audit/receipts"`
		AuditReceiptsExport struct {
			Get swaggerOperation `json:"get"`
		} `json:"/api/v1/audit/receipts/export"`
		GovernanceEnvelopes struct {
			Post swaggerOperation `json:"post"`
		} `json:"/api/v1/governance/envelopes"`
		OperatorsValidate struct {
			Post swaggerOperation `json:"post"`
		} `json:"/api/v1/operators/validate"`
	} `json:"paths"`
}

func TestSwaggerContract_DocumentsCanonicalReceiptsAndOperatorValidation(t *testing.T) {
	var contract swaggerContract
	require.NoError(t, json.Unmarshal(SwaggerJSON, &contract))

	responseCases := []struct {
		name      string
		operation swaggerOperation
		wantRef   string
	}{
		{name: "audit receipt list wrapper", operation: contract.Paths.AuditReceipts.Get, wantRef: "#/definitions/models.AuditReceiptsResponse"},
		{name: "audit receipt export wrapper", operation: contract.Paths.AuditReceiptsExport.Get, wantRef: "#/definitions/models.AuditReceiptsResponse"},
		{name: "governance canonical receipt", operation: contract.Paths.GovernanceEnvelopes.Post, wantRef: "#/definitions/operatorv1.ActionReceipt"},
		{name: "operator validation response", operation: contract.Paths.OperatorsValidate.Post, wantRef: "#/definitions/models.OperatorSessionValidationResponse"},
	}
	for _, tc := range responseCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantRef, tc.operation.Responses.OK.Schema.Ref)
		})
	}

	require.Len(t, contract.Paths.OperatorsValidate.Post.Parameters, 1)
	assert.Equal(t, "binding", contract.Paths.OperatorsValidate.Post.Parameters[0].Name)
	assert.Equal(t, "#/definitions/models.OperatorSessionValidationRequest", contract.Paths.OperatorsValidate.Post.Parameters[0].Schema.Ref)
	assert.Equal(t, "#/definitions/operatorv1.DeterministicStageEvidence", contract.Definitions.ActionReceipt.Properties.DeterministicStageEvidence.Items.Ref)
	assert.Equal(t, "#/definitions/operatorv1.ReceiptPersistenceAttestation", contract.Definitions.ActionReceipt.Properties.FinalPersistenceAttestation.Ref)
	assert.Equal(t, "#/definitions/operatorv1.ActionReceipt", contract.Definitions.ActionReceiptRecord.Properties.ActionReceipt.Ref)
}
