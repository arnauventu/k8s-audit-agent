package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/astrokube/hackathon-1-samples/k8s"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// --- Remediation arg types ---

type DeletePodArgs struct {
	Namespace string `json:"namespace" description:"Kubernetes namespace"`
	Name      string `json:"name" description:"Pod name"`
	Force     bool   `json:"force" description:"Force delete with grace period 0 (for stuck Terminating pods)"`
}

type WorkloadIdentArgs struct {
	Namespace string `json:"namespace" description:"Kubernetes namespace"`
	Name      string `json:"name" description:"Deployment name"`
}

type ScaleArgs struct {
	Namespace string `json:"namespace" description:"Kubernetes namespace"`
	Name      string `json:"name" description:"Workload name"`
	Kind      string `json:"kind" description:"Workload kind: deployment or statefulset"`
	Replicas  int32  `json:"replicas" description:"Desired replica count"`
}

type NodeArgs struct {
	Name string `json:"name" description:"Node name"`
}

type DeleteStuckResourceArgs struct {
	Kind      string `json:"kind" description:"Resource kind: namespace, pvc, or pod"`
	Namespace string `json:"namespace" description:"Resource namespace (empty for cluster-scoped resources like namespaces)"`
	Name      string `json:"name" description:"Resource name"`
}

type SetImageArgs struct {
	Namespace     string `json:"namespace" description:"Kubernetes namespace"`
	Name          string `json:"name" description:"Workload name"`
	Kind          string `json:"kind" description:"Workload kind: deployment, statefulset, or daemonset"`
	ContainerName string `json:"container_name" description:"Container name to update"`
	Image         string `json:"image" description:"New container image (e.g. nginx:1.25.3)"`
}

type SetEnvVarArgs struct {
	Namespace     string `json:"namespace" description:"Kubernetes namespace"`
	Name          string `json:"name" description:"Workload name"`
	Kind          string `json:"kind" description:"Workload kind: deployment, statefulset, or daemonset"`
	ContainerName string `json:"container_name" description:"Container name to update"`
	EnvName       string `json:"env_name" description:"Environment variable name"`
	EnvValue      string `json:"env_value" description:"Environment variable value"`
}

type SetResourceLimitsArgs struct {
	Namespace     string `json:"namespace" description:"Kubernetes namespace"`
	Name          string `json:"name" description:"Workload name"`
	Kind          string `json:"kind" description:"Workload kind: deployment, statefulset, or daemonset"`
	ContainerName string `json:"container_name" description:"Container name to update"`
	CPURequest    string `json:"cpu_request,omitempty" description:"CPU request (e.g. 100m, 0.5). Empty to leave unchanged."`
	CPULimit      string `json:"cpu_limit,omitempty" description:"CPU limit (e.g. 500m, 1). Empty to leave unchanged."`
	MemoryRequest string `json:"memory_request,omitempty" description:"Memory request (e.g. 128Mi, 1Gi). Empty to leave unchanged."`
	MemoryLimit   string `json:"memory_limit,omitempty" description:"Memory limit (e.g. 256Mi, 2Gi). Empty to leave unchanged."`
}

// --- Remediation tools ---

func DeletePod() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "delete_pod",
		Description: "Delete a pod to trigger restart. Use force=true for stuck Terminating pods (sets grace period to 0).",
	}, func(ctx tool.Context, args DeletePodArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		opts := metav1.DeleteOptions{}
		if args.Force {
			grace := int64(0)
			opts.GracePeriodSeconds = &grace
		}

		err = client.CoreV1().Pods(args.Namespace).Delete(context.Background(), args.Name, opts)
		if err != nil {
			return Result{}, fmt.Errorf("failed to delete pod %s/%s: %w", args.Namespace, args.Name, err)
		}

		action := "Deleted"
		if args.Force {
			action = "Force-deleted"
		}
		return Result{
			Summary: fmt.Sprintf("%s pod %s/%s", action, args.Namespace, args.Name),
			Items: []Item{{
				Name:      args.Name,
				Namespace: args.Namespace,
				Status:    "Deleted",
				Details:   fmt.Sprintf("Equivalent: kubectl delete pod %s -n %s%s", args.Name, args.Namespace, boolFlag(args.Force, " --force --grace-period=0")),
			}},
		}, nil
	})
	return t
}

func RolloutRestartDeployment() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "rollout_restart_deployment",
		Description: "Restart a deployment by patching the pod template with a restartedAt annotation (same as kubectl rollout restart).",
	}, func(ctx tool.Context, args WorkloadIdentArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		dep, err := client.AppsV1().Deployments(args.Namespace).Get(context.Background(), args.Name, metav1.GetOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to get deployment %s/%s: %w", args.Namespace, args.Name, err)
		}

		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = make(map[string]string)
		}
		dep.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

		_, err = client.AppsV1().Deployments(args.Namespace).Update(context.Background(), dep, metav1.UpdateOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to restart deployment %s/%s: %w", args.Namespace, args.Name, err)
		}

		return Result{
			Summary: fmt.Sprintf("Triggered rollout restart for deployment %s/%s", args.Namespace, args.Name),
			Items: []Item{{
				Name:      args.Name,
				Namespace: args.Namespace,
				Status:    "Restarting",
				Details:   fmt.Sprintf("Equivalent: kubectl rollout restart deployment %s -n %s", args.Name, args.Namespace),
			}},
		}, nil
	})
	return t
}

func ScaleWorkload() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "scale_workload",
		Description: "Scale a deployment or statefulset to a desired replica count.",
	}, func(ctx tool.Context, args ScaleArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		scale := &autoscalingv1.Scale{
			ObjectMeta: metav1.ObjectMeta{Name: args.Name, Namespace: args.Namespace},
			Spec:       autoscalingv1.ScaleSpec{Replicas: args.Replicas},
		}

		var oldReplicas int32
		switch args.Kind {
		case "deployment":
			current, err := client.AppsV1().Deployments(args.Namespace).GetScale(context.Background(), args.Name, metav1.GetOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to get scale for deployment %s/%s: %w", args.Namespace, args.Name, err)
			}
			oldReplicas = current.Spec.Replicas
			_, err = client.AppsV1().Deployments(args.Namespace).UpdateScale(context.Background(), args.Name, scale, metav1.UpdateOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to scale deployment %s/%s: %w", args.Namespace, args.Name, err)
			}
		case "statefulset":
			current, err := client.AppsV1().StatefulSets(args.Namespace).GetScale(context.Background(), args.Name, metav1.GetOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to get scale for statefulset %s/%s: %w", args.Namespace, args.Name, err)
			}
			oldReplicas = current.Spec.Replicas
			_, err = client.AppsV1().StatefulSets(args.Namespace).UpdateScale(context.Background(), args.Name, scale, metav1.UpdateOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to scale statefulset %s/%s: %w", args.Namespace, args.Name, err)
			}
		default:
			return Result{}, fmt.Errorf("unsupported kind %q: must be deployment or statefulset", args.Kind)
		}

		return Result{
			Summary: fmt.Sprintf("Scaled %s %s/%s from %d to %d replicas", args.Kind, args.Namespace, args.Name, oldReplicas, args.Replicas),
			Items: []Item{{
				Name:      args.Name,
				Namespace: args.Namespace,
				Status:    fmt.Sprintf("%d → %d replicas", oldReplicas, args.Replicas),
				Details:   fmt.Sprintf("Equivalent: kubectl scale %s %s -n %s --replicas=%d", args.Kind, args.Name, args.Namespace, args.Replicas),
			}},
		}, nil
	})
	return t
}

func CordonNode() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "cordon_node",
		Description: "Cordon a node to prevent new pods from being scheduled on it.",
	}, func(ctx tool.Context, args NodeArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		node, err := client.CoreV1().Nodes().Get(context.Background(), args.Name, metav1.GetOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to get node %s: %w", args.Name, err)
		}

		if node.Spec.Unschedulable {
			return Result{
				Summary: fmt.Sprintf("Node %s is already cordoned", args.Name),
				Items:   []Item{{Name: args.Name, Status: "Already cordoned"}},
			}, nil
		}

		node.Spec.Unschedulable = true
		_, err = client.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to cordon node %s: %w", args.Name, err)
		}

		return Result{
			Summary: fmt.Sprintf("Cordoned node %s", args.Name),
			Items: []Item{{
				Name:    args.Name,
				Status:  "Cordoned",
				Details: fmt.Sprintf("Equivalent: kubectl cordon %s", args.Name),
			}},
		}, nil
	})
	return t
}

func UncordonNode() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "uncordon_node",
		Description: "Uncordon a node to allow new pods to be scheduled on it again.",
	}, func(ctx tool.Context, args NodeArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		node, err := client.CoreV1().Nodes().Get(context.Background(), args.Name, metav1.GetOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to get node %s: %w", args.Name, err)
		}

		if !node.Spec.Unschedulable {
			return Result{
				Summary: fmt.Sprintf("Node %s is already schedulable", args.Name),
				Items:   []Item{{Name: args.Name, Status: "Already schedulable"}},
			}, nil
		}

		node.Spec.Unschedulable = false
		_, err = client.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("failed to uncordon node %s: %w", args.Name, err)
		}

		return Result{
			Summary: fmt.Sprintf("Uncordoned node %s", args.Name),
			Items: []Item{{
				Name:    args.Name,
				Status:  "Schedulable",
				Details: fmt.Sprintf("Equivalent: kubectl uncordon %s", args.Name),
			}},
		}, nil
	})
	return t
}

func DeleteStuckResource() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "delete_stuck_resource",
		Description: "Delete a stuck resource by removing its finalizers first, then deleting it. Supports namespace, pvc, and pod kinds.",
	}, func(ctx tool.Context, args DeleteStuckResourceArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		bgCtx := context.Background()

		switch args.Kind {
		case "namespace":
			ns, err := client.CoreV1().Namespaces().Get(bgCtx, args.Name, metav1.GetOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to get namespace %s: %w", args.Name, err)
			}
			ns.Spec.Finalizers = nil
			_, err = client.CoreV1().Namespaces().Finalize(bgCtx, ns, metav1.UpdateOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to remove finalizers from namespace %s: %w", args.Name, err)
			}
			err = client.CoreV1().Namespaces().Delete(bgCtx, args.Name, metav1.DeleteOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to delete namespace %s: %w", args.Name, err)
			}

		case "pvc":
			pvc, err := client.CoreV1().PersistentVolumeClaims(args.Namespace).Get(bgCtx, args.Name, metav1.GetOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to get PVC %s/%s: %w", args.Namespace, args.Name, err)
			}
			pvc.Finalizers = nil
			_, err = client.CoreV1().PersistentVolumeClaims(args.Namespace).Update(bgCtx, pvc, metav1.UpdateOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to remove finalizers from PVC %s/%s: %w", args.Namespace, args.Name, err)
			}
			err = client.CoreV1().PersistentVolumeClaims(args.Namespace).Delete(bgCtx, args.Name, metav1.DeleteOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to delete PVC %s/%s: %w", args.Namespace, args.Name, err)
			}

		case "pod":
			pod, err := client.CoreV1().Pods(args.Namespace).Get(bgCtx, args.Name, metav1.GetOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to get pod %s/%s: %w", args.Namespace, args.Name, err)
			}
			pod.Finalizers = nil
			_, err = client.CoreV1().Pods(args.Namespace).Update(bgCtx, pod, metav1.UpdateOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to remove finalizers from pod %s/%s: %w", args.Namespace, args.Name, err)
			}
			err = client.CoreV1().Pods(args.Namespace).Delete(bgCtx, args.Name, metav1.DeleteOptions{})
			if err != nil {
				return Result{}, fmt.Errorf("failed to delete pod %s/%s: %w", args.Namespace, args.Name, err)
			}

		default:
			return Result{}, fmt.Errorf("unsupported kind %q: must be namespace, pvc, or pod", args.Kind)
		}

		return Result{
			Summary: fmt.Sprintf("Removed finalizers and deleted %s %s", args.Kind, resourceName(args.Namespace, args.Name)),
			Items: []Item{{
				Name:      args.Name,
				Namespace: args.Namespace,
				Status:    "Deleted (finalizers removed)",
				Details:   fmt.Sprintf("Removed all finalizers then deleted the %s", args.Kind),
			}},
		}, nil
	})
	return t
}

// --- Workload modification tools ---

// getWorkloadPodSpec retrieves a workload by kind and returns its PodSpec and an update function.
func getWorkloadPodSpec(client kubernetes.Interface, namespace, name, kind string) (*corev1.PodSpec, func() error, error) {
	bgCtx := context.Background()
	switch kind {
	case "deployment":
		obj, err := client.AppsV1().Deployments(namespace).Get(bgCtx, name, metav1.GetOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
		}
		return &obj.Spec.Template.Spec, func() error {
			_, err := client.AppsV1().Deployments(namespace).Update(bgCtx, obj, metav1.UpdateOptions{})
			return err
		}, nil
	case "statefulset":
		obj, err := client.AppsV1().StatefulSets(namespace).Get(bgCtx, name, metav1.GetOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get statefulset %s/%s: %w", namespace, name, err)
		}
		return &obj.Spec.Template.Spec, func() error {
			_, err := client.AppsV1().StatefulSets(namespace).Update(bgCtx, obj, metav1.UpdateOptions{})
			return err
		}, nil
	case "daemonset":
		obj, err := client.AppsV1().DaemonSets(namespace).Get(bgCtx, name, metav1.GetOptions{})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get daemonset %s/%s: %w", namespace, name, err)
		}
		return &obj.Spec.Template.Spec, func() error {
			_, err := client.AppsV1().DaemonSets(namespace).Update(bgCtx, obj, metav1.UpdateOptions{})
			return err
		}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported kind %q: must be deployment, statefulset, or daemonset", kind)
	}
}

// findContainer finds a container by name in a PodSpec.
func findContainer(podSpec *corev1.PodSpec, containerName string) *corev1.Container {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == containerName {
			return &podSpec.Containers[i]
		}
	}
	return nil
}

func SetImage() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "set_image",
		Description: "Update the container image on a deployment, statefulset, or daemonset.",
	}, func(ctx tool.Context, args SetImageArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		podSpec, updateFn, err := getWorkloadPodSpec(client, args.Namespace, args.Name, args.Kind)
		if err != nil {
			return Result{}, err
		}

		container := findContainer(podSpec, args.ContainerName)
		if container == nil {
			return Result{}, fmt.Errorf("container %q not found in %s %s/%s", args.ContainerName, args.Kind, args.Namespace, args.Name)
		}

		oldImage := container.Image
		container.Image = args.Image

		if err := updateFn(); err != nil {
			return Result{}, fmt.Errorf("failed to update %s %s/%s: %w", args.Kind, args.Namespace, args.Name, err)
		}

		return Result{
			Summary: fmt.Sprintf("Updated image for container %s in %s %s/%s: %s → %s", args.ContainerName, args.Kind, args.Namespace, args.Name, oldImage, args.Image),
			Items: []Item{{
				Name:      args.Name,
				Namespace: args.Namespace,
				Status:    fmt.Sprintf("%s → %s", oldImage, args.Image),
				Details:   fmt.Sprintf("Equivalent: kubectl set image %s/%s %s=%s -n %s", args.Kind, args.Name, args.ContainerName, args.Image, args.Namespace),
			}},
		}, nil
	})
	return t
}

func SetEnvVar() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "set_env_var",
		Description: "Set or update an environment variable on a container in a deployment, statefulset, or daemonset.",
	}, func(ctx tool.Context, args SetEnvVarArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		podSpec, updateFn, err := getWorkloadPodSpec(client, args.Namespace, args.Name, args.Kind)
		if err != nil {
			return Result{}, err
		}

		container := findContainer(podSpec, args.ContainerName)
		if container == nil {
			return Result{}, fmt.Errorf("container %q not found in %s %s/%s", args.ContainerName, args.Kind, args.Namespace, args.Name)
		}

		action := "Set"
		for i := range container.Env {
			if container.Env[i].Name == args.EnvName {
				action = "Updated"
				container.Env[i].Value = args.EnvValue
				container.Env[i].ValueFrom = nil
				break
			}
		}
		if action == "Set" {
			container.Env = append(container.Env, corev1.EnvVar{
				Name:  args.EnvName,
				Value: args.EnvValue,
			})
		}

		if err := updateFn(); err != nil {
			return Result{}, fmt.Errorf("failed to update %s %s/%s: %w", args.Kind, args.Namespace, args.Name, err)
		}

		return Result{
			Summary: fmt.Sprintf("%s env var %s on container %s in %s %s/%s", action, args.EnvName, args.ContainerName, args.Kind, args.Namespace, args.Name),
			Items: []Item{{
				Name:      args.Name,
				Namespace: args.Namespace,
				Status:    fmt.Sprintf("%s %s=%s", action, args.EnvName, args.EnvValue),
				Details:   fmt.Sprintf("Equivalent: kubectl set env %s/%s -c %s %s=%s -n %s", args.Kind, args.Name, args.ContainerName, args.EnvName, args.EnvValue, args.Namespace),
			}},
		}, nil
	})
	return t
}

func SetResourceLimits() tool.Tool {
	t, _ := functiontool.New(functiontool.Config{
		Name:        "set_resource_limits",
		Description: "Set CPU/memory requests and limits on a container in a deployment, statefulset, or daemonset. Only non-empty fields are changed.",
	}, func(ctx tool.Context, args SetResourceLimitsArgs) (Result, error) {
		client, err := k8s.Client()
		if err != nil {
			return Result{}, err
		}

		podSpec, updateFn, err := getWorkloadPodSpec(client, args.Namespace, args.Name, args.Kind)
		if err != nil {
			return Result{}, err
		}

		container := findContainer(podSpec, args.ContainerName)
		if container == nil {
			return Result{}, fmt.Errorf("container %q not found in %s %s/%s", args.ContainerName, args.Kind, args.Namespace, args.Name)
		}

		if container.Resources.Requests == nil {
			container.Resources.Requests = corev1.ResourceList{}
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}

		var changes []string
		oldResources := container.Resources.DeepCopy()

		if args.CPURequest != "" {
			q := resource.MustParse(args.CPURequest)
			container.Resources.Requests[corev1.ResourceCPU] = q
			changes = append(changes, fmt.Sprintf("cpu request=%s", args.CPURequest))
		}
		if args.CPULimit != "" {
			q := resource.MustParse(args.CPULimit)
			container.Resources.Limits[corev1.ResourceCPU] = q
			changes = append(changes, fmt.Sprintf("cpu limit=%s", args.CPULimit))
		}
		if args.MemoryRequest != "" {
			q := resource.MustParse(args.MemoryRequest)
			container.Resources.Requests[corev1.ResourceMemory] = q
			changes = append(changes, fmt.Sprintf("memory request=%s", args.MemoryRequest))
		}
		if args.MemoryLimit != "" {
			q := resource.MustParse(args.MemoryLimit)
			container.Resources.Limits[corev1.ResourceMemory] = q
			changes = append(changes, fmt.Sprintf("memory limit=%s", args.MemoryLimit))
		}

		if len(changes) == 0 {
			return Result{
				Summary: "No resource changes specified",
				Items:   []Item{{Name: args.Name, Namespace: args.Namespace, Status: "No changes"}},
			}, nil
		}

		if err := updateFn(); err != nil {
			return Result{}, fmt.Errorf("failed to update %s %s/%s: %w", args.Kind, args.Namespace, args.Name, err)
		}

		// Build kubectl equivalent
		var reqParts, limParts []string
		if args.CPURequest != "" {
			reqParts = append(reqParts, "cpu="+args.CPURequest)
		}
		if args.MemoryRequest != "" {
			reqParts = append(reqParts, "memory="+args.MemoryRequest)
		}
		if args.CPULimit != "" {
			limParts = append(limParts, "cpu="+args.CPULimit)
		}
		if args.MemoryLimit != "" {
			limParts = append(limParts, "memory="+args.MemoryLimit)
		}
		kubectlCmd := fmt.Sprintf("kubectl set resources %s/%s -c %s", args.Kind, args.Name, args.ContainerName)
		if len(reqParts) > 0 {
			kubectlCmd += " --requests=" + strings.Join(reqParts, ",")
		}
		if len(limParts) > 0 {
			kubectlCmd += " --limits=" + strings.Join(limParts, ",")
		}
		kubectlCmd += " -n " + args.Namespace

		_ = oldResources // old values available for detailed diff if needed

		return Result{
			Summary: fmt.Sprintf("Updated resources for container %s in %s %s/%s: %s", args.ContainerName, args.Kind, args.Namespace, args.Name, strings.Join(changes, ", ")),
			Items: []Item{{
				Name:      args.Name,
				Namespace: args.Namespace,
				Status:    strings.Join(changes, ", "),
				Details:   fmt.Sprintf("Equivalent: %s", kubectlCmd),
			}},
		}, nil
	})
	return t
}

// helpers

func boolFlag(b bool, flag string) string {
	if b {
		return flag
	}
	return ""
}

func resourceName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}
