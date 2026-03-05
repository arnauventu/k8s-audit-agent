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

	// Verify K8s connectivity early (required by PlatformChecker)
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

	repoCheckerAgent, err := agents.NewRepoCheckerRoot(repoCheckerModel)
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

	agentLoader, err := agent.NewMultiLoader(
		repoCheckerAgent,
		platformCheckerAgent,
		correlatorAgent,
		reporterAgent,
	)
	if err != nil {
		log.Fatalf("Failed to create agent loader: %v", err)
	}

	debugPlugin := loggingplugin.MustNew("debug")

	cfg := &launcher.Config{
		AgentLoader: agentLoader,
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{debugPlugin},
		},
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
