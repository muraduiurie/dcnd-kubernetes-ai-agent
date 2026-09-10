package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AgentReference names the cluster-scoped AIAgent that executes a run.
type AgentReference struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// RemediationSpec decides what happens to proposals once the runtime has
// written them.
type RemediationSpec struct {
	// +optional
	// +kubebuilder:default=propose
	Mode RemediationMode `json:"mode,omitempty"`
}

// Approval is written by a human, never by the runtime. It approves exactly
// one proposed action at exactly one digest. The operator recomputes the
// digest before applying and refuses on mismatch.
type Approval struct {
	// +kubebuilder:validation:MinLength=1
	ActionID string `json:"actionID"`

	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	Digest string `json:"digest"`
}

// Decision lets a human end a run without applying anything.
type Decision struct {
	Declined bool `json:"declined"`

	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Reason string `json:"reason,omitempty"`
}

// AgentRunSpec is the intent: which agent, what to find out, and how far it
// may go on its own.
type AgentRunSpec struct {
	AgentRef AgentReference `json:"agentRef"`

	// Objective is untrusted input from whoever created the run. It goes into
	// the user message, never the system prompt.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Objective string `json:"objective"`

	// +optional
	Remediation RemediationSpec `json:"remediation,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=actionID
	Approvals []Approval `json:"approvals,omitempty"`

	// +optional
	Decision *Decision `json:"decision,omitempty"`

	// ApprovalTimeout is how long a run may wait in AwaitingApproval before
	// it fails.
	// +optional
	// +kubebuilder:default="1h"
	ApprovalTimeout *metav1.Duration `json:"approvalTimeout,omitempty"`
}

// RunPhase is the coarse state of a run, kept for printer columns and the
// plugin. Conditions carry the detail.
// +kubebuilder:validation:Enum=Pending;Accepted;Rejected;Investigating;AwaitingApproval;Applying;Verifying;Succeeded;Failed;Declined;Superseded
type RunPhase string

const (
	RunPending          RunPhase = "Pending"
	RunAccepted         RunPhase = "Accepted"
	RunRejected         RunPhase = "Rejected"
	RunInvestigating    RunPhase = "Investigating"
	RunAwaitingApproval RunPhase = "AwaitingApproval"
	RunApplying         RunPhase = "Applying"
	RunVerifying        RunPhase = "Verifying"
	RunSucceeded        RunPhase = "Succeeded"
	RunFailed           RunPhase = "Failed"
	RunDeclined         RunPhase = "Declined"
	RunSuperseded       RunPhase = "Superseded"
)

const (
	// AgentRunConditionAccepted is set by the operator at admission.
	AgentRunConditionAccepted = "Accepted"
	// AgentRunConditionInvestigated is set by the runtime when it has written
	// diagnosis, evidence and proposals.
	AgentRunConditionInvestigated = "Investigated"
	// AgentRunConditionApproved is set by the operator when every proposed
	// action has a matching approval or is auto-approved by policy.
	AgentRunConditionApproved = "Approved"
	// AgentRunConditionApplied is set by the operator after the executors ran.
	AgentRunConditionApplied = "Applied"
	// AgentRunConditionVerified is set by the operator after the target
	// reported Available, or after the verification timeout.
	AgentRunConditionVerified = "Verified"
)

// RunOriginKind says who created the run.
// +kubebuilder:validation:Enum=Alert;Human
type RunOriginKind string

const (
	RunOriginAlert RunOriginKind = "Alert"
	RunOriginHuman RunOriginKind = "Human"
)

// RunOrigin is recorded by the operator at admission so trigger-created and
// human-created runs are distinguishable in the audit trail.
type RunOrigin struct {
	Kind RunOriginKind `json:"kind"`

	// Trigger is the name of the AIAgent trigger that created the run.
	// +optional
	Trigger string `json:"trigger,omitempty"`

	// Fingerprint is the Alertmanager fingerprint. It is the dedup key for
	// storm control and the key a resolved notification uses to supersede.
	// +optional
	Fingerprint string `json:"fingerprint,omitempty"`
}

// ExecutorInfo identifies the runtime pod that claimed the run.
type ExecutorInfo struct {
	Pod string `json:"pod"`

	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
}

// Diagnosis is the runtime's conclusion, in words.
type Diagnosis struct {
	// +kubebuilder:validation:MaxLength=512
	Summary string `json:"summary"`

	// +optional
	// +kubebuilder:validation:MaxLength=4096
	Detail string `json:"detail,omitempty"`
}

// Evidence is one observation produced by a tool call. Ids are assigned by
// the runtime from real tool results; the model may cite them and cannot
// invent them.
type Evidence struct {
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// Tool is the tool that produced this observation.
	// +kubebuilder:validation:MinLength=1
	Tool string `json:"tool"`

	// Step is the model turn in which the tool was called.
	// +optional
	Step int32 `json:"step,omitempty"`

	// +optional
	Kind string `json:"kind,omitempty"`

	// +optional
	Name string `json:"name,omitempty"`

	// Reason is the machine-readable reason where one exists, for example an
	// Event reason or a container waiting reason.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Excerpt is the relevant fragment of the tool result. Status is not a
	// log store; the full trace lives in the runtime logs.
	// +kubebuilder:validation:MaxLength=1024
	Excerpt string `json:"excerpt"`
}

// TargetRef names the object an action changes. It is always in the run's
// namespace; the operator refuses anything else.
type TargetRef struct {
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`

	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// RollbackParams are the parameters of RollbackDeployment.
type RollbackParams struct {
	// ToRevision is the deployment.kubernetes.io/revision of the ReplicaSet
	// whose template is restored.
	// +kubebuilder:validation:Minimum=1
	ToRevision int64 `json:"toRevision"`
}

// SetImageParams are the parameters of SetImage.
type SetImageParams struct {
	// +kubebuilder:validation:MinLength=1
	Container string `json:"container"`

	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
}

// ScaleParams are the parameters of Scale.
type ScaleParams struct {
	// +kubebuilder:validation:Minimum=0
	Replicas int32 `json:"replicas"`
}

// SetEnvParams are the parameters of SetEnv.
type SetEnvParams struct {
	// +kubebuilder:validation:MinLength=1
	Container string `json:"container"`

	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	Value string `json:"value"`
}

// SetResourcesParams are the parameters of SetResources.
type SetResourcesParams struct {
	// +kubebuilder:validation:MinLength=1
	Container string `json:"container"`

	Resources corev1.ResourceRequirements `json:"resources"`
}

// SetConfigMapKeyParams are the parameters of SetConfigMapKey.
type SetConfigMapKeyParams struct {
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`

	Value string `json:"value"`
}

// PatchType selects how a RawPatch body is applied.
// +kubebuilder:validation:Enum=json;merge;strategic
type PatchType string

const (
	PatchTypeJSON      PatchType = "json"
	PatchTypeMerge     PatchType = "merge"
	PatchTypeStrategic PatchType = "strategic"
)

// RawPatchParams are the parameters of RawPatch, the escape hatch. It is
// never auto-applied and only targets kinds the operator allow-lists.
type RawPatchParams struct {
	PatchType PatchType `json:"patchType"`

	// Patch is the patch body, a JSON array for json and an object otherwise.
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Patch runtime.RawExtension `json:"patch"`
}

// DryRunResult is written by the operator after a server-side dry-run of the
// action, before anyone is asked to approve it.
type DryRunResult struct {
	OK bool `json:"ok"`

	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message,omitempty"`

	// +optional
	CheckedAt *metav1.Time `json:"checkedAt,omitempty"`
}

// ProposedAction is one typed change. The runtime writes id, type, target,
// the parameter block for the type, and evidenceRefs. The operator writes
// digest and dryRun.
type ProposedAction struct {
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	Type ActionType `json:"type"`

	Target TargetRef `json:"target"`

	// +optional
	Rollback *RollbackParams `json:"rollback,omitempty"`
	// +optional
	SetImage *SetImageParams `json:"setImage,omitempty"`
	// +optional
	Scale *ScaleParams `json:"scale,omitempty"`
	// +optional
	SetEnv *SetEnvParams `json:"setEnv,omitempty"`
	// +optional
	SetResources *SetResourcesParams `json:"setResources,omitempty"`
	// +optional
	SetConfigMapKey *SetConfigMapKeyParams `json:"setConfigMapKey,omitempty"`
	// +optional
	RawPatch *RawPatchParams `json:"rawPatch,omitempty"`

	// EvidenceRefs are ids from status.evidence. Every action must cite at
	// least one; the runtime fails the run otherwise.
	// +kubebuilder:validation:MinItems=1
	EvidenceRefs []string `json:"evidenceRefs"`

	// Digest is sha256 over the canonical JSON of type, target and the
	// parameter block. Approvals reference it.
	// +optional
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	Digest string `json:"digest,omitempty"`

	// +optional
	DryRun *DryRunResult `json:"dryRun,omitempty"`
}

// ActionResult records what an executor did with one proposed action.
type ActionResult struct {
	// +kubebuilder:validation:MinLength=1
	ActionID string `json:"actionID"`

	Succeeded bool `json:"succeeded"`

	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message,omitempty"`

	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`
}

// AgentRunStatus is the record. The runtime and the operator each own their
// fields through server-side apply with distinct field managers.
type AgentRunStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Phase RunPhase `json:"phase,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Message is a human-readable reason for the current phase, used for
	// terminal phases such as Rejected and Failed.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Message string `json:"message,omitempty"`

	// +optional
	CreatedBy *RunOrigin `json:"createdBy,omitempty"`

	// +optional
	Executor *ExecutorInfo `json:"executor,omitempty"`

	// Attempts counts how many times a runtime pod claimed this run.
	// +optional
	Attempts int32 `json:"attempts,omitempty"`

	// +optional
	Diagnosis *Diagnosis `json:"diagnosis,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=id
	// +kubebuilder:validation:MaxItems=20
	Evidence []Evidence `json:"evidence,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=id
	ProposedActions []ProposedAction `json:"proposedActions,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=actionID
	Applied []ActionResult `json:"applied,omitempty"`

	// +optional
	Usage Usage `json:"usage,omitempty"`

	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ar
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agentRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Origin",type=string,JSONPath=`.status.createdBy.kind`
// +kubebuilder:printcolumn:name="Steps",type=integer,JSONPath=`.status.usage.steps`
// +kubebuilder:printcolumn:name="Tokens",type=integer,JSONPath=`.status.usage.totalTokens`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AgentRun is one bounded investigation in a workload namespace. Spec is the
// intent, status is the evidence, the proposals and what was done with them.
type AgentRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentRunSpec   `json:"spec,omitempty"`
	Status AgentRunStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentRunList contains a list of AgentRun.
type AgentRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentRun `json:"items"`
}
