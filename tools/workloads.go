package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astrokube/hackathon-1-samples/k8s"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// --- Common types ---

type NamespaceArgs struct {
	Namespace  string `json:"namespace" description:"Kubernetes namespace (empty for all namespaces)"`
	IssuesOnly bool   `json:"issues_only" description:"When true, return only resources that have detected issues"`
}

type PodIdentArgs struct {
	Namespace string `json:"namespace" description:"Kubernetes namespace"`
	Name      string `json:"name" description:"Pod name"`
}

type Result struct {
	Summary string   `json:"summary"`
	Items   []Item   `json:"items"`
	Issues  []string `json:"issues"`
}

func (r Result) MarshalJSON() ([]byte, error) {
	if r.Items == nil {
		r.Items = []Item{}
	}
	if r.Issues == nil {
		r.Issues = []string{}
	}
	type Alias Result
	return json.Marshal(Alias(r))
}

type Item struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Status    string `json:"status,omitempty"`
	Details   string `json:"details,omitempty"`
}

// --- Workload tools ---

func ListNamespaces() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_namespaces",
		Description: "List all namespaces in the cluster with their status and labels.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		nss, err := client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		for _, ns := range nss.Items {
			labels := []string{}
			for k, v := range ns.Labels {
				labels = append(labels, fmt.Sprintf("%s=%s", k, v))
			}
			items = append(items, Item{
				Name:    ns.Name,
				Status:  string(ns.Status.Phase),
				Details: strings.Join(labels, ", "),
			})
		}
		return Result{
			Summary: fmt.Sprintf("Found %d namespaces", len(nss.Items)),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func ListPods() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_pods",
		Description: "List pods in a namespace, detecting CrashLoopBackOff, Pending, and high restart counts.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		pods, err := client.CoreV1().Pods(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, p := range pods.Items {
			hasIssue := false
			status := string(p.Status.Phase)
			details := []string{}
			for _, cs := range p.Status.ContainerStatuses {
				if cs.RestartCount > 5 {
					details = append(details, fmt.Sprintf("container %s: %d restarts", cs.Name, cs.RestartCount))
					issues = append(issues, fmt.Sprintf("Pod %s/%s container %s has %d restarts", p.Namespace, p.Name, cs.Name, cs.RestartCount))
					hasIssue = true
				}
				if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
					status = cs.State.Waiting.Reason
					if cs.State.Waiting.Reason == "CrashLoopBackOff" || cs.State.Waiting.Reason == "ImagePullBackOff" {
						issues = append(issues, fmt.Sprintf("Pod %s/%s: %s", p.Namespace, p.Name, cs.State.Waiting.Reason))
						hasIssue = true
					}
				}
			}
			if p.Status.Phase == "Pending" {
				issues = append(issues, fmt.Sprintf("Pod %s/%s is Pending", p.Namespace, p.Name))
				hasIssue = true
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      p.Name,
					Namespace: p.Namespace,
					Status:    status,
					Details:   strings.Join(details, "; "),
				})
			}
		}
		ns := args.Namespace
		if ns == "" {
			ns = "all namespaces"
		}
		summary := fmt.Sprintf("Found %d pods in %s", len(pods.Items), ns)
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d pods with issues out of %d in %s", len(items), len(pods.Items), ns)
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func DescribePod() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "describe_pod",
		Description: "Get detailed info about a specific pod including its events.",
	}, func(ctx tool.Context, args PodIdentArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		pod, err := client.CoreV1().Pods(args.Namespace).Get(context.Background(), args.Name, metav1.GetOptions{})
		if err != nil {
			return Result{}, err
		}

		details := []string{}
		details = append(details, fmt.Sprintf("Phase: %s", pod.Status.Phase))
		details = append(details, fmt.Sprintf("Node: %s", pod.Spec.NodeName))
		for _, c := range pod.Status.ContainerStatuses {
			details = append(details, fmt.Sprintf("Container %s: ready=%v restarts=%d", c.Name, c.Ready, c.RestartCount))
		}

		events, _ := client.CoreV1().Events(args.Namespace).List(context.Background(), metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=Pod", args.Name),
		})

		issues := []string{}
		eventItems := []Item{}
		if events != nil {
			for _, e := range events.Items {
				eventItems = append(eventItems, Item{
					Name:    e.Reason,
					Details: fmt.Sprintf("[%s] %s (count: %d)", e.Type, e.Message, e.Count),
				})
				if e.Type == "Warning" {
					issues = append(issues, fmt.Sprintf("%s: %s", e.Reason, e.Message))
				}
			}
		}

		return Result{
			Summary: fmt.Sprintf("Pod %s/%s - %s\n%s", args.Namespace, args.Name, pod.Status.Phase, strings.Join(details, "\n")),
			Items:   eventItems,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListDeployments() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_deployments",
		Description: "List deployments, detecting unavailable replicas and stalled rollouts.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		deps, err := client.AppsV1().Deployments(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, d := range deps.Items {
			hasIssue := false
			status := fmt.Sprintf("%d/%d ready", d.Status.ReadyReplicas, *d.Spec.Replicas)
			if d.Status.UnavailableReplicas > 0 {
				issues = append(issues, fmt.Sprintf("Deployment %s/%s has %d unavailable replicas", d.Namespace, d.Name, d.Status.UnavailableReplicas))
				hasIssue = true
			}
			for _, c := range d.Status.Conditions {
				if c.Type == "Progressing" && c.Reason == "ProgressDeadlineExceeded" {
					issues = append(issues, fmt.Sprintf("Deployment %s/%s rollout stalled: %s", d.Namespace, d.Name, c.Message))
					hasIssue = true
				}
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      d.Name,
					Namespace: d.Namespace,
					Status:    status,
				})
			}
		}
		summary := fmt.Sprintf("Found %d deployments", len(deps.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d deployments with issues out of %d", len(items), len(deps.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListDaemonSets() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_daemonsets",
		Description: "List daemonsets, detecting nodes with unavailable pods.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		dss, err := client.AppsV1().DaemonSets(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, ds := range dss.Items {
			hasIssue := false
			status := fmt.Sprintf("%d/%d ready", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
			if ds.Status.NumberUnavailable > 0 {
				issues = append(issues, fmt.Sprintf("DaemonSet %s/%s has %d unavailable on nodes", ds.Namespace, ds.Name, ds.Status.NumberUnavailable))
				hasIssue = true
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{Name: ds.Name, Namespace: ds.Namespace, Status: status})
			}
		}
		summary := fmt.Sprintf("Found %d daemonsets", len(dss.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d daemonsets with issues out of %d", len(items), len(dss.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListStatefulSets() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_statefulsets",
		Description: "List statefulsets, detecting replica mismatches.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		sss, err := client.AppsV1().StatefulSets(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, ss := range sss.Items {
			hasIssue := false
			status := fmt.Sprintf("%d/%d ready", ss.Status.ReadyReplicas, *ss.Spec.Replicas)
			if ss.Status.ReadyReplicas != *ss.Spec.Replicas {
				issues = append(issues, fmt.Sprintf("StatefulSet %s/%s: %d/%d replicas ready", ss.Namespace, ss.Name, ss.Status.ReadyReplicas, *ss.Spec.Replicas))
				hasIssue = true
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{Name: ss.Name, Namespace: ss.Namespace, Status: status})
			}
		}
		summary := fmt.Sprintf("Found %d statefulsets", len(sss.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d statefulsets with issues out of %d", len(items), len(sss.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListJobs() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_jobs",
		Description: "List jobs and cronjobs, detecting failed or long-running jobs.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		jobs, err := client.BatchV1().Jobs(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, j := range jobs.Items {
			hasIssue := false
			status := "Running"
			for _, c := range j.Status.Conditions {
				if c.Type == "Failed" && c.Status == "True" {
					status = "Failed"
					issues = append(issues, fmt.Sprintf("Job %s/%s failed: %s", j.Namespace, j.Name, c.Message))
					hasIssue = true
				}
				if c.Type == "Complete" && c.Status == "True" {
					status = "Complete"
				}
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{Name: j.Name, Namespace: j.Namespace, Status: status})
			}
		}

		cronCount := 0
		crons, err := client.BatchV1().CronJobs(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err == nil {
			cronCount = len(crons.Items)
			for _, cj := range crons.Items {
				suspended := ""
				if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
					suspended = " (suspended)"
				}
				if !args.IssuesOnly {
					items = append(items, Item{
						Name:      cj.Name,
						Namespace: cj.Namespace,
						Status:    fmt.Sprintf("CronJob: %s%s", cj.Spec.Schedule, suspended),
					})
				}
			}
		}
		summary := fmt.Sprintf("Found %d jobs, %d cronjobs", len(jobs.Items), cronCount)
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d jobs with issues out of %d jobs, %d cronjobs", len(items), len(jobs.Items), cronCount)
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}
