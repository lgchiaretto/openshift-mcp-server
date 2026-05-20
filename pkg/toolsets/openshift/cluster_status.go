package openshift

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func initClusterStatus() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_clusteroperators_list",
			Description: "List all ClusterOperators with Available/Progressing/Degraded status. Primary tool for checking OpenShift cluster health",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{
				Title:           "ClusterOperators: List",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: clusterOperatorsList},
		{Tool: api.Tool{
			Name:        "openshift_clusteroperator_get",
			Description: "Get full details of a specific ClusterOperator including all conditions, versions, and relatedObjects",
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"name": {Type: "string", Description: "ClusterOperator name (e.g. etcd, kube-apiserver, ingress)"},
				},
				Required: []string{"name"},
			},
			Annotations: api.ToolAnnotations{
				Title:           "ClusterOperator: Get",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: clusterOperatorGet},
		{Tool: api.Tool{
			Name:        "openshift_clusterversion_get",
			Description: "Get ClusterVersion: current OCP version, channel, available updates, and upgrade history",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{
				Title:           "ClusterVersion: Get",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: clusterVersionGet},
		{Tool: api.Tool{
			Name:        "openshift_upgrade_status",
			Description: "Get concise upgrade status: cluster health summary showing degraded, progressing, and unavailable operators",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{
				Title:           "Upgrade: Status",
				ReadOnlyHint:    ptr.To(true),
				DestructiveHint: ptr.To(false),
				OpenWorldHint:   ptr.To(true),
			},
		}, Handler: upgradeStatus},
	}
}

var clusterOperatorGVR = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "clusteroperators"}

func clusterOperatorsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	list, err := params.DynamicClient().Resource(clusterOperatorGVR).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list clusteroperators: %w", err)), nil
	}
	return api.NewToolCallResult(formatClusterOperatorsTable(list), nil), nil
}

func clusterOperatorGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	obj, err := params.DynamicClient().Resource(clusterOperatorGVR).Get(params, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get clusteroperator %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(marshalJSON(obj.Object), nil), nil
}

var clusterVersionGVR = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "clusterversions"}

func clusterVersionGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	obj, err := params.DynamicClient().Resource(clusterVersionGVR).Get(params, "version", metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get clusterversion: %w", err)), nil
	}
	status := nestedMap(obj.Object, "status")
	spec := nestedMap(obj.Object, "spec")
	history := nestedSlice(status, "history")
	available := nestedSlice(status, "availableUpdates")
	conditions := nestedSlice(status, "conditions")

	summary := map[string]any{
		"currentVersion":   nestedString(nestedMap(status, "desired"), "version"),
		"channel":          nestedString(spec, "channel"),
		"clusterID":        nestedString(spec, "clusterID"),
		"conditions":       conditions,
		"history":          history,
		"availableUpdates": available,
	}
	return api.NewToolCallResult(marshalJSON(summary), nil), nil
}

func upgradeStatus(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	cv, err := params.DynamicClient().Resource(clusterVersionGVR).Get(params, "version", metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get clusterversion: %w", err)), nil
	}
	coList, err := params.DynamicClient().Resource(clusterOperatorGVR).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list clusteroperators: %w", err)), nil
	}

	var degraded, progressing, unavailable []string
	for _, co := range coList.Items {
		coName := co.GetName()
		condMap := conditionsToMap(nestedSlice(nestedMap(co.Object, "status"), "conditions"))
		if condMap["Degraded"] == "True" {
			degraded = append(degraded, coName)
		}
		if condMap["Progressing"] == "True" {
			progressing = append(progressing, coName)
		}
		if condMap["Available"] != "True" {
			unavailable = append(unavailable, coName)
		}
	}

	cvStatus := nestedMap(cv.Object, "status")
	summary := map[string]any{
		"currentVersion":       nestedString(nestedMap(cvStatus, "desired"), "version"),
		"channel":              nestedString(nestedMap(cv.Object, "spec"), "channel"),
		"totalOperators":       len(coList.Items),
		"degradedOperators":    degraded,
		"progressingOperators": progressing,
		"unavailableOperators": unavailable,
		"clusterHealthy":       len(degraded) == 0 && len(unavailable) == 0,
	}
	return api.NewToolCallResult(marshalJSON(summary), nil), nil
}

func formatClusterOperatorsTable(list *unstructured.UnstructuredList) string {
	rows := make([][]string, 0, len(list.Items))
	for _, co := range list.Items {
		condMap := conditionsToMap(nestedSlice(nestedMap(co.Object, "status"), "conditions"))
		version := ""
		for _, v := range nestedSlice(nestedMap(co.Object, "status"), "versions") {
			if vm, ok := v.(map[string]any); ok && vm["name"] == "operator" {
				version, _ = vm["version"].(string)
				break
			}
		}
		rows = append(rows, []string{
			co.GetName(), version,
			condMap["Available"], condMap["Progressing"], condMap["Degraded"], condMap["Upgradeable"],
		})
	}
	return formatTable(
		[]string{"NAME", "VERSION", "AVAILABLE", "PROGRESSING", "DEGRADED", "UPGRADEABLE"},
		rows, len(list.Items), "clusteroperators",
	)
}

func conditionsToMap(conditions []any) map[string]string {
	m := make(map[string]string)
	for _, c := range conditions {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _ := cm["type"].(string)
		s, _ := cm["status"].(string)
		m[t] = s
	}
	return m
}

func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("{\"error\": %q}", err.Error())
	}
	return string(b)
}

func nestedMap(obj map[string]any, key string) map[string]any {
	if v, ok := obj[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

func nestedSlice(obj map[string]any, key string) []any {
	if v, ok := obj[key].([]any); ok {
		return v
	}
	return nil
}

func nestedString(obj map[string]any, key string) string {
	v, _ := obj[key].(string)
	return v
}

func nestedInt64(obj map[string]any, key string) int64 {
	switch v := obj[key].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func formatTable(headers []string, rows [][]string, total int, label string) string {
	if len(rows) == 0 {
		return fmt.Sprintf("No %s found.", label)
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	var sb strings.Builder
	for i, h := range headers {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(pad(h, widths[i]))
	}
	sb.WriteByte('\n')
	for _, row := range rows {
		for i := range headers {
			if i > 0 {
				sb.WriteString("  ")
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			sb.WriteString(pad(cell, widths[i]))
		}
		sb.WriteByte('\n')
	}
	sb.WriteString(fmt.Sprintf("\n(%d %s)", total, label))
	return sb.String()
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func dynamicList(_ context.Context, params api.ToolHandlerParams, gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	if namespace != "" {
		return params.DynamicClient().Resource(gvr).Namespace(namespace).List(params, metav1.ListOptions{})
	}
	return params.DynamicClient().Resource(gvr).List(params, metav1.ListOptions{})
}

func dynamicGet(_ context.Context, params api.ToolHandlerParams, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if namespace != "" {
		return params.DynamicClient().Resource(gvr).Namespace(namespace).Get(params, name, metav1.GetOptions{})
	}
	return params.DynamicClient().Resource(gvr).Get(params, name, metav1.GetOptions{})
}

func stripManagedFields(obj map[string]any) map[string]any {
	if meta, ok := obj["metadata"].(map[string]any); ok {
		delete(meta, "managedFields")
	}
	return obj
}
