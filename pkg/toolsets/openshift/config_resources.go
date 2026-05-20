package openshift

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

var (
	infrastructureGVR = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "infrastructures"}
	proxyGVR          = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "proxies"}
	oauthGVR          = schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "oauths"}
)

func initConfigResources() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_configmaps_list",
			Description: "List ConfigMaps. Returns names and data keys only -- values are NOT returned",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace":      {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
				"label_selector": {Type: "string", Description: "Label selector to filter configmaps"},
			}},
			Annotations: api.ToolAnnotations{Title: "ConfigMaps: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: configMapsList},
		{Tool: api.Tool{
			Name:        "openshift_secret_names_list",
			Description: "List Secret NAMES in a namespace. SECRET VALUES ARE NEVER RETURNED -- only names, types, and key names",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace":   {Type: "string", Description: "Namespace to list secrets in"},
				"secret_type": {Type: "string", Description: "Filter by secret type (e.g. kubernetes.io/tls, Opaque)"},
			}, Required: []string{"namespace"}},
			Annotations: api.ToolAnnotations{Title: "Secrets: List Names", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: secretNamesList},
		{Tool: api.Tool{
			Name:        "openshift_configmap_get",
			Description: "Get a specific ConfigMap including its data values",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace": {Type: "string", Description: "ConfigMap namespace"},
				"name":      {Type: "string", Description: "ConfigMap name"},
			}, Required: []string{"namespace", "name"}},
			Annotations: api.ToolAnnotations{Title: "ConfigMap: Get", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: configMapGet},
		{Tool: api.Tool{
			Name:        "openshift_infrastructure_get",
			Description: "Get the cluster Infrastructure resource. Shows platform type, API server URL, and topology",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{Title: "Infrastructure: Get", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: infrastructureGet},
		{Tool: api.Tool{
			Name:        "openshift_proxy_config_get",
			Description: "Get the cluster-wide Proxy configuration. Shows HTTP_PROXY, HTTPS_PROXY, NO_PROXY settings",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{Title: "Proxy: Config", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: proxyConfigGet},
		{Tool: api.Tool{
			Name:        "openshift_oauth_config_get",
			Description: "Get the cluster OAuth configuration. Shows identity providers and token settings",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{Title: "OAuth: Config", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: oauthConfigGet},
	}
}

func configMapsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	labelSelector := p.OptionalString("label_selector", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	opts := metav1.ListOptions{}
	if labelSelector != "" {
		opts.LabelSelector = labelSelector
	}

	var rows [][]string
	if ns != "" {
		list, err := params.CoreV1().ConfigMaps(ns).List(params, opts)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to list configmaps: %w", err)), nil
		}
		for _, cm := range list.Items {
			keys := mapKeys(cm.Data, 5)
			rows = append(rows, []string{cm.Namespace, cm.Name, keys, cm.CreationTimestamp.Format("2006-01-02T15:04:05Z")})
		}
	} else {
		list, err := params.CoreV1().ConfigMaps("").List(params, opts)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to list configmaps: %w", err)), nil
		}
		for _, cm := range list.Items {
			keys := mapKeys(cm.Data, 5)
			rows = append(rows, []string{cm.Namespace, cm.Name, keys, cm.CreationTimestamp.Format("2006-01-02T15:04:05Z")})
		}
	}

	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "DATA KEYS", "CREATED"},
		rows, len(rows), "configmaps",
	), nil), nil
}

func secretNamesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.RequiredString("namespace")
	secretType := p.OptionalString("secret_type", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.CoreV1().Secrets(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list secrets: %w", err)), nil
	}

	var rows [][]string
	for _, s := range list.Items {
		if secretType != "" && string(s.Type) != secretType {
			continue
		}
		keys := byteMapKeys(s.Data, 5)
		rows = append(rows, []string{s.Name, string(s.Type), keys, s.CreationTimestamp.Format("2006-01-02T15:04:05Z")})
	}

	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "TYPE", "DATA KEYS", "CREATED"},
		rows, len(rows), "secrets",
	), nil), nil
}

func configMapGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.RequiredString("namespace")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	cm, err := params.CoreV1().ConfigMaps(ns).Get(params, name, metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get configmap %s: %w", name, err)), nil
	}

	summary := map[string]any{
		"name":           cm.Name,
		"namespace":      cm.Namespace,
		"data":           cm.Data,
		"binaryDataKeys": byteMapKeysList(cm.BinaryData),
		"labels":         cm.Labels,
		"createdAt":      cm.CreationTimestamp,
	}
	return api.NewToolCallResult(marshalJSON(summary), nil), nil
}

func infrastructureGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	obj, err := params.DynamicClient().Resource(infrastructureGVR).Get(params, "cluster", metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get infrastructure: %w", err)), nil
	}
	status := nestedMap(obj.Object, "status")
	platformStatus := nestedMap(status, "platformStatus")
	summary := map[string]any{
		"infrastructureName":     nestedString(status, "infrastructureName"),
		"platform":              nestedString(status, "platform"),
		"apiServerURL":          nestedString(status, "apiServerURL"),
		"controlPlaneTopology":  nestedString(status, "controlPlaneTopology"),
		"infrastructureTopology": nestedString(status, "infrastructureTopology"),
		"platformType":          nestedString(platformStatus, "type"),
	}
	return api.NewToolCallResult(marshalJSON(summary), nil), nil
}

func proxyConfigGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	obj, err := params.DynamicClient().Resource(proxyGVR).Get(params, "cluster", metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get proxy config: %w", err)), nil
	}
	spec := nestedMap(obj.Object, "spec")
	status := nestedMap(obj.Object, "status")
	httpProxy := nestedString(spec, "httpProxy")
	if httpProxy == "" {
		httpProxy = nestedString(status, "httpProxy")
	}
	httpsProxy := nestedString(spec, "httpsProxy")
	if httpsProxy == "" {
		httpsProxy = nestedString(status, "httpsProxy")
	}
	noProxy := nestedString(spec, "noProxy")
	if noProxy == "" {
		noProxy = nestedString(status, "noProxy")
	}
	summary := map[string]any{
		"httpProxy":  httpProxy,
		"httpsProxy": httpsProxy,
		"noProxy":    noProxy,
		"trustedCA":  nestedString(nestedMap(spec, "trustedCA"), "name"),
	}
	return api.NewToolCallResult(marshalJSON(summary), nil), nil
}

func oauthConfigGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	obj, err := params.DynamicClient().Resource(oauthGVR).Get(params, "cluster", metav1.GetOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get oauth config: %w", err)), nil
	}
	spec := nestedMap(obj.Object, "spec")
	idps := nestedSlice(spec, "identityProviders")
	providers := make([]map[string]any, 0, len(idps))
	for _, idp := range idps {
		if m, ok := idp.(map[string]any); ok {
			providers = append(providers, map[string]any{
				"name":          m["name"],
				"type":          m["type"],
				"mappingMethod": m["mappingMethod"],
			})
		}
	}
	return api.NewToolCallResult(marshalJSON(map[string]any{
		"identityProviders": providers,
		"tokenConfig":       spec["tokenConfig"],
	}), nil), nil
}

func mapKeys(data map[string]string, max int) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	if len(keys) > max {
		result := strings.Join(keys[:max], ",")
		return fmt.Sprintf("%s...+%d", result, len(keys)-max)
	}
	return strings.Join(keys, ",")
}

func byteMapKeys(data map[string][]byte, max int) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	if len(keys) > max {
		result := strings.Join(keys[:max], ",")
		return fmt.Sprintf("%s...+%d", result, len(keys)-max)
	}
	return strings.Join(keys, ",")
}

func byteMapKeysList(data map[string][]byte) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	return keys
}
