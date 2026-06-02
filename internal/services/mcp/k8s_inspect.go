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
	"os/exec"
	"strings"
)

// K8sInspectTool provides Kubernetes cluster inspection and pod management operations.
type K8sInspectTool struct{}

// Name returns the tool identifier.
func (t *K8sInspectTool) Name() string {
	return "k8s_inspect"
}

// Description returns a human-readable description.
func (t *K8sInspectTool) Description() string {
	return "Provides Kubernetes cluster inspection including pods, nodes, services, and deployment status."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *K8sInspectTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Kubernetes operation to perform",
				"enum":        []string{"pods", "nodes", "services", "deployments", "namespace", "cluster_info", "pod_logs", "pod_describe"},
			},
			"namespace": map[string]interface{}{
				"type":        "string",
				"description": "Kubernetes namespace (defaults to current or default)",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Resource name for describe or logs operations",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Limit for list operations (default: 50)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *K8sInspectTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req struct {
		Operation string `json:"operation"`
		Namespace string `json:"namespace,omitempty"`
		Name      string `json:"name,omitempty"`
		Limit     int    `json:"limit,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("invalid arguments: %w", err)
	}

	if req.Operation == "" {
		req.Operation = "pods"
	}

	if !kubectlAvailable() {
		return CallToolResult{}, fmt.Errorf("kubectl not found in PATH")
	}

	if req.Namespace != "" {
		if err := validateK8sNamespace(req.Namespace); err != nil {
			result := map[string]interface{}{
				"operation": req.Operation,
				"namespace": req.Namespace,
				"error":     err.Error(),
			}
			resultJSON, _ := json.Marshal(result)
			return CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: string(resultJSON),
					},
				},
			}, nil
		}
	}

	namespace := req.Namespace
	if namespace == "" {
		namespace = getCurrentNamespace()
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	var result map[string]interface{}
	var err error

	switch req.Operation {
	case "pods":
		result, err = k8sListPods(namespace, limit)
	case "nodes":
		result, err = k8sListNodes(limit)
	case "services":
		result, err = k8sListServices(namespace, limit)
	case "deployments":
		result, err = k8sListDeployments(namespace, limit)
	case "namespace":
		result, err = k8sListNamespaces()
	case "cluster_info":
		result, err = k8sClusterInfo()
	case "pod_logs":
		if req.Name == "" {
			return CallToolResult{}, fmt.Errorf("name required for pod_logs operation")
		}
		if err := validateK8sResourceName(req.Name); err != nil {
			result := map[string]interface{}{
				"operation": req.Operation,
				"namespace": namespace,
				"name":      req.Name,
				"error":     err.Error(),
			}
			resultJSON, _ := json.Marshal(result)
			return CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: string(resultJSON),
					},
				},
			}, nil
		}
		result, err = k8sPodLogs(namespace, req.Name)
	case "pod_describe":
		if req.Name == "" {
			return CallToolResult{}, fmt.Errorf("name required for pod_describe operation")
		}
		if err := validateK8sResourceName(req.Name); err != nil {
			result := map[string]interface{}{
				"operation": req.Operation,
				"namespace": namespace,
				"name":      req.Name,
				"error":     err.Error(),
			}
			resultJSON, _ := json.Marshal(result)
			return CallToolResult{
				Content: []TextContent{
					{
						Type: "text",
						Text: string(resultJSON),
					},
				},
			}, nil
		}
		result, err = k8sPodDescribe(namespace, req.Name)
	default:
		return CallToolResult{}, fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	if err != nil {
		result = map[string]interface{}{
			"operation": req.Operation,
			"namespace": namespace,
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

func kubectlAvailable() bool {
	_, err := exec.LookPath("kubectl")
	return err == nil
}

func runKubectlCommand(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func getCurrentNamespace() string {
	if ns, err := runKubectlCommand("config", "view", "--minify", "-o", "jsonpath='{..context.namespace}'"); err == nil {
		ns = strings.Trim(ns, "'")
		if ns != "" {
			return ns
		}
	}
	return "default"
}

func k8sListPods(namespace string, limit int) (map[string]interface{}, error) {
	args := []string{"get", "pods", "-n", namespace, "-o", "json", "--limit", fmt.Sprintf("%d", limit)}
	output, err := runKubectlCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("kubectl get pods failed: %w", err)
	}

	var podList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &podList); err != nil {
		return nil, fmt.Errorf("failed to parse kubectl output: %w", err)
	}

	var pods []map[string]interface{}
	for _, pod := range podList.Items {
		pods = append(pods, map[string]interface{}{
			"name":      pod.Metadata.Name,
			"namespace": pod.Metadata.Namespace,
			"status":    pod.Status.Phase,
		})
	}

	return map[string]interface{}{
		"namespace": namespace,
		"pods":      pods,
		"count":     len(pods),
	}, nil
}

func k8sListNodes(limit int) (map[string]interface{}, error) {
	args := []string{"get", "nodes", "-o", "json", "--limit", fmt.Sprintf("%d", limit)}
	output, err := runKubectlCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("kubectl get nodes failed: %w", err)
	}

	var nodeList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Conditions []struct {
					Type   string `json:"type"`
					Status string `json:"status"`
				} `json:"conditions"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &nodeList); err != nil {
		return nil, fmt.Errorf("failed to parse kubectl output: %w", err)
	}

	var nodes []map[string]interface{}
	for _, node := range nodeList.Items {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				ready = true
				break
			}
		}
		nodes = append(nodes, map[string]interface{}{
			"name":  node.Metadata.Name,
			"ready": ready,
		})
	}

	return map[string]interface{}{
		"nodes": nodes,
		"count": len(nodes),
	}, nil
}

func k8sListServices(namespace string, limit int) (map[string]interface{}, error) {
	args := []string{"get", "services", "-n", namespace, "-o", "json", "--limit", fmt.Sprintf("%d", limit)}
	output, err := runKubectlCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("kubectl get services failed: %w", err)
	}

	var svcList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Type string `json:"type"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &svcList); err != nil {
		return nil, fmt.Errorf("failed to parse kubectl output: %w", err)
	}

	var services []map[string]interface{}
	for _, svc := range svcList.Items {
		services = append(services, map[string]interface{}{
			"name":      svc.Metadata.Name,
			"namespace": svc.Metadata.Namespace,
			"type":      svc.Spec.Type,
		})
	}

	return map[string]interface{}{
		"namespace": namespace,
		"services":  services,
		"count":     len(services),
	}, nil
}

func k8sListDeployments(namespace string, limit int) (map[string]interface{}, error) {
	args := []string{"get", "deployments", "-n", namespace, "-o", "json", "--limit", fmt.Sprintf("%d", limit)}
	output, err := runKubectlCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("kubectl get deployments failed: %w", err)
	}

	var deployList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Spec struct {
				Replicas *int `json:"replicas"`
			} `json:"spec"`
			Status struct {
				Replicas          int `json:"replicas"`
				AvailableReplicas int `json:"availableReplicas"`
				UpdatedReplicas   int `json:"updatedReplicas"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &deployList); err != nil {
		return nil, fmt.Errorf("failed to parse kubectl output: %w", err)
	}

	var deployments []map[string]interface{}
	for _, deploy := range deployList.Items {
		desiredReplicas := 0
		if deploy.Spec.Replicas != nil {
			desiredReplicas = *deploy.Spec.Replicas
		}
		deployments = append(deployments, map[string]interface{}{
			"name":               deploy.Metadata.Name,
			"namespace":          deploy.Metadata.Namespace,
			"desired_replicas":   desiredReplicas,
			"available_replicas": deploy.Status.AvailableReplicas,
			"updated_replicas":   deploy.Status.UpdatedReplicas,
			"ready":              deploy.Status.AvailableReplicas == desiredReplicas,
		})
	}

	return map[string]interface{}{
		"namespace":   namespace,
		"deployments": deployments,
		"count":       len(deployments),
	}, nil
}

func k8sListNamespaces() (map[string]interface{}, error) {
	output, err := runKubectlCommand("get", "namespaces", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("kubectl get namespaces failed: %w", err)
	}

	var nsList struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(output), &nsList); err != nil {
		return nil, fmt.Errorf("failed to parse kubectl output: %w", err)
	}

	var namespaces []map[string]interface{}
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, map[string]interface{}{
			"name":   ns.Metadata.Name,
			"status": ns.Status.Phase,
		})
	}

	return map[string]interface{}{
		"namespaces": namespaces,
		"count":      len(namespaces),
	}, nil
}

func k8sClusterInfo() (map[string]interface{}, error) {
	version, err := runKubectlCommand("version", "--short", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("kubectl version failed: %w", err)
	}

	var versionInfo struct {
		ServerVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if err := json.Unmarshal([]byte(version), &versionInfo); err != nil {
		return nil, fmt.Errorf("failed to parse version output: %w", err)
	}

	context, err := runKubectlCommand("config", "current-context")
	if err != nil {
		context = "unknown"
	}

	cluster, err := runKubectlCommand("config", "view", "--minify", "-o", "jsonpath='{.cluster}'")
	if err != nil {
		cluster = "unknown"
	}
	cluster = strings.Trim(cluster, "'")

	return map[string]interface{}{
		"version": versionInfo.ServerVersion.GitVersion,
		"context": context,
		"cluster": cluster,
	}, nil
}

func k8sPodLogs(namespace string, name string) (map[string]interface{}, error) {
	args := []string{"logs", "-n", namespace, name}
	output, err := runKubectlCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("kubectl logs failed: %w", err)
	}

	lines := strings.Split(output, "\n")
	if len(lines) > 100 {
		lines = lines[len(lines)-100:]
		output = strings.Join(lines, "\n")
	}

	return map[string]interface{}{
		"namespace": namespace,
		"pod":       name,
		"logs":      output,
		"truncated": len(lines) == 100,
	}, nil
}

func k8sPodDescribe(namespace string, name string) (map[string]interface{}, error) {
	args := []string{"describe", "pod", "-n", namespace, name}
	output, err := runKubectlCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("kubectl describe pod failed: %w", err)
	}

	return map[string]interface{}{
		"namespace": namespace,
		"pod":       name,
		"describe":  output,
	}, nil
}
