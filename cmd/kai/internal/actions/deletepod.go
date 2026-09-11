package actions

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// DeletePod deletes one Pod so that its controller recreates it. A Pod with
// no controller is refused, because deleting it would not bring it back.
type DeletePod struct{}

var _ Executor = DeletePod{}

func (DeletePod) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionDeletePod }

func (DeletePod) Targets() []schema.GroupKind { return []schema.GroupKind{podGK} }

func (DeletePod) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	key := in.TargetKey()
	var pod corev1.Pod
	if err := fetch(ctx, c, key, &pod, "pod"); err != nil {
		return Result{}, err
	}

	if pod.DeletionTimestamp != nil {
		return Result{Message: fmt.Sprintf("pod %s is already terminating", pod.Name)}, nil
	}
	owner := metav1.GetControllerOf(&pod)
	if owner == nil {
		return Result{}, &PreconditionError{Reason: fmt.Sprintf("pod %s has no controller; deleting it would not recreate it", key)}
	}

	// The UID precondition makes the delete apply to the pod that was read,
	// not to a namesake created in between.
	opts := []client.DeleteOption{client.Preconditions{UID: &pod.UID}}
	if in.DryRun {
		opts = append(opts, client.DryRunAll)
	}
	if err := c.Delete(ctx, &pod, opts...); err != nil {
		if apierrors.IsNotFound(err) {
			// Gone between the read and the delete: the goal is met.
			return Result{Message: fmt.Sprintf("pod %s was already gone", pod.Name)}, nil
		}
		return Result{}, wrapWrite(fmt.Sprintf("delete pod %s", key), err)
	}
	return Result{Message: fmt.Sprintf("deleted pod %s (owned by %s %s)", pod.Name, owner.Kind, owner.Name)}, nil
}
