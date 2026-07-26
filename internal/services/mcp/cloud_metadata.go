// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// CloudMetadataTool provides cloud provider metadata detection and information for AWS, Azure, and GCP.
type CloudMetadataTool struct{}

var (
	detectCloudProviderFunc = detectCloudProvider
	httpGetFunc             = httpGetWithTimeout
)

func marshalCloudMetadataResult(result interface{}) (CallToolResult, error) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("cloud_metadata: marshal result: %w: %w", constants.ErrMCPMarshalResult, err)
	}
	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

// Name returns the tool identifier.
func (t *CloudMetadataTool) Name() string {
	return "cloud_metadata"
}

// Description returns a human-readable description.
func (t *CloudMetadataTool) Description() string {
	return "Detects cloud provider (AWS, Azure, GCP) and retrieves instance metadata including region, instance type, and availability zone."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *CloudMetadataTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"operation": {
				Type:        "string",
				Description: "Metadata operation to perform",
				Enum:        []string{"detect", "instance", "region", "availability_zone", "instance_type", "all"},
			},
		},
	}
}

// Execute implements the tool logic.
func (t *CloudMetadataTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req CloudMetadataRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("cloud_metadata: unmarshal arguments: %w: %w", constants.ErrMCPUnmarshalArguments, err)
	}

	if req.Operation == "" {
		req.Operation = "detect"
	}

	if err := validateCloudMetadataOperation(req.Operation); err != nil {
		return marshalCloudMetadataResult(CloudMetadataErrorResponse{
			Operation: req.Operation,
			Error:     err.Error(),
		})
	}

	provider := detectCloudProviderFunc(ctx)
	if provider == "unknown" {
		return marshalCloudMetadataResult(CloudMetadataErrorResponse{
			Operation: req.Operation,
			Provider:  "unknown",
			Message:   "Not running on a detected cloud provider or metadata service unavailable",
		})
	}

	var marshalTarget interface{}
	var err error

	switch req.Operation {
	case "detect":
		marshalTarget = CloudMetadataDetectResult{Provider: provider}
	case "instance":
		marshalTarget, err = getInstanceMetadata(ctx, provider)
	case "region":
		marshalTarget, err = getRegion(ctx, provider)
	case "availability_zone":
		marshalTarget, err = getAvailabilityZone(ctx, provider)
	case "instance_type":
		marshalTarget, err = getInstanceType(ctx, provider)
	case "all":
		marshalTarget, err = getAllMetadata(ctx, provider)
	default:
		return CallToolResult{}, fmt.Errorf("cloud_metadata: invalid operation: %w: %s", constants.ErrMCPValidateCloudMetadataInvalidOperation, req.Operation)
	}

	if err != nil {
		marshalTarget = CloudMetadataErrorResponse{
			Operation: req.Operation,
			Provider:  provider,
			Error:     err.Error(),
		}
	}

	return marshalCloudMetadataResult(marshalTarget)
}

func detectCloudProvider(ctx context.Context) string {
	if _, err := os.Stat(constants.PathSysClassDMIIDProductUUID); err == nil {
		if data, err := os.ReadFile(constants.PathSysClassDMIIDProductUUID); err == nil {
			if strings.Contains(string(data), "ec2") {
				return "aws"
			}
		}
	}

	if _, err := os.Stat(constants.PathSysClassDMIIDSysVendor); err == nil {
		if data, err := os.ReadFile(constants.PathSysClassDMIIDSysVendor); err == nil {
			content := strings.ToLower(string(data))
			if strings.Contains(content, "amazon") {
				return "aws"
			}
			if strings.Contains(content, "microsoft") {
				return "azure"
			}
			if strings.Contains(content, "google") {
				return "gcp"
			}
		}
	}

	client := &http.Client{Timeout: 2 * time.Second}

	awsReq, err := http.NewRequestWithContext(ctx, "GET", "http://169.254.169.254/latest/meta-data/", nil)
	if err == nil {
		if resp, err := client.Do(awsReq); err == nil {
			resp.Body.Close()
			return "aws"
		}
	}

	azureReq, err := http.NewRequestWithContext(ctx, "GET", "http://169.254.169.254/metadata/instance?api-version=2021-02-01", nil)
	if err == nil {
		if resp, err := client.Do(azureReq); err == nil {
			resp.Body.Close()
			return "azure"
		}
	}

	gcpReq, err := http.NewRequestWithContext(ctx, "GET", "http://metadata.google.internal/computeMetadata/v1/", nil)
	if err == nil {
		if resp, err := client.Do(gcpReq); err == nil {
			resp.Body.Close()
			return "gcp"
		}
	}

	return "unknown"
}

func httpGetWithTimeout(ctx context.Context, url string, headers map[string]string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestCreateFailed, err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	return string(body), nil
}

func getInstanceMetadata(ctx context.Context, provider string) (CloudMetadataInstanceResult, error) {
	switch provider {
	case "aws":
		return getAWSInstanceMetadata(ctx)
	case "azure":
		return getAzureInstanceMetadata(ctx)
	case "gcp":
		return getGCPInstanceMetadata(ctx)
	default:
		return CloudMetadataInstanceResult{}, fmt.Errorf("%w: %s", constants.ErrMCPValidateCloudMetadataUnsupportedProvider, provider)
	}
}

func getAWSInstanceMetadata(ctx context.Context) (CloudMetadataInstanceResult, error) {
	instanceID, err := httpGetFunc(ctx, "http://169.254.169.254/latest/meta-data/instance-id", nil)
	if err != nil {
		instanceID = "unknown"
	}

	return CloudMetadataInstanceResult{
		Provider:   "aws",
		InstanceID: instanceID,
	}, nil
}

func getAzureInstanceMetadata(ctx context.Context) (CloudMetadataInstanceResult, error) {
	headers := map[string]string{"Metadata": "true"}
	data, err := httpGetFunc(ctx, "http://169.254.169.254/metadata/instance?api-version=2021-02-01", headers)
	if err != nil {
		return CloudMetadataInstanceResult{}, err
	}

	var azureMeta AzureInstanceMetadata
	if err := json.Unmarshal([]byte(data), &azureMeta); err != nil {
		return CloudMetadataInstanceResult{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	return CloudMetadataInstanceResult{
		Provider: "azure",
		VMSize:   azureMeta.Compute.VMSize,
		Location: azureMeta.Compute.Location,
	}, nil
}

func getGCPInstanceMetadata(ctx context.Context) (CloudMetadataInstanceResult, error) {
	headers := map[string]string{"Metadata-Flavor": "Google"}
	instanceID, err := httpGetFunc(ctx, "http://metadata.google.internal/computeMetadata/v1/id", headers)
	if err != nil {
		instanceID = "unknown"
	}

	name, err := httpGetFunc(ctx, "http://metadata.google.internal/computeMetadata/v1/instance/name", headers)
	if err != nil {
		name = "unknown"
	}

	return CloudMetadataInstanceResult{
		Provider:   "gcp",
		InstanceID: instanceID,
		Name:       name,
	}, nil
}

func getRegion(ctx context.Context, provider string) (CloudMetadataRegionResult, error) {
	switch provider {
	case "aws":
		region, err := httpGetFunc(ctx, "http://169.254.169.254/latest/meta-data/placement/region", nil)
		if err != nil {
			az, err2 := httpGetFunc(ctx, "http://169.254.169.254/latest/meta-data/placement/availability-zone", nil)
			if err2 == nil && len(az) > 1 {
				region = az[:len(az)-1]
			} else {
				region = "unknown"
			}
		}
		return CloudMetadataRegionResult{
			Provider: "aws",
			Region:   region,
		}, nil
	case "azure":
		headers := map[string]string{"Metadata": "true"}
		data, err := httpGetFunc(ctx, "http://169.254.169.254/metadata/instance?api-version=2021-02-01", headers)
		if err != nil {
			return CloudMetadataRegionResult{}, err
		}
		var azureMeta AzureInstanceMetadata
		if err := json.Unmarshal([]byte(data), &azureMeta); err != nil {
			return CloudMetadataRegionResult{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
		}
		region := azureMeta.Compute.Location
		if region == "" {
			region = "unknown"
		}
		return CloudMetadataRegionResult{
			Provider: "azure",
			Region:   region,
		}, nil
	case "gcp":
		headers := map[string]string{"Metadata-Flavor": "Google"}
		region, err := httpGetFunc(ctx, "http://metadata.google.internal/computeMetadata/v1/instance/region", headers)
		if err != nil {
			region = "unknown"
		}
		parts := strings.Split(region, "/")
		if len(parts) > 0 {
			region = parts[len(parts)-1]
		}
		return CloudMetadataRegionResult{
			Provider: "gcp",
			Region:   region,
		}, nil
	default:
		return CloudMetadataRegionResult{}, fmt.Errorf("%w: %s", constants.ErrMCPValidateCloudMetadataUnsupportedProvider, provider)
	}
}

func getAvailabilityZone(ctx context.Context, provider string) (CloudMetadataAvailabilityZoneResult, error) {
	switch provider {
	case "aws":
		az, err := httpGetFunc(ctx, "http://169.254.169.254/latest/meta-data/placement/availability-zone", nil)
		if err != nil {
			az = "unknown"
		}
		return CloudMetadataAvailabilityZoneResult{
			Provider:         "aws",
			AvailabilityZone: az,
		}, nil
	case "azure":
		headers := map[string]string{"Metadata": "true"}
		data, err := httpGetFunc(ctx, "http://169.254.169.254/metadata/instance?api-version=2021-02-01", headers)
		if err != nil {
			return CloudMetadataAvailabilityZoneResult{}, err
		}
		var azureMeta AzureInstanceMetadata
		if err := json.Unmarshal([]byte(data), &azureMeta); err != nil {
			return CloudMetadataAvailabilityZoneResult{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
		}
		az := azureMeta.Compute.PlatformFaultDomain
		if az == "" {
			az = "unknown"
		}
		return CloudMetadataAvailabilityZoneResult{
			Provider:         "azure",
			AvailabilityZone: az,
		}, nil
	case "gcp":
		headers := map[string]string{"Metadata-Flavor": "Google"}
		zone, err := httpGetFunc(ctx, "http://metadata.google.internal/computeMetadata/v1/instance/zone", headers)
		if err != nil {
			zone = "unknown"
		}
		parts := strings.Split(zone, "/")
		if len(parts) > 0 {
			zone = parts[len(parts)-1]
		}
		return CloudMetadataAvailabilityZoneResult{
			Provider:         "gcp",
			AvailabilityZone: zone,
		}, nil
	default:
		return CloudMetadataAvailabilityZoneResult{}, fmt.Errorf("%w: %s", constants.ErrMCPValidateCloudMetadataUnsupportedProvider, provider)
	}
}

func getInstanceType(ctx context.Context, provider string) (CloudMetadataInstanceTypeResult, error) {
	switch provider {
	case "aws":
		instanceType, err := httpGetFunc(ctx, "http://169.254.169.254/latest/meta-data/instance-type", nil)
		if err != nil {
			instanceType = "unknown"
		}
		return CloudMetadataInstanceTypeResult{
			Provider:     "aws",
			InstanceType: instanceType,
		}, nil
	case "azure":
		headers := map[string]string{"Metadata": "true"}
		data, err := httpGetFunc(ctx, "http://169.254.169.254/metadata/instance?api-version=2021-02-01", headers)
		if err != nil {
			return CloudMetadataInstanceTypeResult{}, err
		}
		var azureMeta AzureInstanceMetadata
		if err := json.Unmarshal([]byte(data), &azureMeta); err != nil {
			return CloudMetadataInstanceTypeResult{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
		}
		vmSize := azureMeta.Compute.VMSize
		if vmSize == "" {
			vmSize = "unknown"
		}
		return CloudMetadataInstanceTypeResult{
			Provider:     "azure",
			InstanceType: vmSize,
		}, nil
	case "gcp":
		headers := map[string]string{"Metadata-Flavor": "Google"}
		machineType, err := httpGetFunc(ctx, "http://metadata.google.internal/computeMetadata/v1/instance/machine-type", headers)
		if err != nil {
			machineType = "unknown"
		}
		parts := strings.Split(machineType, "/")
		if len(parts) > 0 {
			machineType = parts[len(parts)-1]
		}
		return CloudMetadataInstanceTypeResult{
			Provider:     "gcp",
			InstanceType: machineType,
		}, nil
	default:
		return CloudMetadataInstanceTypeResult{}, fmt.Errorf("%w: %s", constants.ErrMCPValidateCloudMetadataUnsupportedProvider, provider)
	}
}

func getAllMetadata(ctx context.Context, provider string) (CloudMetadataAllResult, error) {
	instance, err := getInstanceMetadata(ctx, provider)
	if err != nil {
		instance = CloudMetadataInstanceResult{Error: err.Error()}
	}

	region, err := getRegion(ctx, provider)
	if err != nil {
		region = CloudMetadataRegionResult{Error: err.Error()}
	}

	az, err := getAvailabilityZone(ctx, provider)
	if err != nil {
		az = CloudMetadataAvailabilityZoneResult{Error: err.Error()}
	}

	instanceType, err := getInstanceType(ctx, provider)
	if err != nil {
		instanceType = CloudMetadataInstanceTypeResult{Error: err.Error()}
	}

	return CloudMetadataAllResult{
		Provider:         provider,
		Instance:         instance,
		Region:           region,
		AvailabilityZone: az,
		InstanceType:     instanceType,
	}, nil
}
