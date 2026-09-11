package actions

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// SetEnv sets one environment variable to a literal value on one container
// of a Deployment, adding it if absent. A variable that was sourced from a
// ConfigMap or Secret is replaced by the literal.
type SetEnv struct{}

var _ Executor = SetEnv{}

func (SetEnv) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionSetEnv }

func (SetEnv) Targets() []schema.GroupKind { return []schema.GroupKind{deploymentGK} }

func (SetEnv) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	p := in.Action.SetEnv
	if p == nil {
		return Result{}, &PreconditionError{Reason: "setEnv parameters are missing"}
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

	verb := "added"
	if env := findEnv(container, p.Name); env != nil {
		if env.ValueFrom == nil && env.Value == p.Value {
			return Result{Message: fmt.Sprintf("env %s on container %s of deployment %s already set", p.Name, p.Container, deploy.Name)}, nil
		}
		verb = "set"
		if env.ValueFrom != nil {
			verb = "replaced sourced"
		}
		env.Value = p.Value
		env.ValueFrom = nil
	} else {
		container.Env = append(container.Env, corev1.EnvVar{Name: p.Name, Value: p.Value})
	}

	if err := c.Patch(ctx, &deploy, lockedPatch(original), patchOptions(in)...); err != nil {
		return Result{}, wrapWrite(fmt.Sprintf("patch deployment %s", key), err)
	}
	// The value is deliberately not repeated here; it already sits in the
	// proposal and may be sensitive.
	return Result{Message: fmt.Sprintf("%s env %s on container %s of deployment %s", verb, p.Name, p.Container, deploy.Name)}, nil
}

// findEnv returns a pointer into the container's env list for name, or nil.
func findEnv(container *corev1.Container, name string) *corev1.EnvVar {
	for i := range container.Env {
		if container.Env[i].Name == name {
			return &container.Env[i]
		}
	}
	return nil
}
