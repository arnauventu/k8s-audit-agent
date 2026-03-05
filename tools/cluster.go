package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astrokube/hackathon-1-samples/k8s"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// --- Arg types ---

type NamespaceOnlyArgs struct {
	Namespace string `json:"namespace" description:"Kubernetes namespace to inspect"`
}

// --- Tools ---

func GetClusterVersion() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_cluster_version",
		Description: "Returns the Kubernetes server version (major, minor, git version). Useful for checking API compatibility.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		version, err := client.Discovery().ServerVersion()
		if err != nil {
			return Result{}, fmt.Errorf("failed to get server version: %w", err)
		}

		details := fmt.Sprintf("Major: %s, Minor: %s, GitVersion: %s, Platform: %s",
			version.Major, version.Minor, version.GitVersion, version.Platform)

		return Result{
			Summary: fmt.Sprintf("Kubernetes %s.%s (%s)", version.Major, version.Minor, version.GitVersion),
			Items: []Item{{
				Name:    "cluster",
				Status:  version.GitVersion,
				Details: details,
			}},
			Issues: []string{},
		}, nil
	})
	return t
}

func ListAPIResources() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_api_resources",
		Description: "Lists all available API groups and versions in the cluster. Use this to verify whether a specific apiVersion (e.g. batch/v1, networking.k8s.io/v1) is available before deploying.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		groups, err := client.Discovery().ServerGroups()
		if err != nil {
			return Result{}, fmt.Errorf("failed to list API groups: %w", err)
		}

		items := make([]Item, 0, len(groups.Groups))
		for _, g := range groups.Groups {
			versions := make([]string, 0, len(g.Versions))
			for _, v := range g.Versions {
				versions = append(versions, v.Version)
			}
			name := g.Name
			if name == "" {
				name = "core"
			}
			items = append(items, Item{
				Name:    name,
				Status:  "available",
				Details: fmt.Sprintf("versions: %s", strings.Join(versions, ", ")),
			})
		}

		return Result{
			Summary: fmt.Sprintf("%d API groups available in this cluster", len(items)),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

// crdList is a minimal representation of the CRD list API response.
type crdList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Group string `json:"group"`
			Names struct {
				Kind string `json:"kind"`
			} `json:"names"`
			Scope    string `json:"scope"`
			Versions []struct {
				Name    string `json:"name"`
				Served  bool   `json:"served"`
				Storage bool   `json:"storage"`
			} `json:"versions"`
		} `json:"spec"`
	} `json:"items"`
}

func ListCRDs() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_crds",
		Description: "Lists all CustomResourceDefinitions (CRDs) installed in the cluster with their group, kind, version, and scope. Use this to verify that required CRDs (e.g. cert-manager Certificate, Prometheus ServiceMonitor) exist before deploying an app that depends on them.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		raw, err := client.RESTClient().
			Get().
			AbsPath("/apis/apiextensions.k8s.io/v1/customresourcedefinitions").
			DoRaw(context.Background())
		if err != nil {
			return Result{}, fmt.Errorf("failed to list CRDs: %w", err)
		}

		var list crdList
		if err := json.Unmarshal(raw, &list); err != nil {
			return Result{}, fmt.Errorf("failed to parse CRD list: %w", err)
		}

		items := make([]Item, 0, len(list.Items))
		for _, crd := range list.Items {
			served := make([]string, 0)
			for _, v := range crd.Spec.Versions {
				if v.Served {
					served = append(served, v.Name)
				}
			}
			items = append(items, Item{
				Name:    crd.Metadata.Name,
				Status:  strings.ToLower(crd.Spec.Scope),
				Details: fmt.Sprintf("group: %s, kind: %s, versions: %s", crd.Spec.Group, crd.Spec.Names.Kind, strings.Join(served, ", ")),
			})
		}

		return Result{
			Summary: fmt.Sprintf("%d CRDs installed in this cluster", len(items)),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func GetNamespaceQuota() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_namespace_quota",
		Description: "Returns ResourceQuota objects in a namespace showing used vs. hard limits for CPU, memory, pod count, and other resources. Identifies if the app's requirements would exceed namespace quotas.",
	}, func(ctx tool.Context, args NamespaceOnlyArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		quotas, err := client.CoreV1().ResourceQuotas(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list resource quotas in %s: %w", args.Namespace, err)
		}

		if len(quotas.Items) == 0 {
			return Result{
				Summary: fmt.Sprintf("No ResourceQuotas found in namespace %s", args.Namespace),
				Items:   []Item{},
				Issues:  []string{},
			}, nil
		}

		items := make([]Item, 0, len(quotas.Items))
		issues := []string{}

		for _, q := range quotas.Items {
			parts := []string{}
			for resource, hard := range q.Spec.Hard {
				used := q.Status.Used[resource]
				usedStr := "0"
				if !used.IsZero() {
					usedStr = used.String()
				}
				line := fmt.Sprintf("%s: %s used / %s hard", resource, usedStr, hard.String())
				parts = append(parts, line)

				// Warn if usage is at or near 100%
				if !hard.IsZero() {
					hardVal := hard.Value()
					usedVal := used.Value()
					if hardVal > 0 && usedVal >= hardVal {
						issues = append(issues, fmt.Sprintf("Quota %s/%s: %s is at hard limit (%s/%s)", args.Namespace, q.Name, resource, usedStr, hard.String()))
					}
				}
			}
			items = append(items, Item{
				Name:    q.Name,
				Status:  "quota",
				Details: strings.Join(parts, "\n"),
			})
		}

		return Result{
			Summary: fmt.Sprintf("%d ResourceQuota(s) in namespace %s", len(quotas.Items), args.Namespace),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func GetNamespaceLimitRange() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_namespace_limitrange",
		Description: "Returns LimitRange objects in a namespace showing default and max CPU/memory limits. Identifies constraints that would be applied to the app's pods if it does not specify its own limits.",
	}, func(ctx tool.Context, args NamespaceOnlyArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		limitRanges, err := client.CoreV1().LimitRanges(args.Namespace).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list limit ranges in %s: %w", args.Namespace, err)
		}

		if len(limitRanges.Items) == 0 {
			return Result{
				Summary: fmt.Sprintf("No LimitRanges found in namespace %s", args.Namespace),
				Items:   []Item{},
				Issues:  []string{},
			}, nil
		}

		items := make([]Item, 0)
		issues := []string{}

		for _, lr := range limitRanges.Items {
			for _, limit := range lr.Spec.Limits {
				parts := []string{fmt.Sprintf("type: %s", limit.Type)}
				if cpu := limit.Default[corev1.ResourceCPU]; !cpu.IsZero() {
					parts = append(parts, fmt.Sprintf("default CPU: %s", cpu.String()))
				}
				if mem := limit.Default[corev1.ResourceMemory]; !mem.IsZero() {
					parts = append(parts, fmt.Sprintf("default memory: %s", mem.String()))
				}
				if cpu := limit.Max[corev1.ResourceCPU]; !cpu.IsZero() {
					parts = append(parts, fmt.Sprintf("max CPU: %s", cpu.String()))
				}
				if mem := limit.Max[corev1.ResourceMemory]; !mem.IsZero() {
					parts = append(parts, fmt.Sprintf("max memory: %s", mem.String()))
				}
				if cpu := limit.DefaultRequest[corev1.ResourceCPU]; !cpu.IsZero() {
					parts = append(parts, fmt.Sprintf("default request CPU: %s", cpu.String()))
				}
				if mem := limit.DefaultRequest[corev1.ResourceMemory]; !mem.IsZero() {
					parts = append(parts, fmt.Sprintf("default request memory: %s", mem.String()))
				}
				items = append(items, Item{
					Name:    lr.Name,
					Status:  string(limit.Type),
					Details: strings.Join(parts, ", "),
				})

				if limit.Type == corev1.LimitTypeContainer || limit.Type == corev1.LimitTypePod {
					if maxCPU := limit.Max[corev1.ResourceCPU]; !maxCPU.IsZero() {
						issues = append(issues, fmt.Sprintf("LimitRange %s/%s enforces max CPU of %s per %s", args.Namespace, lr.Name, maxCPU.String(), limit.Type))
					}
					if maxMem := limit.Max[corev1.ResourceMemory]; !maxMem.IsZero() {
						issues = append(issues, fmt.Sprintf("LimitRange %s/%s enforces max memory of %s per %s", args.Namespace, lr.Name, maxMem.String(), limit.Type))
					}
				}
			}
		}

		return Result{
			Summary: fmt.Sprintf("%d LimitRange(s) in namespace %s", len(limitRanges.Items), args.Namespace),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListStorageClasses() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_storage_classes",
		Description: "Lists all StorageClasses in the cluster with their provisioner and reclaim policy. Use this to verify that storage classes required by the app's PersistentVolumeClaims exist.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		classes, err := client.StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list storage classes: %w", err)
		}

		items := make([]Item, 0, len(classes.Items))
		issues := []string{}

		for _, sc := range classes.Items {
			isDefault := sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true"
			status := "available"
			if isDefault {
				status = "default"
			}

			reclaimPolicy := "Delete"
			if sc.ReclaimPolicy != nil {
				reclaimPolicy = string(*sc.ReclaimPolicy)
			}

			volumeMode := "Filesystem"
			if sc.VolumeBindingMode != nil {
				volumeMode = string(*sc.VolumeBindingMode)
			}

			items = append(items, Item{
				Name:    sc.Name,
				Status:  status,
				Details: fmt.Sprintf("provisioner: %s, reclaimPolicy: %s, volumeBindingMode: %s", sc.Provisioner, reclaimPolicy, volumeMode),
			})

			if sc.Provisioner == "kubernetes.io/no-provisioner" {
				issues = append(issues, fmt.Sprintf("StorageClass %s uses no-provisioner — PVCs must be pre-provisioned manually", sc.Name))
			}
		}

		return Result{
			Summary: fmt.Sprintf("%d StorageClass(es) available", len(classes.Items)),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func ListIngressClasses() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "list_ingress_classes",
		Description: "Lists IngressClasses installed in the cluster with their controller. Use this to verify that the ingress class referenced by the app's Ingress resources is available.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		classes, err := client.NetworkingV1().IngressClasses().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list ingress classes: %w", err)
		}

		if len(classes.Items) == 0 {
			return Result{
				Summary: "No IngressClasses found in this cluster",
				Items:   []Item{},
				Issues:  []string{"No ingress controllers found — apps using Ingress resources may fail to route traffic"},
			}, nil
		}

		items := make([]Item, 0, len(classes.Items))

		for _, ic := range classes.Items {
			isDefault := ic.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true"
			status := "available"
			if isDefault {
				status = "default"
			}
			items = append(items, Item{
				Name:    ic.Name,
				Status:  status,
				Details: fmt.Sprintf("controller: %s", ic.Spec.Controller),
			})
		}

		return Result{
			Summary: fmt.Sprintf("%d IngressClass(es) available: %s", len(items), joinNames(items)),
			Items:   items,
			Issues:  []string{},
		}, nil
	})
	return t
}

func GetNodeSchedulingInfo() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "get_node_scheduling_info",
		Description: "Returns scheduling-relevant details for every node: Ready status, labels, taints, and allocatable CPU/memory. Use this to check whether nodes exist that satisfy an app's nodeSelector, node affinity rules, and tolerations.",
	}, func(ctx tool.Context, args struct{}) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		nodes, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to list nodes: %w", err)
		}

		items := make([]Item, 0, len(nodes.Items))
		issues := []string{}

		for _, node := range nodes.Items {
			ready := "NotReady"
			for _, c := range node.Status.Conditions {
				if c.Type == "Ready" && c.Status == "True" {
					ready = "Ready"
				}
			}

			// Labels
			labelParts := make([]string, 0, len(node.Labels))
			for k, v := range node.Labels {
				labelParts = append(labelParts, k+"="+v)
			}

			// Taints
			taintParts := make([]string, 0, len(node.Spec.Taints))
			for _, t := range node.Spec.Taints {
				s := fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect)
				taintParts = append(taintParts, s)
			}

			allocCPU := node.Status.Allocatable.Cpu().MilliValue()
			allocMem := node.Status.Allocatable.Memory().Value() / 1024 / 1024

			details := fmt.Sprintf(
				"allocatable: %dm CPU / %dMi memory | labels: [%s] | taints: [%s]",
				allocCPU, allocMem,
				strings.Join(labelParts, ", "),
				strings.Join(taintParts, ", "),
			)

			items = append(items, Item{
				Name:    node.Name,
				Status:  ready,
				Details: details,
			})

			if ready == "NotReady" {
				issues = append(issues, fmt.Sprintf("Node %s is NotReady and cannot accept new pods", node.Name))
			}
			if len(node.Spec.Taints) > 0 && ready == "Ready" {
				issues = append(issues, fmt.Sprintf("Node %s has %d taint(s) — pods must tolerate them to be scheduled here: %s",
					node.Name, len(node.Spec.Taints), strings.Join(taintParts, "; ")))
			}
		}

		ready := 0
		for _, it := range items {
			if it.Status == "Ready" {
				ready++
			}
		}

		return Result{
			Summary: fmt.Sprintf("%d nodes total, %d Ready — labels and taints listed for scheduling compatibility check", len(items), ready),
			Items:   items,
			Issues:  issues,
		}, nil
	})
	return t
}

func joinNames(items []Item) string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return strings.Join(names, ", ")
}
