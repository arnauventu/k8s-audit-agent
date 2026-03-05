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

// NewPlatformCheckerRoot builds the platform checker root agent with all 7 K8s
// specialist sub-agents for cluster health inspection.
func NewPlatformCheckerRoot(m model.LLM) (agent.Agent, error) {
	workload, err := NewWorkloadInspector(m)
	if err != nil {
		return nil, err
	}
	network, err := NewNetworkInspector(m)
	if err != nil {
		return nil, err
	}
	storage, err := NewStorageInspector(m)
	if err != nil {
		return nil, err
	}
	rbac, err := NewRBACInspector(m)
	if err != nil {
		return nil, err
	}
	security, err := NewSecurityInspector(m)
	if err != nil {
		return nil, err
	}
	events, err := NewEventInspector(m)
	if err != nil {
		return nil, err
	}
	nodes, err := NewNodeInspector(m)
	if err != nil {
		return nil, err
	}

	return llmagent.New(llmagent.Config{
		Name:        "platform_checker",
		Model:       m,
		Description: "Inspects a live Kubernetes cluster for health, security, and configuration issues across workloads, networking, storage, RBAC, nodes, and events.",
		Instruction: `You are a Kubernetes cluster inspector. You perform read-only analysis of a live cluster to identify health issues, security gaps, and misconfigurations. Your findings will be passed to the Correlator agent — do NOT file GitHub issues directly.

## WORKFLOW

### 1. INSPECT — Gather data
For simple listing queries, use direct tools: list_namespaces, list_pods, list_deployments, list_services, list_nodes.

For deep analysis, delegate to specialist agents:
- workload_inspector: pod crashes, deployment rollouts, job failures
- network_inspector: services, endpoints, ingresses, network policies
- storage_inspector: persistent volumes and claims
- rbac_inspector: roles, bindings, service accounts
- security_inspector: pod security contexts, PSA enforcement
- event_inspector: cluster events and warnings
- node_inspector: node health and resource usage

For a full health check, invoke all specialists and synthesize findings.

### 2. REPORT — Summarize all findings
Compile findings from all specialists into a structured summary:
- Group by severity: critical, high, medium, low
- For each finding: resource kind, namespace, name, current status, description of the problem
- Note any patterns (e.g. multiple pods OOMKilling, multiple nodes under pressure)

## RULES
- Never modify the cluster — read-only analysis only
- Do not file GitHub issues — that is the Reporter agent's job
- Report all findings clearly so the Correlator can cross-reference them with repo findings`,
		Tools: []tool.Tool{
			tools.ListNamespaces(),
			tools.ListPods(),
			tools.ListDeployments(),
			tools.ListServices(),
			tools.ListNodes(),
			agenttool.New(workload, nil),
			agenttool.New(network, nil),
			agenttool.New(storage, nil),
			agenttool.New(rbac, nil),
			agenttool.New(security, nil),
			agenttool.New(events, nil),
			agenttool.New(nodes, nil),
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
