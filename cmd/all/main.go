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

	// Verify K8s connectivity early
	if _, err := k8s.Client(); err != nil {
		log.Fatalf("Failed to connect to Kubernetes cluster: %v", err)
	}

	clientConfig := &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	}

	// Create a model per agent (each may use a different model via config)
	inspectorModel, err := gemini.NewModel(ctx, config.ModelName("inspector"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create inspector model: %v", err)
	}
	plannerModel, err := gemini.NewModel(ctx, config.ModelName("planner"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create planner model: %v", err)
	}
	executorModel, err := gemini.NewModel(ctx, config.ModelName("executor"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create executor model: %v", err)
	}
	verifierModel, err := gemini.NewModel(ctx, config.ModelName("verifier"), clientConfig)
	if err != nil {
		log.Fatalf("Failed to create verifier model: %v", err)
	}

	// Build all 4 root agents
	inspectorAgent, err := agents.NewInspectorRoot(inspectorModel)
	if err != nil {
		log.Fatalf("Failed to create inspector agent: %v", err)
	}
	plannerAgent, err := agents.NewPlannerRoot(plannerModel)
	if err != nil {
		log.Fatalf("Failed to create planner agent: %v", err)
	}
	executorAgent, err := agents.NewExecutorRoot(executorModel)
	if err != nil {
		log.Fatalf("Failed to create executor agent: %v", err)
	}
	verifierAgent, err := agents.NewVerifierRoot(verifierModel)
	if err != nil {
		log.Fatalf("Failed to create verifier agent: %v", err)
	}

	// Serve all agents via a single launcher with agent selector dropdown
	agentLoader, err := agent.NewMultiLoader(
		inspectorAgent,
		plannerAgent,
		executorAgent,
		verifierAgent,
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
