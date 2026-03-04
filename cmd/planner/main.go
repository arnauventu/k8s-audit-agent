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

	if _, err := k8s.Client(); err != nil {
		log.Fatalf("Failed to connect to Kubernetes cluster: %v", err)
	}

	model, err := gemini.NewModel(ctx, config.ModelName("planner"), &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	plannerAgent, err := agents.NewPlannerRoot(model)
	if err != nil {
		log.Fatalf("Failed to create planner agent: %v", err)
	}

	debugPlugin := loggingplugin.MustNew("debug")

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(plannerAgent),
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{debugPlugin},
		},
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
