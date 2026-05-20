package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

var routeGVR = schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
var ingressControllerGVR = schema.GroupVersionResource{Group: "operator.openshift.io", Version: "v1", Resource: "ingresscontrollers"}

func initNetworking(o api.Openshift) []api.ServerTool {
	nsSchema := map[string]*jsonschema.Schema{
		"namespace": {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
	}
	tools := []api.ServerTool{
		{Tool: api.Tool{
			Name:        "services_list",
			Description: "List Kubernetes Services. Shows type, cluster IP, and port mappings",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace":      {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
				"label_selector": {Type: "string", Description: "Label selector to filter services"},
			}},
			Annotations: api.ToolAnnotations{Title: "Services: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: servicesList},
		{Tool: api.Tool{
			Name:        "endpoints_list",
			Description: "List Endpoints. Shows the IP addresses and ports backing each Service",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "Endpoints: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: endpointsList},
		{Tool: api.Tool{
			Name:        "networkpolicies_list",
			Description: "List NetworkPolicies. Shows pod selector and policy types",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "NetworkPolicies: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: networkPoliciesList},
		{Tool: api.Tool{
			Name:        "ingresses_list",
			Description: "List Kubernetes Ingresses. Shows hosts and ingress class",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "Ingresses: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: ingressesList},
	}

	if o.IsOpenShift(context.Background()) {
		tools = append(tools,
			api.ServerTool{Tool: api.Tool{
				Name:        "routes_list",
				Description: "List OpenShift Routes. Shows host, TLS termination, and backing service",
				InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
				Annotations: api.ToolAnnotations{Title: "Routes: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
			}, Handler: routesList},
			api.ServerTool{Tool: api.Tool{
				Name:        "ingresscontrollers_list",
				Description: "List OpenShift IngressControllers. Shows domain, replicas, and endpoint publishing strategy",
				InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
					"namespace": {Type: "string", Description: "Namespace. Default: openshift-ingress-operator"},
				}},
				Annotations: api.ToolAnnotations{Title: "IngressControllers: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
			}, Handler: ingressControllersList},
		)
	}
	return tools
}

func servicesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
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
	list, err := params.CoreV1().Services(ns).List(params, opts)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list services: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, s := range list.Items {
		ports := ""
		if s.Spec.Ports != nil {
			pp := make([]string, 0, len(s.Spec.Ports))
			for _, port := range s.Spec.Ports {
				pStr := fmt.Sprintf("%d/%s", port.Port, port.Protocol)
				if port.NodePort != 0 {
					pStr += fmt.Sprintf(":%d", port.NodePort)
				}
				pp = append(pp, pStr)
			}
			ports = strings.Join(pp, ",")
		}
		rows = append(rows, []string{
			s.Namespace, s.Name,
			string(s.Spec.Type),
			s.Spec.ClusterIP,
			ports,
			s.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "TYPE", "CLUSTER-IP", "PORTS", "CREATED"},
		rows, len(rows), "services",
	), nil), nil
}

func endpointsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.CoreV1().Endpoints(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list endpoints: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, ep := range list.Items {
		var addrs, ports []string
		for _, subset := range ep.Subsets {
			for _, addr := range subset.Addresses {
				addrs = append(addrs, addr.IP)
			}
			for _, port := range subset.Ports {
				ports = append(ports, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
			}
		}
		addrStr := "<none>"
		if len(addrs) > 0 {
			if len(addrs) > 5 {
				addrStr = strings.Join(addrs[:5], ",") + fmt.Sprintf("...+%d", len(addrs)-5)
			} else {
				addrStr = strings.Join(addrs, ",")
			}
		}
		portStr := "<none>"
		if len(ports) > 0 {
			portStr = strings.Join(unique(ports), ",")
		}
		rows = append(rows, []string{ep.Namespace, ep.Name, addrStr, portStr})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "ENDPOINTS", "PORTS"},
		rows, len(rows), "endpoints",
	), nil), nil
}

func networkPoliciesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.NetworkingV1().NetworkPolicies(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list networkpolicies: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, np := range list.Items {
		selector := "<all>"
		if np.Spec.PodSelector.MatchLabels != nil && len(np.Spec.PodSelector.MatchLabels) > 0 {
			parts := make([]string, 0)
			for k, v := range np.Spec.PodSelector.MatchLabels {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
			selector = strings.Join(parts, ",")
		}
		var policyTypes []string
		for _, pt := range np.Spec.PolicyTypes {
			policyTypes = append(policyTypes, string(pt))
		}
		rows = append(rows, []string{
			np.Namespace, np.Name, selector, strings.Join(policyTypes, ","),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "POD-SELECTOR", "POLICY-TYPES"},
		rows, len(rows), "networkpolicies",
	), nil), nil
}

func ingressesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.NetworkingV1().Ingresses(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list ingresses: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, ing := range list.Items {
		var hosts []string
		if ing.Spec.Rules != nil {
			for _, r := range ing.Spec.Rules {
				if r.Host != "" {
					hosts = append(hosts, r.Host)
				} else {
					hosts = append(hosts, "*")
				}
			}
		}
		hostStr := "<none>"
		if len(hosts) > 0 {
			hostStr = strings.Join(hosts, ",")
		}
		class := ""
		if ing.Spec.IngressClassName != nil {
			class = *ing.Spec.IngressClassName
		}
		rows = append(rows, []string{
			ing.Namespace, ing.Name, class, hostStr,
			ing.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "CLASS", "HOSTS", "CREATED"},
		rows, len(rows), "ingresses",
	), nil), nil
}

func routesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	var list *unstructured.UnstructuredList
	var err error
	if ns != "" {
		list, err = params.DynamicClient().Resource(routeGVR).Namespace(ns).List(params, metav1.ListOptions{})
	} else {
		list, err = params.DynamicClient().Resource(routeGVR).List(params, metav1.ListOptions{})
	}
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list routes: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		spec, _ := item.Object["spec"].(map[string]any)
		if spec == nil {
			spec = map[string]any{}
		}
		to, _ := spec["to"].(map[string]any)
		if to == nil {
			to = map[string]any{}
		}
		tls, _ := spec["tls"].(map[string]any)
		tlsTerm := ""
		if tls != nil {
			tlsTerm, _ = tls["termination"].(string)
		}
		host, _ := spec["host"].(string)
		path, _ := spec["path"].(string)
		if path == "" {
			path = "/"
		}
		svcName, _ := to["name"].(string)

		rows = append(rows, []string{
			item.GetNamespace(), item.GetName(), host, path, svcName, tlsTerm,
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "HOST", "PATH", "SERVICE", "TLS"},
		rows, len(rows), "routes",
	), nil), nil
}

func ingressControllersList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "openshift-ingress-operator")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.DynamicClient().Resource(ingressControllerGVR).Namespace(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list ingresscontrollers: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		spec, _ := item.Object["spec"].(map[string]any)
		if spec == nil {
			spec = map[string]any{}
		}
		status, _ := item.Object["status"].(map[string]any)
		if status == nil {
			status = map[string]any{}
		}
		ep, _ := spec["endpointPublishingStrategy"].(map[string]any)
		if ep == nil {
			ep = map[string]any{}
		}
		domain, _ := status["domain"].(string)
		epType, _ := ep["type"].(string)
		rows = append(rows, []string{
			item.GetName(), domain,
			fmt.Sprintf("%v", spec["replicas"]),
			fmt.Sprintf("%v", status["availableReplicas"]),
			epType,
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAME", "DOMAIN", "REPLICAS", "AVAILABLE", "TYPE"},
		rows, len(rows), "ingresscontrollers",
	), nil), nil
}

func unique(input []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
