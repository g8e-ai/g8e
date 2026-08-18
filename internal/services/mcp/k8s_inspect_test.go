// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockKubectlRunner struct {
	lookPathFunc   func() bool
	runCommandFunc func(ctx context.Context, args ...string) (string, error)
}

func (m *mockKubectlRunner) lookPath() bool {
	if m.lookPathFunc != nil {
		return m.lookPathFunc()
	}
	return true
}

func (m *mockKubectlRunner) runCommand(ctx context.Context, args ...string) (string, error) {
	if m.runCommandFunc != nil {
		return m.runCommandFunc(ctx, args...)
	}
	return "", nil
}

func TestK8sInspectTool_Name(t *testing.T) {
	tool := &K8sInspectTool{}
	require.Equal(t, "k8s_inspect", tool.Name())
}

func TestK8sInspectTool_Description(t *testing.T) {
	tool := &K8sInspectTool{}
	require.NotEmpty(t, tool.Description())
}

func TestK8sInspectTool_InputSchema(t *testing.T) {
	tool := &K8sInspectTool{}
	schema := tool.InputSchema()
	require.NotNil(t, schema)
	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Properties, "operation")
	require.Contains(t, schema.Properties, "namespace")
	require.Contains(t, schema.Properties, "name")
	require.Contains(t, schema.Properties, "limit")
}

func TestK8sInspectTool_Execute_InvalidJSON(t *testing.T) {
	tool := &K8sInspectTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid}`))
	require.Error(t, err)
	require.Error(t, err)
}

func TestK8sInspectTool_Execute_KubectlNotFound(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return false
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pods"}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "kubectl not found in PATH")
}

func TestK8sInspectTool_Execute_InvalidNamespace(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pods", "namespace": "invalid@namespace"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.NotEmpty(t, res.Error)
	require.Contains(t, res.Error, "validate K8s namespace")
}

func TestK8sInspectTool_Execute_DefaultOperation(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			// Should default to pods operation
			require.Contains(t, args, "get")
			require.Contains(t, args, "pods")
			return `{"items":[]}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "pods", res.Operation)
}

func TestK8sInspectTool_Execute_Pods(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			require.Contains(t, args, "get")
			require.Contains(t, args, "pods")
			require.Contains(t, args, "-o")
			require.Contains(t, args, "json")
			return `{
				"items": [
					{
						"metadata": {"name": "pod-1", "namespace": "default"},
						"status": {"phase": "Running"}
					},
					{
						"metadata": {"name": "pod-2", "namespace": "default"},
						"status": {"phase": "Pending"}
					}
				]
			}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pods", "namespace": "default"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "pods", res.Operation)
	require.Equal(t, "default", res.Namespace)
	require.Len(t, res.Pods, 2)
	require.Equal(t, "pod-1", res.Pods[0].Name)
	require.Equal(t, "Running", res.Pods[0].Status)
	require.Equal(t, "pod-2", res.Pods[1].Name)
	require.Equal(t, "Pending", res.Pods[1].Status)
	require.Equal(t, 2, res.Count)
}

func TestK8sInspectTool_Execute_Nodes(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			require.Contains(t, args, "get")
			require.Contains(t, args, "nodes")
			return `{
				"items": [
					{
						"metadata": {"name": "node-1"},
						"status": {
							"conditions": [
								{"type": "Ready", "status": "True"}
							]
						}
					},
					{
						"metadata": {"name": "node-2"},
						"status": {
							"conditions": [
								{"type": "Ready", "status": "False"}
							]
						}
					}
				]
			}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "nodes"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "nodes", res.Operation)
	require.Len(t, res.Nodes, 2)
	require.Equal(t, "node-1", res.Nodes[0].Name)
	require.True(t, res.Nodes[0].Ready)
	require.Equal(t, "node-2", res.Nodes[1].Name)
	require.False(t, res.Nodes[1].Ready)
	require.Equal(t, 2, res.Count)
}

func TestK8sInspectTool_Execute_Services(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			require.Contains(t, args, "get")
			require.Contains(t, args, "services")
			return `{
				"items": [
					{
						"metadata": {"name": "svc-1", "namespace": "default"},
						"spec": {"type": "ClusterIP"}
					},
					{
						"metadata": {"name": "svc-2", "namespace": "default"},
						"spec": {"type": "LoadBalancer"}
					}
				]
			}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "services", "namespace": "default"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "services", res.Operation)
	require.Equal(t, "default", res.Namespace)
	require.Len(t, res.Services, 2)
	require.Equal(t, "svc-1", res.Services[0].Name)
	require.Equal(t, "ClusterIP", res.Services[0].Type)
	require.Equal(t, "svc-2", res.Services[1].Name)
	require.Equal(t, "LoadBalancer", res.Services[1].Type)
	require.Equal(t, 2, res.Count)
}

func TestK8sInspectTool_Execute_Deployments(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			require.Contains(t, args, "get")
			require.Contains(t, args, "deployments")
			return `{
				"items": [
					{
						"metadata": {"name": "deploy-1", "namespace": "default"},
						"spec": {"replicas": 3},
						"status": {
							"replicas": 3,
							"availableReplicas": 3,
							"updatedReplicas": 3
						}
					},
					{
						"metadata": {"name": "deploy-2", "namespace": "default"},
						"spec": {"replicas": 2},
						"status": {
							"replicas": 2,
							"availableReplicas": 1,
							"updatedReplicas": 2
						}
					}
				]
			}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "deployments", "namespace": "default"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "deployments", res.Operation)
	require.Equal(t, "default", res.Namespace)
	require.Len(t, res.Deployments, 2)
	require.Equal(t, "deploy-1", res.Deployments[0].Name)
	require.Equal(t, 3, res.Deployments[0].DesiredReplicas)
	require.Equal(t, 3, res.Deployments[0].AvailableReplicas)
	require.True(t, res.Deployments[0].Ready)
	require.Equal(t, "deploy-2", res.Deployments[1].Name)
	require.Equal(t, 2, res.Deployments[1].DesiredReplicas)
	require.Equal(t, 1, res.Deployments[1].AvailableReplicas)
	require.False(t, res.Deployments[1].Ready)
	require.Equal(t, 2, res.Count)
}

func TestK8sInspectTool_Execute_Namespaces(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			require.Contains(t, args, "get")
			require.Contains(t, args, "namespaces")
			return `{
				"items": [
					{
						"metadata": {"name": "default"},
						"status": {"phase": "Active"}
					},
					{
						"metadata": {"name": "kube-system"},
						"status": {"phase": "Active"}
					}
				]
			}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "namespace"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "namespace", res.Operation)
	require.Len(t, res.Namespaces, 2)
	require.Equal(t, "default", res.Namespaces[0].Name)
	require.Equal(t, "Active", res.Namespaces[0].Status)
	require.Equal(t, "kube-system", res.Namespaces[1].Name)
	require.Equal(t, "Active", res.Namespaces[1].Status)
	require.Equal(t, 2, res.Count)
}

func TestK8sInspectTool_Execute_ClusterInfo(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			if args[0] == "version" {
				return `{"serverVersion": {"gitVersion": "v1.28.0"}}`, nil
			}
			if len(args) > 1 && args[0] == "config" && args[1] == "current-context" {
				return "my-context", nil
			}
			if len(args) > 1 && args[0] == "config" && args[1] == "view" {
				return "my-cluster", nil
			}
			return "", nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "cluster_info"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "cluster_info", res.Operation)
	require.NotNil(t, res.ClusterInfo)
	require.Equal(t, "v1.28.0", res.ClusterInfo.Version)
	require.Equal(t, "my-context", res.ClusterInfo.Context)
	require.Equal(t, "my-cluster", res.ClusterInfo.Cluster)
}

func TestK8sInspectTool_Execute_PodLogs(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			require.Contains(t, args, "logs")
			require.Contains(t, args, "my-pod")
			return "line1\nline2\nline3", nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pod_logs", "namespace": "default", "name": "my-pod"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "pod_logs", res.Operation)
	require.NotNil(t, res.PodLogs)
	require.Equal(t, "default", res.PodLogs.Namespace)
	require.Equal(t, "my-pod", res.PodLogs.Pod)
	require.Equal(t, "line1\nline2\nline3", res.PodLogs.Logs)
	require.False(t, res.PodLogs.Truncated)
}

func TestK8sInspectTool_Execute_PodLogs_Truncated(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			// Generate 101 lines to trigger truncation
			logs := ""
			for i := 0; i < 101; i++ {
				logs += fmt.Sprintf("line%d\n", i)
			}
			return logs, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pod_logs", "namespace": "default", "name": "my-pod"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.NotNil(t, res.PodLogs)
	require.True(t, res.PodLogs.Truncated)
	// Should only have last 100 lines
	lines := len(res.PodLogs.Logs)
	require.LessOrEqual(t, lines, 700) // Approximate check for 100 lines
}

func TestK8sInspectTool_Execute_PodLogs_MissingName(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pod_logs", "namespace": "default"}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	require.Error(t, err)
}

func TestK8sInspectTool_Execute_PodLogs_InvalidName(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pod_logs", "namespace": "default", "name": "invalid@pod"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.NotEmpty(t, res.Error)
	require.Contains(t, res.Error, "validate K8s resource name")
}

func TestK8sInspectTool_Execute_PodDescribe(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			require.Contains(t, args, "describe")
			require.Contains(t, args, "pod")
			require.Contains(t, args, "my-pod")
			return "Name: my-pod\nNamespace: default\nStatus: Running", nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pod_describe", "namespace": "default", "name": "my-pod"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Equal(t, "pod_describe", res.Operation)
	require.NotNil(t, res.PodDescribe)
	require.Equal(t, "default", res.PodDescribe.Namespace)
	require.Equal(t, "my-pod", res.PodDescribe.Pod)
	require.Equal(t, "Name: my-pod\nNamespace: default\nStatus: Running", res.PodDescribe.Describe)
}

func TestK8sInspectTool_Execute_PodDescribe_MissingName(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pod_describe", "namespace": "default"}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	require.Error(t, err)
}

func TestK8sInspectTool_Execute_PodDescribe_InvalidName(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pod_describe", "namespace": "default", "name": "invalid@pod"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.NotEmpty(t, res.Error)
	require.Contains(t, res.Error, "validate K8s resource name")
}

func TestK8sInspectTool_Execute_UnsupportedOperation(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "invalid_operation"}`)
	_, err := tool.Execute(context.Background(), args)
	require.Error(t, err)
	require.Error(t, err)
}

func TestK8sInspectTool_Execute_KubectlCommandError(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			return "", fmt.Errorf("kubectl command failed: connection refused")
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pods"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err) // Tool handles errors by returning result with Error field

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.NotEmpty(t, res.Error)
	require.Contains(t, res.Error, "get pods")
}

func TestK8sInspectTool_Execute_DefaultLimit(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			// Check that limit defaults to 50
			require.Contains(t, args, "--limit")
			require.Contains(t, args, "50")
			return `{"items":[]}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pods"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
}

func TestK8sInspectTool_Execute_CustomLimit(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			// Check that custom limit is used
			require.Contains(t, args, "--limit")
			require.Contains(t, args, "10")
			return `{"items":[]}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "pods", "limit": 10}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
}

func TestK8sInspectTool_Execute_DeploymentWithNilReplicas(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			return `{
				"items": [
					{
						"metadata": {"name": "deploy-1", "namespace": "default"},
						"spec": {},
						"status": {
							"replicas": 0,
							"availableReplicas": 0,
							"updatedReplicas": 0
						}
					}
				]
			}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "deployments", "namespace": "default"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Len(t, res.Deployments, 1)
	require.Equal(t, 0, res.Deployments[0].DesiredReplicas)
	require.True(t, res.Deployments[0].Ready) // 0 == 0 is ready
}

func TestK8sInspectTool_Execute_NodeWithoutReadyCondition(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			return `{
				"items": [
					{
						"metadata": {"name": "node-1"},
						"status": {
							"conditions": [
								{"type": "MemoryPressure", "status": "False"}
							]
						}
					}
				]
			}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "nodes"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Len(t, res.Nodes, 1)
	require.Equal(t, "node-1", res.Nodes[0].Name)
	require.False(t, res.Nodes[0].Ready)
}

func TestK8sInspectTool_Execute_ClusterInfoWithContextError(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			if args[0] == "version" {
				return `{"serverVersion": {"gitVersion": "v1.28.0"}}`, nil
			}
			if args[0] == "config" && args[1] == "current-context" {
				return "", fmt.Errorf("config error")
			}
			return "", nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "cluster_info"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.NotNil(t, res.ClusterInfo)
	require.Equal(t, "v1.28.0", res.ClusterInfo.Version)
	require.Equal(t, "unknown", res.ClusterInfo.Context) // Falls back to unknown on error
}

func TestK8sInspectTool_Execute_DeploymentNilReplicasPointer(t *testing.T) {
	mock := &mockKubectlRunner{
		lookPathFunc: func() bool {
			return true
		},
		runCommandFunc: func(ctx context.Context, args ...string) (string, error) {
			return `{
				"items": [
					{
						"metadata": {"name": "deploy-1", "namespace": "default"},
						"spec": {"replicas": null},
						"status": {
							"replicas": 0,
							"availableReplicas": 0,
							"updatedReplicas": 0
						}
					}
				]
			}`, nil
		},
	}
	tool := &K8sInspectTool{runner: mock}

	args := json.RawMessage(`{"operation": "deployments", "namespace": "default"}`)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)

	var res K8sInspectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	require.Len(t, res.Deployments, 1)
	require.Equal(t, 0, res.Deployments[0].DesiredReplicas) // nil replicas should default to 0
}
