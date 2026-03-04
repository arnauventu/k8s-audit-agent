package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astrokube/hackathon-1-samples/k8s"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

func ListNodes() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_nodes",
		Description: "List cluster nodes, detecting NotReady status and pressure conditions.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		nodes, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, node := range nodes.Items {
			ready := "Unknown"
			conditions := []string{}
			for _, c := range node.Status.Conditions {
				if c.Type == "Ready" {
					if c.Status == "True" {
						ready = "Ready"
					} else {
						ready = "NotReady"
						issues = append(issues, fmt.Sprintf("Node %s is NotReady: %s", node.Name, c.Message))
					}
				}
				if c.Type == "MemoryPressure" && c.Status == "True" {
					conditions = append(conditions, "MemoryPressure")
					issues = append(issues, fmt.Sprintf("Node %s has MemoryPressure", node.Name))
				}
				if c.Type == "DiskPressure" && c.Status == "True" {
					conditions = append(conditions, "DiskPressure")
					issues = append(issues, fmt.Sprintf("Node %s has DiskPressure", node.Name))
				}
				if c.Type == "PIDPressure" && c.Status == "True" {
					conditions = append(conditions, "PIDPressure")
					issues = append(issues, fmt.Sprintf("Node %s has PIDPressure", node.Name))
				}
			}

			details := fmt.Sprintf("Kubelet: %s", node.Status.NodeInfo.KubeletVersion)
			if len(conditions) > 0 {
				details += fmt.Sprintf(", Conditions: %v", conditions)
			}
			items = append(items, Item{
				Name:    node.Name,
				Status:  ready,
				Details: details,
			})
		}
		return Result{
			Summary: fmt.Sprintf("Found %d nodes", len(nodes.Items)),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func GetNodeResourceUsage() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_node_resource_usage",
		Description: "Get node CPU and memory usage from metrics-server, detecting >80% utilization.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		mc := k8s.MetricsClient()
		if mc == nil {
			return Result{
				Summary: "Metrics server not available",
				Items:   []Item{},
				Issues:  []string{"Cannot retrieve node resource usage: metrics-server not configured or unreachable"},
			}, nil
		}

		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		nodeMetrics, err := mc.MetricsV1beta1().NodeMetricses().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{
				Summary: "Failed to query metrics-server",
				Items:   []Item{},
				Issues:  []string{fmt.Sprintf("Metrics query failed: %v", err)},
			}, nil
		}

		nodes, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		allocatable := make(map[string][2]int64) // node -> [cpu_milli, mem_bytes]
		for _, node := range nodes.Items {
			cpu := node.Status.Allocatable.Cpu().MilliValue()
			mem := node.Status.Allocatable.Memory().Value()
			allocatable[node.Name] = [2]int64{cpu, mem}
		}

		items := []Item{}
		issues := []string{}
		for _, nm := range nodeMetrics.Items {
			cpuUsed := nm.Usage.Cpu().MilliValue()
			memUsed := nm.Usage.Memory().Value()

			alloc, ok := allocatable[nm.Name]
			if !ok {
				continue
			}

			cpuPct := float64(cpuUsed) / float64(alloc[0]) * 100
			memPct := float64(memUsed) / float64(alloc[1]) * 100

			items = append(items, Item{
				Name:    nm.Name,
				Status:  fmt.Sprintf("CPU: %.1f%%, Memory: %.1f%%", cpuPct, memPct),
				Details: fmt.Sprintf("CPU: %dm/%dm, Memory: %dMi/%dMi", cpuUsed, alloc[0], memUsed/1024/1024, alloc[1]/1024/1024),
			})
			if cpuPct > 80 {
				issues = append(issues, fmt.Sprintf("Node %s CPU usage %.1f%% > 80%%", nm.Name, cpuPct))
			}
			if memPct > 80 {
				issues = append(issues, fmt.Sprintf("Node %s memory usage %.1f%% > 80%%", nm.Name, memPct))
			}
		}
		return Result{
			Summary: fmt.Sprintf("Resource usage for %d nodes", len(nodeMetrics.Items)),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func GetPodResourceUsage() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_pod_resource_usage",
		Description: "Get pod CPU and memory usage, detecting pods without resource limits.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		mc := k8s.MetricsClient()
		if mc == nil {
			return Result{
				Summary: "Metrics server not available",
				Items:   []Item{},
				Issues:  []string{"Cannot retrieve pod resource usage: metrics-server not configured or unreachable"},
			}, nil
		}

		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		podMetrics, err := mc.MetricsV1beta1().PodMetricses(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{
				Summary: "Failed to query pod metrics",
				Items:   []Item{},
				Issues:  []string{fmt.Sprintf("Metrics query failed: %v", err)},
			}, nil
		}

		pods, err := client.CoreV1().Pods(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		// Build map of pod limits
		type limits struct {
			cpuLimit int64
			memLimit int64
		}
		podLimits := make(map[string]limits)
		for _, pod := range pods.Items {
			key := pod.Namespace + "/" + pod.Name
			var l limits
			for _, c := range pod.Spec.Containers {
				if c.Resources.Limits.Cpu() != nil {
					l.cpuLimit += c.Resources.Limits.Cpu().MilliValue()
				}
				if c.Resources.Limits.Memory() != nil {
					l.memLimit += c.Resources.Limits.Memory().Value()
				}
			}
			podLimits[key] = l
		}

		items := []Item{}
		issues := []string{}
		for _, pm := range podMetrics.Items {
			hasIssue := false
			key := pm.Namespace + "/" + pm.Name
			var cpuUsed, memUsed int64
			for _, c := range pm.Containers {
				cpuUsed += c.Usage.Cpu().MilliValue()
				memUsed += c.Usage.Memory().Value()
			}

			l := podLimits[key]
			status := fmt.Sprintf("CPU: %dm, Memory: %dMi", cpuUsed, memUsed/1024/1024)
			if l.cpuLimit == 0 && l.memLimit == 0 {
				issues = append(issues, fmt.Sprintf("Pod %s has no resource limits set", key))
				hasIssue = true
			}

			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      pm.Name,
					Namespace: pm.Namespace,
					Status:    status,
				})
			}
		}
		summary := fmt.Sprintf("Resource usage for %d pods", len(podMetrics.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d pods without resource limits out of %d", len(items), len(podMetrics.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}
