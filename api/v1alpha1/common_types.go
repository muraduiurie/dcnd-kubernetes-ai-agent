package v1alpha1

// ActionType is the closed vocabulary of changes the operator knows how to
// apply. Each value has exactly one deterministic executor. The runtime may
// propose these and nothing else.
// +kubebuilder:validation:Enum=RollbackDeployment;SetImage;Scale;RestartRollout;SetEnv;SetResources;SetConfigMapKey;DeletePod;RawPatch
type ActionType string

const (
	// ActionRollbackDeployment copies the pod template of an earlier
	// ReplicaSet revision back into the Deployment.
	ActionRollbackDeployment ActionType = "RollbackDeployment"
	// ActionSetImage replaces the image of one container in a Deployment.
	ActionSetImage ActionType = "SetImage"
	// ActionScale sets spec.replicas on a Deployment.
	ActionScale ActionType = "Scale"
	// ActionRestartRollout triggers a rolling restart of a Deployment.
	ActionRestartRollout ActionType = "RestartRollout"
	// ActionSetEnv sets one environment variable on one container.
	ActionSetEnv ActionType = "SetEnv"
	// ActionSetResources replaces the resource requirements of one container.
	ActionSetResources ActionType = "SetResources"
	// ActionSetConfigMapKey sets one key in a ConfigMap.
	ActionSetConfigMapKey ActionType = "SetConfigMapKey"
	// ActionDeletePod deletes one Pod so its controller recreates it.
	ActionDeletePod ActionType = "DeletePod"
	// ActionRawPatch applies an arbitrary patch to an allow-listed kind. It
	// always requires approval and must be enabled per AIAgent.
	ActionRawPatch ActionType = "RawPatch"
)

// RemediationMode decides what the operator does with proposed actions. The
// runtime behaves identically in both modes.
// +kubebuilder:validation:Enum=propose;auto
type RemediationMode string

const (
	// RemediationPropose waits for a human approval on every action.
	RemediationPropose RemediationMode = "propose"
	// RemediationAuto applies actions immediately unless their type is listed
	// in the agent's policy.approvalRequiredFor.
	RemediationAuto RemediationMode = "auto"
)

// Usage records what an investigation consumed. Tokens and steps are
// enforced; estimatedCost is derived from them and reported only.
type Usage struct {
	// +optional
	InputTokens int64 `json:"inputTokens,omitempty"`
	// +optional
	OutputTokens int64 `json:"outputTokens,omitempty"`
	// +optional
	TotalTokens int64 `json:"totalTokens,omitempty"`
	// Steps is the number of model turns, each of which may call one tool.
	// +optional
	Steps int32 `json:"steps,omitempty"`
	// EstimatedCost is informational, for example "0.0123 USD". It is never
	// used to enforce a limit.
	// +optional
	EstimatedCost string `json:"estimatedCost,omitempty"`
}
