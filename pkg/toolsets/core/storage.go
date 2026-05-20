package core

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func initStorage() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "pvs_list",
			Description: "List PersistentVolumes. Shows capacity, access modes, reclaim policy, status, and bound claim",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"storage_class": {Type: "string", Description: "Filter by StorageClass name"},
			}},
			Annotations: api.ToolAnnotations{Title: "PVs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: pvsList},
		{Tool: api.Tool{
			Name:        "pvcs_list",
			Description: "List PersistentVolumeClaims. Shows requested storage, phase, and bound volume. Pending PVCs indicate no matching PV",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace":     {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
				"storage_class": {Type: "string", Description: "Filter by StorageClass name"},
			}},
			Annotations: api.ToolAnnotations{Title: "PVCs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: pvcsList},
		{Tool: api.Tool{
			Name:        "storageclasses_list",
			Description: "List StorageClasses. Shows provisioner, reclaim policy, and whether it is the default class",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{Title: "StorageClasses: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: storageClassesList},
	}
}

func pvsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	storageClass := p.OptionalString("storage_class", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.CoreV1().PersistentVolumes().List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list pvs: %w", err)), nil
	}

	var rows [][]string
	for _, pv := range list.Items {
		if storageClass != "" && pv.Spec.StorageClassName != storageClass {
			continue
		}
		capacity := ""
		if pv.Spec.Capacity != nil {
			if q, ok := pv.Spec.Capacity["storage"]; ok {
				capacity = q.String()
			}
		}
		claim := ""
		if pv.Spec.ClaimRef != nil {
			claim = pv.Spec.ClaimRef.Namespace + "/" + pv.Spec.ClaimRef.Name
		}
		accessModes := make([]string, 0, len(pv.Spec.AccessModes))
		for _, am := range pv.Spec.AccessModes {
			accessModes = append(accessModes, string(am))
		}
		rows = append(rows, []string{
			pv.Name, capacity,
			strings.Join(accessModes, ","),
			string(pv.Spec.PersistentVolumeReclaimPolicy),
			string(pv.Status.Phase),
			claim,
			pv.Spec.StorageClassName,
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAME", "CAPACITY", "ACCESS MODES", "RECLAIM", "STATUS", "CLAIM", "STORAGECLASS"},
		rows, len(rows), "pvs",
	), nil), nil
}

func pvcsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	storageClass := p.OptionalString("storage_class", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.CoreV1().PersistentVolumeClaims(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list pvcs: %w", err)), nil
	}

	var rows [][]string
	for _, pvc := range list.Items {
		sc := ""
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}
		if storageClass != "" && sc != storageClass {
			continue
		}
		capacity := ""
		if pvc.Spec.Resources.Requests != nil {
			if q, ok := pvc.Spec.Resources.Requests["storage"]; ok {
				capacity = q.String()
			}
		}
		rows = append(rows, []string{
			pvc.Namespace, pvc.Name,
			string(pvc.Status.Phase),
			pvc.Spec.VolumeName,
			capacity, sc,
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "STATUS", "VOLUME", "CAPACITY", "STORAGECLASS"},
		rows, len(rows), "pvcs",
	), nil), nil
}

func storageClassesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	list, err := params.StorageV1().StorageClasses().List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list storageclasses: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, sc := range list.Items {
		annotations := sc.Annotations
		isDefault := annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true"
		name := sc.Name
		if isDefault {
			name += " (default)"
		}
		bindingMode := ""
		if sc.VolumeBindingMode != nil {
			bindingMode = string(*sc.VolumeBindingMode)
		}
		expand := false
		if sc.AllowVolumeExpansion != nil {
			expand = *sc.AllowVolumeExpansion
		}
		rows = append(rows, []string{
			name, sc.Provisioner,
			string(*sc.ReclaimPolicy),
			bindingMode,
			fmt.Sprintf("%v", expand),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAME", "PROVISIONER", "RECLAIM", "BINDING MODE", "EXPAND"},
		rows, len(rows), "storageclasses",
	), nil), nil
}
