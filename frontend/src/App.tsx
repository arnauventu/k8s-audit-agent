import { useState, useRef, useCallback } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import {
  Search,
  Server,
  GitMerge,
  Bell,
  CheckCircle2,
  XCircle,
  Loader2,
  ChevronDown,
  ChevronUp,
  Shield,
  Github,
  Globe,
  Play,
  Square,
} from "lucide-react";
import "./App.css";

type StageStatus = "idle" | "running" | "done" | "error";

interface Stage {
  id: string;
  label: string;
  description: string;
  icon: React.ReactNode;
  status: StageStatus;
  output: string;
}

const STAGE_DEFS = [
  {
    id: "repo_checker",
    label: "Repository Checker",
    description: "Scanning code for secrets, vulnerabilities & misconfigs",
    icon: <Search size={18} />,
  },
  {
    id: "platform_checker",
    label: "Platform Checker",
    description: "Inspecting live Kubernetes cluster compatibility",
    icon: <Server size={18} />,
  },
  {
    id: "correlator",
    label: "Correlator",
    description: "Cross-referencing findings & generating audit report",
    icon: <GitMerge size={18} />,
  },
  {
    id: "reporter",
    label: "Reporter",
    description: "Creating GitHub Issues, PR & sending Slack notification",
    icon: <Bell size={18} />,
  },
];

function makeStages(): Stage[] {
  return STAGE_DEFS.map((s) => ({ ...s, status: "idle", output: "" }));
}

const DEFAULT_PROMPT =
  "Audit the repository for security vulnerabilities, hardcoded secrets, Kubernetes misconfigurations, and check cluster deployment readiness. Correlate findings and report.";

// ---------------------------------------------------------------------------
// Mock data for dev preview
// ---------------------------------------------------------------------------
const MOCK_OUTPUTS: Record<string, string> = {
  repo_checker: `## Repository Security Findings

### [REPO-001] CRITICAL — Hardcoded database password
- **File:** \`main.go\`, line 14
- **Category:** Secret / Credential Leak
- **Evidence:** \`db_password := "supersecret123"\`
- **Impact:** Database fully compromised if repo is public or leaked
- **Remediation:** Move to environment variable or secret manager

### [REPO-002] HIGH — Outdated base image (golang:1.16)
- **File:** \`Dockerfile\`, line 1
- **Category:** Vulnerable Dependency
- **Evidence:** \`FROM golang:1.16\` — EOL since Aug 2022
- **Impact:** Known CVEs unfixed in runtime
- **Remediation:** Upgrade to \`golang:1.23-alpine\` with multi-stage build

### [REPO-003] HIGH — RBAC wildcard ClusterRole
- **File:** \`k8s/rbac.yaml\`, line 9
- **Category:** Overprivileged RBAC
- **Evidence:** \`resources: ["*"]  verbs: ["*"]\`
- **Impact:** Service account has cluster-admin equivalent
- **Remediation:** Scope to least-privilege resources and verbs

## Severity Summary
| Severity | Count |
|----------|-------|
| Critical | 1     |
| High     | 2     |
| Medium   | 0     |`,

  platform_checker: `## Platform / Cluster Findings

### [PLAT-001] HIGH — No resource limits on deployment
- **Namespace:** \`default\`
- **Workload:** \`task-manager\`
- **Evidence:** \`resources: {}\` — no limits or requests set
- **Impact:** Pod can consume all node CPU/memory, causing noisy-neighbour issues
- **Remediation:** Set \`resources.requests\` and \`resources.limits\`

### [PLAT-002] MEDIUM — No liveness/readiness probes
- **Namespace:** \`default\`
- **Workload:** \`task-manager\`
- **Impact:** Kubernetes cannot detect unhealthy containers
- **Remediation:** Add \`livenessProbe\` and \`readinessProbe\`

### [PLAT-003] MEDIUM — NodePort service exposed
- **Service:** \`task-manager-service\`, port 30080
- **Impact:** Service directly reachable from outside the cluster without ingress/TLS
- **Remediation:** Switch to ClusterIP + Ingress with TLS termination`,

  correlator: `# Security Audit Report — astrokube/hackathon-1-team-2-a

**Generated:** 2026-03-05
**Risk Level:** 🔴 CRITICAL

---

## Executive Summary

The audit identified **6 findings** (1 Critical, 4 High, 2 Medium) across repository code and the live Kubernetes cluster. The most severe issue is a hardcoded database credential that is directly exposed at runtime via a Kubernetes secret stored as a plain env var — a full chain of compromise.

## Cross-Reference: Repo ↔ Cluster

| Repo Finding | Cluster Finding | Combined Risk |
|---|---|---|
| REPO-001 Hardcoded password | Deployed as plain \`DB_PASSWORD\` env var | 🔴 CRITICAL chain |
| REPO-002 EOL base image | Running in production pod | 🔴 CVEs in prod |
| REPO-003 Wildcard RBAC | ClusterRole bound to default SA | 🔴 Cluster takeover |

## Top Priorities

1. **Rotate database credentials immediately** — REPO-001 + PLAT env var leak
2. **Rebuild image with golang:1.23** — REPO-002
3. **Replace wildcard ClusterRole** — REPO-003 + PLAT-001`,

  reporter: `## Reporter Summary

✅ **GitHub Issue #42** created: *[CRITICAL] Hardcoded database password exposed at runtime*
✅ **GitHub Issue #43** created: *[HIGH] EOL base image golang:1.16 running in production*
✅ **GitHub Issue #44** created: *[HIGH] Wildcard ClusterRole grants cluster-admin to default SA*
✅ **Pull Request #12** opened: *security: remove hardcoded creds, upgrade base image, scope RBAC*
✅ **Slack notification** sent to #security-alerts`,
};

// Simulate streaming tokens from a string
async function* streamText(text: string, chunkSize = 8, delayMs = 15) {
  for (let i = 0; i < text.length; i += chunkSize) {
    yield text.slice(i, i + chunkSize);
    await new Promise((r) => setTimeout(r, delayMs));
  }
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------
export default function App() {
  const [githubRepo, setGithubRepo] = useState("");
  const [clusterUrl, setClusterUrl] = useState("");
  const [stages, setStages] = useState<Stage[]>(makeStages());
  const [running, setRunning] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expandedStage, setExpandedStage] = useState<string | null>(null);
  const [liveText, setLiveText] = useState("");
  const abortRef = useRef<AbortController | null>(null);
  const cancelledRef = useRef(false);

  const updateStage = useCallback((id: string, patch: Partial<Stage>) => {
    setStages((prev) => prev.map((s) => (s.id === id ? { ...s, ...patch } : s)));
  }, []);

  // Process an SSE event emitted from either real API or mock
  const processEvent = useCallback(
    (
      ev: { stage: string; status: string; text?: string },
      currentOutputs: Record<string, string>,
      onDone: () => void,
      onError: (msg: string) => void
    ) => {
      const { stage, status, text } = ev;

      if (stage === "pipeline" && (status === "complete" || status === "connected")) {
        if (status === "complete") onDone();
        return;
      }

      if (status === "started") {
        currentOutputs[stage] = "";
        updateStage(stage, { status: "running", output: "" });
        setExpandedStage(stage);
        setLiveText("");
      } else if (status === "token" && text) {
        currentOutputs[stage] = (currentOutputs[stage] ?? "") + text;
        setLiveText(currentOutputs[stage].slice(-200));
      } else if (status === "done") {
        const finalOutput = text ?? currentOutputs[stage] ?? "";
        currentOutputs[stage] = finalOutput;
        updateStage(stage, { status: "done", output: finalOutput });
        setLiveText("");
      } else if (status === "error") {
        updateStage(stage, { status: "error", output: text ?? "Unknown error" });
        onError(`Error in ${stage}: ${text}`);
      }
    },
    [updateStage]
  );

  // ── Mock run (no backend needed) ──────────────────────────────────────────
  const runMock = useCallback(async () => {
    cancelledRef.current = false;
    const currentOutputs: Record<string, string> = {};

    for (const stageDef of STAGE_DEFS) {
      if (cancelledRef.current) break;

      processEvent(
        { stage: stageDef.id, status: "started" },
        currentOutputs,
        () => {},
        () => {}
      );

      const mockText = MOCK_OUTPUTS[stageDef.id] ?? "No output.";
      for await (const chunk of streamText(mockText)) {
        if (cancelledRef.current) break;
        processEvent(
          { stage: stageDef.id, status: "token", text: chunk },
          currentOutputs,
          () => {},
          () => {}
        );
      }

      if (cancelledRef.current) break;
      processEvent(
        { stage: stageDef.id, status: "done", text: mockText },
        currentOutputs,
        () => {},
        () => {}
      );

      // Brief pause between stages
      await new Promise((r) => setTimeout(r, 400));
    }

    if (!cancelledRef.current) {
      setDone(true);
      setRunning(false);
      setLiveText("");
    }
  }, [processEvent]);

  // ── Real API run ──────────────────────────────────────────────────────────
  const runReal = useCallback(async () => {
    abortRef.current = new AbortController();
    const currentOutputs: Record<string, string> = {};

    let finished = false;
    const onDone = () => {
      finished = true;
      setDone(true);
      setRunning(false);
      setLiveText("");
    };
    const onError = (msg: string) => {
      finished = true;
      setError(msg);
      setRunning(false);
      setLiveText("");
    };

    try {
      const response = await fetch("/api/audit", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: DEFAULT_PROMPT }),
        signal: abortRef.current.signal,
      });

      if (!response.ok) {
        const msg = await response.text();
        throw new Error(msg || `Server error: ${response.status}`);
      }

      const reader = response.body!.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (!finished) {
        const { done: streamDone, value } = await reader.read();
        if (streamDone) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() ?? "";

        for (const line of lines) {
          if (!line.startsWith("data: ")) continue;
          const raw = line.slice(6).trim();
          if (!raw) continue;
          try {
            const ev = JSON.parse(raw);
            processEvent(ev, currentOutputs, onDone, onError);
          } catch {
            // ignore malformed lines
          }
        }
      }
    } catch (err: unknown) {
      if (err instanceof Error && err.name !== "AbortError") {
        setError(err.message);
      }
      setRunning(false);
      setLiveText("");
    }
  }, [processEvent]);

  // ── Submit handler ────────────────────────────────────────────────────────
  const handleSubmit = useCallback(
    async (mock = false) => {
      if (running) return;
      setRunning(true);
      setDone(false);
      setError(null);
      setLiveText("");
      setExpandedStage(null);
      setStages(makeStages());
      if (mock) {
        await runMock();
      } else {
        await runReal();
      }
    },
    [running, runMock, runReal]
  );

  const handleStop = () => {
    cancelledRef.current = true;
    abortRef.current?.abort();
    setRunning(false);
    setLiveText("");
  };

  const hasStarted = running || done || stages.some((s) => s.status !== "idle");
  const activeStageId = stages.find((s) => s.status === "running")?.id;

  return (
    <div className="app">
      {/* Header */}
      <header className="header">
        <div className="header-inner">
          <div className="logo-mark">
            <Shield size={20} />
            <span>AstroKube Audit</span>
          </div>
        </div>
      </header>

      <main className="main">
        {/* Hero */}
        {!hasStarted && (
          <div className="hero">
            <h1 className="hero-title">
              Security audit,
              <br />
              <span className="gradient-text">start to finish.</span>
            </h1>
            <p className="hero-sub">
              Set your targets and launch. Four agents, full pipeline — repo
              scan, cluster inspection, correlation, and reporting.
            </p>
          </div>
        )}

        {/* Targets */}
        <div className={`targets-wrap ${hasStarted ? "targets-compact" : ""}`}>
          {hasStarted ? (
            <div className="targets-chips">
              {githubRepo && (
                <span className="target-chip">
                  <Github size={12} />
                  {githubRepo}
                </span>
              )}
              {clusterUrl && (
                <span className="target-chip">
                  <Globe size={12} />
                  {clusterUrl}
                </span>
              )}
            </div>
          ) : (
            <div className="targets-grid">
              <div className="target-card">
                <div className="target-card-label">
                  <Github size={14} />
                  GitHub Repository
                </div>
                <input
                  className="target-input"
                  type="text"
                  placeholder="owner/repo"
                  value={githubRepo}
                  onChange={(e) => setGithubRepo(e.target.value)}
                  spellCheck={false}
                />
                <span className="target-hint">e.g. astrokube/hackathon-1-team-2</span>
              </div>
              <div className="target-card">
                <div className="target-card-label">
                  <Globe size={14} />
                  Cluster URL
                </div>
                <input
                  className="target-input"
                  type="text"
                  placeholder="https://k8s.example.com"
                  value={clusterUrl}
                  onChange={(e) => setClusterUrl(e.target.value)}
                  spellCheck={false}
                />
                <span className="target-hint">e.g. https://api.cluster.internal</span>
              </div>
            </div>
          )}
        </div>

        {/* Run / Stop button */}
        {!hasStarted ? (
          <div className="run-wrap">
            <button
              className="btn-run-big"
              onClick={() => handleSubmit(false)}
              disabled={running}
            >
              <Play size={18} fill="currentColor" />
              Run audit
            </button>
            <button
              className="btn-preview-big"
              onClick={() => handleSubmit(true)}
              disabled={running}
              title="Run with mock data"
            >
              Preview with mock data
            </button>
          </div>
        ) : running ? (
          <div className="run-wrap run-wrap-compact">
            <button className="btn-stop-big" onClick={handleStop}>
              <Square size={14} fill="currentColor" />
              Stop
            </button>
          </div>
        ) : null}

        {/* Error banner */}
        {error && (
          <div className="error-banner">
            <XCircle size={16} />
            {error}
          </div>
        )}

        {/* Pipeline cards */}
        {hasStarted && (
          <div className="pipeline">
            {stages.map((stage, idx) => {
              const isExpanded = expandedStage === stage.id;
              const isActive = stage.status === "running";

              return (
                <div key={stage.id} className={`stage-card ${stage.status}`}>
                  <div
                    className="stage-header"
                    onClick={() =>
                      stage.output
                        ? setExpandedStage(isExpanded ? null : stage.id)
                        : undefined
                    }
                  >
                    <div className="stage-left">
                      <div className={`stage-step ${stage.status}`}>
                        {idx + 1}
                      </div>
                      <div className="stage-icon">{stage.icon}</div>
                      <div className="stage-meta">
                        <span className="stage-label">{stage.label}</span>
                        <span className="stage-desc">{stage.description}</span>
                      </div>
                    </div>
                    <div className="stage-right">
                      <StatusBadge status={stage.status} />
                      {stage.output && (
                        <span className="expand-btn">
                          {isExpanded ? (
                            <ChevronUp size={15} />
                          ) : (
                            <ChevronDown size={15} />
                          )}
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Live streaming preview */}
                  {isActive && liveText && activeStageId === stage.id && (
                    <div className="live-preview">
                      <span className="live-text">{liveText}</span>
                      <span className="cursor-blink" />
                    </div>
                  )}

                  {/* Expanded markdown output */}
                  {isExpanded && stage.output && (
                    <div className="stage-output">
                      <ReactMarkdown remarkPlugins={[remarkGfm]}>
                        {stage.output}
                      </ReactMarkdown>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        {/* Done banner */}
        {done && (
          <div className="done-banner">
            <CheckCircle2 size={18} />
            Audit complete — GitHub Issues, PR, and Slack notification sent.
          </div>
        )}
      </main>
    </div>
  );
}

function StatusBadge({ status }: { status: StageStatus }) {
  switch (status) {
    case "running":
      return (
        <span className="status-badge running">
          <Loader2 size={13} className="spin" /> Running
        </span>
      );
    case "done":
      return (
        <span className="status-badge done">
          <CheckCircle2 size={13} /> Done
        </span>
      );
    case "error":
      return (
        <span className="status-badge error">
          <XCircle size={13} /> Error
        </span>
      );
    default:
      return <span className="status-badge idle">Waiting</span>;
  }
}
