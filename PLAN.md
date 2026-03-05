# Implementation Plan

## Phase 0 — Cleanup existing code

- [ ] Delete `cmd/inspector/`, `cmd/planner/`, `cmd/executor/`, `cmd/verifier/` (replaced by new agents)
- [ ] Remove `tools/remediation.go` (remediation actions no longer part of this pipeline)
- [ ] Update `config/config.go` — replace `Inspector/Planner/Executor/Verifier` model fields with `RepoChecker/PlatformChecker/Correlator/Reporter`
- [ ] Update `cmd/all/main.go` to wire the 4 new agents

---

## Phase 1 — New tools

### `tools/repo.go` — GitHub repo reading
- [ ] `get_repo_info` — metadata: default branch, language breakdown, topics, visibility, last push
- [ ] `list_repo_directory` — list files/dirs at a path (`owner`, `repo`, `path`, `ref`)
- [ ] `read_repo_file` — fetch file content via GitHub API, base64-decode, size-limit to 100KB
- [ ] `get_repo_tree` — recursive file listing (all paths in the repo, useful for overview)
- [ ] `scan_repo_for_secrets` — read files and apply regex patterns: AWS keys, GH tokens, private keys, generic passwords, JWTs

### `tools/report.go` — Report generation
- [ ] `write_markdown_report` — write content to `reports/audit.md`, create dir if missing
- [ ] `convert_report_to_pdf` — shell out to `pandoc reports/audit.md -o reports/audit.pdf`; return graceful error if pandoc not found

### `tools/notification.go` — Slack
- [ ] `send_slack_message` — POST to `SLACK_WEBHOOK_URL` with a message payload; include report summary and links to GH issues

---

## Phase 2 — New specialist agents

### `agents/repo_agents.go`
- [ ] `NewCodeSecurityAgent` — reviews source files for OWASP issues, hardcoded secrets, insecure patterns; tools: `read_repo_file`, `scan_repo_for_secrets`
- [ ] `NewConfigReviewAgent` — reviews Dockerfiles (root user, `latest` tag, no healthcheck), K8s manifests in repo (no limits, privileged, hostNetwork), CI configs; tools: `list_repo_directory`, `read_repo_file`, `get_repo_tree`

---

## Phase 3 — Root agents (`agents/factory.go`)

Replace existing root constructors with:

- [ ] `NewRepoCheckerRoot` — orchestrates `CodeSecurityAgent` + `ConfigReviewAgent`; direct tools: `get_repo_info`, `get_repo_tree`; instruction: read-only, report findings, do not modify anything
- [ ] `NewPlatformCheckerRoot` — same as existing `NewInspectorRoot` but without `CreateInspectionIssue` tool and with updated instruction (findings go to Correlator, not GitHub)
- [ ] `NewCorrelatorRoot` — no sub-agents; tools: `write_markdown_report`, `convert_report_to_pdf`; instruction: receives findings from both checkers in context, identifies cross-cutting risks, writes report
- [ ] `NewReporterRoot` — no sub-agents; tools: `create_inspection_issue`, `create_remediation_pr`, `send_slack_message`; instruction: reads report, creates one GH issue per finding, opens one PR with suggested fixes, sends Slack summary

---

## Phase 4 — Command entrypoints

- [ ] `cmd/repochecker/main.go` — K8s init skipped, GitHub client checked, launch `NewRepoCheckerRoot`
- [ ] `cmd/platformchecker/main.go` — init K8s client, launch `NewPlatformCheckerRoot`
- [ ] `cmd/correlator/main.go` — no K8s/GitHub required at startup, launch `NewCorrelatorRoot`
- [ ] `cmd/reporter/main.go` — GitHub client checked, Slack webhook checked, launch `NewReporterRoot`
- [ ] `cmd/all/main.go` — wire all 4 agents into `agent.NewMultiLoader` for the ADK UI dropdown

---

## Phase 5 — Config & environment

- [ ] Update `config/config.go` `ModelsConfig` struct: add `RepoChecker`, `PlatformChecker`, `Correlator`, `Reporter` fields; remove old ones
- [ ] Update `config/config.go` `ModelForAgent` switch to handle new agent names
- [ ] Add `config.yaml.example` with all 4 agent model names pre-filled
- [ ] Document `SLACK_WEBHOOK_URL` and `REPORT_REPO` (the repo where issues/PRs are opened, which may differ from the audited repo) in `CLAUDE.md`

---

## Phase 6 — Validation

- [ ] `go build ./...` passes with no errors
- [ ] `go vet ./...` passes clean
- [ ] Run RepoChecker against a test repo, verify findings are printed
- [ ] Run PlatformChecker against the local cluster, verify findings are printed
- [ ] Run Correlator with findings from both, verify `reports/audit.md` is written
- [ ] Run Reporter, verify GH issue is created and Slack message is sent
- [ ] Run `cmd/all/` and verify all 4 agents appear in the ADK UI dropdown
