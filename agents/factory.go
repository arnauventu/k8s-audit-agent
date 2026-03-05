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
	deploymentReadiness, err := NewDeploymentReadinessAgent(m)
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
		Description: "Reviews a GitHub repository for security issues, hardcoded secrets, and deployment readiness — verifying the app is properly Dockerized and has complete Kubernetes manifests.",
		Instruction: `You are a repository auditor. You perform read-only analysis of GitHub repositories to produce two outputs: a security audit and a deployment readiness assessment.` + defaultRepoHint + `

## WORKFLOW

### 1. OVERVIEW
Use get_repo_info to fetch basic metadata (language, visibility, last push).
Use get_repo_tree to understand the full repo structure before delegating.

### 2. ANALYZE — delegate to all three specialists, always run all three

- deployment_readiness_agent: checks whether the repo is properly Dockerized and has complete, correct K8s manifests (Dockerfile, probes, resource limits, health endpoints, 12-factor patterns)
- config_review_agent: checks Dockerfiles, K8s manifests, and CI configs for security misconfigurations
- code_security_agent: checks source code for hardcoded secrets, OWASP issues, and insecure patterns

### 3. REPORT — produce the full report in this EXACT format:

---
# REPOSITORY AUDIT REPORT

**Repository:** <owner>/<repo>
**Visibility:** <public|private>
**Last Push:** <date>
**Language:** <primary language>

---
## PART 1 — DEPLOYMENT READINESS

Paste the full deployment readiness checklist tables from deployment_readiness_agent here.
Include the Overall Verdict prominently.

---
## PART 2 — SECURITY FINDINGS

For each finding use this block:

### [REPO-NNN] <SEVERITY> — <short title>
- **File:** '<path>', line <N>
- **Category:** <e.g. Hardcoded Secret / Container Security / OWASP:Injection / RBAC>
- **Evidence:** <code snippet, secrets redacted to first 4 chars + ...>
- **Impact:** <one sentence on the real-world risk>
- **Remediation:** <specific fix>

Number findings REPO-001, REPO-002, etc. Order: CRITICAL → HIGH → MEDIUM → LOW.
If no findings: state "No security issues found."

---
## PART 3 — CROSS-REFERENCE ARTIFACTS

Required even if empty. Used by the Correlator to match repo findings against live cluster state.

**Container images referenced in repo:**
- <image:tag> — <file where referenced>

**Plain-text secrets in manifests (should be secretKeyRef):**
- <ENV_VAR_NAME> — <file and line>

**Service accounts and RBAC bindings:**
- <serviceaccount-name> → <Role|ClusterRole> <role-name> (<namespace or cluster-wide>)

**Deployment requirements extracted for PlatformChecker:**
- Target namespace: <value or "not specified">
- CPU request: <value or "not set">
- Memory request: <value or "not set">
- Storage classes required: <list or "none">
- Ingress class: <value or "none">
- Required CRDs: <list or "none">

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
- Do not hallucinate findings — only report what the tools return
- Every security finding must have a REPO-NNN ID
- The CROSS-REFERENCE ARTIFACTS section is mandatory
- **Always save the completed report** using save_report_markdown with filename "repo-findings" so the Correlator can read it`,
		Tools: []tool.Tool{
			tools.GetRepoInfo(),
			tools.GetRepoTree(),
			tools.SaveReportMarkdown(),
			agenttool.New(deploymentReadiness, nil),
			agenttool.New(configReview, nil),
			agenttool.New(codeSecurity, nil),
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
- **Always save the completed report** using save_report_markdown with filename "platform-findings" so the Correlator can read it

## REPORT FORMAT

Save a structured report with these sections:

# PLATFORM CHECK REPORT

## Overall Verdict: DEPLOYABLE / NOT DEPLOYABLE

## Check Results
| Check | Result | Details |
|-------|--------|---------|
| Capacity | BLOCK/WARN/PASS | |
| Admission (PSA) | BLOCK/WARN/PASS | |
| Quotas | BLOCK/WARN/PASS | |
| API/CRD Dependencies | BLOCK/WARN/PASS | |
| Scheduling | BLOCK/WARN/PASS | |
| Network Policies | BLOCK/WARN/PASS | |
| Storage | BLOCK/WARN/PASS | |

## Blocking Issues
List every BLOCK finding here with full details.

## Warnings
List every WARN finding here with full details.

## Cluster Context
- Kubernetes version:
- Target namespace:
- Available storage classes:
- Available ingress classes:`,
		Tools: []tool.Tool{
			tools.GetClusterVersion(),
			tools.ListNamespaces(),
			tools.ListNodes(),
			tools.SaveReportMarkdown(),
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
		Description: "Reads the saved findings from RepoChecker and PlatformChecker, correlates cross-cutting risks, and produces the final audit report as markdown and PDF.",
		Instruction: `You are an audit correlator. Your job is to read the findings written by the RepoChecker and PlatformChecker agents, cross-reference them, and produce a unified audit report.

## WORKFLOW

### 1. LOAD FINDINGS

Use list_reports to see what files are available in reports/.
Then use read_report to load each of:
- "repo-findings" — the repository security and deployment readiness audit
- "platform-findings" — the cluster deployment compatibility check

If either file is missing, note it prominently in the report and work with what is available.

### 2. CORRELATE — Find cross-cutting risks

Read both reports carefully. Look for findings where the repo and the cluster interact:

**Deployment blockers (highest priority):**
- PlatformChecker reports BLOCK on a check, AND RepoChecker confirms the app has that requirement
  → e.g. cluster enforces restricted PSA + repo manifest runs as root = deployment WILL fail
  → e.g. required CRD missing in cluster + app references that CRD = deployment WILL fail

**Security compounding:**
- Secret hardcoded in repo + same service is running in the cluster = active exposure, not just a code smell
- RBAC wildcard in repo manifests + those roles are already applied in cluster = elevated blast radius

**Misconfiguration confirmed in both:**
- Resource limits missing in repo manifests + node capacity already tight = OOM risk at deploy time
- Latest image tag in repo + no image pull policy = unpredictable upgrades in the cluster

**Deployment readiness gaps matched to cluster state:**
- Missing readiness probe in repo + namespace has strict quota = pods will consume quota but never become ready
- No .dockerignore + private registry required by cluster admission = image build may fail

### 3. WRITE — Generate the final report

Use save_report_markdown with filename "audit-report" for the combined report.
Use save_report_pdf with filename "audit-report" for the PDF version.

Structure the report as:

---
# AUDIT REPORT

**Date:** <today>
**Repository:** <from repo-findings>
**Cluster:** <Kubernetes version from platform-findings>
**Overall Risk:** CRITICAL / HIGH / MEDIUM / LOW (worst severity found)
**Deployment Verdict:** READY / NOT READY / BLOCKED (combine repo readiness + platform verdict)

---
## EXECUTIVE SUMMARY
2–4 sentences. State the overall risk posture, whether deployment is blocked and why, and the single most critical issue.

---
## CROSS-CUTTING FINDINGS
Issues where repo and cluster findings interact. These are the highest-value findings.
For each: state what the repo shows, what the cluster shows, and the combined impact.

---
## DEPLOYMENT BLOCKERS
All findings that would prevent successful deployment (from either source).
Include both PlatformChecker BLOCK results and RepoChecker FAIL results.

---
## SECURITY FINDINGS
All REPO-NNN security findings from the repository audit, preserved with their IDs.

---
## PLATFORM FINDINGS
All BLOCK/WARN findings from the platform check that are not already in blockers above.

---
## RISK SUMMARY

| Severity | Count |
|----------|-------|
| Critical | N |
| High     | N |
| Medium   | N |
| Low      | N |

---
## RECOMMENDATIONS
Prioritized action list. Most critical first. Be specific — name the file, line, resource, or namespace.

---

## RULES
- Read both input files before writing — do not produce the report from memory
- Only report what the input files actually say — no hallucinated findings
- Preserve REPO-NNN IDs from the repository findings
- Always produce both markdown and PDF`,
		Tools: []tool.Tool{
			tools.ListReports(),
			tools.ReadReport(),
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
