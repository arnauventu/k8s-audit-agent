package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v68/github"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// --- Arg types ---

type CreateRemediationPRArgs struct {
	IssueNumber int    `json:"issue_number" description:"GitHub issue number this PR remediates"`
	PlanContent string `json:"plan_content" description:"Markdown content for the remediation plan file"`
	Title       string `json:"title" description:"PR title summarizing the remediation plan"`
}

type ListRemediationPRsArgs struct {
	State string `json:"state" description:"PR state filter: open, closed, or all (default: open)"`
}

type GetRemediationPRArgs struct {
	Number int `json:"number" description:"GitHub PR number"`
}

type GetPRApprovalStatusArgs struct {
	Number int `json:"number" description:"GitHub PR number"`
}

type CommentOnPRArgs struct {
	Number  int    `json:"number" description:"GitHub PR number"`
	Comment string `json:"comment" description:"Comment body (markdown supported)"`
}

type MergeRemediationPRArgs struct {
	Number  int    `json:"number" description:"GitHub PR number"`
	Comment string `json:"comment" description:"Comment to add before merging"`
}

type ClosePRArgs struct {
	Number  int    `json:"number" description:"GitHub PR number"`
	Comment string `json:"comment" description:"Comment explaining why the PR is being closed"`
}

type GetPRPlanFileArgs struct {
	Number int `json:"number" description:"GitHub PR number"`
}

type CommitFileToBranchArgs struct {
	BranchName string `json:"branch_name" description:"Branch name to commit to (e.g. remediation/issue-42)"`
	FilePath   string `json:"file_path" description:"Repo-relative path to create or update (e.g. Dockerfile, k8s/deployment.yaml)"`
	Content    string `json:"content" description:"Full corrected file content to write"`
	Message    string `json:"message" description:"Commit message describing the fix"`
}

// --- Tools ---

func CreateRemediationPR() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "create_remediation_pr",
		Description: "Create a GitHub PR with a remediation plan file. Creates a branch, commits the plan file, and opens a PR linked to the issue.",
	}, func(ctx tool.Context, args CreateRemediationPRArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}
		bgCtx := context.Background()

		// Get default branch ref
		repoObj, _, err := client.Repositories.Get(bgCtx, owner, repo)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get repo: %w", err)
		}
		defaultBranch := repoObj.GetDefaultBranch()

		baseRef, _, err := client.Git.GetRef(bgCtx, owner, repo, "refs/heads/"+defaultBranch)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get ref for %s: %w", defaultBranch, err)
		}

		// Create branch
		branchName := fmt.Sprintf("remediation/issue-%d", args.IssueNumber)
		_, _, err = client.Git.CreateRef(bgCtx, owner, repo, &github.Reference{
			Ref:    github.Ptr("refs/heads/" + branchName),
			Object: baseRef.Object,
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to create branch %s: %w", branchName, err)
		}

		// Commit plan file
		filePath := fmt.Sprintf("remediation-plans/issue-%d.md", args.IssueNumber)
		_, _, err = client.Repositories.CreateFile(bgCtx, owner, repo, filePath, &github.RepositoryContentFileOptions{
			Message: github.Ptr(fmt.Sprintf("Add remediation plan for issue #%d", args.IssueNumber)),
			Content: []byte(args.PlanContent),
			Branch:  github.Ptr(branchName),
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to commit plan file: %w", err)
		}

		// Create PR with plan content in the body
		body := fmt.Sprintf("Fixes #%d\n\n---\n\n%s", args.IssueNumber, args.PlanContent)
		pr, _, err := client.PullRequests.Create(bgCtx, owner, repo, &github.NewPullRequest{
			Title: github.Ptr(args.Title),
			Body:  github.Ptr(body),
			Head:  github.Ptr(branchName),
			Base:  github.Ptr(defaultBranch),
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to create PR: %w", err)
		}

		// Add labels
		_, _, _ = client.Issues.AddLabelsToIssue(bgCtx, owner, repo, pr.GetNumber(), []string{"k8s-remediation", "remediation-plan"})

		return Result{
			Summary: fmt.Sprintf("Created remediation PR #%d: %s", pr.GetNumber(), pr.GetHTMLURL()),
			Items: []Item{{
				Name:    fmt.Sprintf("PR #%d: %s", pr.GetNumber(), args.Title),
				Status:  "Open",
				Details: fmt.Sprintf("Branch: %s, Plan: %s, URL: %s", branchName, filePath, pr.GetHTMLURL()),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func ListRemediationPRs() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_remediation_prs",
		Description: "List GitHub PRs labeled k8s-remediation. Returns PR numbers, titles, and approval status.",
	}, func(ctx tool.Context, args ListRemediationPRsArgs) (Result, error) {
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

		// GitHub API doesn't support label filtering on PRs directly,
		// so we search using the Issues endpoint (PRs are issues in GitHub).
		issues, _, err := client.Issues.ListByRepo(context.Background(), owner, repo, &github.IssueListByRepoOptions{
			Labels:      []string{"k8s-remediation"},
			State:       state,
			ListOptions: github.ListOptions{PerPage: 30},
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list PRs: %w", err)
		}

		items := make([]Item, 0)
		for _, iss := range issues {
			if !iss.IsPullRequest() {
				continue
			}
			labels := make([]string, 0, len(iss.Labels))
			for _, l := range iss.Labels {
				labels = append(labels, l.GetName())
			}
			items = append(items, Item{
				Name:    fmt.Sprintf("PR #%d: %s", iss.GetNumber(), iss.GetTitle()),
				Status:  iss.GetState(),
				Details: fmt.Sprintf("Labels: %s, Created: %s", strings.Join(labels, ", "), iss.GetCreatedAt().Format(time.RFC3339)),
			})
		}

		return Result{
			Summary: fmt.Sprintf("Found %d k8s-remediation PRs (%s)", len(items), state),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func GetRemediationPR() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_remediation_pr",
		Description: "Fetch full PR details including body, linked issue number, head branch, and labels.",
	}, func(ctx tool.Context, args GetRemediationPRArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		pr, _, err := client.PullRequests.Get(context.Background(), owner, repo, args.Number)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get PR #%d: %w", args.Number, err)
		}

		labels := make([]string, 0, len(pr.Labels))
		for _, l := range pr.Labels {
			labels = append(labels, l.GetName())
		}

		return Result{
			Summary: fmt.Sprintf("PR #%d: %s (%s)", pr.GetNumber(), pr.GetTitle(), pr.GetState()),
			Items: []Item{{
				Name:    fmt.Sprintf("PR #%d: %s", pr.GetNumber(), pr.GetTitle()),
				Status:  pr.GetState(),
				Details: fmt.Sprintf("Branch: %s\nLabels: %s\nBody:\n%s", pr.GetHead().GetRef(), strings.Join(labels, ", "), pr.GetBody()),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func GetPRApprovalStatus() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_pr_approval_status",
		Description: "Check if a PR has been approved via GitHub review. Returns approval status and reviewer names.",
	}, func(ctx tool.Context, args GetPRApprovalStatusArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		reviews, _, err := client.PullRequests.ListReviews(context.Background(), owner, repo, args.Number, &github.ListOptions{PerPage: 50})
		if err != nil {
			return Result{}, fmt.Errorf("failed to get reviews for PR #%d: %w", args.Number, err)
		}

		// Track latest review state per reviewer
		latestByUser := make(map[string]string)
		for _, r := range reviews {
			user := r.GetUser().GetLogin()
			state := r.GetState()
			if state == "APPROVED" || state == "CHANGES_REQUESTED" {
				latestByUser[user] = state
			}
		}

		status := "pending"
		var approvers, requesters []string
		for user, state := range latestByUser {
			switch state {
			case "APPROVED":
				approvers = append(approvers, user)
			case "CHANGES_REQUESTED":
				requesters = append(requesters, user)
			}
		}

		if len(requesters) > 0 {
			status = "changes_requested"
		} else if len(approvers) > 0 {
			status = "approved"
		}

		details := fmt.Sprintf("Status: %s", status)
		if len(approvers) > 0 {
			details += fmt.Sprintf("\nApproved by: %s", strings.Join(approvers, ", "))
		}
		if len(requesters) > 0 {
			details += fmt.Sprintf("\nChanges requested by: %s", strings.Join(requesters, ", "))
		}

		return Result{
			Summary: fmt.Sprintf("PR #%d approval status: %s", args.Number, status),
			Items: []Item{{
				Name:    fmt.Sprintf("PR #%d", args.Number),
				Status:  status,
				Details: details,
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func CommentOnPR() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "comment_on_pr",
		Description: "Add a comment to a GitHub PR. Used to report execution results or verification status.",
	}, func(ctx tool.Context, args CommentOnPRArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}

		// PRs share the Issues comment API in GitHub
		comment, _, err := client.Issues.CreateComment(context.Background(), owner, repo, args.Number, &github.IssueComment{
			Body: github.Ptr(args.Comment),
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to comment on PR #%d: %w", args.Number, err)
		}

		return Result{
			Summary: fmt.Sprintf("Added comment to PR #%d", args.Number),
			Items: []Item{{
				Name:    fmt.Sprintf("PR #%d comment", args.Number),
				Status:  "Created",
				Details: comment.GetHTMLURL(),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func MergeRemediationPR() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "merge_remediation_pr",
		Description: "Merge a remediation PR after successful verification. Adds a comment then squash-merges.",
	}, func(ctx tool.Context, args MergeRemediationPRArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}
		bgCtx := context.Background()

		if args.Comment != "" {
			_, _, err = client.Issues.CreateComment(bgCtx, owner, repo, args.Number, &github.IssueComment{
				Body: github.Ptr(args.Comment),
			})
			if err != nil {
				return Result{}, fmt.Errorf("failed to comment on PR #%d: %w", args.Number, err)
			}
		}

		result, _, err := client.PullRequests.Merge(bgCtx, owner, repo, args.Number, "Remediation verified and complete", &github.PullRequestOptions{
			MergeMethod: "squash",
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to merge PR #%d: %w", args.Number, err)
		}

		return Result{
			Summary: fmt.Sprintf("Merged PR #%d: %s", args.Number, result.GetMessage()),
			Items: []Item{{
				Name:   fmt.Sprintf("PR #%d", args.Number),
				Status: "Merged",
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func ClosePR() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "close_pr",
		Description: "Close a PR without merging. Used when verification fails or the remediation is no longer needed.",
	}, func(ctx tool.Context, args ClosePRArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}
		bgCtx := context.Background()

		if args.Comment != "" {
			_, _, err = client.Issues.CreateComment(bgCtx, owner, repo, args.Number, &github.IssueComment{
				Body: github.Ptr(args.Comment),
			})
			if err != nil {
				return Result{}, fmt.Errorf("failed to comment on PR #%d: %w", args.Number, err)
			}
		}

		_, _, err = client.PullRequests.Edit(bgCtx, owner, repo, args.Number, &github.PullRequest{
			State: github.Ptr("closed"),
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to close PR #%d: %w", args.Number, err)
		}

		return Result{
			Summary: fmt.Sprintf("Closed PR #%d without merging", args.Number),
			Items: []Item{{
				Name:   fmt.Sprintf("PR #%d", args.Number),
				Status: "Closed",
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

type UpdateRemediationPRArgs struct {
	Number      int    `json:"number" description:"GitHub PR number to update"`
	PlanContent string `json:"plan_content" description:"Updated markdown content for the remediation plan file"`
	Title       string `json:"title,omitempty" description:"Updated PR title (optional, keeps current if empty)"`
}

type GetPRCommentsArgs struct {
	Number int `json:"number" description:"GitHub PR number"`
}

func UpdateRemediationPR() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "update_remediation_pr",
		Description: "Update an existing remediation PR: replaces the plan file on the branch and updates the PR description. Use this when a PR already exists for an issue or when addressing reviewer change requests.",
	}, func(ctx tool.Context, args UpdateRemediationPRArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}
		bgCtx := context.Background()

		// Get the PR to find the branch and issue number
		pr, _, err := client.PullRequests.Get(bgCtx, owner, repo, args.Number)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get PR #%d: %w", args.Number, err)
		}

		branch := pr.GetHead().GetRef()

		// Extract issue number from branch name (remediation/issue-N)
		var issueNum int
		fmt.Sscanf(branch, "remediation/issue-%d", &issueNum)
		if issueNum == 0 {
			return Result{}, fmt.Errorf("could not determine issue number from branch name: %s", branch)
		}

		// Get existing file to obtain its SHA (required for update)
		filePath := fmt.Sprintf("remediation-plans/issue-%d.md", issueNum)
		existingFile, _, _, err := client.Repositories.GetContents(bgCtx, owner, repo, filePath, &github.RepositoryContentGetOptions{
			Ref: branch,
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to get existing plan file %s: %w", filePath, err)
		}

		// Update the plan file on the branch
		_, _, err = client.Repositories.UpdateFile(bgCtx, owner, repo, filePath, &github.RepositoryContentFileOptions{
			Message: github.Ptr(fmt.Sprintf("Update remediation plan for issue #%d", issueNum)),
			Content: []byte(args.PlanContent),
			Branch:  github.Ptr(branch),
			SHA:     github.Ptr(existingFile.GetSHA()),
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to update plan file: %w", err)
		}

		// Update PR body with the new plan content
		updatePR := &github.PullRequest{
			Body: github.Ptr(fmt.Sprintf("Fixes #%d\n\n---\n\n%s", issueNum, args.PlanContent)),
		}
		if args.Title != "" {
			updatePR.Title = github.Ptr(args.Title)
		}

		pr, _, err = client.PullRequests.Edit(bgCtx, owner, repo, args.Number, updatePR)
		if err != nil {
			return Result{}, fmt.Errorf("failed to update PR #%d: %w", args.Number, err)
		}

		return Result{
			Summary: fmt.Sprintf("Updated remediation PR #%d: %s", pr.GetNumber(), pr.GetHTMLURL()),
			Items: []Item{{
				Name:    fmt.Sprintf("PR #%d: %s", pr.GetNumber(), pr.GetTitle()),
				Status:  "Updated",
				Details: fmt.Sprintf("Branch: %s, Plan: %s, URL: %s", branch, filePath, pr.GetHTMLURL()),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func GetPRComments() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_pr_comments",
		Description: "Get all comments on a GitHub PR, including regular comments and review comments. Use this to read reviewer feedback and change requests.",
	}, func(ctx tool.Context, args GetPRCommentsArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}
		bgCtx := context.Background()

		items := make([]Item, 0)

		// Get regular issue comments (these include general PR comments)
		issueComments, _, err := client.Issues.ListComments(bgCtx, owner, repo, args.Number, &github.IssueListCommentsOptions{
			Sort:        github.Ptr("created"),
			Direction:   github.Ptr("asc"),
			ListOptions: github.ListOptions{PerPage: 50},
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list comments on PR #%d: %w", args.Number, err)
		}
		for _, c := range issueComments {
			items = append(items, Item{
				Name:    fmt.Sprintf("Comment by %s at %s", c.GetUser().GetLogin(), c.GetCreatedAt().Format(time.RFC3339)),
				Status:  "comment",
				Details: c.GetBody(),
			})
		}

		// Get review comments (inline code review comments)
		reviewComments, _, err := client.PullRequests.ListComments(bgCtx, owner, repo, args.Number, &github.PullRequestListCommentsOptions{
			Sort:        "created",
			Direction:   "asc",
			ListOptions: github.ListOptions{PerPage: 50},
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list review comments on PR #%d: %w", args.Number, err)
		}
		for _, c := range reviewComments {
			items = append(items, Item{
				Name:    fmt.Sprintf("Review comment by %s at %s", c.GetUser().GetLogin(), c.GetCreatedAt().Format(time.RFC3339)),
				Status:  "review_comment",
				Details: fmt.Sprintf("File: %s, Line: %d\n%s", c.GetPath(), c.GetLine(), c.GetBody()),
			})
		}

		// Get reviews themselves (approval/changes_requested with body)
		reviews, _, err := client.PullRequests.ListReviews(bgCtx, owner, repo, args.Number, &github.ListOptions{PerPage: 50})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list reviews on PR #%d: %w", args.Number, err)
		}
		for _, r := range reviews {
			if r.GetBody() == "" && r.GetState() == "PENDING" {
				continue
			}
			items = append(items, Item{
				Name:    fmt.Sprintf("Review by %s at %s", r.GetUser().GetLogin(), r.GetSubmittedAt().Format(time.RFC3339)),
				Status:  r.GetState(),
				Details: r.GetBody(),
			})
		}

		return Result{
			Summary: fmt.Sprintf("Found %d comments/reviews on PR #%d", len(items), args.Number),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func CommitFileToBranch() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "commit_file_to_branch",
		Description: "Create or update a single file on an existing branch. Use this to commit actual fixes to a remediation PR branch.",
	}, func(ctx tool.Context, args CommitFileToBranchArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}
		bgCtx := context.Background()

		// Check if the file already exists on this branch (we need its SHA to update it).
		existing, _, _, _ := client.Repositories.GetContents(bgCtx, owner, repo, args.FilePath, &github.RepositoryContentGetOptions{
			Ref: args.BranchName,
		})

		opts := &github.RepositoryContentFileOptions{
			Message: github.Ptr(args.Message),
			Content: []byte(args.Content),
			Branch:  github.Ptr(args.BranchName),
		}
		if existing != nil {
			opts.SHA = github.Ptr(existing.GetSHA())
			_, _, err = client.Repositories.UpdateFile(bgCtx, owner, repo, args.FilePath, opts)
		} else {
			_, _, err = client.Repositories.CreateFile(bgCtx, owner, repo, args.FilePath, opts)
		}
		if err != nil {
			return Result{}, fmt.Errorf("failed to commit %s to branch %s: %w", args.FilePath, args.BranchName, err)
		}

		action := "Created"
		if existing != nil {
			action = "Updated"
		}
		return Result{
			Summary: fmt.Sprintf("%s %s on branch %s", action, args.FilePath, args.BranchName),
			Items: []Item{{
				Name:    args.FilePath,
				Status:  action,
				Details: fmt.Sprintf("Branch: %s, Commit: %s", args.BranchName, args.Message),
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func GetPRPlanFile() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_pr_plan_file",
		Description: "Read the remediation plan file from a PR branch. Returns the markdown content of the plan.",
	}, func(ctx tool.Context, args GetPRPlanFileArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}
		owner, repo, err := parseOwnerRepo()
		if err != nil {
			return Result{}, err
		}
		bgCtx := context.Background()

		// Get the PR to find the branch and issue number
		pr, _, err := client.PullRequests.Get(bgCtx, owner, repo, args.Number)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get PR #%d: %w", args.Number, err)
		}

		branch := pr.GetHead().GetRef()

		// Extract issue number from branch name (remediation/issue-N)
		var issueNum int
		fmt.Sscanf(branch, "remediation/issue-%d", &issueNum)
		if issueNum == 0 {
			return Result{}, fmt.Errorf("could not determine issue number from branch name: %s", branch)
		}

		filePath := fmt.Sprintf("remediation-plans/issue-%d.md", issueNum)
		fileContent, _, _, err := client.Repositories.GetContents(bgCtx, owner, repo, filePath, &github.RepositoryContentGetOptions{
			Ref: branch,
		})
		if err != nil {
			return Result{}, fmt.Errorf("failed to read plan file %s from branch %s: %w", filePath, branch, err)
		}

		decoded, err := fileContent.GetContent()
		if err != nil {
			return Result{}, fmt.Errorf("failed to decode plan file content: %w", err)
		}
		content := decoded

		return Result{
			Summary: fmt.Sprintf("Plan file for PR #%d (issue #%d)", args.Number, issueNum),
			Items: []Item{{
				Name:    filePath,
				Status:  "Retrieved",
				Details: content,
			}},
			Issues: []string{},
		}, nil
	})
	return t
}
