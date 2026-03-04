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

func ListRoles() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_roles",
		Description: "List Roles and ClusterRoles, detecting wildcard permissions.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}

		// ClusterRoles
		crs, err := client.RbacV1().ClusterRoles().List(context.Background(), metav1.ListOptions{})
		if err == nil {
			for _, cr := range crs.Items {
				hasIssue := false
				for _, rule := range cr.Rules {
					for _, verb := range rule.Verbs {
						if verb == "*" {
							issues = append(issues, fmt.Sprintf("ClusterRole %s has wildcard verb permissions", cr.Name))
							hasIssue = true
							break
						}
					}
					for _, res := range rule.Resources {
						if res == "*" {
							issues = append(issues, fmt.Sprintf("ClusterRole %s has wildcard resource permissions", cr.Name))
							hasIssue = true
							break
						}
					}
				}
				if !args.IssuesOnly || hasIssue {
					items = append(items, Item{
						Name:    cr.Name,
						Status:  "ClusterRole",
						Details: fmt.Sprintf("%d rules", len(cr.Rules)),
					})
				}
			}
		}

		// Namespaced Roles
		roles, err := client.RbacV1().Roles(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err == nil {
			for _, r := range roles.Items {
				hasIssue := false
				for _, rule := range r.Rules {
					for _, verb := range rule.Verbs {
						if verb == "*" {
							issues = append(issues, fmt.Sprintf("Role %s/%s has wildcard verb permissions", r.Namespace, r.Name))
							hasIssue = true
							break
						}
					}
				}
				if !args.IssuesOnly || hasIssue {
					items = append(items, Item{
						Name:      r.Name,
						Namespace: r.Namespace,
						Status:    "Role",
						Details:   fmt.Sprintf("%d rules", len(r.Rules)),
					})
				}
			}
		}
		summary := fmt.Sprintf("Found %d roles/clusterroles", len(items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d roles/clusterroles with issues", len(items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListRoleBindings() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_role_bindings",
		Description: "List RoleBindings and ClusterRoleBindings, detecting cluster-admin grants.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}

		crbs, err := client.RbacV1().ClusterRoleBindings().List(context.Background(), metav1.ListOptions{})
		if err == nil {
			for _, crb := range crbs.Items {
				hasIssue := false
				subjects := []string{}
				for _, s := range crb.Subjects {
					subjects = append(subjects, fmt.Sprintf("%s/%s", s.Kind, s.Name))
				}
				if crb.RoleRef.Name == "cluster-admin" {
					issues = append(issues, fmt.Sprintf("ClusterRoleBinding %s grants cluster-admin to: %s", crb.Name, strings.Join(subjects, ", ")))
					hasIssue = true
				}
				if !args.IssuesOnly || hasIssue {
					items = append(items, Item{
						Name:    crb.Name,
						Status:  "ClusterRoleBinding",
						Details: fmt.Sprintf("Role: %s, Subjects: %s", crb.RoleRef.Name, strings.Join(subjects, ", ")),
					})
				}
			}
		}

		rbs, err := client.RbacV1().RoleBindings(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err == nil {
			for _, rb := range rbs.Items {
				subjects := []string{}
				for _, s := range rb.Subjects {
					subjects = append(subjects, fmt.Sprintf("%s/%s", s.Kind, s.Name))
				}
				if !args.IssuesOnly {
					items = append(items, Item{
						Name:      rb.Name,
						Namespace: rb.Namespace,
						Status:    "RoleBinding",
						Details:   fmt.Sprintf("Role: %s, Subjects: %s", rb.RoleRef.Name, strings.Join(subjects, ", ")),
					})
				}
			}
		}
		summary := fmt.Sprintf("Found %d role bindings", len(items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d role bindings with issues", len(items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListServiceAccounts() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_service_accounts",
		Description: "List service accounts, detecting automount token issues.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		sas, err := client.CoreV1().ServiceAccounts(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, sa := range sas.Items {
			hasIssue := false
			automount := "default"
			if sa.AutomountServiceAccountToken != nil {
				if *sa.AutomountServiceAccountToken {
					automount = "enabled"
				} else {
					automount = "disabled"
				}
			}
			if sa.Name != "default" && (sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken) {
				issues = append(issues, fmt.Sprintf("ServiceAccount %s/%s has automount token enabled", sa.Namespace, sa.Name))
				hasIssue = true
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      sa.Name,
					Namespace: sa.Namespace,
					Details:   fmt.Sprintf("Automount: %s, Secrets: %d", automount, len(sa.Secrets)),
				})
			}
		}
		summary := fmt.Sprintf("Found %d service accounts", len(sas.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d service accounts with issues out of %d", len(items), len(sas.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}
