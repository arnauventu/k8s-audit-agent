# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Multi-agent Kubernetes cluster inspector and remediation system built with Google ADK. Four agents form a pipeline using GitHub Issues and PRs as the communication backbone:

1. **Inspector** (`cmd/inspector/main.go`): Read-only investigation + GitHub issue filing
2. **Planner** (`cmd/planner/main.go`): Reads issues, creates remediation plan PRs
3. **Executor** (`cmd/executor/main.go`): Reads approved PRs, applies remediation plans
4. **Verifier** (`cmd/verifier/main.go`): Independently verifies fixes, merges/closes PRs

The inspector routes queries to 7 specialist sub-agents and files investigation findings as GitHub issues. The planner reads these issues and creates PRs with detailed remediation plans. After human approval, the executor applies the plan. The verifier independently confirms success and manages the PR lifecycle.

## Commands

```bash
# Run the inspector agent
go run ./cmd/inspector/

# Run the planner agent
go run ./cmd/planner/

# Run the executor agent
go run ./cmd/executor/

# Run the verifier agent
go run ./cmd/verifier/

# Build all binaries
go build ./...

# Build individual agents
go build -o inspector ./cmd/inspector/
go build -o planner ./cmd/planner/
go build -o executor ./cmd/executor/
go build -o verifier ./cmd/verifier/

# Tidy dependencies
go mod tidy

# Vet all packages
go vet ./...
```

## Environment Setup

- `GOOGLE_API_KEY` — required for Gemini model access. A `.env` file with `export GOOGLE_API_KEY='...'` is gitignored.
- `KUBECONFIG` — optional, defaults to `~/.kube/config`. In-cluster config is tried first.
- `GITHUB_REPO` — required, GitHub repo in `owner/name` format.
- `GITHUB_TOKEN` — required for GitHub issue and PR operations. A personal access token with `repo` scope.

## Architecture

```
cmd/
  inspector/
    main.go           -- Inspector: main(), model init, specialist agents, root agent, launcher
  planner/
    main.go           -- Planner: reads issues, creates remediation plan PRs
  executor/
    main.go           -- Executor: reads approved PRs, applies remediation
  verifier/
    main.go           -- Verifier: verifies fixes, merges/closes PRs
k8s/
  client.go           -- K8s client-go singleton (sync.Once, in-cluster + kubeconfig fallback)
tools/
  workloads.go        -- 6 tools: pods, deployments, daemonsets, statefulsets, jobs
  networking.go       -- 4 tools: services, endpoints, ingresses, network policies
  storage.go          -- 2 tools: persistent volumes, PVCs
  rbac.go             -- 3 tools: roles, role bindings, service accounts
  security.go         -- 2 tools: pod security analysis, PSA enforcement
  events.go           -- 2 tools: cluster events, recent events
  nodes.go            -- 3 tools: node status, node resource usage, pod resource usage
  remediation.go      -- 6 tools: delete pod, rollout restart, scale, cordon/uncordon, delete stuck
  github.go           -- 7 tools: create inspection issue, create/list/get/close remediation issues, comment on issue, get comments
  github_pr.go        -- 8 tools: create/list/get PR, approval status, comment, merge, close, read plan file
agents/
  agents.go           -- 7 specialist agent constructors (each returns agent.Agent)
```

### Four-Agent Pipeline

```
User query
    │
    ▼
┌─────────────────────┐     ┌──────────────┐
│  Inspector Agent     │────▶│ GitHub Issue  │
│  (read-only)         │     │ k8s-remediation
│  - 7 specialists     │     └──────┬───────┘
│  - create_inspection │            │
│    _issue            │            ▼
└─────────────────────┘     ┌──────────────┐     ┌──────────────┐
                            │ Planner Agent │────▶│ GitHub PR    │
                            │ - read issues │     │ + plan file  │
                            │ - create PR   │     └──────┬───────┘
                            └──────────────┘            │
                                                  [Human Approval]
                                                        │
                                                        ▼
                            ┌──────────────┐     ┌──────────────┐
                            │ Executor Agent│◀────│ Approved PR  │
                            │ (read+write)  │     └──────────────┘
                            │ - 6 remediation
                            │   tools       │
                            │ - comment PR  │────▶ Execution Report
                            └──────────────┘            │
                                                        ▼
                            ┌──────────────┐     ┌──────────────┐
                            │ Verifier Agent│◀────│ PR + Report  │
                            │ - inspect     │     └──────────────┘
                            │ - verify fixes│
                            │ - merge/close │────▶ PR Merged + Issue Closed
                            └──────────────┘
```

- **Inspector** (`k8s_cluster_inspector`): Investigates cluster, files GitHub issues with investigation findings (evidence, reasoning, affected resources)
- **Planner** (`k8s_remediation_planner`): Reads open issues, creates PRs with detailed plan files in `remediation-plans/issue-<N>.md`
- **Executor** (`k8s_remediation_executor`): Reads approved PRs, applies remediation steps, comments execution results on the PR
- **Verifier** (`k8s_remediation_verifier`): Independently verifies fixes using inspection tools, merges PR on success or closes on failure
- **Specialist agents**: Each wraps domain-specific `functiontool.New()` tools and is exposed to the inspector via `agenttool.New(agent, nil)`
- **K8s client**: Singleton `*kubernetes.Clientset` via `sync.Once`, shared across all tools
- **Metrics**: Best-effort `metrics-server` client; graceful degradation when unavailable
- **Tool return format**: All tools return `Result{Summary, Items, Issues}` for consistent LLM consumption

### Key Dependencies

- `google.golang.org/adk` v0.5.0 — Agent Development Kit
- `google.golang.org/genai` — Gemini model client
- `k8s.io/client-go` — Kubernetes API client
- `k8s.io/metrics` — Metrics server client
- `github.com/google/go-github/v68` — GitHub API client

Go module path: `github.com/astrokube/hackathon-1-samples`
