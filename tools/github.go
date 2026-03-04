package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v68/github"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// --- Helpers ---

func githubClient() (*github.Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}
	return github.NewClient(nil).WithAuthToken(token), nil
}

func parseOwnerRepo() (string, string, error) {
	repo := os.Getenv("GITHUB_REPO")
	if repo == "" {
		return "", "", fmt.Errorf("GITHUB_REPO environment variable is required (format: owner/repo)")
	}
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("GITHUB_REPO must be in owner/repo format, got: %s", repo)
	}
	return parts[0], parts[1], nil
}

// --- Arg types ---

type CreateRemediationIssueArgs struct {
	Title       string `json:"title" description:"Issue title summarizing the remediation"`
	RootCause   string `json:"root_cause" description:"Root cause analysis summary"`
	Severity    string `json:"severity" description:"Severity level: critical, high, medium, or low"`
	Actions     string `json:"actions" description:"JSON array of remediation actions, each with tool, args, and kubectl_equivalent fields"`
	Context     string `json:"context" description:"Investigation findings and context"`
	ClusterName string `json:"cluster_name" description:"Kubernetes cluster name or context"`
}

type CreateInspectionIssueArgs struct {
	Title             string `json:"title" description:"Issue title summarizing the finding"`
	Severity          string `json:"severity" description:"Severity level: critical, high, medium, or low"`
	Summary           string `json:"summary" description:"Brief description of the problem found"`
	Evidence          string `json:"evidence" description:"Detailed evidence: resource states, error messages, metrics, events"`
	Reasoning         string `json:"reasoning" description:"Analysis explaining why this is a problem and likely root cause"`
	References        string `json:"references" description:"Links to K8s docs, known issues, articles explaining the problem"`
	AffectedResources string `json:"affected_resources" description:"JSON array of affected resources, each with kind, namespace, name, and status fields"`
	ClusterName       string `json:"cluster_name" description:"Kubernetes cluster name or context"`
}

type ListRemediationIssuesArgs struct {
	State string `json:"state" description:"Issue state filter: open, closed, or all (default: open)"`
	Limit int    `json:"limit" description:"Maximum number of issues to return (default: 20)"`
}

type GetRemediationIssueArgs struct {
	Number int `json:"number" description:"GitHub issue number"`
}

type CloseRemediationIssueArgs struct {
	Number  int    `json:"number" description:"GitHub issue number"`
	Comment string `json:"comment" description:"Summary comment to add before closing"`
}

type CommentOnIssueArgs struct {
	Number  int    `json:"number" description:"GitHub issue number"`
	Comment string `json:"comment" description:"Comment body (markdown supported)"`
}

type GetIssueCommentsArgs struct {
	Number int `json:"number" description:"GitHub issue number"`
}

// --- Tools ---

func CreateRemediationIssue() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "create_remediation_issue",
		Description: "Create a GitHub issue with a structured Kubernetes remediation plan. The issue contains machine-parseable JSON actions and human-readable markdown.",
	}, func(ctx tool.Context, args CreateRemediationIssueArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		// Parse actions JSON to build the markdown table
		var actions []struct {
			Tool              string         `json:"tool"`
			Args              map[string]any `json:"args"`
			KubectlEquivalent string         `json:"kubectl_equivalent"`
		}
		if err := json.Unmarshal([]byte(args.Actions), &actions); err != nil {
			return Result{}, fmt.Errorf("invalid actions JSON: %w", err)
		}

		// Build action table rows
		var tableRows strings.Builder
		for i, a := range actions {
			ns, _ := a.Args["namespace"].(string)
			name, _ := a.Args["name"].(string)
			resource := name
			if ns != "" {
				resource = ns + "/" + name
			}
			tableRows.WriteString(fmt.Sprintf("| %d | %s | %s | %s | `%s` |\n",
				i+1, a.Tool, resource, ns, a.KubectlEquivalent))
		}

		body := fmt.Sprintf(`## Remediation Plan

**Root Cause:** %s
**Cluster:** %s
**Severity:** %s
**Detected:** %s

### Actions

| # | Action | Resource | Namespace | kubectl equivalent |
|---|--------|----------|-----------|--------------------|
%s
### Action Details
`+"```json\n%s\n```"+`

### Context
%s
`,
			args.RootCause,
			args.ClusterName,
			args.Severity,
			time.Now().UTC().Format(time.RFC3339),
			tableRows.String(),
			args.Actions,
			args.Context,
		)

		severityLabel := "severity/" + args.Severity

		issue, _, err := client.Issues.Create(context.Background(), owner, repo, &github.IssueRequest{
			Title:  github.Ptr(args.Title),
			Body:   github.Ptr(body),
			Labels: &[]string{"k8s-remediation", severityLabel},
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to create issue: %w", err)
		}

		return Result{
			Summary: fmt.Sprintf("Created remediation issue: %s", issue.GetHTMLURL()),
			Items: []Item{{
				Name:    args.Title,
				Status:  "Created",
				Details: issue.GetHTMLURL(),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func CreateInspectionIssue() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "create_inspection_issue",
		Description: "Create a GitHub issue with investigation findings for a Kubernetes cluster problem. Contains evidence, reasoning, and affected resources — no remediation plan.",
	}, func(ctx tool.Context, args CreateInspectionIssueArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		// Parse affected resources JSON to build the markdown table
		var resources []struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			Status    string `json:"status"`
		}
		var resourceTable strings.Builder
		if args.AffectedResources != "" {
			if err := json.Unmarshal([]byte(args.AffectedResources), &resources); err != nil {
				return Result{}, fmt.Errorf("invalid affected_resources JSON: %w", err)
			}
			resourceTable.WriteString("| Kind | Namespace | Name | Status |\n")
			resourceTable.WriteString("|------|-----------|------|--------|\n")
			for _, r := range resources {
				resourceTable.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", r.Kind, r.Namespace, r.Name, r.Status))
			}
		}

		body := fmt.Sprintf(`## Inspection Finding

**Severity:** %s
**Cluster:** %s
**Detected:** %s

## Summary
%s

## Affected Resources
%s
## Evidence
%s

## Reasoning
%s

## References
%s
`,
			args.Severity,
			args.ClusterName,
			time.Now().UTC().Format(time.RFC3339),
			args.Summary,
			resourceTable.String(),
			args.Evidence,
			args.Reasoning,
			args.References,
		)

		severityLabel := "severity/" + args.Severity

		issue, _, err := client.Issues.Create(context.Background(), owner, repo, &github.IssueRequest{
			Title:  github.Ptr(args.Title),
			Body:   github.Ptr(body),
			Labels: &[]string{"k8s-remediation", severityLabel},
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to create issue: %w", err)
		}

		return Result{
			Summary: fmt.Sprintf("Created inspection issue: %s", issue.GetHTMLURL()),
			Items: []Item{{
				Name:    args.Title,
				Status:  "Created",
				Details: issue.GetHTMLURL(),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func ListRemediationIssues() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_remediation_issues",
		Description: "List GitHub issues labeled k8s-remediation. Returns issue numbers, titles, and states.",
	}, func(ctx tool.Context, args ListRemediationIssuesArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		state := args.State
		if state == "" {
			state = "open"
		}
		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}

		issues, _, err := client.Issues.ListByRepo(context.Background(), owner, repo, &github.IssueListByRepoOptions{
			Labels:      []string{"k8s-remediation"},
			State:       state,
			ListOptions: github.ListOptions{PerPage: limit},
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list issues: %w", err)
		}

		items := make([]Item, 0, len(issues))
		for _, iss := range issues {
			labels := make([]string, 0, len(iss.Labels))
			for _, l := range iss.Labels {
				labels = append(labels, l.GetName())
			}
			items = append(items, Item{
				Name:    fmt.Sprintf("#%d: %s", iss.GetNumber(), iss.GetTitle()),
				Status:  iss.GetState(),
				Details: fmt.Sprintf("Labels: %s, Created: %s", strings.Join(labels, ", "), iss.GetCreatedAt().Format(time.RFC3339)),
			})
		}

		return Result{
			Summary: fmt.Sprintf("Found %d k8s-remediation issues (%s)", len(issues), state),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func GetRemediationIssue() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_remediation_issue",
		Description: "Fetch the full body of a GitHub remediation issue for parsing action details.",
	}, func(ctx tool.Context, args GetRemediationIssueArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		issue, _, err := client.Issues.Get(context.Background(), owner, repo, args.Number)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get issue #%d: %w", args.Number, err)
		}

		labels := make([]string, 0, len(issue.Labels))
		for _, l := range issue.Labels {
			labels = append(labels, l.GetName())
		}

		return Result{
			Summary: fmt.Sprintf("Issue #%d: %s (%s)", issue.GetNumber(), issue.GetTitle(), issue.GetState()),
			Items: []Item{{
				Name:    fmt.Sprintf("#%d: %s", issue.GetNumber(), issue.GetTitle()),
				Status:  issue.GetState(),
				Details: issue.GetBody(),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func CloseRemediationIssue() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "close_remediation_issue",
		Description: "Close a GitHub remediation issue with a summary comment describing the outcome.",
	}, func(ctx tool.Context, args CloseRemediationIssueArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		if args.Comment != "" {
			_, _, err = client.Issues.CreateComment(context.Background(), owner, repo, args.Number, &github.IssueComment{
				Body: github.Ptr(args.Comment),
			})
			if err != nil {
				return Result{}, fmt.Errorf("failed to comment on issue #%d: %w", args.Number, err)
			}
		}

		_, _, err = client.Issues.Edit(context.Background(), owner, repo, args.Number, &github.IssueRequest{
			State: github.Ptr("closed"),
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to close issue #%d: %w", args.Number, err)
		}

		return Result{
			Summary: fmt.Sprintf("Closed issue #%d with comment", args.Number),
			Items: []Item{{
				Name:   fmt.Sprintf("#%d", args.Number),
				Status: "Closed",
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func CommentOnIssue() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "comment_on_issue",
		Description: "Add a comment to a GitHub issue. Used by the Planner to ask clarifying questions or provide status updates.",
	}, func(ctx tool.Context, args CommentOnIssueArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		comment, _, err := client.Issues.CreateComment(context.Background(), owner, repo, args.Number, &github.IssueComment{
			Body: github.Ptr(args.Comment),
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to comment on issue #%d: %w", args.Number, err)
		}

		return Result{
			Summary: fmt.Sprintf("Added comment to issue #%d", args.Number),
			Items: []Item{{
				Name:    fmt.Sprintf("#%d comment", args.Number),
				Status:  "Created",
				Details: comment.GetHTMLURL(),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func GetIssueComments() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_issue_comments",
		Description: "Fetch all comments on a GitHub issue. Returns comment bodies with authors and timestamps.",
	}, func(ctx tool.Context, args GetIssueCommentsArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		comments, _, err := client.Issues.ListComments(context.Background(), owner, repo, args.Number, &github.IssueListCommentsOptions{
			ListOptions: github.ListOptions{PerPage: 50},
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list comments on issue #%d: %w", args.Number, err)
		}

		items := make([]Item, 0, len(comments))
		for _, c := range comments {
			items = append(items, Item{
				Name:    fmt.Sprintf("@%s at %s", c.GetUser().GetLogin(), c.GetCreatedAt().Format(time.RFC3339)),
				Status:  "comment",
				Details: c.GetBody(),
			})
		}

		return Result{
			Summary: fmt.Sprintf("Found %d comments on issue #%d", len(comments), args.Number),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}
