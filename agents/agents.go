package agents

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"

	"github.com/astrokube/hackathon-1-samples/tools"
)

func NewWorkloadInspector(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "workload_inspector",
		Model:       m,
		Description: "Analyzes Kubernetes workload issues: pod crashes, deployment rollouts, daemonset health, statefulset problems, and job failures.",
		Instruction: `You are a Kubernetes workload inspector. Use your tools to analyze pods, deployments, daemonsets, statefulsets, and jobs.
Look for: CrashLoopBackOff pods, high restart counts, pending pods, unavailable replicas, stalled rollouts, failed jobs.
Always report findings with namespace and resource names. Summarize issues clearly.

When issues are found, suggest remediation approaches:
- CrashLoopBackOff: recommend checking pod logs for the crash reason; if config/env issue, fix and redeploy; if transient, a pod delete may help
- High restart counts: recommend rollout restart if the issue appears resolved but pods are in a bad state
- Stalled rollouts: recommend checking events and pod status; rollback may be needed if new image is broken
- Pending pods: check for insufficient resources, node affinity issues, or missing PVCs
- Failed jobs: check job logs and events for failure reason`,
		Tools: []tool.Tool{
			tools.ListPods(),
			tools.DescribePod(),
			tools.ListDeployments(),
			tools.ListDaemonSets(),
			tools.ListStatefulSets(),
			tools.ListJobs(),
		},
	})
}

func NewNetworkInspector(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "network_inspector",
		Model:       m,
		Description: "Inspects Kubernetes networking: services, endpoints, ingresses, and network policies.",
		Instruction: `You are a Kubernetes network inspector. Use your tools to analyze services, endpoints, ingresses, and network policies.
Look for: services with no endpoints, ingresses without addresses, endpoints with zero ready addresses.
Always report findings with namespace and resource names.

When issues are found, suggest remediation approaches:
- Services with no endpoints: check if pod labels match the service selector; verify target pods are running and ready
- Ingresses without addresses: check if the ingress controller is running and healthy; verify ingress class annotation
- Network policy issues: verify that policies are not accidentally blocking required traffic`,
		Tools: []tool.Tool{
			tools.ListServices(),
			tools.ListEndpoints(),
			tools.ListIngresses(),
			tools.ListNetworkPolicies(),
		},
	})
}

func NewStorageInspector(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "storage_inspector",
		Model:       m,
		Description: "Inspects Kubernetes storage: persistent volumes and persistent volume claims.",
		Instruction: `You are a Kubernetes storage inspector. Use your tools to analyze persistent volumes and persistent volume claims.
Look for: Released or Failed PVs, Pending PVCs, storage class issues.
Always report findings with resource names and current status.

When issues are found, suggest remediation approaches:
- Pending PVCs: check if the storage class provisioner is running; verify storage class exists and has available capacity
- Released PVs: can be cleaned up if data is no longer needed, or reclaim policy can be changed
- Failed PVs: check events for provisioning errors; may need to delete and recreate the PVC`,
		Tools: []tool.Tool{
			tools.ListPersistentVolumes(),
			tools.ListPersistentVolumeClaims(),
		},
	})
}

func NewRBACInspector(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "rbac_inspector",
		Model:       m,
		Description: "Inspects Kubernetes RBAC: roles, role bindings, and service accounts.",
		Instruction: `You are a Kubernetes RBAC inspector. Use your tools to audit roles, role bindings, and service accounts.
Look for: wildcard permissions, cluster-admin grants, service accounts with automount token enabled.
Flag any overly permissive configurations.`,
		Tools: []tool.Tool{
			tools.ListRoles(),
			tools.ListRoleBindings(),
			tools.ListServiceAccounts(),
		},
	})
}

func NewSecurityInspector(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "security_inspector",
		Model:       m,
		Description: "Inspects Kubernetes pod security: security contexts, privileged containers, and PSA enforcement.",
		Instruction: `You are a Kubernetes security inspector. Use your tools to analyze pod security contexts and namespace PSA labels.
Look for: privileged containers, root users (UID 0), hostNetwork/hostPID, dangerous capabilities (SYS_ADMIN, NET_ADMIN, ALL), missing PSA enforcement.
Report all security concerns with pod/namespace names.`,
		Tools: []tool.Tool{
			tools.AnalyzePodSecurity(),
			tools.ListPodSecurityStandards(),
		},
	})
}

func NewEventInspector(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "event_inspector",
		Model:       m,
		Description: "Inspects Kubernetes cluster events: warnings, failures, and recent issues.",
		Instruction: `You are a Kubernetes event inspector. Use your tools to analyze cluster events.
Look for: Warning events, high-count repeated events, recent failures.
Summarize event patterns and highlight the most critical issues.

When issues are found, suggest next steps:
- Correlate warning events with affected resources to identify root causes
- For pod-related events (FailedScheduling, BackOff, Unhealthy): recommend consulting the workload inspector
- For node-related events (NodeNotReady, OOMKilling): recommend consulting the node inspector
- For network events (FailedMount, DNSConfigForming): recommend consulting the network or storage inspector`,
		Tools: []tool.Tool{
			tools.ListEvents(),
			tools.ListRecentEvents(),
		},
	})
}

func NewNodeInspector(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "node_inspector",
		Model:       m,
		Description: "Inspects Kubernetes nodes: health status, resource usage, and pressure conditions.",
		Instruction: `You are a Kubernetes node inspector. Use your tools to analyze node health and resource usage.
Look for: NotReady nodes, memory/disk/PID pressure, CPU/memory usage above 80%, pods without resource limits.
Report node names and specific issues found.

When issues are found, suggest remediation approaches:
- NotReady nodes: recommend cordoning to prevent new scheduling, then investigate kubelet logs
- Memory/disk pressure: recommend identifying top resource consumers and consider evicting non-critical pods
- High CPU/memory usage (>80%): recommend cordoning the node and redistributing workloads
- Pods without limits: flag as a risk for resource contention; recommend adding resource requests/limits`,
		Tools: []tool.Tool{
			tools.ListNodes(),
			tools.GetNodeResourceUsage(),
			tools.GetPodResourceUsage(),
		},
	})
}
