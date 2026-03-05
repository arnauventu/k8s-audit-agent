package main

import (
	"context"
	"log"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/plugin"
	"google.golang.org/adk/plugin/loggingplugin"
	"google.golang.org/adk/runner"
	"google.golang.org/genai"

	"github.com/astrokube/hackathon-1-samples/agents"
	"github.com/astrokube/hackathon-1-samples/config"
	"github.com/astrokube/hackathon-1-samples/k8s"
)

func main() {
	ctx := context.Background()

	if os.Getenv("GITHUB_TOKEN") == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}
	if os.Getenv("GITHUB_REPO") == "" {
		log.Fatal("GITHUB_REPO environment variable is required (format: owner/repo)")
	}
	if os.Getenv("SLACK_WEBHOOK_URL") == "" {
		log.Fatal("SLACK_WEBHOOK_URL environment variable is required")
	}
	if _, err := k8s.Client(); err != nil {
		log.Fatalf("Failed to connect to Kubernetes cluster: %v", err)
	}

	clientConfig := &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	}

	repoCheckerModel, err := gemini.NewModel(ctx, config.ModelName("repo_checker"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create repo_checker model: %v", err)
	}
	platformCheckerModel, err := gemini.NewModel(ctx, config.ModelName("platform_checker"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create platform_checker model: %v", err)
	}
	correlatorModel, err := gemini.NewModel(ctx, config.ModelName("correlator"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create correlator model: %v", err)
	}
	reporterModel, err := gemini.NewModel(ctx, config.ModelName("reporter"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create reporter model: %v", err)
	}
	orchestratorModel, err := gemini.NewModel(ctx, config.ModelName("orchestrator"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create orchestrator model: %v", err)
	}

	repoCheckerAgent, err := agents.NewRepoCheckerRoot(repoCheckerModel, os.Getenv("GITHUB_REPO"))
	if err != nil {
		log.Fatalf("Failed to create repo_checker agent: %v", err)
	}
	platformCheckerAgent, err := agents.NewPlatformCheckerRoot(platformCheckerModel)
	if err != nil {
		log.Fatalf("Failed to create platform_checker agent: %v", err)
	}
	correlatorAgent, err := agents.NewCorrelatorRoot(correlatorModel)
	if err != nil {
		log.Fatalf("Failed to create correlator agent: %v", err)
	}
	reporterAgent, err := agents.NewReporterRoot(reporterModel)
	if err != nil {
		log.Fatalf("Failed to create reporter agent: %v", err)
	}

	orchestratorAgent, err := agents.NewAuditOrchestratorRoot(
		orchestratorModel,
		repoCheckerAgent,
		platformCheckerAgent,
		correlatorAgent,
		reporterAgent,
	)
	if err != nil {
		log.Fatalf("Failed to create orchestrator agent: %v", err)
	}

	debugPlugin := loggingplugin.MustNew("debug")

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(orchestratorAgent),
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{debugPlugin},
		},
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
