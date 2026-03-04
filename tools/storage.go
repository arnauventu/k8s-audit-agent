package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astrokube/hackathon-1-samples/k8s"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

func ListPersistentVolumes() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_persistent_volumes",
		Description: "List persistent volumes, detecting Released or Failed PVs.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		pvs, err := client.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, pv := range pvs.Items {
			phase := string(pv.Status.Phase)
			items = append(items, Item{
				Name:    pv.Name,
				Status:  phase,
				Details: fmt.Sprintf("Capacity: %s, ReclaimPolicy: %s", pv.Spec.Capacity.Storage().String(), string(pv.Spec.PersistentVolumeReclaimPolicy)),
			})
			if pv.Status.Phase == "Released" || pv.Status.Phase == "Failed" {
				issues = append(issues, fmt.Sprintf("PV %s is %s", pv.Name, phase))
			}
		}
		return Result{
			Summary: fmt.Sprintf("Found %d persistent volumes", len(pvs.Items)),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListPersistentVolumeClaims() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_persistent_volume_claims",
		Description: "List persistent volume claims, detecting Pending PVCs.",
	}, func(ctx tool.Context, args NamespaceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}
		pvcs, err := client.CoreV1().PersistentVolumeClaims(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, err
		}

		items := []Item{}
		issues := []string{}
		for _, pvc := range pvcs.Items {
			hasIssue := false
			phase := string(pvc.Status.Phase)
			if pvc.Status.Phase == "Pending" {
				issues = append(issues, fmt.Sprintf("PVC %s/%s is Pending", pvc.Namespace, pvc.Name))
				hasIssue = true
			}
			if !args.IssuesOnly || hasIssue {
				items = append(items, Item{
					Name:      pvc.Name,
					Namespace: pvc.Namespace,
					Status:    phase,
					Details:   fmt.Sprintf("Volume: %s, StorageClass: %s", pvc.Spec.VolumeName, stringOrDefault(pvc.Spec.StorageClassName)),
				})
			}
		}
		summary := fmt.Sprintf("Found %d PVCs", len(pvcs.Items))
		if args.IssuesOnly {
			summary = fmt.Sprintf("Found %d PVCs with issues out of %d", len(items), len(pvcs.Items))
		}
		return Result{
			Summary: summary,
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func stringOrDefault(s *string) string {
	if s == nil {
		return "<none>"
	}
	return *s
}
