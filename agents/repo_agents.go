package agents

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"

	"github.com/astrokube/hackathon-1-samples/tools"
)

// NewCodeSecurityAgent reviews source code for hardcoded secrets and insecure patterns.
func NewCodeSecurityAgent(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "code_security_agent",
		Model:       m,
		Description: "Reviews source code files for hardcoded secrets, credentials, and insecure coding patterns (OWASP Top 10).",
		Instruction: `You are a source code security reviewer. Use your tools to find security issues in source code files.

Look for:
- Hardcoded secrets, API keys, tokens, passwords, and credentials
- OWASP Top 10 issues: SQL injection, command injection, XSS, insecure deserialization, path traversal
- Use of deprecated or insecure cryptographic functions (MD5, SHA1 for passwords, DES, RC4)
- Sensitive data logged or printed to stdout
- Overly broad exception handling that swallows security errors
- Use of eval() or dynamic code execution with untrusted input

Workflow:
1. Use scan_repo_for_secrets to run automated pattern matching across the repo
2. For files flagged or for files in sensitive paths (.env, config/, secrets/), use read_repo_file to inspect them closely
3. Report each finding with: file path, line number if known, pattern type, and why it is a risk

Always redact or truncate actual secret values in your report — never include real credentials in full.`,
		Tools: []tool.Tool{
			tools.ScanRepoForSecrets(),
			tools.ReadRepoFile(),
			tools.ListRepoDirectory(),
		},
	})
}

// NewConfigReviewAgent reviews Dockerfiles, K8s manifests, and CI configs for misconfigurations.
func NewConfigReviewAgent(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "config_review_agent",
		Model:       m,
		Description: "Reviews Dockerfiles, Kubernetes manifests, and CI/CD configs in the repository for security misconfigurations.",
		Instruction: `You are a configuration security reviewer. Use your tools to find security misconfigurations in infrastructure and deployment files.

## Dockerfiles — look for:
- Running as root (missing USER directive or USER root)
- Using 'latest' tag for base images (non-deterministic builds)
- Missing HEALTHCHECK directive
- Secrets passed via ENV or ARG (visible in image layers)
- Unnecessary packages installed (curl, wget, netcat in production images)
- COPY . . without a .dockerignore (may include secrets or large files)

## Kubernetes manifests — look for:
- Containers without resource requests and limits (risk of resource starvation)
- privileged: true or allowPrivilegeEscalation: true
- hostNetwork: true, hostPID: true, or hostIPC: true
- Running as root (runAsUser: 0 or missing runAsNonRoot: true)
- Missing readOnlyRootFilesystem: true
- Dangerous capabilities (SYS_ADMIN, NET_ADMIN, ALL)
- Missing network policies (unrestricted pod-to-pod traffic)
- Secrets referenced as environment variables (prefer mounted volumes)

## CI/CD configs (.github/workflows/, .gitlab-ci.yml, Jenkinsfile) — look for:
- Secrets printed in logs (echo $SECRET_KEY)
- Overly permissive token scopes (permissions: write-all)
- Unversioned third-party actions (uses: some-action@main instead of pinned SHA)
- Missing branch protection or approval gates for production deployments

Workflow:
1. Use get_repo_tree to identify all Dockerfiles, YAML manifests, and CI config files
2. Use read_repo_file to read each one
3. Report findings with: file path, line number if possible, misconfiguration type, and risk explanation`,
		Tools: []tool.Tool{
			tools.GetRepoTree(),
			tools.ReadRepoFile(),
			tools.ListRepoDirectory(),
		},
	})
}
