package openshift

import (
	"fmt"
	"sort"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

var (
	buildConfigGVR  = schema.GroupVersionResource{Group: "build.openshift.io", Version: "v1", Resource: "buildconfigs"}
	buildGVR        = schema.GroupVersionResource{Group: "build.openshift.io", Version: "v1", Resource: "builds"}
	imageStreamGVR  = schema.GroupVersionResource{Group: "image.openshift.io", Version: "v1", Resource: "imagestreams"}
)

func initBuildsAndImages() []api.ServerTool {
	nsSchema := map[string]*jsonschema.Schema{
		"namespace": {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
	}
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_buildconfigs_list",
			Description: "List OpenShift BuildConfigs. Shows strategy, source, and output image",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "BuildConfigs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: buildConfigsList},
		{Tool: api.Tool{
			Name:        "openshift_builds_list",
			Description: "List OpenShift Builds. Shows phase, strategy, and duration",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "Builds: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: buildsList},
		{Tool: api.Tool{
			Name:        "openshift_build_logs",
			Description: "Get logs from a specific OpenShift Build. Use to diagnose build failures",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace":  {Type: "string", Description: "Build namespace"},
				"name":       {Type: "string", Description: "Build name (e.g. my-app-1)"},
				"tail_lines": {Type: "integer", Description: "Lines from end. Default: 200", Default: api.ToRawMessage(200), Minimum: ptr.To(float64(1))},
			}, Required: []string{"namespace", "name"}},
			Annotations: api.ToolAnnotations{Title: "Build: Logs", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: buildLogs},
		{Tool: api.Tool{
			Name:        "openshift_imagestreams_list",
			Description: "List OpenShift ImageStreams. Shows tag count and docker image repository",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "ImageStreams: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: imageStreamsList},
		{Tool: api.Tool{
			Name:        "openshift_imagestream_get",
			Description: "Get full details of a specific ImageStream including all tags, history, and image digests",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace": {Type: "string", Description: "ImageStream namespace"},
				"name":      {Type: "string", Description: "ImageStream name"},
			}, Required: []string{"namespace", "name"}},
			Annotations: api.ToolAnnotations{Title: "ImageStream: Get", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: imageStreamGet},
	}
}

func buildConfigsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, buildConfigGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list buildconfigs: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		spec := nestedMap(item.Object, "spec")
		status := nestedMap(item.Object, "status")
		strategy := nestedMap(spec, "strategy")
		source := nestedMap(spec, "source")
		outputTo := nestedMap(nestedMap(spec, "output"), "to")
		output := ""
		if nestedString(outputTo, "kind") != "" {
			output = nestedString(outputTo, "kind") + "/" + nestedString(outputTo, "name")
		}
		rows = append(rows, []string{
			item.GetNamespace(), item.GetName(),
			nestedString(strategy, "type"),
			nestedString(source, "type"),
			output,
			fmt.Sprintf("%v", status["lastVersion"]),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "TYPE", "SOURCE", "OUTPUT", "LATEST"},
		rows, len(list.Items), "buildconfigs",
	), nil), nil
}

func buildsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, buildGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list builds: %w", err)), nil
	}
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].GetCreationTimestamp().After(list.Items[j].GetCreationTimestamp().Time)
	})
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		status := nestedMap(item.Object, "status")
		strategy := nestedMap(nestedMap(item.Object, "spec"), "strategy")
		duration := formatDuration(status["duration"])
		rows = append(rows, []string{
			item.GetNamespace(), item.GetName(),
			nestedString(status, "phase"),
			nestedString(strategy, "type"),
			duration,
			item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "PHASE", "TYPE", "DURATION", "CREATED"},
		rows, len(list.Items), "builds",
	), nil), nil
}

func buildLogs(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.RequiredString("namespace")
	name := p.RequiredString("name")
	tailLines := p.OptionalInt64("tail_lines", 200)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	build, err := dynamicGet(params, params, buildGVR, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get build %s: %w", name, err)), nil
	}
	annotations := build.GetAnnotations()
	podName := annotations["openshift.io/build.pod-name"]
	if podName == "" {
		podName = name + "-build"
	}

	tail := tailLines
	logs, err := params.CoreV1().Pods(ns).GetLogs(podName, &corev1.PodLogOptions{TailLines: &tail}).Do(params).Raw()
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get build logs for %s (pod: %s): %w", name, podName, err)), nil
	}
	if len(logs) == 0 {
		return api.NewToolCallResult("(no log output)", nil), nil
	}
	return api.NewToolCallResult(string(logs), nil), nil
}

func imageStreamsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	list, err := dynamicList(params, params, imageStreamGVR, ns)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list imagestreams: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		status := nestedMap(item.Object, "status")
		tags := nestedSlice(status, "tags")
		rows = append(rows, []string{
			item.GetNamespace(), item.GetName(),
			nestedString(status, "dockerImageRepository"),
			fmt.Sprintf("%d", len(tags)),
			item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "IMAGE REPOSITORY", "TAGS", "CREATED"},
		rows, len(list.Items), "imagestreams",
	), nil), nil
}

func imageStreamGet(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.RequiredString("namespace")
	name := p.RequiredString("name")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}
	obj, err := dynamicGet(params, params, imageStreamGVR, ns, name)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to get imagestream %s: %w", name, err)), nil
	}
	return api.NewToolCallResult(marshalJSON(stripManagedFields(obj.Object)), nil), nil
}

func formatDuration(v any) string {
	switch d := v.(type) {
	case float64:
		secs := int64(d) / 1_000_000_000
		if secs >= 60 {
			return fmt.Sprintf("%dm%ds", secs/60, secs%60)
		}
		return fmt.Sprintf("%ds", secs)
	case int64:
		secs := d / 1_000_000_000
		if secs >= 60 {
			return fmt.Sprintf("%dm%ds", secs/60, secs%60)
		}
		return fmt.Sprintf("%ds", secs)
	default:
		return ""
	}
}
