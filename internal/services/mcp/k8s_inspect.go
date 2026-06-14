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
	"strconv"
	"strings"
)

// kubectlRunner is an interface for running kubectl commands, allowing dependency injection for testing.
type kubectlRunner interface {
	lookPath() bool
	runCommand(ctx context.Context, args ...string) (string, error)
}

// realKubectlRunner is the production implementation that actually runs kubectl commands.
type realKubectlRunner struct{}

func (r *realKubectlRunner) lookPath() bool {
	_, err := exec.LookPath("kubectl")
	return err == nil
}

func (r *realKubectlRunner) runCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("kubectl command failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// K8sInspectTool provides Kubernetes cluster inspection and pod management operations.
type K8sInspectTool struct {
	runner kubectlRunner
}

// Name returns the tool identifier.
func (t *K8sInspectTool) Name() string {
	return "k8s_inspect"
}

// Description returns a human-readable description.
func (t *K8sInspectTool) Description() string {
	return "Provides Kubernetes cluster inspection including pods, nodes, services, and deployment status."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *K8sInspectTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"operation": {
				Type:        "string",
				Description: "Kubernetes operation to perform",
				Enum:        []string{"pods", "nodes", "services", "deployments", "namespace", "cluster_info", "pod_logs", "pod_describe"},
			},
			"namespace": {
				Type:        "string",
				Description: "Kubernetes namespace (defaults to current or default)",
			},
			"name": {
				Type:        "string",
				Description: "Resource name for describe or logs operations",
			},
			"limit": {
				Type:        "integer",
				Description: "Limit for list operations (default: 50)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *K8sInspectTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req K8sInspectRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("k8s_inspect: unmarshal arguments: %w", err)
	}

	if req.Operation == "" {
		req.Operation = "pods"
	}

	runner := t.runner
	if runner == nil {
		runner = &realKubectlRunner{}
	}

	if !runner.lookPath() {
		return CallToolResult{}, fmt.Errorf("k8s_inspect: kubectl not found in PATH")
	}

	if req.Namespace != "" {
		if err := validateK8sNamespace(req.Namespace); err != nil {
			result := K8sInspectResult{
				Operation: req.Operation,
				Namespace: req.Namespace,
				Error:     err.Error(),
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
		namespace = getCurrentNamespace(ctx)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	var result K8sInspectResult
	var err error

	switch req.Operation {
	case "pods":
		result, err = k8sListPods(ctx, namespace, limit, runner)
	case "nodes":
		result, err = k8sListNodes(ctx, limit, runner)
	case "services":
		result, err = k8sListServices(ctx, namespace, limit, runner)
	case "deployments":
		result, err = k8sListDeployments(ctx, namespace, limit, runner)
	case "namespace":
		result, err = k8sListNamespaces(ctx, runner)
	case "cluster_info":
		result, err = k8sClusterInfo(ctx, runner)
	case "pod_logs":
		if req.Name == "" {
			return CallToolResult{}, fmt.Errorf("k8s_inspect: name required for pod_logs operation")
		}
		if err := validateK8sResourceName(req.Name); err != nil {
			result := K8sInspectResult{
				Operation: req.Operation,
				Namespace: namespace,
				Error:     err.Error(),
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
		result, err = k8sPodLogs(ctx, namespace, req.Name, runner)
	case "pod_describe":
		if req.Name == "" {
			return CallToolResult{}, fmt.Errorf("k8s_inspect: name required for pod_describe operation")
		}
		if err := validateK8sResourceName(req.Name); err != nil {
			result := K8sInspectResult{
				Operation: req.Operation,
				Namespace: namespace,
				Error:     err.Error(),
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
		result, err = k8sPodDescribe(ctx, namespace, req.Name, runner)
	default:
		return CallToolResult{}, fmt.Errorf("k8s_inspect: unsupported operation: %s", req.Operation)
	}

	if err != nil {
		result = K8sInspectResult{
			Operation: req.Operation,
			Namespace: namespace,
			Error:     err.Error(),
		}
	}

	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return CallToolResult{}, fmt.Errorf("k8s_inspect: marshal result: %w", marshalErr)
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

func runKubectlCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("kubectl command failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func getCurrentNamespace(ctx context.Context) string {
	if ctx.Err() != nil {
		return "default"
	}
	ns, err := runKubectlCommand(ctx, "config", "view", "--minify", "-o", "jsonpath='{..context.namespace}'")
	if err == nil {
		ns = strings.Trim(ns, "'")
		if ns != "" {
			return ns
		}
	}
	return "default"
}

func k8sListPods(ctx context.Context, namespace string, limit int, runner kubectlRunner) (K8sInspectResult, error) {
	args := []string{"get", "pods", "-n", namespace, "-o", "json", "--limit", strconv.Itoa(limit)}
	var output string
	var err error
	if runner != nil {
		output, err = runner.runCommand(ctx, args...)
	} else {
		output, err = runKubectlCommand(ctx, args...)
	}
	if err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: get pods: %w", err)
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
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: parse pods output: %w", err)
	}

	var pods []K8sPodInfo
	for _, pod := range podList.Items {
		pods = append(pods, K8sPodInfo{
			Name:      pod.Metadata.Name,
			Namespace: pod.Metadata.Namespace,
			Status:    pod.Status.Phase,
		})
	}

	return K8sInspectResult{
		Operation: "pods",
		Namespace: namespace,
		Pods:      pods,
		Count:     len(pods),
	}, nil
}

func k8sListNodes(ctx context.Context, limit int, runner kubectlRunner) (K8sInspectResult, error) {
	args := []string{"get", "nodes", "-o", "json", "--limit", strconv.Itoa(limit)}
	var output string
	var err error
	if runner != nil {
		output, err = runner.runCommand(ctx, args...)
	} else {
		output, err = runKubectlCommand(ctx, args...)
	}
	if err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: get nodes: %w", err)
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
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: parse nodes output: %w", err)
	}

	var nodes []K8sNodeInfo
	for _, node := range nodeList.Items {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" {
				ready = true
				break
			}
		}
		nodes = append(nodes, K8sNodeInfo{
			Name:  node.Metadata.Name,
			Ready: ready,
		})
	}

	return K8sInspectResult{
		Operation: "nodes",
		Nodes:     nodes,
		Count:     len(nodes),
	}, nil
}

func k8sListServices(ctx context.Context, namespace string, limit int, runner kubectlRunner) (K8sInspectResult, error) {
	args := []string{"get", "services", "-n", namespace, "-o", "json", "--limit", strconv.Itoa(limit)}
	var output string
	var err error
	if runner != nil {
		output, err = runner.runCommand(ctx, args...)
	} else {
		output, err = runKubectlCommand(ctx, args...)
	}
	if err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: get services: %w", err)
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
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: parse services output: %w", err)
	}

	var services []K8sServiceInfo
	for _, svc := range svcList.Items {
		services = append(services, K8sServiceInfo{
			Name:      svc.Metadata.Name,
			Namespace: svc.Metadata.Namespace,
			Type:      svc.Spec.Type,
		})
	}

	return K8sInspectResult{
		Operation: "services",
		Namespace: namespace,
		Services:  services,
		Count:     len(services),
	}, nil
}

func k8sListDeployments(ctx context.Context, namespace string, limit int, runner kubectlRunner) (K8sInspectResult, error) {
	args := []string{"get", "deployments", "-n", namespace, "-o", "json", "--limit", strconv.Itoa(limit)}
	var output string
	var err error
	if runner != nil {
		output, err = runner.runCommand(ctx, args...)
	} else {
		output, err = runKubectlCommand(ctx, args...)
	}
	if err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: get deployments: %w", err)
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
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: parse deployments output: %w", err)
	}

	var deployments []K8sDeploymentInfo
	for _, deploy := range deployList.Items {
		desiredReplicas := 0
		if deploy.Spec.Replicas != nil {
			desiredReplicas = *deploy.Spec.Replicas
		}
		deployments = append(deployments, K8sDeploymentInfo{
			Name:              deploy.Metadata.Name,
			Namespace:         deploy.Metadata.Namespace,
			DesiredReplicas:   desiredReplicas,
			AvailableReplicas: deploy.Status.AvailableReplicas,
			UpdatedReplicas:   deploy.Status.UpdatedReplicas,
			Ready:             deploy.Status.AvailableReplicas == desiredReplicas,
		})
	}

	return K8sInspectResult{
		Operation:   "deployments",
		Namespace:   namespace,
		Deployments: deployments,
		Count:       len(deployments),
	}, nil
}

func k8sListNamespaces(ctx context.Context, runner kubectlRunner) (K8sInspectResult, error) {
	var output string
	var err error
	if runner != nil {
		output, err = runner.runCommand(ctx, "get", "namespaces", "-o", "json")
	} else {
		output, err = runKubectlCommand(ctx, "get", "namespaces", "-o", "json")
	}
	if err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: get namespaces: %w", err)
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
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: parse namespaces output: %w", err)
	}

	var namespaces []K8sNamespaceInfo
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, K8sNamespaceInfo{
			Name:   ns.Metadata.Name,
			Status: ns.Status.Phase,
		})
	}

	return K8sInspectResult{
		Operation:  "namespace",
		Namespaces: namespaces,
		Count:      len(namespaces),
	}, nil
}

func k8sClusterInfo(ctx context.Context, runner kubectlRunner) (K8sInspectResult, error) {
	var version string
	var err error
	if runner != nil {
		version, err = runner.runCommand(ctx, "version", "--short", "-o", "json")
	} else {
		version, err = runKubectlCommand(ctx, "version", "--short", "-o", "json")
	}
	if err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: get version: %w", err)
	}

	var versionInfo struct {
		ServerVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if err := json.Unmarshal([]byte(version), &versionInfo); err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: parse version output: %w", err)
	}

	contextName := "unknown"
	if ctx.Err() == nil {
		if runner != nil {
			contextName, err = runner.runCommand(ctx, "config", "current-context")
		} else {
			contextName, err = runKubectlCommand(ctx, "config", "current-context")
		}
		if err != nil {
			contextName = "unknown"
		}
	}

	clusterName := "unknown"
	if ctx.Err() == nil {
		if runner != nil {
			clusterName, err = runner.runCommand(ctx, "config", "view", "--minify", "-o", "jsonpath='{.cluster}'")
		} else {
			clusterName, err = runKubectlCommand(ctx, "config", "view", "--minify", "-o", "jsonpath='{.cluster}'")
		}
		if err != nil {
			clusterName = "unknown"
		}
		clusterName = strings.Trim(clusterName, "'")
	}

	return K8sInspectResult{
		Operation: "cluster_info",
		ClusterInfo: &K8sClusterInfo{
			Version: versionInfo.ServerVersion.GitVersion,
			Context: contextName,
			Cluster: clusterName,
		},
	}, nil
}

func k8sPodLogs(ctx context.Context, namespace string, name string, runner kubectlRunner) (K8sInspectResult, error) {
	args := []string{"logs", "-n", namespace, name}
	var output string
	var err error
	if runner != nil {
		output, err = runner.runCommand(ctx, args...)
	} else {
		output, err = runKubectlCommand(ctx, args...)
	}
	if err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: get pod logs: %w", err)
	}

	lines := strings.Split(output, "\n")
	truncated := false
	if len(lines) > 100 {
		lines = lines[len(lines)-100:]
		output = strings.Join(lines, "\n")
		truncated = true
	}

	return K8sInspectResult{
		Operation: "pod_logs",
		PodLogs: &K8sPodLogs{
			Namespace: namespace,
			Pod:       name,
			Logs:      output,
			Truncated: truncated,
		},
	}, nil
}

func k8sPodDescribe(ctx context.Context, namespace string, name string, runner kubectlRunner) (K8sInspectResult, error) {
	args := []string{"describe", "pod", "-n", namespace, name}
	var output string
	var err error
	if runner != nil {
		output, err = runner.runCommand(ctx, args...)
	} else {
		output, err = runKubectlCommand(ctx, args...)
	}
	if err != nil {
		return K8sInspectResult{}, fmt.Errorf("k8s_inspect: describe pod: %w", err)
	}

	return K8sInspectResult{
		Operation: "pod_describe",
		PodDescribe: &K8sPodDescribe{
			Namespace: namespace,
			Pod:       name,
			Describe:  output,
		},
	}, nil
}
