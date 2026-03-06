package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/astrokube/hackathon-1-samples/agents"
	"github.com/astrokube/hackathon-1-samples/config"
	"github.com/astrokube/hackathon-1-samples/frontend"
	"github.com/astrokube/hackathon-1-samples/k8s"
)

type auditRequest struct {
	Prompt string `json:"prompt"`
}

type sseEvent struct {
	Stage  string `json:"stage"`
	Status string `json:"status"`
	Text   string `json:"text,omitempty"`
}

func writeSSE(w http.ResponseWriter, ev sseEvent) {
	data, _ := json.Marshal(ev)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// runStage runs a single agent, streams its tokens to the SSE connection,
// and returns the full accumulated output text.
func runStage(ctx context.Context, ag agent.Agent, svc session.Service, userID, appName, stage, prompt string, w http.ResponseWriter) (string, bool) {
	writeSSE(w, sseEvent{Stage: stage, Status: "started"})
	log.Printf("[%s] starting", stage)

	sessID := fmt.Sprintf("sess-%d-%s", time.Now().UnixNano(), stage)
	if _, err := svc.Create(ctx, &session.CreateRequest{
		AppName:   appName,
		UserID:    userID,
		SessionID: sessID,
	}); err != nil {
		writeSSE(w, sseEvent{Stage: stage, Status: "error", Text: err.Error()})
		return "", false
	}

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          ag,
		SessionService: svc,
	})
	if err != nil {
		writeSSE(w, sseEvent{Stage: stage, Status: "error", Text: err.Error()})
		return "", false
	}

	msg := &genai.Content{
		Role:  "user",
		Parts: []*genai.Part{{Text: prompt}},
	}

	var sb strings.Builder
	for event, err := range r.Run(ctx, userID, sessID, msg, agent.RunConfig{}) {
		if err != nil {
			writeSSE(w, sseEvent{Stage: stage, Status: "error", Text: err.Error()})
			return sb.String(), false
		}
		if event == nil || event.LLMResponse.Content == nil {
			continue
		}
		for _, part := range event.LLMResponse.Content.Parts {
			if part.Text != "" {
				sb.WriteString(part.Text)
				writeSSE(w, sseEvent{Stage: stage, Status: "token", Text: part.Text})
			}
		}
	}

	output := sb.String()
	log.Printf("[%s] done (%d chars)", stage, len(output))
	writeSSE(w, sseEvent{Stage: stage, Status: "done", Text: output})
	return output, true
}

func handleAudit(
	ctx context.Context,
	repoAgent, platformAgent, correlatorAgent, reporterAgent agent.Agent,
	clientConfig *genai.ClientConfig,
) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body auditRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Prompt == "" {
			http.Error(w, "prompt is required", http.StatusBadRequest)
			return
		}
		log.Printf("[audit] request received: %q", body.Prompt)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		writeSSE(w, sseEvent{Stage: "pipeline", Status: "connected"})

		userID := fmt.Sprintf("user-%d", time.Now().UnixNano())
		appName := "audit-pipeline"

		// Each stage gets its own session service so sessions don't bleed over
		repoSvc := session.InMemoryService()
		platformSvc := session.InMemoryService()
		correlatorSvc := session.InMemoryService()
		reporterSvc := session.InMemoryService()

		// Stage 1: RepoChecker
		repoOut, ok := runStage(ctx, repoAgent, repoSvc, userID, appName,
			"repo_checker", body.Prompt, w)
		if !ok {
			return
		}

		// Stage 2: PlatformChecker — receives user prompt + repo findings
		platformPrompt := body.Prompt + "\n\n--- REPO CHECKER FINDINGS ---\n" + repoOut
		platformOut, ok := runStage(ctx, platformAgent, platformSvc, userID, appName,
			"platform_checker", platformPrompt, w)
		if !ok {
			return
		}

		// Stage 3: Correlator — receives both findings
		correlatorPrompt := "Correlate the following findings and generate the final audit report.\n\n" +
			"--- REPO CHECKER FINDINGS ---\n" + repoOut + "\n\n" +
			"--- PLATFORM CHECKER FINDINGS ---\n" + platformOut
		correlatorOut, ok := runStage(ctx, correlatorAgent, correlatorSvc, userID, appName,
			"correlator", correlatorPrompt, w)
		if !ok {
			return
		}

		// Stage 4: Reporter — receives the final report
		reporterPrompt := "Distribute the following audit report: create GitHub Issues, open a PR, and send a Slack notification.\n\n" +
			"--- AUDIT REPORT ---\n" + correlatorOut
		_, ok = runStage(ctx, reporterAgent, reporterSvc, userID, appName,
			"reporter", reporterPrompt, w)
		if !ok {
			return
		}

		writeSSE(w, sseEvent{Stage: "pipeline", Status: "complete"})
		log.Printf("[audit] pipeline complete")
	}
}

func main() {
	ctx := context.Background()

	if _, err := k8s.Client(); err != nil {
		log.Fatalf("Failed to connect to Kubernetes cluster: %v", err)
	}

	clientConfig := &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	}

	makeModel := func(name string) model.LLM {
		m, err := gemini.NewModel(ctx, config.ModelName(name), clientConfig)
		if err != nil {
			log.Fatalf("Failed to create model %s: %v", name, err)
		}
		return m
	}

	repoAgent, err := agents.NewRepoCheckerRoot(makeModel("repo_checker"), makeModel("repo_checker_sub_agents"), os.Getenv("GITHUB_REPO"))
	if err != nil {
		log.Fatalf("Failed to create repo_checker: %v", err)
	}
	platformAgent, err := agents.NewPlatformCheckerRoot(makeModel("platform_checker"), makeModel("platform_checker_sub_agents"))
	if err != nil {
		log.Fatalf("Failed to create platform_checker: %v", err)
	}
	correlatorAgent, err := agents.NewCorrelatorRoot(makeModel("correlator"))
	if err != nil {
		log.Fatalf("Failed to create correlator: %v", err)
	}
	reporterAgent, err := agents.NewReporterRoot(makeModel("reporter"))
	if err != nil {
		log.Fatalf("Failed to create reporter: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/audit", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		handleAudit(ctx, repoAgent, platformAgent, correlatorAgent, reporterAgent, clientConfig)(w, req)
	})

	// Serve embedded frontend
	sub, err := fs.Sub(frontend.DistFS, "dist")
	if err != nil {
		log.Fatalf("Failed to create frontend sub-filesystem: %v", err)
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			if stat, ok := sub.(fs.StatFS); ok {
				if _, statErr := stat.Stat(strings.TrimPrefix(req.URL.Path, "/")); statErr != nil {
					req.URL.Path = "/"
				}
			}
		}
		fileServer.ServeHTTP(w, req)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
