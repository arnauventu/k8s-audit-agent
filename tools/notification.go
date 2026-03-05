package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ── send_slack_message ────────────────────────────────────────────────────────

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

// ── send_slack_file ───────────────────────────────────────────────────────────

type SendSlackFileArgs struct {
	FilePath       string `json:"file_path" description:"Absolute or relative path to the file to upload (e.g. reports/audit-2026-03-05.pdf)"`
	Title          string `json:"title" description:"Title shown in Slack above the file"`
	InitialComment string `json:"initial_comment" description:"Optional message posted alongside the file"`
}

// SendSlackFile uploads a file (e.g. a PDF) to a Slack channel using the
// Files API v2. Requires SLACK_BOT_TOKEN and SLACK_CHANNEL_ID env vars.
func SendSlackFile() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "send_slack_file",
		Description: "Upload a file (PDF, markdown, etc.) to a Slack channel. Requires SLACK_BOT_TOKEN (bot OAuth token) and SLACK_CHANNEL_ID env vars.",
	}, func(ctx tool.Context, args SendSlackFileArgs) (Result, error) {
		token := os.Getenv("SLACK_BOT_TOKEN")
		if token == "" {
			return Result{}, fmt.Errorf("SLACK_BOT_TOKEN environment variable is required")
		}
		channelID := os.Getenv("SLACK_CHANNEL_ID")
		if channelID == "" {
			return Result{}, fmt.Errorf("SLACK_CHANNEL_ID environment variable is required")
		}

		data, err := os.ReadFile(args.FilePath)
		if err != nil {
			return Result{}, fmt.Errorf("reading file %s: %w", args.FilePath, err)
		}
		fileSize := len(data)
		filename := filepath.Base(args.FilePath)

		uploadURL, fileID, err := slackGetUploadURL(token, filename, fileSize)
		if err != nil {
			return Result{}, fmt.Errorf("getting Slack upload URL: %w", err)
		}
		if err := slackUploadContent(uploadURL, data); err != nil {
			return Result{}, fmt.Errorf("uploading file content to Slack: %w", err)
		}
		if err := slackCompleteUpload(token, channelID, fileID, args.Title, args.InitialComment); err != nil {
			return Result{}, fmt.Errorf("completing Slack file upload: %w", err)
		}

		return Result{
			Summary: fmt.Sprintf("File %q uploaded to Slack channel %s", filename, channelID),
			Items:   []Item{{Name: filename, Status: "uploaded", Details: args.FilePath}},
		}, nil
	})
	return t
}

// ── send_report_to_slack ──────────────────────────────────────────────────────

type SendReportToSlackArgs struct {
	PDFPath         string `json:"pdf_path" description:"Path to the PDF report file to upload"`
	MarkdownContent string `json:"markdown_content" description:"Full markdown content of the report to send as a code block"`
}

// SendReportToSlack sends the three Slack messages in guaranteed order:
// 1. Greeting text, 2. PDF file upload, 3. Markdown as code block.
func SendReportToSlack() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "send_report_to_slack",
		Description: "Send the audit report to Slack in three steps: greeting message, PDF upload, then markdown as a code block. Order is guaranteed.",
	}, func(ctx tool.Context, args SendReportToSlackArgs) (Result, error) {
		webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
		if webhookURL == "" {
			return Result{}, fmt.Errorf("SLACK_WEBHOOK_URL environment variable is required")
		}
		token := os.Getenv("SLACK_BOT_TOKEN")
		if token == "" {
			return Result{}, fmt.Errorf("SLACK_BOT_TOKEN environment variable is required")
		}
		channelID := os.Getenv("SLACK_CHANNEL_ID")
		if channelID == "" {
			return Result{}, fmt.Errorf("SLACK_CHANNEL_ID environment variable is required")
		}

		// Step 1: greeting
		if err := slackPostText(webhookURL, "Hello, I'm the AstroReporter, here is your report"); err != nil {
			return Result{}, fmt.Errorf("sending greeting: %w", err)
		}

		// Step 2: PDF upload
		data, err := os.ReadFile(args.PDFPath)
		if err != nil {
			return Result{}, fmt.Errorf("reading PDF %s: %w", args.PDFPath, err)
		}
		filename := filepath.Base(args.PDFPath)
		uploadURL, fileID, err := slackGetUploadURL(token, filename, len(data))
		if err != nil {
			return Result{}, fmt.Errorf("getting upload URL: %w", err)
		}
		if err := slackUploadContent(uploadURL, data); err != nil {
			return Result{}, fmt.Errorf("uploading PDF: %w", err)
		}
		if err := slackCompleteUpload(token, channelID, fileID, filename, ""); err != nil {
			return Result{}, fmt.Errorf("completing PDF upload: %w", err)
		}

		// Step 3: markdown as code block
		codeBlock := "```\n" + args.MarkdownContent + "\n```"
		if err := slackPostText(webhookURL, codeBlock); err != nil {
			return Result{}, fmt.Errorf("sending markdown block: %w", err)
		}

		return Result{
			Summary: "Report sent to Slack: greeting + PDF + markdown code block",
			Items: []Item{
				{Name: "greeting", Status: "sent"},
				{Name: filename, Status: "uploaded"},
				{Name: "markdown", Status: "sent"},
			},
		}, nil
	})
	return t
}

// ── internal helpers ──────────────────────────────────────────────────────────

func slackPostText(webhookURL, text string) error {
	payload, _ := json.Marshal(map[string]string{"text": text})
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

func slackGetUploadURL(token, filename string, length int) (string, string, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://slack.com/api/files.getUploadURLExternal", nil)
	q := req.URL.Query()
	q.Set("filename", filename)
	q.Set("length", fmt.Sprintf("%d", length))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result struct {
		OK        bool   `json:"ok"`
		UploadURL string `json:"upload_url"`
		FileID    string `json:"file_id"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	if !result.OK {
		return "", "", fmt.Errorf("files.getUploadURLExternal: %s", result.Error)
	}
	return result.UploadURL, result.FileID, nil
}

func slackUploadContent(uploadURL string, data []byte) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fw, err := w.CreateFormFile("file", "file")
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	w.Close()

	req, err := http.NewRequest(http.MethodPost, uploadURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

func slackCompleteUpload(token, channelID, fileID, title, comment string) error {
	filesEntry := []map[string]string{{"id": fileID, "title": title}}
	filesJSON, _ := json.Marshal(filesEntry)

	payload, _ := json.Marshal(map[string]string{
		"files":           string(filesJSON),
		"channel_id":      channelID,
		"initial_comment": comment,
	})

	req, err := http.NewRequest(http.MethodPost,
		"https://slack.com/api/files.completeUploadExternal",
		bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("files.completeUploadExternal: %s", result.Error)
	}
	return nil
}
