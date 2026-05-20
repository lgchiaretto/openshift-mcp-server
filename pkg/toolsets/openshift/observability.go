package openshift

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"
)

const (
	thanosNS          = "openshift-monitoring"
	thanosSvc         = "thanos-querier"
	thanosPort        = 9091
	alertmanagerSvc   = "alertmanager-main"
	alertmanagerPort  = 9094
	maxOutputChars    = 100000
)

func initObservability() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_prometheus_query",
			Description: "Execute an instant PromQL query against the cluster's Thanos Querier. Returns current metric values. Use for point-in-time checks like API server health, pod counts, memory usage by namespace, or any Prometheus metric",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"query": {Type: "string", Description: "PromQL query string (e.g. 'up{job=\"apiserver\"}', 'sum by(namespace) (container_memory_usage_bytes)')"},
				"time":  {Type: "string", Description: "Evaluation timestamp in RFC3339 or Unix format. Defaults to current time"},
			}, Required: []string{"query"}},
			Annotations: api.ToolAnnotations{Title: "Prometheus: Query", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: prometheusQuery},
		{Tool: api.Tool{
			Name:        "openshift_prometheus_query_range",
			Description: "Execute a range PromQL query against the cluster's Thanos Querier. Returns metric values over a time range. Use for trends, historical analysis, and time-series data",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"query": {Type: "string", Description: "PromQL query string"},
				"start": {Type: "string", Description: "Start time. RFC3339, Unix timestamp, or relative (e.g. '-1h')"},
				"end":   {Type: "string", Description: "End time. RFC3339, Unix timestamp, 'now', or relative (e.g. '-5m')"},
				"step":  {Type: "string", Description: "Query resolution step (e.g. '15s', '1m', '5m'). Default: '1m'"},
			}, Required: []string{"query", "start", "end"}},
			Annotations: api.ToolAnnotations{Title: "Prometheus: Query Range", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: prometheusQueryRange},
		{Tool: api.Tool{
			Name:        "openshift_alertmanager_alerts",
			Description: "Query active and pending alerts from the cluster's Alertmanager. Use for monitoring cluster health, detecting firing alerts, checking severity levels",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"active":    {Type: "boolean", Description: "Filter for active (firing) alerts. Default: true"},
				"silenced":  {Type: "boolean", Description: "Include silenced alerts. Default: false"},
				"inhibited": {Type: "boolean", Description: "Include inhibited alerts. Default: false"},
				"filter":    {Type: "string", Description: "Alertmanager filter syntax (e.g. 'alertname=Watchdog', 'severity=critical')"},
			}},
			Annotations: api.ToolAnnotations{Title: "Alertmanager: Alerts", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: alertmanagerAlerts},
	}
}

func serviceProxyGet(params api.ToolHandlerParams, namespace, service string, port int, path string, queryParams url.Values) ([]byte, error) {
	restClient := params.CoreV1().RESTClient()
	req := restClient.Get().
		Namespace(namespace).
		Resource("services").
		Name(fmt.Sprintf("%s:%d", service, port)).
		SubResource("proxy").
		Suffix(path)
	for k, vals := range queryParams {
		for _, v := range vals {
			req.Param(k, v)
		}
	}
	result := req.Do(params)
	if err := result.Error(); err != nil {
		return nil, err
	}
	raw, err := result.Raw()
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func prometheusQuery(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	query := p.RequiredString("query")
	evalTime := p.OptionalString("time", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	qp := url.Values{"query": {query}}
	if evalTime != "" {
		qp.Set("time", evalTime)
	}

	raw, err := serviceProxyGet(params, thanosNS, thanosSvc, thanosPort, "/api/v1/query", qp)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("prometheus query failed: %w", err)), nil
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to parse prometheus response: %w", err)), nil
	}

	if status, _ := data["status"].(string); status != "success" {
		return api.NewToolCallResult(marshalJSON(map[string]any{
			"error": data["error"], "errorType": data["errorType"],
		}), nil), nil
	}

	return api.NewToolCallResult(formatInstantResult(data), nil), nil
}

func prometheusQueryRange(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	query := p.RequiredString("query")
	start := p.RequiredString("start")
	end := p.RequiredString("end")
	step := p.OptionalString("step", "1m")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	qp := url.Values{"query": {query}, "start": {start}, "end": {end}, "step": {step}}
	raw, err := serviceProxyGet(params, thanosNS, thanosSvc, thanosPort, "/api/v1/query_range", qp)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("prometheus range query failed: %w", err)), nil
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to parse prometheus response: %w", err)), nil
	}

	if status, _ := data["status"].(string); status != "success" {
		return api.NewToolCallResult(marshalJSON(map[string]any{
			"error": data["error"], "errorType": data["errorType"],
		}), nil), nil
	}

	return api.NewToolCallResult(formatRangeResult(data), nil), nil
}

func alertmanagerAlerts(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	active := p.OptionalBool("active", true)
	silenced := p.OptionalBool("silenced", false)
	inhibited := p.OptionalBool("inhibited", false)
	filter := p.OptionalString("filter", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	qp := url.Values{
		"active":    {fmt.Sprintf("%v", active)},
		"silenced":  {fmt.Sprintf("%v", silenced)},
		"inhibited": {fmt.Sprintf("%v", inhibited)},
	}
	if filter != "" {
		qp.Set("filter", filter)
	}

	raw, err := serviceProxyGet(params, thanosNS, alertmanagerSvc, alertmanagerPort, "/api/v2/alerts", qp)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("alertmanager query failed: %w", err)), nil
	}

	var alerts []map[string]any
	if err := json.Unmarshal(raw, &alerts); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to parse alertmanager response: %w", err)), nil
	}

	if len(alerts) == 0 {
		return api.NewToolCallResult("No alerts match the criteria.", nil), nil
	}

	type alertRow struct {
		name, severity, ns, state, summary string
	}
	parsed := make([]alertRow, 0, len(alerts))
	for _, alert := range alerts {
		labels, _ := alert["labels"].(map[string]any)
		annotations, _ := alert["annotations"].(map[string]any)
		status, _ := alert["status"].(map[string]any)
		summary, _ := annotations["summary"].(string)
		if summary == "" {
			summary, _ = annotations["description"].(string)
		}
		if len(summary) > 80 {
			summary = summary[:80]
		}
		name, _ := labels["alertname"].(string)
		sev, _ := labels["severity"].(string)
		ns, _ := labels["namespace"].(string)
		st, _ := status["state"].(string)
		parsed = append(parsed, alertRow{name, sev, ns, st, summary})
	}

	sort.Slice(parsed, func(i, j int) bool {
		si := severityOrder(parsed[i].severity)
		sj := severityOrder(parsed[j].severity)
		if si != sj {
			return si < sj
		}
		return parsed[i].name < parsed[j].name
	})

	rows := make([][]string, 0, len(parsed))
	for _, a := range parsed {
		rows = append(rows, []string{a.name, a.severity, a.ns, a.state, a.summary})
	}

	return api.NewToolCallResult(formatTable(
		[]string{"ALERTNAME", "SEVERITY", "NAMESPACE", "STATE", "SUMMARY"},
		rows, len(rows), "alerts",
	), nil), nil
}

func severityOrder(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func formatInstantResult(data map[string]any) string {
	resultData, _ := data["data"].(map[string]any)
	resultType, _ := resultData["resultType"].(string)
	results, _ := resultData["result"].([]any)

	if len(results) == 0 {
		return "No results returned."
	}

	if resultType == "vector" {
		rows := make([][]string, 0, len(results))
		for _, r := range results {
			rm, _ := r.(map[string]any)
			metric, _ := rm["metric"].(map[string]any)
			value, _ := rm["value"].([]any)
			labels := formatLabels(metric)
			val := "<none>"
			if len(value) > 1 {
				val = fmt.Sprintf("%v", value[1])
			}
			rows = append(rows, []string{labels, val})
		}
		return formatTable([]string{"METRIC", "VALUE"}, rows, len(rows), "results")
	}

	output := marshalJSON(resultData)
	if len(output) > maxOutputChars {
		return output[:maxOutputChars] + "\n\n[truncated]"
	}
	return output
}

func formatRangeResult(data map[string]any) string {
	resultData, _ := data["data"].(map[string]any)
	results, _ := resultData["result"].([]any)

	if len(results) == 0 {
		return "No results returned."
	}

	var sb strings.Builder
	for _, r := range results {
		rm, _ := r.(map[string]any)
		metric, _ := rm["metric"].(map[string]any)
		values, _ := rm["values"].([]any)
		sb.WriteString(fmt.Sprintf("--- %s ---\n", formatLabels(metric)))
		for _, v := range values {
			pair, _ := v.([]any)
			if len(pair) >= 2 {
				sb.WriteString(fmt.Sprintf("  %v: %v\n", pair[0], pair[1]))
			}
		}
	}
	sb.WriteString(fmt.Sprintf("\n(%d series)", len(results)))

	output := sb.String()
	if len(output) > maxOutputChars {
		return output[:maxOutputChars] + "\n\n[truncated]"
	}
	return output
}

func formatLabels(metric map[string]any) string {
	if len(metric) == 0 {
		return "(no labels)"
	}
	keys := make([]string, 0, len(metric))
	for k := range metric {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, fmt.Sprintf("%v", metric[k])))
	}
	return strings.Join(parts, ", ")
}

