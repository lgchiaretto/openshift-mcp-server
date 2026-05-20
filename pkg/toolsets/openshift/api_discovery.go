package openshift

import (
	"fmt"
	"sort"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

var crdGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

func initAPIDiscovery() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_api_resources_list",
			Description: "List all available API resources. Shows API group, kind, and verbs. Equivalent to 'oc api-resources'",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"search": {Type: "string", Description: "Case-insensitive substring to filter resource names or API groups"},
			}},
			Annotations: api.ToolAnnotations{Title: "API Resources: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: apiResourcesList},
		{Tool: api.Tool{
			Name:        "openshift_crds_list",
			Description: "List CustomResourceDefinitions. Shows group, kind, scope, and versions",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"search": {Type: "string", Description: "Filter CRD names (e.g. 'machine', 'kubevirt')"},
				"group":  {Type: "string", Description: "Filter by API group"},
			}},
			Annotations: api.ToolAnnotations{Title: "CRDs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: crdsList},
	}
}

func apiResourcesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	search := p.OptionalString("search", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	searchLower := strings.ToLower(search)

	_, apiResourceLists, err := params.DiscoveryClient().ServerGroupsAndResources()
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to discover api resources: %w", err)), nil
	}

	type apiRes struct {
		group, version, kind, name string
		namespaced                 bool
		verbs                      string
	}
	var resources []apiRes

	for _, resList := range apiResourceLists {
		gv, parseErr := schema.ParseGroupVersion(resList.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, r := range resList.APIResources {
			if strings.Contains(r.Name, "/") {
				continue
			}
			group := gv.Group
			if group == "" {
				group = "core"
			}
			entry := apiRes{
				group:      group,
				version:    gv.Version,
				kind:       r.Kind,
				name:       r.Name,
				namespaced: r.Namespaced,
				verbs:      strings.Join(r.Verbs, ","),
			}
			if searchLower != "" {
				combined := strings.ToLower(entry.name + " " + entry.group + " " + entry.kind)
				if !strings.Contains(combined, searchLower) {
					continue
				}
			}
			resources = append(resources, entry)
		}
	}

	sort.Slice(resources, func(i, j int) bool {
		if resources[i].group != resources[j].group {
			return resources[i].group < resources[j].group
		}
		return resources[i].name < resources[j].name
	})

	rows := make([][]string, 0, len(resources))
	for _, r := range resources {
		rows = append(rows, []string{r.name, r.group, r.kind, fmt.Sprintf("%v", r.namespaced), r.verbs})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "APIGROUP", "KIND", "NAMESPACED", "VERBS"},
		rows, len(rows), "api-resources",
	), nil), nil
}

func crdsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	search := p.OptionalString("search", "")
	group := p.OptionalString("group", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.DynamicClient().Resource(crdGVR).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list crds: %w", err)), nil
	}

	var rows [][]string
	for _, item := range list.Items {
		crdName := item.GetName()
		spec := nestedMap(item.Object, "spec")
		crdGroup := nestedString(spec, "group")
		names := nestedMap(spec, "names")

		if search != "" && !strings.Contains(strings.ToLower(crdName), strings.ToLower(search)) {
			continue
		}
		if group != "" && !strings.EqualFold(crdGroup, group) {
			continue
		}

		versions := nestedSlice(spec, "versions")
		var servedVersions []string
		for _, v := range versions {
			if vm, ok := v.(map[string]any); ok {
				if served, _ := vm["served"].(bool); served {
					if vName, _ := vm["name"].(string); vName != "" {
						servedVersions = append(servedVersions, vName)
					}
				}
			}
		}

		established := false
		status := nestedMap(item.Object, "status")
		for _, c := range nestedSlice(status, "conditions") {
			if cm, ok := c.(map[string]any); ok {
				if cm["type"] == "Established" && cm["status"] == "True" {
					established = true
					break
				}
			}
		}

		rows = append(rows, []string{
			crdName, crdGroup,
			nestedString(names, "kind"),
			nestedString(spec, "scope"),
			strings.Join(servedVersions, ","),
			fmt.Sprintf("%v", established),
		})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })

	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "GROUP", "KIND", "SCOPE", "VERSIONS", "ESTABLISHED"},
		rows, len(rows), "crds",
	), nil), nil
}
