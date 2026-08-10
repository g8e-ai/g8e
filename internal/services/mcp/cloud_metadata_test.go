package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

func TestNewStdioConfigSimple_EmptyPath(t *testing.T) {
	t.Parallel()

	cfg, err := NewStdioConfigSimple("")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, constants.ErrMCPConfigBinaryPathEmpty)
}

func TestNewStdioConfigSimple_WhitespaceOnly(t *testing.T) {
	t.Parallel()

	cfg, err := NewStdioConfigSimple("   ")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.ErrorIs(t, err, constants.ErrMCPConfigBinaryPathWhitespace)
}

func TestNewStdioConfigSimple_ValidPath(t *testing.T) {
	t.Parallel()

	cfg, err := NewStdioConfigSimple("/usr/local/bin/g8e")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	server, ok := cfg.MCPServers["g8e-native"]
	require.True(t, ok, "g8e-native server should be present")
	assert.Equal(t, "/usr/local/bin/g8e", server.Command)
	assert.Equal(t, []string{"mcp", "stdio"}, server.Args)
	assert.False(t, server.Disabled)
}

func TestConvertToFieldValue_Nil(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue(nil)
	assert.True(t, v.Null)
}

func TestConvertToFieldValue_String(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue("hello")
	require.NotNil(t, v.Str)
	assert.Equal(t, "hello", *v.Str)
	assert.False(t, v.Null)
}

func TestConvertToFieldValue_Float64(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue(3.14)
	require.NotNil(t, v.Float64)
	assert.Equal(t, 3.14, *v.Float64)
}

func TestConvertToFieldValue_Bool(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue(true)
	require.NotNil(t, v.Bool)
	assert.True(t, *v.Bool)
}

func TestConvertToFieldValue_Array(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue([]interface{}{"a", "b", "c"})
	require.Len(t, v.Array, 3)
	require.NotNil(t, v.Array[0].Str)
	assert.Equal(t, "a", *v.Array[0].Str)
	require.NotNil(t, v.Array[1].Str)
	assert.Equal(t, "b", *v.Array[1].Str)
	require.NotNil(t, v.Array[2].Str)
	assert.Equal(t, "c", *v.Array[2].Str)
}

func TestConvertToFieldValue_Object(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue(map[string]interface{}{
		"name":  "test",
		"count": float64(42),
	})
	require.Contains(t, v.Object, "name")
	assert.Equal(t, "test", *v.Object["name"].Str)
	require.Contains(t, v.Object, "count")
	assert.Equal(t, float64(42), *v.Object["count"].Float64)
}

func TestConvertToFieldValue_NestedArrayInObject(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue(map[string]interface{}{
		"items": []interface{}{"x", "y"},
	})
	require.Contains(t, v.Object, "items")
	require.Len(t, v.Object["items"].Array, 2)
	assert.Equal(t, "x", *v.Object["items"].Array[0].Str)
	assert.Equal(t, "y", *v.Object["items"].Array[1].Str)
}

func TestConvertToFieldValue_UnknownType(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue(42)
	require.NotNil(t, v.Str)
	assert.Equal(t, "42", *v.Str)
}

func TestConvertToFieldValue_EmptyArray(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue([]interface{}{})
	assert.Len(t, v.Array, 0)
}

func TestConvertToFieldValue_EmptyObject(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue(map[string]interface{}{})
	assert.Empty(t, v.Object)
}

func TestConvertToFieldValue_NestedNullInArray(t *testing.T) {
	t.Parallel()

	v := ConvertToFieldValue([]interface{}{nil, "a"})
	require.Len(t, v.Array, 2)
	assert.True(t, v.Array[0].Null)
	require.NotNil(t, v.Array[1].Str)
	assert.Equal(t, "a", *v.Array[1].Str)
}

func TestRuntimeDeps_L2ConsensusDeliberator(t *testing.T) {
	t.Parallel()

	t.Run("nil deliberator by default", func(t *testing.T) {
		t.Parallel()
		g := &GatewayService{
			logger: slog.Default(),
			envProc: &fakeEnvelopeProcessor{},
			stateRootProvider: &fakeStateRootProvider{root: "test"},
			downstreamURL: "http://downstream",
		}
		assert.Nil(t, g.l2ConsensusDeliberator)
	})

	t.Run("deliberator accessible via field", func(t *testing.T) {
		t.Parallel()
		g := &GatewayService{
			logger: slog.Default(),
			envProc: &fakeEnvelopeProcessor{},
			stateRootProvider: &fakeStateRootProvider{root: "test"},
			downstreamURL: "http://downstream",
		}
		mock := &mockL2ConsensusDeliberator{}
		g.SetL2ConsensusDeliberator(mock)
		assert.Same(t, mock, g.l2ConsensusDeliberator)
	})
}

type mockL2ConsensusDeliberator struct{}

func (m *mockL2ConsensusDeliberator) Deliberate(ctx context.Context, envelopeBytes []byte) ([]byte, error) {
	return nil, nil
}

func TestCloudMetadataTool_Metadata(t *testing.T) {
	t.Parallel()

	tool := &CloudMetadataTool{}
	assert.Equal(t, "cloud_metadata", tool.Name())
	assert.NotEmpty(t, tool.Description())

	schema := tool.InputSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "operation")
	assert.Contains(t, schema.Properties["operation"].Enum, "detect")
	assert.Contains(t, schema.Properties["operation"].Enum, "all")
}

func TestCloudMetadataTool_Execute_InvalidJSON(t *testing.T) {
	t.Parallel()

	tool := &CloudMetadataTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMCPUnmarshalArguments)
}

func TestCloudMetadataTool_Execute_InvalidOperation(t *testing.T) {
	t.Parallel()

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"bogus"}`))
	require.NoError(t, err)
	assert.Len(t, result.Content, 1)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "bogus", payload["operation"])
	assert.Contains(t, payload["error"], "invalid operation")
}

func TestCloudMetadataTool_Execute_DefaultOperation(t *testing.T) {
	origDetect := detectCloudProviderFunc
	t.Cleanup(func() { detectCloudProviderFunc = origDetect })
	detectCloudProviderFunc = func(ctx context.Context) string { return "unknown" }

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Len(t, result.Content, 1)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "detect", payload["operation"])
	assert.Equal(t, "unknown", payload["provider"])
}

func TestCloudMetadataTool_Execute_DetectAWS(t *testing.T) {
	origDetect := detectCloudProviderFunc
	t.Cleanup(func() { detectCloudProviderFunc = origDetect })
	detectCloudProviderFunc = func(ctx context.Context) string { return "aws" }

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"detect"}`))
	require.NoError(t, err)
	assert.Len(t, result.Content, 1)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "aws", payload["provider"])
}

func TestCloudMetadataTool_Execute_AWSInstance(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "aws" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return "i-1234567890abcdef0", nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"instance"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "aws", payload["provider"])
	assert.Equal(t, "i-1234567890abcdef0", payload["instance_id"])
}

func TestCloudMetadataTool_Execute_AWSRegion(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "aws" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		if strings.Contains(url, "placement/region") {
			return "us-west-2", nil
		}
		return "", fmt.Errorf("unexpected URL: %s", url)
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"region"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "aws", payload["provider"])
	assert.Equal(t, "us-west-2", payload["region"])
}

func TestCloudMetadataTool_Execute_AWSRegionFallbackToAZ(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "aws" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		if strings.Contains(url, "placement/region") {
			return "", fmt.Errorf("not available")
		}
		if strings.Contains(url, "placement/availability-zone") {
			return "us-east-1a", nil
		}
		return "", fmt.Errorf("unexpected URL: %s", url)
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"region"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "us-east-1", payload["region"], "region should be derived from AZ by stripping last char")
}

func TestCloudMetadataTool_Execute_AWSAvailabilityZone(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "aws" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return "us-west-2a", nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"availability_zone"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "aws", payload["provider"])
	assert.Equal(t, "us-west-2a", payload["availability_zone"])
}

func TestCloudMetadataTool_Execute_AWSInstanceType(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "aws" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return "t3.medium", nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"instance_type"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "aws", payload["provider"])
	assert.Equal(t, "t3.medium", payload["instance_type"])
}

func TestCloudMetadataTool_Execute_GCPInstance(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "gcp" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		if strings.Contains(url, "/id") {
			return "1234567890", nil
		}
		if strings.Contains(url, "/instance/name") {
			return "my-vm", nil
		}
		return "", fmt.Errorf("unexpected URL: %s", url)
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"instance"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "gcp", payload["provider"])
	assert.Equal(t, "1234567890", payload["instance_id"])
	assert.Equal(t, "my-vm", payload["name"])
}

func TestCloudMetadataTool_Execute_GCPRegion(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "gcp" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return "projects/123/regions/us-central1", nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"region"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "gcp", payload["provider"])
	assert.Equal(t, "us-central1", payload["region"], "region should be last path segment")
}

func TestCloudMetadataTool_Execute_GCPAvailabilityZone(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "gcp" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return "projects/123/zones/us-central1-a", nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"availability_zone"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "gcp", payload["provider"])
	assert.Equal(t, "us-central1-a", payload["availability_zone"])
}

func TestCloudMetadataTool_Execute_GCPInstanceType(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "gcp" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return "projects/123/machineTypes/e2-medium", nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"instance_type"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "gcp", payload["provider"])
	assert.Equal(t, "e2-medium", payload["instance_type"])
}

func TestCloudMetadataTool_Execute_AzureInstance(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "azure" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return `{"compute":{"vmSize":"Standard_D2s_v3","location":"eastus","platformFaultDomain":"1"}}`, nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"instance"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "azure", payload["provider"])
}

func TestCloudMetadataTool_Execute_AzureRegion(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "azure" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return `{"compute":{"location":"westus2"}}`, nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"region"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "azure", payload["provider"])
	assert.Equal(t, "westus2", payload["region"])
}

func TestCloudMetadataTool_Execute_AzureRegionMissingCompute(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "azure" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return `{"network":{}}`, nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"region"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "azure", payload["provider"])
	assert.Equal(t, "unknown", payload["region"])
}

func TestCloudMetadataTool_Execute_AzureAvailabilityZone(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "azure" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return `{"compute":{"platformFaultDomain":"2"}}`, nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"availability_zone"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "azure", payload["provider"])
	assert.Equal(t, "2", payload["availability_zone"])
}

func TestCloudMetadataTool_Execute_AzureInstanceType(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "azure" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return `{"compute":{"vmSize":"Standard_D4s_v3"}}`, nil
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"instance_type"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "azure", payload["provider"])
	assert.Equal(t, "Standard_D4s_v3", payload["instance_type"])
}

func TestCloudMetadataTool_Execute_AzureInstanceTypeError(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "azure" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return "", fmt.Errorf("connection refused")
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"instance_type"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "azure", payload["provider"])
	assert.Contains(t, payload["error"], "connection refused")
}

func TestCloudMetadataTool_Execute_AllAWS(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "aws" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		switch {
		case strings.Contains(url, "instance-id"):
			return "i-abc123", nil
		case strings.Contains(url, "placement/region"):
			return "eu-west-1", nil
		case strings.Contains(url, "placement/availability-zone"):
			return "eu-west-1a", nil
		case strings.Contains(url, "instance-type"):
			return "m5.large", nil
		default:
			return "", fmt.Errorf("unexpected URL: %s", url)
		}
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"all"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "aws", payload["provider"])

	instance, ok := payload["instance"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "i-abc123", instance["instance_id"])

	region, ok := payload["region"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "eu-west-1", region["region"])
}

func TestCloudMetadataTool_Execute_UnsupportedProvider(t *testing.T) {
	origDetect := detectCloudProviderFunc
	origGet := httpGetFunc
	t.Cleanup(func() {
		detectCloudProviderFunc = origDetect
		httpGetFunc = origGet
	})
	detectCloudProviderFunc = func(ctx context.Context) string { return "digitalocean" }
	httpGetFunc = func(ctx context.Context, url string, headers map[string]string) (string, error) {
		return "", fmt.Errorf("should not be called")
	}

	tool := &CloudMetadataTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"operation":"instance"}`))
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].Text), &payload))
	assert.Equal(t, "digitalocean", payload["provider"])
	assert.Contains(t, payload["error"], "unsupported provider")
}

func TestGetInstanceMetadata_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := getInstanceMetadata(context.Background(), "digitalocean")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMCPValidateCloudMetadataUnsupportedProvider)
}

func TestGetRegion_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := getRegion(context.Background(), "digitalocean")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMCPValidateCloudMetadataUnsupportedProvider)
}

func TestGetAvailabilityZone_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := getAvailabilityZone(context.Background(), "digitalocean")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMCPValidateCloudMetadataUnsupportedProvider)
}

func TestGetInstanceType_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := getInstanceType(context.Background(), "digitalocean")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMCPValidateCloudMetadataUnsupportedProvider)
}
