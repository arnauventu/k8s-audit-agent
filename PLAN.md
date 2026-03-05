# Implementation Plan

## Phase 0 — Cleanup existing code

- [x] Delete `cmd/inspector/`, `cmd/planner/`, `cmd/executor/`, `cmd/verifier/` (replaced by new agents)
- [x] Remove `tools/remediation.go` (remediation actions no longer part of this pipeline)
- [x] Update `config/config.go` — replace `Inspector/Planner/Executor/Verifier` model fields with `RepoChecker/PlatformChecker/Correlator/Reporter`
- [x] Update `cmd/all/main.go` to wire the 4 new agents

---

## Phase 1 — New tools

### `tools/repo.go` — GitHub repo reading
- [x] `get_repo_info` — metadata: default branch, language breakdown, topics, visibility, last push
- [x] `list_repo_directory` — list files/dirs at a path (`owner`, `repo`, `path`, `ref`)
- [x] `read_repo_file` — fetch file content via GitHub API, base64-decode, size-limit to 100KB
- [x] `get_repo_tree` — recursive file listing (all paths in the repo, useful for overview)
- [x] `scan_repo_for_secrets` — read files and apply regex patterns: AWS keys, GH tokens, private keys, generic passwords, JWTs

### `tools/report.go` — Report generation
- [x] `save_report_markdown` — write content to `reports/<filename>.md`, create dir if missing
- [x] `save_report_pdf` — render markdown to PDF using `go-pdf/fpdf` (no external dependencies)

### `tools/notification.go` — Slack
- [x] `send_slack_message` — POST to `SLACK_WEBHOOK_URL` with a message payload

---

## Phase 2 — New specialist agents

### `agents/repo_agents.go`
- [x] `NewCodeSecurityAgent` — reviews source files for OWASP issues, hardcoded secrets, insecure patterns; tools: `read_repo_file`, `scan_repo_for_secrets`
- [x] `NewConfigReviewAgent` — reviews Dockerfiles (root user, `latest` tag, no healthcheck), K8s manifests in repo (no limits, privileged, hostNetwork), CI configs; tools: `list_repo_directory`, `read_repo_file`, `get_repo_tree`

---

## Phase 3 — Root agents (`agents/factory.go`)

- [x] `NewRepoCheckerRoot` — orchestrates `CodeSecurityAgent` + `ConfigReviewAgent`; direct tools: `get_repo_info`, `get_repo_tree`
- [x] `NewPlatformCheckerRoot` — 7 K8s specialist sub-agents, no issue filing (findings go to Correlator)
- [x] `NewCorrelatorRoot` — tools: `save_report_markdown`, `save_report_pdf`; cross-references repo + cluster findings, writes report
- [x] `NewReporterRoot` — tools: `create_inspection_issue`, `create_remediation_pr`, `send_slack_message`

---

## Phase 4 — Command entrypoints

- [ ] `cmd/repochecker/main.go` — K8s init skipped, GitHub client checked, launch `NewRepoCheckerRoot`
- [ ] `cmd/platformchecker/main.go` — init K8s client, launch `NewPlatformCheckerRoot`
- [ ] `cmd/correlator/main.go` — no K8s/GitHub required at startup, launch `NewCorrelatorRoot`
- [x] `cmd/reporter/main.go` — GitHub client checked, Slack webhook checked, launch `NewReporterRoot`
- [x] `cmd/all/main.go` — all 4 agents wired into `agent.NewMultiLoader` for the ADK UI dropdown

---

## Phase 5 — Config & environment

- [x] Update `config/config.go` `ModelsConfig` struct: `RepoChecker`, `PlatformChecker`, `Correlator`, `Reporter` fields
- [x] Update `config/config.go` `ModelForAgent` switch to handle new agent names
- [ ] Add `config.yaml.example` with all 4 agent model names pre-filled
- [ ] Document `SLACK_WEBHOOK_URL` and `REPORT_REPO` in `CLAUDE.md`

---

## Phase 6 — Validation

- [ ] `go build ./...` passes with no errors
- [ ] `go vet ./...` passes clean
- [ ] Run RepoChecker against a test repo, verify findings are printed
- [ ] Run PlatformChecker against the local cluster, verify findings are printed
- [ ] Run Correlator with findings from both, verify `reports/audit.md` is written
- [ ] Run Reporter, verify GH issue is created and Slack message is sent
- [ ] Run `cmd/all/` and verify all 4 agents appear in the ADK UI dropdown
