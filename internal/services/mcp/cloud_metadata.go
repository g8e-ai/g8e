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
)

// CloudMetadataTool provides cloud provider metadata detection and information for AWS, Azure, and GCP.
type CloudMetadataTool struct{}

// Name returns the tool identifier.
func (t *CloudMetadataTool) Name() string {
	return "cloud_metadata"
}

// Description returns a human-readable description.
func (t *CloudMetadataTool) Description() string {
	return "Detects cloud provider (AWS, Azure, GCP) and retrieves instance metadata including region, instance type, and availability zone."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *CloudMetadataTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Metadata operation to perform",
				"enum":        []string{"detect", "instance", "region", "availability_zone", "instance_type", "all"},
			},
		},
	}
}

// Execute implements the tool logic.
func (t *CloudMetadataTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.Operation == "" {
		req.Operation = "detect"
	}

	if err := validateCloudMetadataOperation(req.Operation); err != nil {
		result := map[string]interface{}{
			"operation": req.Operation,
			"error":     err.Error(),
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{{Type: "text", Text: string(resultJSON)}},
		}, nil
	}

	provider := detectCloudProvider()
	if provider == "unknown" {
		result := map[string]interface{}{
			"operation": req.Operation,
			"provider":  "unknown",
			"message":   "Not running on a detected cloud provider or metadata service unavailable",
		}
		resultJSON, _ := json.Marshal(result)
		return CallToolResult{
			Content: []TextContent{{Type: "text", Text: string(resultJSON)}},
		}, nil
	}

	var result map[string]interface{}
	var err error

	switch req.Operation {
	case "detect":
		result = map[string]interface{}{
			"provider": provider,
		}
	case "instance":
		result, err = getInstanceMetadata(provider)
	case "region":
		result, err = getRegion(provider)
	case "availability_zone":
		result, err = getAvailabilityZone(provider)
	case "instance_type":
		result, err = getInstanceType(provider)
	case "all":
		result, err = getAllMetadata(provider)
	default:
		return CallToolResult{}, fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	if err != nil {
		result = map[string]interface{}{
			"operation": req.Operation,
			"provider":  provider,
			"error":     err.Error(),
		}
	}

	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return CallToolResult{}, fmt.Errorf("failed to marshal result: %w", marshalErr)
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

func detectCloudProvider() string {
	if _, err := os.Stat("/sys/class/dmi/id/product_uuid"); err == nil {
		if data, err := os.ReadFile("/sys/class/dmi/id/product_uuid"); err == nil {
			if strings.Contains(string(data), "ec2") {
				return "aws"
			}
		}
	}

	if _, err := os.Stat("/sys/class/dmi/id/sys_vendor"); err == nil {
		if data, err := os.ReadFile("/sys/class/dmi/id/sys_vendor"); err == nil {
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

	if _, err := client.Get("http://169.254.169.254/latest/meta-data/"); err == nil {
		return "aws"
	}

	if _, err := client.Get("http://169.254.169.254/metadata/instance?api-version=2021-02-01"); err == nil {
		return "azure"
	}

	if _, err := client.Get("http://metadata.google.internal/computeMetadata/v1/"); err == nil {
		return "gcp"
	}

	return "unknown"
}

func httpGetWithTimeout(url string, headers map[string]string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func getInstanceMetadata(provider string) (map[string]interface{}, error) {
	switch provider {
	case "aws":
		return getAWSInstanceMetadata()
	case "azure":
		return getAzureInstanceMetadata()
	case "gcp":
		return getGCPInstanceMetadata()
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func getAWSInstanceMetadata() (map[string]interface{}, error) {
	instanceID, err := httpGetWithTimeout("http://169.254.169.254/latest/meta-data/instance-id", nil)
	if err != nil {
		instanceID = "unknown"
	}

	return map[string]interface{}{
		"provider":    "aws",
		"instance_id": instanceID,
	}, nil
}

func getAzureInstanceMetadata() (map[string]interface{}, error) {
	headers := map[string]string{"Metadata": "true"}
	data, err := httpGetWithTimeout("http://169.254.169.254/metadata/instance?api-version=2021-02-01", headers)
	if err != nil {
		return nil, err
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(data), &metadata); err != nil {
		return nil, err
	}

	metadata["provider"] = "azure"
	return metadata, nil
}

func getGCPInstanceMetadata() (map[string]interface{}, error) {
	headers := map[string]string{"Metadata-Flavor": "Google"}
	instanceID, err := httpGetWithTimeout("http://metadata.google.internal/computeMetadata/v1/id", headers)
	if err != nil {
		instanceID = "unknown"
	}

	name, err := httpGetWithTimeout("http://metadata.google.internal/computeMetadata/v1/instance/name", headers)
	if err != nil {
		name = "unknown"
	}

	return map[string]interface{}{
		"provider":    "gcp",
		"instance_id": instanceID,
		"name":        name,
	}, nil
}

func getRegion(provider string) (map[string]interface{}, error) {
	switch provider {
	case "aws":
		region, err := httpGetWithTimeout("http://169.254.169.254/latest/meta-data/placement/region", nil)
		if err != nil {
			az, err2 := httpGetWithTimeout("http://169.254.169.254/latest/meta-data/placement/availability-zone", nil)
			if err2 == nil && len(az) > 1 {
				region = az[:len(az)-1]
			} else {
				region = "unknown"
			}
		}
		return map[string]interface{}{
			"provider": "aws",
			"region":   region,
		}, nil
	case "azure":
		headers := map[string]string{"Metadata": "true"}
		data, err := httpGetWithTimeout("http://169.254.169.254/metadata/instance?api-version=2021-02-01", headers)
		if err != nil {
			return nil, err
		}
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(data), &metadata); err != nil {
			return nil, err
		}
		if location, ok := metadata["compute"].(map[string]interface{})["location"].(string); ok {
			return map[string]interface{}{
				"provider": "azure",
				"region":   location,
			}, nil
		}
		return map[string]interface{}{
			"provider": "azure",
			"region":   "unknown",
		}, nil
	case "gcp":
		headers := map[string]string{"Metadata-Flavor": "Google"}
		region, err := httpGetWithTimeout("http://metadata.google.internal/computeMetadata/v1/instance/region", headers)
		if err != nil {
			region = "unknown"
		}
		parts := strings.Split(region, "/")
		if len(parts) > 0 {
			region = parts[len(parts)-1]
		}
		return map[string]interface{}{
			"provider": "gcp",
			"region":   region,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func getAvailabilityZone(provider string) (map[string]interface{}, error) {
	switch provider {
	case "aws":
		az, err := httpGetWithTimeout("http://169.254.169.254/latest/meta-data/placement/availability-zone", nil)
		if err != nil {
			az = "unknown"
		}
		return map[string]interface{}{
			"provider":          "aws",
			"availability_zone": az,
		}, nil
	case "azure":
		headers := map[string]string{"Metadata": "true"}
		data, err := httpGetWithTimeout("http://169.254.169.254/metadata/instance?api-version=2021-02-01", headers)
		if err != nil {
			return nil, err
		}
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(data), &metadata); err != nil {
			return nil, err
		}
		if faultDomain, ok := metadata["compute"].(map[string]interface{})["platformFaultDomain"].(string); ok {
			return map[string]interface{}{
				"provider":          "azure",
				"availability_zone": faultDomain,
			}, nil
		}
		return map[string]interface{}{
			"provider":          "azure",
			"availability_zone": "unknown",
		}, nil
	case "gcp":
		headers := map[string]string{"Metadata-Flavor": "Google"}
		zone, err := httpGetWithTimeout("http://metadata.google.internal/computeMetadata/v1/instance/zone", headers)
		if err != nil {
			zone = "unknown"
		}
		parts := strings.Split(zone, "/")
		if len(parts) > 0 {
			zone = parts[len(parts)-1]
		}
		return map[string]interface{}{
			"provider":          "gcp",
			"availability_zone": zone,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func getInstanceType(provider string) (map[string]interface{}, error) {
	switch provider {
	case "aws":
		instanceType, err := httpGetWithTimeout("http://169.254.169.254/latest/meta-data/instance-type", nil)
		if err != nil {
			instanceType = "unknown"
		}
		return map[string]interface{}{
			"provider":      "aws",
			"instance_type": instanceType,
		}, nil
	case "azure":
		headers := map[string]string{"Metadata": "true"}
		data, err := httpGetWithTimeout("http://169.254.169.254/metadata/instance?api-version=2021-02-01", headers)
		if err != nil {
			return nil, err
		}
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(data), &metadata); err != nil {
			return nil, err
		}
		if vmSize, ok := metadata["compute"].(map[string]interface{})["vmSize"].(string); ok {
			return map[string]interface{}{
				"provider":      "azure",
				"instance_type": vmSize,
			}, nil
		}
		return map[string]interface{}{
			"provider":      "azure",
			"instance_type": "unknown",
		}, nil
	case "gcp":
		headers := map[string]string{"Metadata-Flavor": "Google"}
		machineType, err := httpGetWithTimeout("http://metadata.google.internal/computeMetadata/v1/instance/machine-type", headers)
		if err != nil {
			machineType = "unknown"
		}
		parts := strings.Split(machineType, "/")
		if len(parts) > 0 {
			machineType = parts[len(parts)-1]
		}
		return map[string]interface{}{
			"provider":      "gcp",
			"instance_type": machineType,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func getAllMetadata(provider string) (map[string]interface{}, error) {
	instance, err := getInstanceMetadata(provider)
	if err != nil {
		instance = map[string]interface{}{"error": err.Error()}
	}

	region, err := getRegion(provider)
	if err != nil {
		region = map[string]interface{}{"error": err.Error()}
	}

	az, err := getAvailabilityZone(provider)
	if err != nil {
		az = map[string]interface{}{"error": err.Error()}
	}

	instanceType, err := getInstanceType(provider)
	if err != nil {
		instanceType = map[string]interface{}{"error": err.Error()}
	}

	return map[string]interface{}{
		"provider":          provider,
		"instance":          instance,
		"region":            region,
		"availability_zone": az,
		"instance_type":     instanceType,
	}, nil
}
