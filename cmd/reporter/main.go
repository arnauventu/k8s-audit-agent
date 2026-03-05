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
	if os.Getenv("SLACK_WEBHOOK_URL") == "" {
		log.Fatal("SLACK_WEBHOOK_URL environment variable is required")
	}
	if os.Getenv("SLACK_BOT_TOKEN") == "" {
		log.Fatal("SLACK_BOT_TOKEN environment variable is required (needed for file uploads)")
	}
	if os.Getenv("SLACK_CHANNEL_ID") == "" {
		log.Fatal("SLACK_CHANNEL_ID environment variable is required (needed for file uploads)")
	}

	model, err := gemini.NewModel(ctx, config.ModelName("reporter"), &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to create reporter model: %v", err)
	}

	reporterAgent, err := agents.NewReporterRoot(model)
	if err != nil {
		log.Fatalf("Failed to create reporter agent: %v", err)
	}

	debugPlugin := loggingplugin.MustNew("debug")

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(reporterAgent),
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{debugPlugin},
		},
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
