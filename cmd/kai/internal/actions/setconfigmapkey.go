package actions

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// SetConfigMapKey sets one key in a ConfigMap's data, adding it if absent.
//
// Changing a ConfigMap does not restart the pods that read it. A plan that
// fixes configuration is therefore SetConfigMapKey followed by
// RestartRollout.
type SetConfigMapKey struct{}

var _ Executor = SetConfigMapKey{}

func (SetConfigMapKey) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionSetConfigMapKey }

func (SetConfigMapKey) Targets() []schema.GroupKind { return []schema.GroupKind{configMapGK} }

func (SetConfigMapKey) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	p := in.Action.SetConfigMapKey
	if p == nil {
		return Result{}, &PreconditionError{Reason: "setConfigMapKey parameters are missing"}
	}

	key := in.TargetKey()
	var cm corev1.ConfigMap
	if err := fetch(ctx, c, key, &cm, "configmap"); err != nil {
		return Result{}, err
	}

	if cm.Immutable != nil && *cm.Immutable {
		return Result{}, &PreconditionError{Reason: fmt.Sprintf("configmap %s is immutable", key)}
	}
	if _, inBinary := cm.BinaryData[p.Key]; inBinary {
		return Result{}, &PreconditionError{Reason: fmt.Sprintf("key %q of configmap %s is in binaryData and cannot be set as text", p.Key, key)}
	}
	current, exists := cm.Data[p.Key]
	if exists && current == p.Value {
		return Result{Message: fmt.Sprintf("key %s of configmap %s already set", p.Key, cm.Name)}, nil
	}

	original := cm.DeepCopy()
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[p.Key] = p.Value

	if err := c.Patch(ctx, &cm, lockedPatch(original), patchOptions(in)...); err != nil {
		return Result{}, wrapWrite(fmt.Sprintf("patch configmap %s", key), err)
	}
	verb := "set"
	if !exists {
		verb = "added"
	}
	// The value is deliberately not repeated here; it already sits in the
	// proposal and may be sensitive.
	return Result{Message: fmt.Sprintf("%s key %s in configmap %s", verb, p.Key, cm.Name)}, nil
}
