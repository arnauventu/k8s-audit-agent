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

// --- PlatformChecker redesign: deployment-compatibility specialists ---

func NewCapacityChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "capacity_checker",
		Model:       m,
		Description: "Checks whether the cluster has enough available CPU and memory capacity to schedule the app's resource requests.",
		Instruction: `You are a cluster capacity checker. Your job is to determine whether the cluster has enough allocatable resources to deploy a new application.

Use list_nodes to enumerate nodes and their allocatable CPU/memory.
Use get_node_resource_usage to see current consumption (may be unavailable if metrics-server is absent).

Report:
- Total allocatable CPU and memory across all Ready nodes
- Current usage per node (if metrics available)
- Estimated free capacity
- BLOCK if no node has enough free resources to fit the app's requests
- WARN if total free capacity is less than 20% of total allocatable
- PASS if sufficient capacity exists`,
		Tools: []tool.Tool{
			tools.ListNodes(),
			tools.GetNodeResourceUsage(),
		},
	})
}

func NewAdmissionChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "admission_checker",
		Model:       m,
		Description: "Checks Pod Security Admission (PSA) enforcement labels on the target namespace and available ingress classes.",
		Instruction: `You are an admission policy checker. Your job is to identify admission controls that could reject the app at deploy time.

Use list_pod_security_standards to check PSA enforcement labels on the target namespace.
Use list_ingress_classes to check if the required ingress controller is present.

Report:
- PSA enforcement level on target namespace (privileged / baseline / restricted)
- BLOCK if namespace enforces restricted PSA and the app runs as root or uses privileged containers
- WARN if namespace enforces baseline PSA and the app uses privileged mode
- BLOCK if the app requires an ingress class that is not available`,
		Tools: []tool.Tool{
			tools.ListPodSecurityStandards(),
			tools.ListIngressClasses(),
		},
	})
}

func NewQuotaChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "quota_checker",
		Model:       m,
		Description: "Checks ResourceQuota and LimitRange constraints in the target namespace.",
		Instruction: `You are a namespace quota checker. Your job is to determine whether namespace-level resource constraints would block the app from being deployed.

Use get_namespace_quota to check ResourceQuota usage vs. hard limits.
Use get_namespace_limitrange to check LimitRange defaults and maximums.

Report:
- Current quota usage vs. hard limits for CPU, memory, and pod count
- LimitRange defaults that would be injected into the app's pods
- BLOCK if the namespace quota is already at or near its hard limit (pod count, CPU, or memory)
- WARN if LimitRange maximums are lower than the app's requested limits`,
		Tools: []tool.Tool{
			tools.GetNamespaceQuota(),
			tools.GetNamespaceLimitRange(),
		},
	})
}

func NewDependencyChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "dependency_checker",
		Model:       m,
		Description: "Checks whether the cluster supports the API versions and CRDs required by the app's manifests.",
		Instruction: `You are a deployment dependency checker. Your job is to verify that all cluster-level prerequisites for the app exist.

Use get_cluster_version to identify the Kubernetes version.
Use list_api_resources to verify that required apiVersions are available (e.g. batch/v1, networking.k8s.io/v1).
Use list_crds to verify that required CustomResourceDefinitions are installed (e.g. cert-manager Certificate, Prometheus ServiceMonitor, external-secrets ExternalSecret).

Report:
- Cluster Kubernetes version
- Any apiVersion used by the app that is not available in this cluster → BLOCK
- Any required CRD that is missing → BLOCK
- List of available CRDs for reference`,
		Tools: []tool.Tool{
			tools.GetClusterVersion(),
			tools.ListAPIResources(),
			tools.ListCRDs(),
		},
	})
}

func NewSchedulingChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "scheduling_checker",
		Model:       m,
		Description: "Checks whether nodes exist that satisfy the app's nodeSelector, node affinity rules, and tolerations.",
		Instruction: `You are a pod scheduling checker. Your job is to determine whether the app's pods can actually be scheduled on existing nodes given their scheduling constraints.

Use get_node_scheduling_info to retrieve every node's labels, taints, allocatable CPU/memory, and Ready status.

For each scheduling constraint the app defines, evaluate it against the node list:

**nodeSelector** — check that at least one Ready node has ALL the required key=value labels.
  → BLOCK if no Ready node matches all required labels.

**Node affinity (requiredDuringSchedulingIgnoredDuringExecution)** — treat like a hard nodeSelector.
  → BLOCK if no Ready node satisfies all required match expressions.

**Node affinity (preferredDuringSchedulingIgnoredDuringExecution)** — soft preference.
  → WARN if no node satisfies the preference (scheduling still works, but placement is suboptimal).

**Tolerations** — check that for every taint on candidate nodes, the app either tolerates it or the taint effect is PreferNoSchedule.
  → BLOCK if every Ready node has a NoSchedule or NoExecute taint that the app does not tolerate.
  → WARN if some nodes are excluded by taints but at least one schedulable node remains.

**Overall verdict:**
- BLOCK if no Ready node can satisfy all hard scheduling constraints
- WARN if scheduling works but with reduced node pool
- PASS if multiple Ready nodes satisfy all constraints`,
		Tools: []tool.Tool{
			tools.GetNodeSchedulingInfo(),
		},
	})
}

func NewNetworkPolicyChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "network_policy_checker",
		Model:       m,
		Description: "Checks existing NetworkPolicies in the target namespace that could block traffic to or from the new app.",
		Instruction: `You are a network policy checker. Your job is to identify existing NetworkPolicies that could block ingress or egress traffic to the new application.

Use list_network_policies to enumerate policies in the target namespace.
Use list_namespaces to understand namespace labels used in policy selectors.

Report:
- All NetworkPolicies in the target namespace
- Policies that select all pods (could inadvertently restrict the new app)
- WARN if default-deny ingress or egress policies exist that would block the app's traffic
- PASS if no blocking policies are found`,
		Tools: []tool.Tool{
			tools.ListNetworkPolicies(),
			tools.ListNamespaces(),
		},
	})
}

func NewSecretExistenceChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "secret_existence_checker",
		Model:       m,
		Description: "Checks whether Kubernetes Secrets referenced by the app (secretKeyRef, envFrom.secretRef, imagePullSecrets) exist in the target namespace.",
		Instruction: `You are a secret existence checker. Your job is to verify that all Kubernetes Secrets the app depends on are present in the target namespace before deployment.

Use list_namespace_secrets to retrieve all secrets in the target namespace (names and types only — no values).

Cross-reference the result against the secrets the app requires. These may be provided as context by the orchestrator or extracted from the repo findings. Look for:

**secretKeyRef / secretRef:**
- Any env var in the deployment that uses secretKeyRef.name or envFrom.secretRef.name
- Check each referenced secret name against the list returned by list_namespace_secrets
- BLOCK if any referenced secret is missing from the namespace

**imagePullSecrets:**
- Any imagePullSecrets field in the pod spec referencing a secret name
- Check each name against secrets of type kubernetes.io/dockerconfigjson or kubernetes.io/dockercfg
- BLOCK if a required image pull secret is missing

**Report:**
- List all secrets present in the namespace (name and type)
- For each required secret: PRESENT or MISSING
- BLOCK for any missing required secret (deployment will fail with ImagePullBackOff or CreateContainerConfigError)
- PASS if all required secrets are present or no secrets are required`,
		Tools: []tool.Tool{
			tools.ListNamespaceSecrets(),
		},
	})
}

func NewRBACCompatibilityChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "rbac_compatibility_checker",
		Model:       m,
		Description: "Checks whether the service account referenced by the app exists in the target namespace and has the required RBAC permissions.",
		Instruction: `You are an RBAC compatibility checker. Your job is to verify that the app's service account and permissions are in place in the target namespace.

Use list_service_accounts to check if the service account referenced by the app exists in the target namespace.
Use list_role_bindings to check what roles are bound to that service account.
Use list_roles to inspect what permissions those roles grant.

**Service account check:**
- If the deployment specifies a serviceAccountName, verify it exists in the target namespace
- BLOCK if the service account does not exist (pod will fail to start)
- If no serviceAccountName is specified, the default SA will be used — note this

**RBAC permissions check:**
- List role bindings for the target service account
- If the app needs specific permissions (e.g. to read ConfigMaps, list pods, create jobs), verify those are granted
- WARN if the service account uses the default SA with no explicit bindings (may lack needed permissions)
- WARN if wildcard permissions (* verbs or * resources) are granted — overly permissive
- PASS if service account exists and has appropriate, least-privilege permissions`,
		Tools: []tool.Tool{
			tools.ListServiceAccounts(),
			tools.ListRoleBindings(),
			tools.ListRoles(),
		},
	})
}

func NewStorageCompatibilityChecker(m model.LLM) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "storage_compatibility_checker",
		Model:       m,
		Description: "Checks whether the storage classes required by the app's PersistentVolumeClaims exist and have working provisioners.",
		Instruction: `You are a storage compatibility checker. Your job is to verify that the cluster can provision the storage the app needs.

Use list_storage_classes to enumerate available storage classes and their provisioners.
Use list_persistent_volumes to check existing PV availability and binding status.

Report:
- Available storage classes and their provisioners
- BLOCK if a storage class required by the app does not exist
- WARN if a required storage class uses no-provisioner (manual pre-provisioning required)
- PASS if all required storage classes exist with functioning provisioners`,
		Tools: []tool.Tool{
			tools.ListStorageClasses(),
			tools.ListPersistentVolumes(),
		},
	})
}
