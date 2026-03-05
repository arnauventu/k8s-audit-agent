package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SendSlackMessageArgs struct {
	Message string `json:"message" description:"Message text to send to Slack (markdown supported)"`
}

func SendSlackMessage() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "send_slack_message",
		Description: "Send a message to a Slack channel via an incoming webhook. The webhook URL is read from the SLACK_WEBHOOK_URL environment variable.",
	}, func(ctx tool.Context, args SendSlackMessageArgs) (Result, error) {
		webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
		if webhookURL == "" {
			return Result{}, fmt.Errorf("SLACK_WEBHOOK_URL environment variable is required")
		}

		payload, err := json.Marshal(map[string]string{"text": args.Message})
		if err != nil {
			return Result{}, fmt.Errorf("failed to marshal Slack payload: %w", err)
		}

		resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(payload))
		if err != nil {
			return Result{}, fmt.Errorf("failed to send Slack message: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return Result{}, fmt.Errorf("Slack webhook returned status %d", resp.StatusCode)
		}

		return Result{
			Summary: "Slack message sent successfully",
			Items:   []Item{{Name: "slack", Status: "sent", Details: args.Message}},
			Issues:  []string{},
		}, nil
	})
	return t
}
