package actions

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// SetImage replaces the image of one container in a Deployment. It is the
// executor behind what `kubectl set image` does.
type SetImage struct{}

var _ Executor = SetImage{}

func (SetImage) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionSetImage }

func (SetImage) Targets() []schema.GroupKind {
	return []schema.GroupKind{{Group: "apps", Kind: "Deployment"}}
}

// Apply reads the Deployment, finds the container by name in containers and
// then initContainers, and sends a strategic merge patch carrying only the
// new image. The patch also carries the resourceVersion it was computed
// from, so a concurrent change to the Deployment becomes a conflict and a
// retry rather than a silent overwrite.
func (SetImage) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	p := in.Action.SetImage
	if p == nil {
		return Result{}, &PreconditionError{Reason: "setImage parameters are missing"}
	}

	key := in.TargetKey()
	var deploy appsv1.Deployment
	if err := c.Get(ctx, key, &deploy); err != nil {
		if apierrors.IsNotFound(err) {
			return Result{}, &PreconditionError{Reason: fmt.Sprintf("deployment %s not found", key)}
		}
		return Result{}, fmt.Errorf("get deployment %s: %w", key, err)
	}

	original := deploy.DeepCopy()
	container := findContainer(&deploy.Spec.Template.Spec, p.Container)
	if container == nil {
		return Result{}, &PreconditionError{Reason: fmt.Sprintf("deployment %s has no container %q", key, p.Container)}
	}
	if container.Image == p.Image {
		return Result{Message: fmt.Sprintf("container %s of deployment %s already runs %s", p.Container, deploy.Name, p.Image)}, nil
	}
	previous := container.Image
	container.Image = p.Image

	patch := client.StrategicMergeFrom(original, client.MergeFromWithOptimisticLock{})
	var opts []client.PatchOption
	if in.DryRun {
		opts = append(opts, client.DryRunAll)
	}
	if err := c.Patch(ctx, &deploy, patch, opts...); err != nil {
		return Result{}, fmt.Errorf("patch deployment %s: %w", key, err)
	}

	return Result{Message: fmt.Sprintf("set image of container %s on deployment %s from %s to %s", p.Container, deploy.Name, previous, p.Image)}, nil
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
