package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelProvider selects the adapter the runtime uses to talk to a model.
// +kubebuilder:validation:Enum=anthropic;openai;ollama
type ModelProvider string

const (
	ProviderAnthropic ModelProvider = "anthropic"
	// ProviderOpenAI covers OpenAI and any OpenAI-compatible endpoint.
	ProviderOpenAI ModelProvider = "openai"
	// ProviderOllama talks to an in-cluster Ollama through its
	// OpenAI-compatible endpoint. It is the fallback when the venue network
	// fails.
	ProviderOllama ModelProvider = "ollama"
)

// ToolSet names a group of read-only tools the runtime exposes to the model.
// +kubebuilder:validation:Enum=kubernetes
type ToolSet string

const (
	// ToolSetKubernetes exposes get_deployment, get_replicasets, get_pods,
	// get_events, get_logs and get_configmap, all read-only, all executed with
	// the per-run agent-reader token.
	ToolSetKubernetes ToolSet = "kubernetes"
)

// ModelSpec identifies the model and how to authenticate to it.
type ModelSpec struct {
	Provider ModelProvider `json:"provider"`

	// Model is the provider-specific model identifier.
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// BaseURL overrides the provider endpoint. Required for ollama and for
	// OpenAI-compatible gateways.
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// SecretRef points at the key holding the provider API key. The Secret
	// must live in the operator namespace, since AIAgent is cluster-scoped.
	// +optional
	SecretRef *corev1.SecretKeySelector `json:"secretRef,omitempty"`
}

// PermissionsSpec defines what the runtime may read, and where. Listing a
// resource grants get, list and watch. There is no verbs field on purpose:
// read-only is structural, not a convention.
type PermissionsSpec struct {
	// Namespaces the agent may investigate. Runs in any other namespace are
	// rejected at admission and never reach the runtime.
	// +kubebuilder:validation:MinItems=1
	Namespaces []string `json:"namespaces"`

	// Resources the agent-reader Role grants, by plural resource name, with
	// subresources as "pods/log". The operator resolves the API group through
	// discovery when it generates the Role.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:example={pods,"pods/log",events,replicasets,deployments}
	Resources []string `json:"resources"`
}

// PolicySpec controls which proposed actions may be applied without a human.
type PolicySpec struct {
	// ApprovalRequiredFor lists action types that always need approval, even
	// when a run is in auto mode. RawPatch requires approval regardless of
	// this list.
	// +optional
	ApprovalRequiredFor []ActionType `json:"approvalRequiredFor,omitempty"`

	// AllowRawPatch enables the RawPatch escape hatch for this agent. Off by
	// default.
	// +optional
	// +kubebuilder:default=false
	AllowRawPatch bool `json:"allowRawPatch,omitempty"`
}

// LimitsSpec bounds one investigation. All three are enforced inside the
// runtime loop and reported in the run status.
type LimitsSpec struct {
	// MaxSteps caps model turns per run.
	// +optional
	// +kubebuilder:default=20
	// +kubebuilder:validation:Minimum=1
	MaxSteps int32 `json:"maxSteps,omitempty"`

	// MaxTokensPerRun caps input plus output tokens per run.
	// +optional
	// +kubebuilder:default=50000
	// +kubebuilder:validation:Minimum=1
	MaxTokensPerRun int64 `json:"maxTokensPerRun,omitempty"`

	// Timeout caps the wall clock of the investigation phase.
	// +optional
	// +kubebuilder:default="10m"
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// TriggerSpec creates runs from Alertmanager notifications. A firing alert
// whose labels match creates a run in the namespace named by the alert's
// namespace label. A resolved notification supersedes that run if nothing
// has been applied yet.
type TriggerSpec struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Match is a set of alert labels that must all be equal for the trigger
	// to fire.
	// +kubebuilder:validation:MinProperties=1
	Match map[string]string `json:"match"`

	// Objective is a Go template rendered with the alert's Labels,
	// Annotations and Fingerprint. The result becomes the run's objective.
	// +kubebuilder:validation:MinLength=1
	Objective string `json:"objective"`

	// Cooldown is how long after a run for the same alert fingerprint reaches
	// a terminal phase before a new one may be created.
	// +optional
	// +kubebuilder:default="10m"
	Cooldown *metav1.Duration `json:"cooldown,omitempty"`

	// RemediationMode is copied into the runs this trigger creates.
	// +optional
	// +kubebuilder:default=propose
	RemediationMode RemediationMode `json:"remediationMode,omitempty"`
}

// AIAgentSpec is the agent definition: who the agent is, what it may read,
// what it may propose, and what it costs at most.
type AIAgentSpec struct {
	Model ModelSpec `json:"model"`

	// Instructions become the system prompt. Never put run-specific text
	// here; that belongs in AgentRun.spec.objective.
	// +kubebuilder:validation:MinLength=1
	Instructions string `json:"instructions"`

	// +kubebuilder:validation:MinItems=1
	Tools []ToolSet `json:"tools"`

	Permissions PermissionsSpec `json:"permissions"`

	// +optional
	Policy PolicySpec `json:"policy,omitempty"`

	// +optional
	Limits LimitsSpec `json:"limits,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name
	Triggers []TriggerSpec `json:"triggers,omitempty"`

	// MaxConcurrentRuns caps runs in a non-terminal phase for this agent.
	// Admission rejects runs above it.
	// +optional
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	MaxConcurrentRuns int32 `json:"maxConcurrentRuns,omitempty"`
}

const (
	// AIAgentConditionReady is True when the runtime Deployment is available
	// and every permitted namespace has its agent-reader identity in place.
	AIAgentConditionReady = "Ready"
)

// AIAgentStatus is written by the operator only.
type AIAgentStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ActiveRuns counts runs in a non-terminal phase.
	// +optional
	ActiveRuns int32 `json:"activeRuns,omitempty"`

	// Usage aggregates consumption across all runs of this agent.
	// +optional
	Usage Usage `json:"usage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=aia
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Provider",type=string,JSONPath=`.spec.model.provider`
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model.model`
// +kubebuilder:printcolumn:name="Runs",type=integer,JSONPath=`.status.activeRuns`
// +kubebuilder:printcolumn:name="Tokens",type=integer,JSONPath=`.status.usage.totalTokens`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AIAgent is the cluster-scoped agent definition. The operator provisions a
// runtime for it and generates the read-only identities it may borrow.
type AIAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AIAgentSpec   `json:"spec,omitempty"`
	Status AIAgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AIAgentList contains a list of AIAgent.
type AIAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AIAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AIAgent{}, &AIAgentList{})
}
