package openshift

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/google/jsonschema-go/jsonschema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func initCertificates() []api.ServerTool {
	return []api.ServerTool{
		{Tool: api.Tool{
			Name:        "openshift_csrs_list",
			Description: "List CertificateSigningRequests. Shows requester, signer, and status. Pending CSRs block node bootstrap",
			InputSchema: &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
				"pending_only": {Type: "boolean", Description: "If true, only show Pending CSRs"},
			}},
			Annotations: api.ToolAnnotations{Title: "CSRs: List", ReadOnlyHint: ptr.To(true), DestructiveHint: ptr.To(false), OpenWorldHint: ptr.To(true)},
		}, Handler: csrsList},
	}
}

func csrsList(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	pendingOnly := p.OptionalBool("pending_only", false)
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", err), nil
	}

	list, err := params.CertificatesV1().CertificateSigningRequests().List(params, metav1.ListOptions{})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list csrs: %w", err)), nil
	}

	var rows [][]string
	for _, csr := range list.Items {
		status := "Pending"
		for _, c := range csr.Status.Conditions {
			if c.Type == "Approved" {
				status = "Approved"
				break
			} else if c.Type == "Denied" {
				status = "Denied"
				break
			}
		}
		if pendingOnly && status != "Pending" {
			continue
		}
		rows = append(rows, []string{
			csr.Name,
			csr.Spec.SignerName,
			csr.Spec.Username,
			status,
			csr.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
		})
	}

	return api.NewToolCallResult(formatTable(
		[]string{"NAME", "SIGNER", "REQUESTOR", "STATUS", "CREATED"},
		rows, len(rows), "csrs",
	), nil), nil
}
