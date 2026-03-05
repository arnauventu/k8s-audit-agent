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

## WHAT TO LOOK FOR

**Hardcoded secrets and credentials:**
- Hardcoded passwords, API keys, tokens, private keys, connection strings
- Secrets logged to stdout/stderr at startup or in error paths

**OWASP Top 10 patterns:**
- SQL injection: string concatenation to build queries instead of parameterized queries
- Command injection: user input passed to exec/system calls
- Missing input validation: no size limits on request bodies, no type checks, no sanitization
- Sensitive data exposure: secrets, PII, or internal error details returned to callers

**Insecure server configuration:**
- HTTP servers without TLS (ListenAndServe instead of ListenAndServeTLS)
- HTTP servers without read/write timeouts (slowloris vulnerability)
- CORS policy set to wildcard (Access-Control-Allow-Origin: *)
- No authentication or authorization on sensitive endpoints
- Missing IDOR protection (e.g. user can access/delete any resource by ID without ownership check)

**Insecure crypto:**
- MD5 or SHA1 for password hashing
- DES, RC4, or other weak ciphers

## WORKFLOW

1. Use scan_repo_for_secrets to run automated pattern matching across all files
2. Use list_repo_directory to discover all source files in the repo root and subdirectories
3. Use read_repo_file to read EVERY source file found (.go, .py, .js, .ts, .java, .rb, .php, .env, config files)
4. Analyze each file line by line for all the patterns above
5. For each finding report:
   - File path and line number
   - The vulnerable code snippet (redact secrets: show only first 4 chars + ***)
   - Severity: CRITICAL / HIGH / MEDIUM / LOW
   - Why it is a risk

Always redact actual secret values — show only first 4 chars followed by *** (e.g. "supe***").`,
		Tools: []tool.Tool{
			tools.ScanRepoForSecrets(),
			tools.ReadRepoFile(),
			tools.ListRepoDirectory(),
		},
	})
}

// NewDeploymentReadinessAgent checks whether the repo is properly Dockerized and
// has complete, correct Kubernetes manifests ready for deployment.
func NewDeploymentReadinessAgent(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "deployment_readiness_agent",
		Model:       m,
		Description: "Checks whether the repository is properly Dockerized and has complete Kubernetes manifests ready for deployment.",
		Instruction: `You are a deployment readiness reviewer. Your job is to determine whether this application is ready to be built as a container image and deployed on Kubernetes.

Use get_repo_tree to discover all files first, then read_repo_file to inspect each relevant file.

---

## CHECK 1 — Dockerfile

Find the Dockerfile (check root, docker/, build/, or any Dockerfile.* variant).

For each Dockerfile found, verify:
- [ ] Base image uses a specific, pinned tag — NOT 'latest' (e.g. 'golang:1.22-alpine' is OK, 'golang:latest' is NOT)
- [ ] Multi-stage build is used to keep the final image small (look for multiple FROM statements)
- [ ] USER directive sets a non-root user before the final CMD/ENTRYPOINT
- [ ] WORKDIR is set
- [ ] EXPOSE declares the application port
- [ ] HEALTHCHECK directive is present
- [ ] CMD or ENTRYPOINT is defined
- [ ] .dockerignore exists alongside the Dockerfile

**If no Dockerfile exists: report FAIL — the repo cannot be containerized.**

---

## CHECK 2 — Kubernetes manifests

Look for manifests in: k8s/, kubernetes/, deploy/, manifests/, helm/, chart/, charts/, or root-level YAML files.
Also accept: Helm chart (Chart.yaml + templates/), Kustomize (kustomization.yaml).

**Minimum required manifests:**
- [ ] Deployment (or StatefulSet if stateful) exists
- [ ] Service exists to expose the app
- [ ] ConfigMap exists if the app has non-secret configuration
- [ ] Secret or secretKeyRef usage if the app needs credentials (NOT plain env.value)
- [ ] Ingress exists if the app should be externally accessible (optional — only flag if the app clearly listens on HTTP/HTTPS)

**If no manifests exist: report FAIL — the app has no deployment definition.**

---

## CHECK 3 — Manifest completeness

For each Deployment/StatefulSet found, verify:
- [ ] image tag is specific — NOT 'latest'
- [ ] resources.requests (CPU and memory) are set on every container
- [ ] resources.limits (CPU and memory) are set on every container
- [ ] livenessProbe is defined on every container
- [ ] readinessProbe is defined on every container
- [ ] securityContext sets runAsNonRoot: true OR runAsUser > 0
- [ ] Labels are defined and the selector matches the pod template labels exactly
- [ ] Replica count is explicit (not relying on default of 1 with no HPA)

For each Service found, verify:
- [ ] selector matches the Deployment pod template labels
- [ ] Port mapping is correct and matches what the app actually exposes

---

## CHECK 4 — Cloud-native / 12-factor readiness

Inspect source code files for:
- [ ] Configuration via environment variables — look for os.Getenv, process.env, os.environ, etc. NOT hardcoded URLs, ports, or credentials
- [ ] Logging to stdout/stderr — NOT writing to log files or /var/log
- [ ] Graceful shutdown — SIGTERM handler present (signal.Notify, process.on('SIGTERM'), etc.)
- [ ] Health endpoint — a /health, /healthz, /ready, or /ping HTTP route exists that returns 200 OK (this is what the K8s probe will call)
- [ ] No local file system state — app does not write files to local disk unless a PVC is declared

---

## REPORT FORMAT

Produce a deployment readiness checklist. Use this exact structure:

# DEPLOYMENT READINESS REPORT

## Dockerfile
| Check | Result | Notes |
|-------|--------|-------|
| Dockerfile exists | PASS/FAIL | path |
| Pinned base image tag | PASS/FAIL/WARN | e.g. uses golang:latest |
| Multi-stage build | PASS/FAIL/WARN | |
| Non-root USER | PASS/FAIL | |
| WORKDIR set | PASS/FAIL | |
| EXPOSE declared | PASS/FAIL | |
| HEALTHCHECK present | PASS/FAIL | |
| CMD or ENTRYPOINT | PASS/FAIL | |
| .dockerignore present | PASS/WARN | |

## Kubernetes Manifests
| Check | Result | Notes |
|-------|--------|-------|
| Deployment manifest | PASS/FAIL | path |
| Service manifest | PASS/FAIL | path |
| ConfigMap | PASS/WARN | path or "not found" |
| Secrets via secretKeyRef | PASS/FAIL/WARN | |
| Ingress | PASS/WARN/N/A | |

## Manifest Completeness
| Check | Result | Notes |
|-------|--------|-------|
| Pinned image tag | PASS/FAIL | |
| Resource requests set | PASS/FAIL | |
| Resource limits set | PASS/FAIL | |
| Liveness probe | PASS/FAIL | |
| Readiness probe | PASS/FAIL | |
| Non-root securityContext | PASS/FAIL | |
| Labels and selector match | PASS/FAIL | |

## Cloud-Native Readiness
| Check | Result | Notes |
|-------|--------|-------|
| Config via env vars | PASS/FAIL/WARN | |
| Stdout/stderr logging | PASS/FAIL/WARN | |
| SIGTERM handler | PASS/FAIL/WARN | |
| Health endpoint exposed | PASS/FAIL/WARN | path found e.g. /healthz |
| No local file system state | PASS/WARN | |

## Overall Verdict
**READY FOR DEPLOYMENT** — all critical checks pass
OR
**NOT READY** — list blocking issues here, one per line`,
		Tools: []tool.Tool{
			tools.GetRepoTree(),
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
- Running as root (missing USER directive)
- Using 'latest' or EOL tag for base images (non-deterministic / unpatched builds)
- Missing HEALTHCHECK directive
- Secrets passed via ENV or ARG (visible in image layers)
- COPY . . without a .dockerignore (may include secrets or large files)
- Single-stage build shipping the full toolchain (should use multi-stage with distroless/alpine)
- Missing EXPOSE declaration

## Kubernetes manifests — look for:
- Containers without resource requests and limits (risk of resource starvation)
- privileged: true or allowPrivilegeEscalation: true
- hostNetwork: true, hostPID: true, or hostIPC: true
- Running as root (runAsUser: 0 or missing runAsNonRoot: true and securityContext entirely)
- Missing readOnlyRootFilesystem: true
- Dangerous capabilities (SYS_ADMIN, NET_ADMIN, ALL)
- Secrets stored as plain literal env vars (should use secretKeyRef to Kubernetes Secrets)
- Using 'latest' image tag in pod specs
- Missing liveness and readiness probes
- RBAC ClusterRoles/Roles with wildcard verbs or resources
- Service type NodePort exposing internal services externally
- Missing network policies

## CI/CD configs — look for:
- Secrets printed in logs
- Overly permissive token scopes
- Unversioned third-party actions

## WORKFLOW

1. Use get_repo_tree to identify ALL files in the repo
2. Read every Dockerfile, YAML manifest (k8s/, deploy/, charts/, helm/), and CI config file
3. For each finding report:
   - File path and line number
   - Misconfiguration type and severity: CRITICAL / HIGH / MEDIUM / LOW
   - Risk explanation
4. At the end, list these CROSS-REFERENCE ARTIFACTS explicitly:
   - All container image names referenced (e.g. "golang:1.16", "taskmanager:latest")
   - All plain-text secrets used as env vars (name only, not value)
   - All service account names with their bound roles/clusterroles`,
		Tools: []tool.Tool{
			tools.GetRepoTree(),
			tools.ReadRepoFile(),
			tools.ListRepoDirectory(),
		},
	})
}
