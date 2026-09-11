package actions

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Kinds the executors may target.
var (
	deploymentGK = appsv1.SchemeGroupVersion.WithKind("Deployment").GroupKind()
	configMapGK  = corev1.SchemeGroupVersion.WithKind("ConfigMap").GroupKind()
	podGK        = corev1.SchemeGroupVersion.WithKind("Pod").GroupKind()
	serviceGK    = corev1.SchemeGroupVersion.WithKind("Service").GroupKind()
)

// fetch loads the target into obj. A missing target is a precondition
// failure, since retrying cannot make it appear. Anything else is transient.
func fetch(ctx context.Context, c client.Client, key client.ObjectKey, obj client.Object, kind string) error {
	if err := c.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return &PreconditionError{Reason: fmt.Sprintf("%s %s not found", kind, key)}
		}
		return fmt.Errorf("get %s %s: %w", kind, key, err)
	}
	return nil
}

// lockedPatch is the patch every read-modify-write executor sends: a
// strategic merge computed against original, carrying the resourceVersion it
// was read at, so a concurrent change becomes a conflict and a retry rather
// than a silent overwrite.
func lockedPatch(original client.Object) client.Patch {
	return client.StrategicMergeFrom(original, client.MergeFromWithOptimisticLock{})
}

// patchOptions adds dryRun=All when the Input asks for it.
func patchOptions(in Input) []client.PatchOption {
	if in.DryRun {
		return []client.PatchOption{client.DryRunAll}
	}
	return nil
}

// wrapWrite classifies the error from a write. The API server rejecting the
// change as invalid is permanent, so it becomes a precondition failure and
// the reconciler stops instead of retrying forever. Everything else is
// transient.
func wrapWrite(op string, err error) error {
	if err == nil {
		return nil
	}
	if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
		return &PreconditionError{Reason: fmt.Sprintf("%s rejected: %v", op, err)}
	}
	return fmt.Errorf("%s: %w", op, err)
}

// findContainer returns a pointer into spec for the named container, looking
// at containers first and initContainers second, or nil if neither has it.
func findContainer(spec *corev1.PodSpec, name string) *corev1.Container {
	for i := range spec.Containers {
		if spec.Containers[i].Name == name {
			return &spec.Containers[i]
		}
	}
	for i := range spec.InitContainers {
		if spec.InitContainers[i].Name == name {
			return &spec.InitContainers[i]
		}
	}
	return nil
}
