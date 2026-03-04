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

func ListServices() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_services",
		Description: "List services, detecting services with no endpoints.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		svcs, err := client.CoreV1().Services(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, svc := range svcs.Items {
			hasIssue := false
			svcType := string(svc.Spec.Type)

			if svc.Spec.Type != "ExternalName" && svc.Spec.ClusterIP != "None" {
				ep, epErr := client.CoreV1().Endpoints(svc.Namespace).Get(context.Background(), svc.Name, metav1.GetOptions{})
				if epErr == nil {
					ready := 0
					for _, subset := range ep.Subsets {
						ready += len(subset.Addresses)
					}
					if ready == 0 {
						issues = append(issues, fmt.Sprintf("Service %s/%s has no ready endpoints", svc.Namespace, svc.Name))
						hasIssue = true
					}
				}
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      svc.Name,
					Namespace: svc.Namespace,
					Status:    svcType,
					Details:   fmt.Sprintf("ClusterIP: %s, Ports: %d", svc.Spec.ClusterIP, len(svc.Spec.Ports)),
				})
			}
		}
		summary := fmt.Sprintf("Found %d services", len(svcs.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d services with issues out of %d", len(items), len(svcs.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListEndpoints() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_endpoints",
		Description: "List endpoints, detecting those with zero ready addresses.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		eps, err := client.CoreV1().Endpoints(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, ep := range eps.Items {
			hasIssue := false
			ready := 0
			notReady := 0
			for _, subset := range ep.Subsets {
				ready += len(subset.Addresses)
				notReady += len(subset.NotReadyAddresses)
			}
			if ready == 0 && len(ep.Subsets) > 0 {
				issues = append(issues, fmt.Sprintf("Endpoints %s/%s: zero ready addresses", ep.Namespace, ep.Name))
				hasIssue = true
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      ep.Name,
					Namespace: ep.Namespace,
					Status:    fmt.Sprintf("%d ready, %d not-ready", ready, notReady),
				})
			}
		}
		summary := fmt.Sprintf("Found %d endpoints", len(eps.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d endpoints with issues out of %d", len(items), len(eps.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListIngresses() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_ingresses",
		Description: "List ingresses, detecting those with no address assigned.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		ings, err := client.NetworkingV1().Ingresses(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, ing := range ings.Items {
			hasIssue := false
			hosts := []string{}
			for _, rule := range ing.Spec.Rules {
				hosts = append(hosts, rule.Host)
			}
			hasAddr := len(ing.Status.LoadBalancer.Ingress) > 0
			status := "Active"
			if !hasAddr {
				status = "No address"
				issues = append(issues, fmt.Sprintf("Ingress %s/%s has no address assigned", ing.Namespace, ing.Name))
				hasIssue = true
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      ing.Name,
					Namespace: ing.Namespace,
					Status:    status,
					Details:   strings.Join(hosts, ", "),
				})
			}
		}
		summary := fmt.Sprintf("Found %d ingresses", len(ings.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d ingresses with issues out of %d", len(items), len(ings.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListNetworkPolicies() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_network_policies",
		Description: "List network policies for security audit.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		nps, err := client.NetworkingV1().NetworkPolicies(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		for _, np := range nps.Items {
			policyTypes := []string{}
			for _, pt := range np.Spec.PolicyTypes {
				policyTypes = append(policyTypes, string(pt))
			}
			items = append(items, Item{
				Name:      np.Name,
				Namespace: np.Namespace,
				Details:   fmt.Sprintf("Types: %s", strings.Join(policyTypes, ", ")),
			})
		}
		return Result{
			Summary: fmt.Sprintf("Found %d network policies", len(nps.Items)),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}
