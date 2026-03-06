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

	clientConfig := &genai.ClientConfig{APIKey: os.Getenv("GOOGLE_API_KEY")}

	model, err := gemini.NewModel(ctx, config.ModelName("platform_checker"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create platform_checker model: %v", err)
	}
	subModel, err := gemini.NewModel(ctx, config.ModelName("platform_checker_sub_agents"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create platform_checker sub-agent model: %v", err)
	}

	platformCheckerAgent, err := agents.NewPlatformCheckerRoot(model, subModel)
	if err != nil {
		log.Fatalf("Failed to create platform_checker agent: %v", err)
	}

	debugPlugin := loggingplugin.MustNew("debug")

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(platformCheckerAgent),
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{debugPlugin},
		},
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
