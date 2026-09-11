package actions

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// Scale sets spec.replicas on a Deployment. It is the executor behind what
// `kubectl scale` does.
type Scale struct{}

var _ Executor = Scale{}

func (Scale) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionScale }

func (Scale) Targets() []schema.GroupKind { return []schema.GroupKind{deploymentGK} }

func (Scale) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	p := in.Action.Scale
	if p == nil {
		return Result{}, &PreconditionError{Reason: "scale parameters are missing"}
	}

	key := in.TargetKey()
	var deploy appsv1.Deployment
	if err := fetch(ctx, c, key, &deploy, "deployment"); err != nil {
		return Result{}, err
	}

	// An unset spec.replicas means one, per the API default.
	current := int32(1)
	if deploy.Spec.Replicas != nil {
		current = *deploy.Spec.Replicas
	}
	if current == p.Replicas {
		return Result{Message: fmt.Sprintf("deployment %s already has %d replicas", deploy.Name, current)}, nil
	}

	original := deploy.DeepCopy()
	replicas := p.Replicas
	deploy.Spec.Replicas = &replicas

	if err := c.Patch(ctx, &deploy, lockedPatch(original), patchOptions(in)...); err != nil {
		return Result{}, wrapWrite(fmt.Sprintf("patch deployment %s", key), err)
	}
	return Result{Message: fmt.Sprintf("scaled deployment %s from %d to %d replicas", deploy.Name, current, p.Replicas)}, nil
}
