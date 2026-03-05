# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Multi-agent application and Kubernetes audit system built with Google ADK. Four agents form a sequential pipeline — each agent feeds its output into the next:

1. **RepoChecker** (`cmd/repochecker/main.go`): Reviews a GitHub repository for code issues, secrets, misconfigurations, and vulnerabilities
2. **PlatformChecker** (`cmd/platformchecker/main.go`): Inspects the live Kubernetes cluster for health, security, and configuration issues
3. **Correlator** (`cmd/correlator/main.go`): Correlates findings from RepoChecker and PlatformChecker, identifies cross-cutting risks, and generates a markdown/PDF report
4. **Reporter** (`cmd/reporter/main.go`): Distributes the report — creates GitHub Issues, opens a PR with suggested fixes, and sends the report via Slack

## Commands

> **Important:** Always build binaries into `bin/`. That directory is gitignored. Never build binaries into the repo root or any other location.

```bash
# Run individual agents
go run ./cmd/repochecker/
go run ./cmd/platformchecker/
go run ./cmd/correlator/
go run ./cmd/reporter/

# Build all binaries — always output to bin/ (which is gitignored)
mkdir -p bin
go build -o bin/repochecker ./cmd/repochecker/
go build -o bin/platformchecker ./cmd/platformchecker/
go build -o bin/correlator ./cmd/correlator/
go build -o bin/reporter ./cmd/reporter/

# Tidy dependencies
go mod tidy

# Vet all packages
go vet ./...
```

## Environment Setup

- `GOOGLE_API_KEY` — required for Gemini model access. A `.env` file with `export GOOGLE_API_KEY='...'` is gitignored.
- `KUBECONFIG` — optional, defaults to `~/.kube/config`. In-cluster config is tried first.
- `GITHUB_REPO` — required by RepoChecker, in `owner/name` format. This is the **target repo being audited**.
- `REPORT_REPO` — required by Reporter, in `owner/name` format. This is the repo where GitHub Issues and PRs are created. Can be the same as `GITHUB_REPO` or a separate ops/security repo.
- `GITHUB_TOKEN` — required for GitHub API operations. A personal access token with `repo` scope.
- `SLACK_WEBHOOK_URL` — required for Reporter Slack notifications. Incoming webhook URL.

## Architecture

```
cmd/
  repochecker/
    main.go           -- RepoChecker: main(), model init, specialist agents, root agent, launcher
  platformchecker/
    main.go           -- PlatformChecker: main(), model init, specialist agents, root agent, launcher
  correlator/
    main.go           -- Correlator: reads both outputs, generates report
  reporter/
    main.go           -- Reporter: creates GH Issues/PR, sends Slack notification
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
  repo.go             -- tools: list repo directory, read file, get repo info, get repo tree, scan for secrets
  report.go           -- tools: write markdown report, convert to PDF
  notification.go     -- tools: send Slack message
  github.go           -- 7 tools: create inspection issue, create/list/get/close issues, comment, get comments
  github_pr.go        -- 8 tools: create/list/get PR, approval status, comment, merge, close, read plan file
agents/
  agents.go           -- K8s specialist agent constructors (workload, network, storage, rbac, security, events, nodes)
  repo_agents.go      -- Repo specialist agent constructors (code security, config review)
  factory.go          -- Root agent constructors for all four pipeline agents
```

### Four-Agent Pipeline

```
User: "audit repo X against cluster Y"
         │
         ▼
┌──────────────────────┐    ┌──────────────────────┐
│  RepoChecker         │    │  PlatformChecker     │
│  (repo_checker)      │    │  (platform_checker)  │
│                      │    │                      │
│  sub-agents:         │    │  sub-agents:         │
│  - code_security     │    │  - workload_inspector│
│  - config_review     │    │  - network_inspector │
│                      │    │  - storage_inspector │
│  finds:              │    │  - rbac_inspector    │
│  - secrets/creds     │    │  - security_inspector│
│  - vuln deps         │    │  - event_inspector   │
│  - Dockerfile issues │    │  - node_inspector    │
│  - K8s manifest      │    │                      │
│    misconfigs        │    │  finds:              │
│  - code smells       │    │  - unhealthy pods    │
└──────────┬───────────┘    │  - RBAC issues       │
           │                │  - security gaps     │
           │                │  - resource pressure │
           │                └──────────┬───────────┘
           │                           │
           └─────────────┬─────────────┘
                         ▼
              ┌──────────────────────┐
              │  Correlator          │
              │  (audit_correlator)  │
              │                      │
              │  - cross-references  │
              │    repo + cluster    │
              │  - prioritizes risks │
              │  - generates report  │
              │                      │
              │  output:             │
              │  reports/audit.md    │
              │  reports/audit.pdf   │
              └──────────┬───────────┘
                         │
                         ▼
              ┌──────────────────────┐
              │  Reporter            │
              │  (audit_reporter)    │
              │                      │
              │  - GitHub Issues     │
              │    (one per finding) │
              │  - GitHub PR         │
              │    (suggested fixes) │
              │  - Slack message     │
              │    (report summary)  │
              └──────────────────────┘
```

- **RepoChecker** (`repo_checker`): Routes to specialist sub-agents that read repo files, scan for secrets, review Dockerfiles and K8s manifests in the repo, and identify code-level security issues
- **PlatformChecker** (`platform_checker`): Routes to 7 K8s specialist sub-agents covering workloads, networking, storage, RBAC, security contexts, events, and node health
- **Correlator** (`audit_correlator`): Consumes findings from both checkers, identifies cross-cutting issues (e.g. a vulnerable image in the repo that is also running in production), assigns severity, and writes `reports/audit.md` (+ PDF if pandoc is available)
- **Reporter** (`audit_reporter`): Reads the report and findings, creates one GitHub Issue per significant finding, opens a PR with suggested fixes, and posts a summary to Slack
- **K8s sub-agents**: Each wraps domain-specific `functiontool.New()` tools and is exposed to PlatformChecker via `agenttool.New(agent, nil)`
- **Repo sub-agents**: Each wraps GitHub content API tools and is exposed to RepoChecker via `agenttool.New(agent, nil)`
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
