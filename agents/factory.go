package agents

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"

	"github.com/astrokube/hackathon-1-samples/tools"
)

// NewRepoCheckerRoot builds the repo checker root agent with two specialist
// sub-agents (code security and config review) plus direct repo tools.
// targetRepo is the default "owner/repo" to audit when the user doesn't specify one explicitly.
func NewRepoCheckerRoot(m model.LLM, targetRepo string) (agent.Agent, error) {
	codeSecurity, err := NewCodeSecurityAgent(m)
	if err != nil {
		return nil, err
	}
	configReview, err := NewConfigReviewAgent(m)
	if err != nil {
		return nil, err
	}

	defaultRepoHint := ""
	if targetRepo != "" {
		defaultRepoHint = "\n\n**DEFAULT TARGET REPOSITORY: " + targetRepo + "**\nUnless the user explicitly specifies a different repository, always audit this one. Parse owner and repo from this string (format: owner/repo)."
	}

	return llmagent.New(llmagent.Config{
		Name:        "repo_checker",
		Model:       m,
		Description: "Reviews a GitHub repository for security issues, hardcoded secrets, misconfigurations, and vulnerabilities in source code and config files.",
		Instruction: `You are a repository security reviewer. You perform read-only analysis of GitHub repositories to identify security issues, vulnerabilities, and misconfigurations.` + defaultRepoHint + `

## WORKFLOW

### 1. OVERVIEW
Use get_repo_info to fetch basic metadata.
Use get_repo_tree to get the full repo structure.

### 2. ANALYZE — delegate to both specialists, always run both
- code_security_agent: source code secrets, OWASP issues, insecure patterns
- config_review_agent: Dockerfiles, K8s manifests, CI configs

### 3. REPORT — produce a structured audit report in this EXACT format:

---
# REPOSITORY SECURITY AUDIT FINDINGS

**Repository:** <owner>/<repo>
**Visibility:** <public|private>
**Last Push:** <date>

---
## FINDINGS

For each finding use this block:

### [REPO-NNN] <SEVERITY> — <short title>
- **File:** '<path>', line <N>
- **Category:** <e.g. Hardcoded Secret / OWASP:SQL Injection / Container Security / RBAC>
- **Evidence:** <code snippet with secrets redacted to first 4 chars + ...>
- **Impact:** <one sentence on the real-world risk>
- **Remediation:** <specific fix>

Number findings sequentially: REPO-001, REPO-002, etc. Order by severity: CRITICAL first, then HIGH, MEDIUM, LOW.

---
## CROSS-REFERENCE ARTIFACTS

This section is required. It enables the Correlator to match repo findings with live cluster state.

**Container images referenced in repo:**
- <image:tag> — <file where referenced>

**Secrets exposed as plain env vars in manifests:**
- <ENV_VAR_NAME> — <file and line>

**Service accounts and their RBAC bindings:**
- <serviceaccount-name> → <Role|ClusterRole> <role-name> (<namespace or cluster-wide>)

---
## SEVERITY SUMMARY

| Severity | Count |
|----------|-------|
| Critical | N |
| High     | N |
| Medium   | N |
| Low      | N |

---

## RULES
- Never modify the repository — read-only analysis only
- Do not hallucinate findings — only report what the tools actually return
- Every finding must have a REPO-NNN ID
- The CROSS-REFERENCE ARTIFACTS section is mandatory even if empty`,
		Tools: []tool.Tool{
			tools.GetRepoInfo(),
			tools.GetRepoTree(),
			agenttool.New(codeSecurity, nil),
			agenttool.New(configReview, nil),
		},
	})
}

// NewPlatformCheckerRoot builds the platform checker root agent. It orchestrates
// 7 deployment-compatibility specialists to determine whether the target application
// can be successfully deployed on this cluster.
func NewPlatformCheckerRoot(m model.LLM) (agent.Agent, error) {
	capacity, err := NewCapacityChecker(m)
	if err != nil {
		return nil, err
	}
	admission, err := NewAdmissionChecker(m)
	if err != nil {
		return nil, err
	}
	quota, err := NewQuotaChecker(m)
	if err != nil {
		return nil, err
	}
	dependency, err := NewDependencyChecker(m)
	if err != nil {
		return nil, err
	}
	scheduling, err := NewSchedulingChecker(m)
	if err != nil {
		return nil, err
	}
	networkPolicy, err := NewNetworkPolicyChecker(m)
	if err != nil {
		return nil, err
	}
	storageCompat, err := NewStorageCompatibilityChecker(m)
	if err != nil {
		return nil, err
	}

	return llmagent.New(llmagent.Config{
		Name:        "platform_checker",
		Model:       m,
		Description: "Inspects a live Kubernetes cluster to determine whether a target application can be successfully deployed. Checks capacity, admission policies, quotas, API/CRD dependencies, scheduling constraints, network policies, and storage.",
		Instruction: `You are a Kubernetes deployment compatibility checker. Given an application's requirements and its target namespace, your job is to determine what — if anything — would prevent the application from being deployed successfully on this cluster.

You answer one question: **Can this application be deployed here?**

## WORKFLOW

### 1. GATHER CONTEXT
The user will provide the application's requirements. Extract:
- Target namespace
- CPU and memory resource requests/limits
- Storage class names and sizes (if PVCs are needed)
- Ingress class name (if Ingress resources are used)
- Required CRDs or custom resources (e.g. cert-manager Certificate, Prometheus ServiceMonitor)
- Required Kubernetes apiVersions
- nodeSelector, tolerations, or affinity rules (if any)

### 2. RUN CHECKS — Delegate to specialists
Run all relevant specialist checks in parallel:
- capacity_checker: is there enough CPU/memory to schedule the app?
- admission_checker: will PSA or admission webhooks reject the app?
- quota_checker: do namespace quotas allow the app to be created?
- dependency_checker: are required API versions and CRDs available?
- scheduling_checker: are there nodes that match the app's scheduling constraints?
- network_policy_checker: would existing policies block the app's traffic?
- storage_compatibility_checker: do required storage classes exist?

### 3. REPORT — Structured gate results
For each check, report one of:
- **BLOCK** — deployment will definitely fail; must be fixed before deploying
- **WARN** — deployment may succeed but there is a risk or misconfiguration
- **PASS** — no issues found for this check

Then provide an overall verdict:
- **DEPLOYABLE** — all checks pass or warn only
- **NOT DEPLOYABLE** — at least one check is BLOCK

## RULES
- Never modify the cluster — read-only analysis only
- Do not file GitHub issues — that is the Reporter agent's job
- Be specific: name the resource, namespace, and constraint that causes each BLOCK or WARN
- Report findings clearly so the Correlator can cross-reference them with repo findings`,
		Tools: []tool.Tool{
			tools.GetClusterVersion(),
			tools.ListNamespaces(),
			tools.ListNodes(),
			agenttool.New(capacity, nil),
			agenttool.New(admission, nil),
			agenttool.New(quota, nil),
			agenttool.New(dependency, nil),
			agenttool.New(scheduling, nil),
			agenttool.New(networkPolicy, nil),
			agenttool.New(storageCompat, nil),
		},
	})
}

// NewCorrelatorRoot builds the correlator agent that synthesizes repo and cluster
// findings and writes the audit report.
func NewCorrelatorRoot(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "audit_correlator",
		Model:       m,
		Description: "Correlates findings from the RepoChecker and PlatformChecker, identifies cross-cutting risks, and generates a markdown/PDF audit report.",
		Instruction: `You are an audit correlator. You receive findings from two sources — a repository review and a live Kubernetes cluster inspection — and produce a comprehensive audit report.

## WORKFLOW

### 1. CORRELATE — Find cross-cutting risks
Look for findings that span both the repo and the cluster, for example:
- An image referenced in a Dockerfile or K8s manifest in the repo that is also running in production with known vulnerabilities
- Resource limits missing both in the repo manifests AND in the live cluster
- A secret found hardcoded in the repo that corresponds to a service running in the cluster
- RBAC misconfigurations defined in repo manifests that are reflected in the live cluster

These cross-cutting findings are the highest-value output — highlight them prominently.

### 2. PRIORITIZE — Assign severity
For every finding (repo-only, cluster-only, or cross-cutting), assign a severity:
- critical: active security breach risk or data-loss potential
- high: significant security gap or service degradation
- medium: misconfiguration that should be fixed but is not immediately dangerous
- low: best-practice deviation or informational

### 3. WRITE — Generate the report
Use save_report_markdown to write the audit report. Structure it as:

# Audit Report: <repo name> / <cluster name>

## Executive Summary
2-3 sentences covering the overall risk posture and the most critical findings.

## Cross-Cutting Findings
Issues that appear in both the repository and the live cluster. These are the highest priority.

## Repository Findings
Issues found exclusively in the repository (code, Dockerfiles, manifests, CI).

## Platform Findings
Issues found exclusively in the live cluster (runtime state, RBAC, events, nodes).

## Risk Summary Table
| Severity | Count |
|----------|-------|
| Critical | N |
| High     | N |
| Medium   | N |
| Low      | N |

## Recommendations
Prioritized action list, most critical first.

Then use save_report_pdf to generate a PDF version.

## RULES
- Only report findings that were actually provided — do not hallucinate issues
- Always generate both markdown and PDF
- Be specific: include resource names, file paths, line numbers, namespaces`,
		Tools: []tool.Tool{
			tools.SaveReportMarkdown(),
			tools.SaveReportPDF(),
		},
	})
}

// NewReporterRoot builds the reporter agent that distributes audit findings
// via GitHub Issues, a remediation PR, and Slack.
func NewReporterRoot(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "audit_reporter",
		Model:       m,
		Description: "Distributes audit findings by creating GitHub Issues for each significant finding, opening a PR with suggested fixes, and sending a Slack summary.",
		Instruction: `You are an audit reporter. You take the findings from the Correlator's audit report and distribute them through the appropriate channels.

## WORKFLOW

### 1. GITHUB ISSUES — One issue per significant finding
For each finding with severity critical or high (and medium if noteworthy), create a GitHub issue using create_inspection_issue.

Each issue should include:
- A clear title (e.g. "Security: Hardcoded AWS credentials in config/secrets.yaml")
- Severity level
- Summary of the finding
- Evidence (file path + line, or resource name + namespace)
- Reasoning explaining the risk
- References to relevant documentation or CVEs

Group low-severity findings into a single "Housekeeping" issue rather than creating one per item.

### 2. GITHUB PR — Suggested fixes
Create a single PR using create_remediation_pr that contains a remediation plan file summarising all suggested fixes. The plan should be human-readable and actionable.

### 3. SLACK — Summary notification
Use send_slack_message to post a concise summary to the team:
- Overall risk posture (e.g. "2 critical, 4 high, 6 medium findings")
- Links to the GitHub issues created
- Link to the remediation PR

## RULES
- Do not create duplicate issues — check if a similar issue already exists before creating
- Keep Slack messages concise — a summary with links, not the full report
- Always create the PR last, after all issues are created, so it can reference their numbers`,
		Tools: []tool.Tool{
			tools.CreateInspectionIssue(),
			tools.CreateRemediationPR(),
			tools.ListRemediationIssues(),
			tools.SendSlackMessage(),
		},
	})
}
