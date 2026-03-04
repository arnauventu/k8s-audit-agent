package tools

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astrokube/hackathon-1-samples/k8s"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

func ListEvents() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_events",
		Description: "List cluster events, focusing on Warning events and high-count events.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		events, err := client.CoreV1().Events(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		warnings := 0
		for _, e := range events.Items {
			if e.Type == "Warning" {
				warnings++
				hasIssue := false
				if e.Count > 5 {
					issues = append(issues, fmt.Sprintf("High-count warning: %s/%s %s - %s (count: %d)", e.Namespace, e.InvolvedObject.Name, e.Reason, e.Message, e.Count))
					hasIssue = true
				}
				if !args.IssuesOnly || hasIssue {
					items = append(items, Item{
						Name:      fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
						Namespace: e.Namespace,
						Status:    e.Reason,
						Details:   fmt.Sprintf("[Warning] %s (count: %d)", e.Message, e.Count),
					})
				}
			}
		}
		summary := fmt.Sprintf("Found %d events total, %d warnings", len(events.Items), warnings)
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d high-count warning events out of %d warnings (%d total events)", len(items), warnings, len(events.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListRecentEvents() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_recent_events",
		Description: "List events from the last hour, focusing on recent failures.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		events, err := client.CoreV1().Events(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		cutoff := time.Now().Add(-1 * time.Hour)
		items := []Item{}
		issues := []string{}
		total := 0
		for _, e := range events.Items {
			eventTime := e.LastTimestamp.Time
			if eventTime.IsZero() {
				eventTime = e.CreationTimestamp.Time
			}
			if eventTime.Before(cutoff) {
				continue
			}
			total++
			hasIssue := false
			if e.Type == "Warning" {
				issues = append(issues, fmt.Sprintf("Recent warning: %s/%s %s - %s", e.Namespace, e.InvolvedObject.Name, e.Reason, e.Message))
				hasIssue = true
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
					Namespace: e.Namespace,
					Status:    fmt.Sprintf("%s [%s]", e.Reason, e.Type),
					Details:   fmt.Sprintf("%s (count: %d, last: %s)", e.Message, e.Count, eventTime.Format(time.RFC3339)),
				})
			}
		}
		summary := fmt.Sprintf("Found %d events in the last hour", total)
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d warning events out of %d in the last hour", len(items), total)
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}
