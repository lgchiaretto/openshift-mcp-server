package openshift

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

var (
	csvGVR             = schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions"}
	subscriptionGVR    = schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "subscriptions"}
	installPlanGVR     = schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "installplans"}
	catalogSourceGVR   = schema.GroupVersionResource{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "catalogsources"}
	packageManifestGVR = schema.GroupVersionResource{Group: "packages.operators.coreos.com", Version: "v1", Resource: "packagemanifests"}
)

func initOLM() []api.ServerTool {
	nsSchema := map[string]*jsonschema.Schema{
		"namespace": {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
	}
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_csvs_list",
			Description: "List ClusterServiceVersions (installed operators). Shows phase (Succeeded/Failed/Installing)",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "CSVs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: csvsList},
		{Tool: api.Tool{
			Name:        "openshift_subscriptions_list",
			Description: "List OLM Subscriptions. Shows package, channel, source, and installed CSV",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "Subscriptions: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: subscriptionsList},
		{Tool: api.Tool{
			Name:        "openshift_installplans_list",
			Description: "List OLM InstallPlans. Shows approval status and phase",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "InstallPlans: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: installPlansList},
		{Tool: api.Tool{
			Name:        "openshift_catalogsources_list",
			Description: "List OLM CatalogSources. Shows source type, image, and connection state",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "CatalogSources: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: catalogSourcesList},
		{Tool: api.Tool{
			Name:        "openshift_packagemanifests_list",
			Description: "List available operators from OLM catalogs. Shows package name, catalog, and default channel",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace": {Type: "string", Description: "Namespace (default: openshift-marketplace)"},
				"search":    {Type: "string", Description: "Filter package names (e.g. 'logging')"},
			}},
			Annotations: api.ToolAnnotations{Title: "PackageManifests: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: packageManifestsList},
	}
}

func csvsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, csvGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list csvs: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		spec := nestedMap(item.Object, "spec")
		status := nestedMap(item.Object, "status")
		rows = append(rows, []string{
			item.GetNamespace(), item.GetName(),
			nestedString(spec, "displayName"),
			nestedString(spec, "version"),
			nestedString(status, "phase"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "DISPLAY NAME", "VERSION", "PHASE"},
		rows, len(list.Items), "csvs",
	), nil), nil
}

func subscriptionsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, subscriptionGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list subscriptions: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		spec := nestedMap(item.Object, "spec")
		status := nestedMap(item.Object, "status")
		rows = append(rows, []string{
			item.GetNamespace(), item.GetName(),
			nestedString(spec, "name"),
			nestedString(spec, "channel"),
			nestedString(spec, "source"),
			nestedString(status, "installedCSV"),
			nestedString(status, "state"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "PACKAGE", "CHANNEL", "SOURCE", "INSTALLED CSV", "STATE"},
		rows, len(list.Items), "subscriptions",
	), nil), nil
}

func installPlansList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, installPlanGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list installplans: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		spec := nestedMap(item.Object, "spec")
		status := nestedMap(item.Object, "status")
		rows = append(rows, []string{
			item.GetNamespace(), item.GetName(),
			nestedString(spec, "approval"),
			fmt.Sprintf("%v", spec["approved"]),
			nestedString(status, "phase"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "APPROVAL", "APPROVED", "PHASE"},
		rows, len(list.Items), "installplans",
	), nil), nil
}

func catalogSourcesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, catalogSourceGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list catalogsources: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		spec := nestedMap(item.Object, "spec")
		status := nestedMap(item.Object, "status")
		conn := nestedMap(status, "connectionState")
		rows = append(rows, []string{
			item.GetNamespace(), item.GetName(),
			nestedString(spec, "displayName"),
			nestedString(spec, "sourceType"),
			nestedString(conn, "lastObservedState"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "DISPLAY NAME", "TYPE", "CONNECTION"},
		rows, len(list.Items), "catalogsources",
	), nil), nil
}

func packageManifestsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "openshift-marketplace")
	search := p.OptionalString("search", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, packageManifestGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list packagemanifests: %w", err)), nil
	}
	rows := make([][]string, 0)
	for _, item := range list.Items {
		status := nestedMap(item.Object, "status")
		pkgName := nestedString(status, "packageName")
		if pkgName == "" {
			pkgName = item.GetName()
		}
		if search != "" && !strings.Contains(strings.ToLower(pkgName), strings.ToLower(search)) {
			continue
		}
		channels := nestedSlice(status, "channels")
		rows = append(rows, []string{
			pkgName,
			nestedString(status, "catalogSource"),
			nestedString(status, "defaultChannel"),
			fmt.Sprintf("%d", len(channels)),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "CATALOG", "DEFAULT CHANNEL", "CHANNELS"},
		rows, len(rows), "packagemanifests",
	), nil), nil
}
