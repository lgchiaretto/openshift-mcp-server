package core

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func initWorkloads() []api.ServerTool {
	nsSchema := map[string]*jsonschema.Schema{
		"namespace": {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
	}
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "deployments_list",
			Description: "List Deployments. Shows desired vs ready vs available replicas. Use to identify degraded deployments",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace":      {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
				"label_selector": {Type: "string", Description: "Label selector (e.g. app=my-app)"},
			}},
			Annotations: api.ToolAnnotations{Title: "Deployments: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: deploymentsList},
		{Tool: api.Tool{
			Name:        "deployment_get",
			Description: "Get full details of a specific Deployment including pod template spec, rollout strategy, conditions, and resource requirements",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace": {Type: "string", Description: "Deployment namespace"},
				"name":      {Type: "string", Description: "Deployment name"},
			}, Required: []string{"namespace", "name"}},
			Annotations: api.ToolAnnotations{Title: "Deployment: Get", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: deploymentGet},
		{Tool: api.Tool{
			Name:        "statefulsets_list",
			Description: "List StatefulSets. Shows replica counts",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "StatefulSets: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: statefulSetsList},
		{Tool: api.Tool{
			Name:        "replicasets_list",
			Description: "List ReplicaSets. Shows replica counts and owner Deployment",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "ReplicaSets: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: replicaSetsList},
		{Tool: api.Tool{
			Name:        "daemonsets_list",
			Description: "List DaemonSets. Shows desired/ready/available counts. A mismatch indicates scheduling issues",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "DaemonSets: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: daemonSetsList},
		{Tool: api.Tool{
			Name:        "jobs_list",
			Description: "List Jobs. Shows completions, active/succeeded/failed counts",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "Jobs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: jobsList},
		{Tool: api.Tool{
			Name:        "cronjobs_list",
			Description: "List CronJobs. Shows schedule, suspend status, last schedule time, and active jobs",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "CronJobs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: cronJobsList},
		{Tool: api.Tool{
			Name:        "hpas_list",
			Description: "List HorizontalPodAutoscalers. Shows target, min/max/current replicas",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "HPAs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: hpasList},
	}
}

func deploymentsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
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
	list, err := params.AppsV1().Deployments(ns).List(params, opts)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list deployments: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, d := range list.Items {
		desired := int32(0)
		if d.Spec.Replicas != nil {
			desired = *d.Spec.Replicas
		}
		ready := d.Status.ReadyReplicas
		available := d.Status.AvailableReplicas
		rows = append(rows, []string{
			d.Namespace, d.Name,
			fmt.Sprintf("%d/%d", ready, desired),
			fmt.Sprintf("%d", available),
			d.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "READY", "AVAILABLE", "CREATED"},
		rows, len(rows), "deployments",
	), nil), nil
}

func deploymentGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.RequiredString("namespace")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	gvk := &schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	ret, err := kubernetes.NewCore(params).ResourcesGet(params, gvk, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get deployment %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(output.MarshalYaml(ret)), nil
}

func statefulSetsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := params.AppsV1().StatefulSets(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list statefulsets: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, s := range list.Items {
		desired := int32(0)
		if s.Spec.Replicas != nil {
			desired = *s.Spec.Replicas
		}
		ready := s.Status.ReadyReplicas
		rows = append(rows, []string{
			s.Namespace, s.Name,
			fmt.Sprintf("%d/%d", ready, desired),
			s.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "READY", "CREATED"},
		rows, len(rows), "statefulsets",
	), nil), nil
}

func replicaSetsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := params.AppsV1().ReplicaSets(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list replicasets: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, rs := range list.Items {
		desired := int32(0)
		if rs.Spec.Replicas != nil {
			desired = *rs.Spec.Replicas
		}
		ready := rs.Status.ReadyReplicas
		owner := ""
		if len(rs.OwnerReferences) > 0 {
			owner = rs.OwnerReferences[0].Name
		}
		rows = append(rows, []string{
			rs.Namespace, rs.Name,
			fmt.Sprintf("%d/%d", ready, desired),
			owner,
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "READY", "OWNER"},
		rows, len(rows), "replicasets",
	), nil), nil
}

func daemonSetsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := params.AppsV1().DaemonSets(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list daemonsets: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, ds := range list.Items {
		rows = append(rows, []string{
			ds.Namespace, ds.Name,
			fmt.Sprintf("%d", ds.Status.DesiredNumberScheduled),
			fmt.Sprintf("%d", ds.Status.NumberReady),
			fmt.Sprintf("%d", ds.Status.NumberAvailable),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "DESIRED", "READY", "AVAILABLE"},
		rows, len(rows), "daemonsets",
	), nil), nil
}

func jobsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := params.BatchV1().Jobs(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list jobs: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, j := range list.Items {
		completions := fmt.Sprintf("%d", j.Status.Succeeded)
		if j.Spec.Completions != nil {
			completions = fmt.Sprintf("%d/%d", j.Status.Succeeded, *j.Spec.Completions)
		}
		active := int32(0)
		if j.Status.Active > 0 {
			active = j.Status.Active
		}
		failed := int32(0)
		if j.Status.Failed > 0 {
			failed = j.Status.Failed
		}
		rows = append(rows, []string{
			j.Namespace, j.Name,
			completions,
			fmt.Sprintf("%d", active),
			fmt.Sprintf("%d", failed),
			j.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "COMPLETIONS", "ACTIVE", "FAILED", "CREATED"},
		rows, len(rows), "jobs",
	), nil), nil
}

func cronJobsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := params.BatchV1().CronJobs(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list cronjobs: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, cj := range list.Items {
		suspend := false
		if cj.Spec.Suspend != nil {
			suspend = *cj.Spec.Suspend
		}
		lastSchedule := "<none>"
		if cj.Status.LastScheduleTime != nil {
			lastSchedule = cj.Status.LastScheduleTime.Format("2006-01-02T15:04:05Z")
		}
		rows = append(rows, []string{
			cj.Namespace, cj.Name,
			cj.Spec.Schedule,
			fmt.Sprintf("%v", suspend),
			fmt.Sprintf("%d", len(cj.Status.Active)),
			lastSchedule,
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "SCHEDULE", "SUSPEND", "ACTIVE", "LAST SCHEDULE"},
		rows, len(rows), "cronjobs",
	), nil), nil
}

func hpasList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := params.AutoscalingV2().HorizontalPodAutoscalers(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list hpas: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, hpa := range list.Items {
		target := fmt.Sprintf("%s/%s", hpa.Spec.ScaleTargetRef.Kind, hpa.Spec.ScaleTargetRef.Name)
		minReplicas := int32(1)
		if hpa.Spec.MinReplicas != nil {
			minReplicas = *hpa.Spec.MinReplicas
		}
		rows = append(rows, []string{
			hpa.Namespace, hpa.Name,
			target,
			fmt.Sprintf("%d", minReplicas),
			fmt.Sprintf("%d", hpa.Spec.MaxReplicas),
			fmt.Sprintf("%d", hpa.Status.CurrentReplicas),
		})
	}
	return api.NewToolCallResult(coreFormatTable(
		[]string{"NAMESPACE", "NAME", "TARGET", "MIN", "MAX", "CURRENT"},
		rows, len(rows), "hpas",
	), nil), nil
}
