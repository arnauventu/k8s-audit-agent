package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v68/github"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// --- Arg types ---

type GetRepoInfoArgs struct {
	Owner string `json:"owner" description:"GitHub repository owner (user or org)"`
	Repo  string `json:"repo" description:"GitHub repository name"`
}

type ListRepoDirArgs struct {
	Owner string `json:"owner" description:"GitHub repository owner"`
	Repo  string `json:"repo" description:"GitHub repository name"`
	Path  string `json:"path" description:"Directory path to list (empty string for root)"`
	Ref   string `json:"ref" description:"Branch, tag, or commit SHA (empty for default branch)"`
}

type ReadRepoFileArgs struct {
	Owner string `json:"owner" description:"GitHub repository owner"`
	Repo  string `json:"repo" description:"GitHub repository name"`
	Path  string `json:"path" description:"File path within the repository"`
	Ref   string `json:"ref" description:"Branch, tag, or commit SHA (empty for default branch)"`
}

type GetRepoTreeArgs struct {
	Owner string `json:"owner" description:"GitHub repository owner"`
	Repo  string `json:"repo" description:"GitHub repository name"`
	Ref   string `json:"ref" description:"Branch, tag, or commit SHA (empty for default branch)"`
}

type ScanRepoForSecretsArgs struct {
	Owner string   `json:"owner" description:"GitHub repository owner"`
	Repo  string   `json:"repo" description:"GitHub repository name"`
	Paths []string `json:"paths" description:"File paths to scan. If empty, scans common sensitive file types across the repo."`
	Ref   string   `json:"ref" description:"Branch, tag, or commit SHA (empty for default branch)"`
}

// --- Secret patterns ---

type secretPattern struct {
	name    string
	pattern *regexp.Regexp
}

var secretPatterns = []secretPattern{
	{"AWS Access Key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"AWS Secret Key", regexp.MustCompile(`(?i)aws.{0,20}secret.{0,20}['\"][0-9a-zA-Z/+]{40}['\"]`)},
	{"GitHub Token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`)},
	{"Private Key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`)},
	{"Generic API Key", regexp.MustCompile(`(?i)api[_-]?key\s*[=:]\s*['"][A-Za-z0-9_\-]{16,}['"]`)},
	{"Generic Secret", regexp.MustCompile(`(?i)secret\s*[=:]\s*['"][A-Za-z0-9_\-]{8,}['"]`)},
	{"Generic Password", regexp.MustCompile(`(?i)password\s*[=:]\s*['"][^'"]{6,}['"]`)},
	{"JWT Token", regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)},
	{"Slack Webhook", regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Z0-9]+/B[A-Z0-9]+/[A-Za-z0-9]+`)},
	{"SendGrid Key", regexp.MustCompile(`SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`)},
}

// sensitiveFileSuffixes lists file name patterns worth scanning.
var sensitiveFileSuffixes = []string{
	".env", ".env.local", ".env.production", ".env.staging",
	"credentials", "secrets", "secret.yaml", "secret.yml",
	".pem", ".key", ".p12", ".pfx",
	"config.yaml", "config.yml", "config.json",
	"values.yaml", "values.yml",
	"docker-compose.yml", "docker-compose.yaml",
	".github/workflows",
}

// --- Tools ---

func GetRepoInfo() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_repo_info",
		Description: "Fetch GitHub repository metadata: default branch, language breakdown, topics, visibility, description, and last push time.",
	}, func(ctx tool.Context, args GetRepoInfoArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}

		repo, _, err := client.Repositories.Get(context.Background(), args.Owner, args.Repo)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get repo %s/%s: %w", args.Owner, args.Repo, err)
		}

		langs, _, _ := client.Repositories.ListLanguages(context.Background(), args.Owner, args.Repo)

		langParts := make([]string, 0, len(langs))
		for lang, bytes := range langs {
			langParts = append(langParts, fmt.Sprintf("%s (%d bytes)", lang, bytes))
		}

		topics := strings.Join(repo.Topics, ", ")
		if topics == "" {
			topics = "none"
		}

		visibility := "public"
		if repo.GetPrivate() {
			visibility = "private"
		}

		lastPush := "unknown"
		if t := repo.GetPushedAt(); !t.IsZero() {
			lastPush = t.Format("2006-01-02 15:04:05 UTC")
		}

		details := fmt.Sprintf(
			"Default branch: %s\nVisibility: %s\nLast push: %s\nTopics: %s\nLanguages: %s\nDescription: %s\nStars: %d\nForks: %d\nOpen issues: %d",
			repo.GetDefaultBranch(),
			visibility,
			lastPush,
			topics,
			strings.Join(langParts, ", "),
			repo.GetDescription(),
			repo.GetStargazersCount(),
			repo.GetForksCount(),
			repo.GetOpenIssuesCount(),
		)

		return Result{
			Summary: fmt.Sprintf("Repository %s/%s — %s, default branch: %s, last push: %s",
				args.Owner, args.Repo, visibility, repo.GetDefaultBranch(), lastPush),
			Items: []Item{{
				Name:    fmt.Sprintf("%s/%s", args.Owner, args.Repo),
				Status:  visibility,
				Details: details,
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func ListRepoDirectory() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_repo_directory",
		Description: "List files and directories at a given path within a GitHub repository. Use empty path for the root.",
	}, func(ctx tool.Context, args ListRepoDirArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}

		opts := &github.RepositoryContentGetOptions{Ref: args.Ref}
		_, contents, _, err := client.Repositories.GetContents(context.Background(), args.Owner, args.Repo, args.Path, opts)
		if err != nil {
			return Result{}, fmt.Errorf("failed to list %s/%s@%s/%s: %w", args.Owner, args.Repo, args.Ref, args.Path, err)
		}

		items := make([]Item, 0, len(contents))
		for _, entry := range contents {
			size := ""
			if entry.GetType() == "file" {
				size = fmt.Sprintf("%d bytes", entry.GetSize())
			}
			items = append(items, Item{
				Name:    entry.GetName(),
				Status:  entry.GetType(), // "file" or "dir"
				Details: fmt.Sprintf("path: %s  sha: %s  %s", entry.GetPath(), shortSHA(entry.GetSHA()), size),
			})
		}

		return Result{
			Summary: fmt.Sprintf("Listed %d entries in %s/%s:%s", len(items), args.Owner, args.Repo, args.Path),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func ReadRepoFile() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "read_repo_file",
		Description: "Read the content of a file in a GitHub repository. Limited to 100 KB. Returns the decoded text content.",
	}, func(ctx tool.Context, args ReadRepoFileArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}

		opts := &github.RepositoryContentGetOptions{Ref: args.Ref}
		file, _, _, err := client.Repositories.GetContents(context.Background(), args.Owner, args.Repo, args.Path, opts)
		if err != nil {
			return Result{}, fmt.Errorf("failed to read %s: %w", args.Path, err)
		}
		if file == nil {
			return Result{}, fmt.Errorf("%s is a directory, not a file", args.Path)
		}
		if file.GetSize() > 100*1024 {
			return Result{}, fmt.Errorf("file %s is %d bytes, exceeds 100 KB limit", args.Path, file.GetSize())
		}

		content, err := file.GetContent()
		if err != nil {
			return Result{}, fmt.Errorf("failed to decode file content: %w", err)
		}

		return Result{
			Summary: fmt.Sprintf("Read %s (%d bytes)", args.Path, len(content)),
			Items: []Item{{
				Name:    args.Path,
				Status:  "file",
				Details: content,
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func GetRepoTree() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_repo_tree",
		Description: "Recursively list all file paths in a GitHub repository. Useful for getting a complete overview of the repo structure.",
	}, func(ctx tool.Context, args GetRepoTreeArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}

		ref := args.Ref
		if ref == "" {
			repo, _, err := client.Repositories.Get(context.Background(), args.Owner, args.Repo)
			if err != nil {
				return Result{}, fmt.Errorf("failed to get repo default branch: %w", err)
			}
			ref = repo.GetDefaultBranch()
		}

		tree, _, err := client.Git.GetTree(context.Background(), args.Owner, args.Repo, ref, true)
		if err != nil {
			return Result{}, fmt.Errorf("failed to get repo tree: %w", err)
		}

		items := make([]Item, 0, len(tree.Entries))
		for _, entry := range tree.Entries {
			items = append(items, Item{
				Name:    entry.GetPath(),
				Status:  entry.GetType(), // "blob" (file) or "tree" (dir)
				Details: fmt.Sprintf("size: %d bytes", entry.GetSize()),
			})
		}

		return Result{
			Summary: fmt.Sprintf("Repo tree for %s/%s@%s — %d entries", args.Owner, args.Repo, ref, len(items)),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func shortSHA(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

func ScanRepoForSecrets() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "scan_repo_for_secrets",
		Description: "Scan files in a GitHub repository for hardcoded secrets, credentials, and sensitive data using pattern matching. Checks for AWS keys, GitHub tokens, private keys, passwords, JWTs, and more.",
	}, func(ctx tool.Context, args ScanRepoForSecretsArgs) (Result, error) {
		client, err := githubClient()
		if err != nil {
			return Result{}, err
		}

		// If no specific paths given, discover sensitive files from the tree
		paths := args.Paths
		if len(paths) == 0 {
			ref := args.Ref
			if ref == "" {
				repo, _, err := client.Repositories.Get(context.Background(), args.Owner, args.Repo)
				if err != nil {
					return Result{}, fmt.Errorf("failed to resolve default branch: %w", err)
				}
				ref = repo.GetDefaultBranch()
			}
			tree, _, err := client.Git.GetTree(context.Background(), args.Owner, args.Repo, ref, true)
			if err != nil {
				return Result{}, fmt.Errorf("failed to list repo tree: %w", err)
			}
			for _, entry := range tree.Entries {
				if entry.GetType() != "blob" {
					continue
				}
				p := entry.GetPath()
				for _, suffix := range sensitiveFileSuffixes {
					if strings.HasSuffix(p, suffix) || strings.Contains(p, suffix) {
						paths = append(paths, p)
						break
					}
				}
			}
		}

		var findings []string
		scanned := 0

		for _, path := range paths {
			opts := &github.RepositoryContentGetOptions{Ref: args.Ref}
			file, _, _, err := client.Repositories.GetContents(context.Background(), args.Owner, args.Repo, path, opts)
			if err != nil || file == nil || file.GetSize() > 100*1024 {
				continue
			}

			content, err := file.GetContent()
			if err != nil {
				continue
			}

			scanned++
			lines := strings.Split(content, "\n")

			for _, sp := range secretPatterns {
				for lineNum, line := range lines {
					if sp.pattern.MatchString(line) {
						// Redact the match before reporting
						redacted := sp.pattern.ReplaceAllString(line, "[REDACTED]")
						findings = append(findings, fmt.Sprintf(
							"[%s] %s:%d — %s",
							sp.name, path, lineNum+1, strings.TrimSpace(redacted),
						))
					}
				}
			}
		}

		issues := make([]string, 0)
		items := make([]Item, 0, len(findings))
		for _, f := range findings {
			items = append(items, Item{
				Name:    f,
				Status:  "secret-detected",
				Details: f,
			})
			issues = append(issues, f)
		}

		summary := fmt.Sprintf("Scanned %d files in %s/%s — %d potential secrets found", scanned, args.Owner, args.Repo, len(findings))

		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}
