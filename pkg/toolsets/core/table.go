package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func coreFormatTable(headers []string, rows [][]string, total int, label string) string {
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
		sb.WriteString(corePad(h, widths[i]))
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
			sb.WriteString(corePad(cell, widths[i]))
		}
		sb.WriteByte('\n')
	}
	sb.WriteString(fmt.Sprintf("\n(%d %s)", total, label))
	return sb.String()
}

func corePad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func coreDynamicList(_ context.Context, params api.ToolHandlerParams, gvr schema.GroupVersionResource, namespace string) (*unstructured.UnstructuredList, error) {
	if namespace != "" {
		return params.DynamicClient().Resource(gvr).Namespace(namespace).List(params, metav1.ListOptions{})
	}
	return params.DynamicClient().Resource(gvr).List(params, metav1.ListOptions{})
}

func coreNestedMap(obj map[string]any, key string) map[string]any {
	if v, ok := obj[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

func coreNestedString(obj map[string]any, key string) string {
	v, _ := obj[key].(string)
	return v
}
