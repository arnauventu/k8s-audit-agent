package tools

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astrokube/hackathon-1-samples/k8s"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

func AnalyzePodSecurity() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "analyze_pod_security",
		Description: "Analyze pod security contexts, detecting privileged containers, root users, hostNetwork, and dangerous capabilities.",
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
		for _, pod := range pods.Items {
			podIssues := []string{}

			if pod.Spec.HostNetwork {
				podIssues = append(podIssues, "hostNetwork=true")
			}
			if pod.Spec.HostPID {
				podIssues = append(podIssues, "hostPID=true")
			}

			for _, c := range pod.Spec.Containers {
				sc := c.SecurityContext
				if sc == nil {
					continue
				}
				if sc.Privileged != nil && *sc.Privileged {
					podIssues = append(podIssues, fmt.Sprintf("container %s is privileged", c.Name))
				}
				if sc.RunAsUser != nil && *sc.RunAsUser == 0 {
					podIssues = append(podIssues, fmt.Sprintf("container %s runs as root (UID 0)", c.Name))
				}
				if sc.Capabilities != nil {
					for _, cap := range sc.Capabilities.Add {
						capStr := string(cap)
						if capStr == "SYS_ADMIN" || capStr == "NET_ADMIN" || capStr == "ALL" {
							podIssues = append(podIssues, fmt.Sprintf("container %s has dangerous capability: %s", c.Name, capStr))
						}
					}
				}
			}

			if len(podIssues) > 0 {
				items = append(items, Item{
					Name:      pod.Name,
					Namespace: pod.Namespace,
					Status:    "Security concerns",
					Details:   strings.Join(podIssues, "; "),
				})
				for _, issue := range podIssues {
					issues = append(issues, fmt.Sprintf("Pod %s/%s: %s", pod.Namespace, pod.Name, issue))
				}
			}
		}
		return Result{
			Summary: fmt.Sprintf("Analyzed %d pods, %d have security concerns", len(pods.Items), len(items)),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListPodSecurityStandards() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_pod_security_standards",
		Description: "List namespace Pod Security Admission (PSA) enforcement levels.",
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
		issues := []string{}
		for _, ns := range nss.Items {
			enforce := ns.Labels["pod-security.kubernetes.io/enforce"]
			audit := ns.Labels["pod-security.kubernetes.io/audit"]
			warn := ns.Labels["pod-security.kubernetes.io/warn"]

			if enforce == "" && audit == "" && warn == "" {
				issues = append(issues, fmt.Sprintf("Namespace %s has no PSA labels configured", ns.Name))
				items = append(items, Item{
					Name:   ns.Name,
					Status: "No PSA",
				})
			} else {
				items = append(items, Item{
					Name:    ns.Name,
					Status:  fmt.Sprintf("enforce=%s", enforce),
					Details: fmt.Sprintf("audit=%s, warn=%s", audit, warn),
				})
			}
		}
		return Result{
			Summary: fmt.Sprintf("Checked %d namespaces for PSA labels", len(nss.Items)),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}
