.PHONY: run run-orchestrator build build-frontend clean

# ── Frontend ──────────────────────────────────────────────────────────────────

frontend/node_modules: frontend/package.json frontend/package-lock.json
	cd frontend && npm install

frontend/dist: frontend/node_modules $(shell find frontend/src -type f) frontend/index.html
	cd frontend && npm run build

build-frontend: frontend/dist

# ── Custom server (cmd/server) ────────────────────────────────────────────────
# Builds the frontend first (embedded into the binary), then launches the server.
# No SSE write timeout needed — the custom HTTP server streams without deadline.

run: frontend/dist
	go run ./cmd/server

# ── ADK orchestrator (cmd/orchestrator) ───────────────────────────────────────
# Uses the ADK web launcher. SSE timeout must be generous since multi-agent
# audit pipelines routinely take 5-15 minutes.

SSE_TIMEOUT ?= 30m
PORT        ?= 8080

run-orchestrator:
	go run ./cmd/orchestrator/ web --port $(PORT) api --sse-write-timeout $(SSE_TIMEOUT) webui

# ── Build / clean ─────────────────────────────────────────────────────────────

build: frontend/dist
	go build ./...

clean:
	rm -rf frontend/dist frontend/node_modules
	go clean ./...
