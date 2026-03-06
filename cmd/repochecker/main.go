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
)

func main() {
	ctx := context.Background()

	if os.Getenv("GITHUB_TOKEN") == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}
	if os.Getenv("GITHUB_REPO") == "" {
		log.Fatal("GITHUB_REPO environment variable is required (format: owner/repo)")
	}

	clientConfig := &genai.ClientConfig{APIKey: os.Getenv("GOOGLE_API_KEY")}

	model, err := gemini.NewModel(ctx, config.ModelName("repo_checker"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create repo_checker model: %v", err)
	}
	subModel, err := gemini.NewModel(ctx, config.ModelName("repo_checker_sub_agents"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create repo_checker sub-agent model: %v", err)
	}

	repoCheckerAgent, err := agents.NewRepoCheckerRoot(model, subModel, os.Getenv("GITHUB_REPO"))
	if err != nil {
		log.Fatalf("Failed to create repo_checker agent: %v", err)
	}

	debugPlugin := loggingplugin.MustNew("debug")

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(repoCheckerAgent),
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{debugPlugin},
		},
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
