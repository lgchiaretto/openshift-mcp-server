package openshift

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

var (
	machineSetGVR        = schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machinesets"}
	machineGVR           = schema.GroupVersionResource{Group: "machine.openshift.io", Version: "v1beta1", Resource: "machines"}
	machineConfigGVR     = schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigs"}
	machineConfigPoolGVR = schema.GroupVersionResource{Group: "machineconfiguration.openshift.io", Version: "v1", Resource: "machineconfigpools"}
)

const defaultMachineNS = "openshift-machine-api"

func initMachines() []api.ServerTool {
	machineNSSchema := map[string]*jsonschema.Schema{
		"namespace": {Type: "string", Description: "Namespace. Default: openshift-machine-api"},
	}
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_machinesets_list",
			Description: "List MachineSets. Shows desired/ready/available replicas",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: machineNSSchema},
			Annotations: api.ToolAnnotations{Title: "MachineSets: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: machineSetsList},
		{Tool: api.Tool{
			Name:        "openshift_machineset_get",
			Description: "Get full details of a specific MachineSet including the provider spec",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"name":      {Type: "string", Description: "MachineSet name"},
				"namespace": {Type: "string", Description: "Namespace. Default: openshift-machine-api"},
			}, Required: []string{"name"}},
			Annotations: api.ToolAnnotations{Title: "MachineSet: Get", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: machineSetGet},
		{Tool: api.Tool{
			Name:        "openshift_machines_list",
			Description: "List Machines. Shows phase, associated node, and MachineSet",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace":   {Type: "string", Description: "Namespace. Default: openshift-machine-api"},
				"machine_set": {Type: "string", Description: "Filter by MachineSet"},
				"phase":       {Type: "string", Description: "Filter by phase (Running, Provisioning, Provisioned, Deleting, Failed)"},
			}},
			Annotations: api.ToolAnnotations{Title: "Machines: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: machinesList},
		{Tool: api.Tool{
			Name:        "openshift_machineconfigs_list",
			Description: "List MachineConfigs. Shows applied role",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"role": {Type: "string", Description: "Filter by role (e.g. master, worker)"},
			}},
			Annotations: api.ToolAnnotations{Title: "MachineConfigs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: machineConfigsList},
		{Tool: api.Tool{
			Name:        "openshift_machineconfigpools_list",
			Description: "List MachineConfigPools. Shows machine count, ready/degraded/unavailable counts, and current config",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{Title: "MachineConfigPools: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: machineConfigPoolsList},
	}
}

func machineSetsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", defaultMachineNS)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, machineSetGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list machinesets: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		spec := nestedMap(item.Object, "spec")
		status := nestedMap(item.Object, "status")
		rows = append(rows, []string{
			item.GetName(),
			fmt.Sprintf("%v", spec["replicas"]),
			fmt.Sprintf("%v", status["readyReplicas"]),
			fmt.Sprintf("%v", status["availableReplicas"]),
			item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "DESIRED", "READY", "AVAILABLE", "CREATED"},
		rows, len(list.Items), "machinesets",
	), nil), nil
}

func machineSetGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	name := p.RequiredString("name")
	ns := p.OptionalString("namespace", defaultMachineNS)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	obj, err := dynamicGet(params, params, machineSetGVR, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get machineset %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(marshalJSON(stripManagedFields(obj.Object)), nil), nil
}

func machinesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", defaultMachineNS)
	machineSet := p.OptionalString("machine_set", "")
	phase := p.OptionalString("phase", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, machineGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list machines: %w", err)), nil
	}
	rows := make([][]string, 0)
	for _, item := range list.Items {
		status := nestedMap(item.Object, "status")
		labels := item.GetLabels()
		msLabel := labels["machine.openshift.io/cluster-api-machineset"]
		itemPhase := nestedString(status, "phase")

		if machineSet != "" && msLabel != machineSet {
			continue
		}
		if phase != "" && itemPhase != phase {
			continue
		}

		nodeRef := nestedMap(status, "nodeRef")
		rows = append(rows, []string{
			item.GetName(), itemPhase,
			nestedString(nodeRef, "name"),
			msLabel,
			item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "PHASE", "NODE", "MACHINESET", "CREATED"},
		rows, len(rows), "machines",
	), nil), nil
}

func machineConfigsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	role := p.OptionalString("role", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, machineConfigGVR, "")
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list machineconfigs: %w", err)), nil
	}
	rows := make([][]string, 0)
	for _, item := range list.Items {
		labels := item.GetLabels()
		itemRole := labels["machineconfiguration.openshift.io/role"]
		if role != "" && itemRole != role {
			continue
		}
		rows = append(rows, []string{
			item.GetName(), itemRole,
			item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "ROLE", "CREATED"},
		rows, len(rows), "machineconfigs",
	), nil), nil
}

func machineConfigPoolsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	list, err := dynamicList(params, params, machineConfigPoolGVR, "")
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list machineconfigpools: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		status := nestedMap(item.Object, "status")
		config := nestedMap(status, "configuration")
		rows = append(rows, []string{
			item.GetName(),
			nestedString(config, "name"),
			fmt.Sprintf("%v", status["machineCount"]),
			fmt.Sprintf("%v", status["readyMachineCount"]),
			fmt.Sprintf("%v", status["updatedMachineCount"]),
			fmt.Sprintf("%v", status["degradedMachineCount"]),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "CONFIG", "MACHINES", "READY", "UPDATED", "DEGRADED"},
		rows, len(list.Items), "machineconfigpools",
	), nil), nil
}
