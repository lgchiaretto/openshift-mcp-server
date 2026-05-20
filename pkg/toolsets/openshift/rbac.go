package openshift

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

var (
	ocpUserGVR  = schema.GroupVersionResource{Group: "user.openshift.io", Version: "v1", Resource: "users"}
	ocpGroupGVR = schema.GroupVersionResource{Group: "user.openshift.io", Version: "v1", Resource: "groups"}
)

func initRBAC() []api.ServerTool {
	nsSchema := map[string]*jsonschema.Schema{
		"namespace": {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
	}
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_roles_list",
			Description: "List namespaced Roles. Shows the number of rules per role",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "Roles: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: rolesList},
		{Tool: api.Tool{
			Name:        "openshift_clusterroles_list",
			Description: "List ClusterRoles. Shows the number of rules per role",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"label_selector": {Type: "string", Description: "Label selector to filter ClusterRoles"},
			}},
			Annotations: api.ToolAnnotations{Title: "ClusterRoles: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: clusterRolesList},
		{Tool: api.Tool{
			Name:        "openshift_rolebindings_list",
			Description: "List RoleBindings. Shows which subjects are bound to which roles",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"namespace":    {Type: "string", Description: "Namespace to filter. Omit for all namespaces"},
				"subject_name": {Type: "string", Description: "Filter by subject name"},
			}},
			Annotations: api.ToolAnnotations{Title: "RoleBindings: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: roleBindingsList},
		{Tool: api.Tool{
			Name:        "openshift_clusterrolebindings_list",
			Description: "List ClusterRoleBindings. Shows subjects bound to ClusterRoles. Use to find who has cluster-admin",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"subject_name": {Type: "string", Description: "Filter by subject name"},
			}},
			Annotations: api.ToolAnnotations{Title: "ClusterRoleBindings: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: clusterRoleBindingsList},
		{Tool: api.Tool{
			Name:        "openshift_serviceaccounts_list",
			Description: "List ServiceAccounts. Shows secrets count",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: nsSchema},
			Annotations: api.ToolAnnotations{Title: "ServiceAccounts: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: serviceAccountsList},
		{Tool: api.Tool{
			Name:        "openshift_users_list",
			Description: "List OpenShift Users. Shows identities and groups",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{Title: "OCP Users: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: ocpUsersList},
		{Tool: api.Tool{
			Name:        "openshift_groups_list",
			Description: "List OpenShift Groups. Shows group members",
			InputSchema: &jsonschema.Schema{Type: "object"},
			Annotations: api.ToolAnnotations{Title: "OCP Groups: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: ocpGroupsList},
		{Tool: api.Tool{
			Name:        "openshift_rbac_for_subject",
			Description: "Find ALL RoleBindings and ClusterRoleBindings referencing a specific subject. Returns a consolidated permission view",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"subject_name": {Type: "string", Description: "User, Group, or ServiceAccount name"},
				"subject_kind": {Type: "string", Description: "Kind: User, Group, or ServiceAccount"},
				"namespace":    {Type: "string", Description: "Scope to a specific namespace. Omit for cluster-wide"},
			}, Required: []string{"subject_name", "subject_kind"}},
			Annotations: api.ToolAnnotations{Title: "RBAC: For Subject", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: rbacForSubject},
	}
}

func rolesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.RbacV1().Roles(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list roles: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, r := range list.Items {
		rows = append(rows, []string{
			r.Namespace, r.Name,
			fmt.Sprintf("%d", len(r.Rules)),
			r.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "RULES", "CREATED"},
		rows, len(rows), "roles",
	), nil), nil
}

func clusterRolesList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	labelSelector := p.OptionalString("label_selector", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	opts := metav1.ListOptions{}
	if labelSelector != "" {
		opts.LabelSelector = labelSelector
	}
	list, err := params.RbacV1().ClusterRoles().List(params, opts)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list clusterroles: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, r := range list.Items {
		rows = append(rows, []string{
			r.Name,
			fmt.Sprintf("%d", len(r.Rules)),
			fmt.Sprintf("%v", r.AggregationRule != nil),
			r.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "RULES", "AGGREGATION", "CREATED"},
		rows, len(rows), "clusterroles",
	), nil), nil
}

func roleBindingsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	subjectName := p.OptionalString("subject_name", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.RbacV1().RoleBindings(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list rolebindings: %w", err)), nil
	}

	var rows [][]string
	for _, b := range list.Items {
		if subjectName != "" && !hasSubject(b.Subjects, subjectName, "") {
			continue
		}
		subjects := formatSubjects(b.Subjects, 3)
		rows = append(rows, []string{
			b.Namespace, b.Name,
			fmt.Sprintf("%s/%s", b.RoleRef.Kind, b.RoleRef.Name),
			subjects,
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "ROLE", "SUBJECTS"},
		rows, len(rows), "rolebindings",
	), nil), nil
}

func clusterRoleBindingsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	subjectName := p.OptionalString("subject_name", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.RbacV1().ClusterRoleBindings().List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list clusterrolebindings: %w", err)), nil
	}

	var rows [][]string
	for _, b := range list.Items {
		if subjectName != "" && !hasSubject(b.Subjects, subjectName, "") {
			continue
		}
		subjects := formatSubjects(b.Subjects, 3)
		rows = append(rows, []string{
			b.Name,
			fmt.Sprintf("%s/%s", b.RoleRef.Kind, b.RoleRef.Name),
			subjects,
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "ROLE", "SUBJECTS"},
		rows, len(rows), "clusterrolebindings",
	), nil), nil
}

func serviceAccountsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.CoreV1().ServiceAccounts(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list serviceaccounts: %w", err)), nil
	}

	rows := make([][]string, 0, len(list.Items))
	for _, sa := range list.Items {
		rows = append(rows, []string{
			sa.Namespace, sa.Name,
			fmt.Sprintf("%d", len(sa.Secrets)),
			sa.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAMESPACE", "NAME", "SECRETS", "CREATED"},
		rows, len(rows), "serviceaccounts",
	), nil), nil
}

func ocpUsersList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	list, err := dynamicList(params, params, ocpUserGVR, "")
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list ocp users: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		identities := sliceToString(nestedSlice(item.Object, "identities"))
		groups := sliceToString(nestedSlice(item.Object, "groups"))
		rows = append(rows, []string{item.GetName(), identities, groups})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "IDENTITIES", "GROUPS"},
		rows, len(rows), "users",
	), nil), nil
}

func ocpGroupsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	list, err := dynamicList(params, params, ocpGroupGVR, "")
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list ocp groups: %w", err)), nil
	}
	rows := make([][]string, 0, len(list.Items))
	for _, item := range list.Items {
		users := sliceToString(nestedSlice(item.Object, "users"))
		rows = append(rows, []string{item.GetName(), users})
	}
	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "USERS"},
		rows, len(rows), "groups",
	), nil), nil
}

func rbacForSubject(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	subjectName := p.RequiredString("subject_name")
	subjectKind := p.RequiredString("subject_kind")
	ns := p.OptionalString("namespace", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	rbList, err := params.RbacV1().RoleBindings(ns).List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list rolebindings: %w", err)), nil
	}
	crbList, err := params.RbacV1().ClusterRoleBindings().List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list clusterrolebindings: %w", err)), nil
	}

	var matchingRBs []map[string]any
	for _, b := range rbList.Items {
		if hasSubject(b.Subjects, subjectName, subjectKind) {
			matchingRBs = append(matchingRBs, map[string]any{
				"type": "RoleBinding", "name": b.Name, "namespace": b.Namespace, "role": b.RoleRef.Name,
			})
		}
	}
	var matchingCRBs []map[string]any
	for _, b := range crbList.Items {
		if hasSubject(b.Subjects, subjectName, subjectKind) {
			matchingCRBs = append(matchingCRBs, map[string]any{
				"type": "ClusterRoleBinding", "name": b.Name, "role": b.RoleRef.Name,
			})
		}
	}

	summary := map[string]any{
		"subject":              map[string]any{"name": subjectName, "kind": subjectKind},
		"totalBindings":        len(matchingRBs) + len(matchingCRBs),
		"roleBindings":         matchingRBs,
		"clusterRoleBindings":  matchingCRBs,
	}
	return api.NewToolCallResult(marshalJSON(summary), nil), nil
}

func hasSubject(subjects []rbacv1.Subject, name, kind string) bool {
	for _, s := range subjects {
		if s.Name == name && (kind == "" || s.Kind == kind) {
			return true
		}
	}
	return false
}

func formatSubjects(subjects []rbacv1.Subject, max int) string {
	if len(subjects) == 0 {
		return ""
	}
	parts := make([]string, 0, len(subjects))
	for i, s := range subjects {
		if i >= max {
			parts = append(parts, fmt.Sprintf("...+%d", len(subjects)-max))
			break
		}
		parts = append(parts, fmt.Sprintf("%s/%s", s.Kind, s.Name))
	}
	return strings.Join(parts, ",")
}

func sliceToString(items []any) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			parts = append(parts, s)
		}
	}
	if len(parts) > 5 {
		return strings.Join(parts[:5], ",") + fmt.Sprintf("...+%d", len(parts)-5)
	}
	return strings.Join(parts, ",")
}
