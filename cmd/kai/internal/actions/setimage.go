package actions

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// SetImage replaces the image of one container in a Deployment. It is the
// executor behind what `kubectl set image` does.
type SetImage struct{}

var _ Executor = SetImage{}

func (SetImage) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionSetImage }

func (SetImage) Targets() []schema.GroupKind { return []schema.GroupKind{deploymentGK} }

// Apply reads the Deployment, finds the container by name in containers and
// then initContainers, and sends a strategic merge patch carrying only the
// new image.
func (SetImage) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	p := in.Action.SetImage
	if p == nil {
		return Result{}, &PreconditionError{Reason: "setImage parameters are missing"}
	}

	key := in.TargetKey()
	var deploy appsv1.Deployment
	if err := fetch(ctx, c, key, &deploy, "deployment"); err != nil {
		return Result{}, err
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

	if err := c.Patch(ctx, &deploy, lockedPatch(original), patchOptions(in)...); err != nil {
		return Result{}, wrapWrite(fmt.Sprintf("patch deployment %s", key), err)
	}
	return Result{Message: fmt.Sprintf("set image of container %s on deployment %s from %s to %s", p.Container, deploy.Name, previous, p.Image)}, nil
}
