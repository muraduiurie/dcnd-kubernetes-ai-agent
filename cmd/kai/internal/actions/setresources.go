package actions

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// SetResources replaces the resource requirements of one container of a
// Deployment. The whole requirements block is replaced, so a limit absent
// from the proposal is removed, not kept.
type SetResources struct{}

var _ Executor = SetResources{}

func (SetResources) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionSetResources }

func (SetResources) Targets() []schema.GroupKind { return []schema.GroupKind{deploymentGK} }

func (SetResources) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	p := in.Action.SetResources
	if p == nil {
		return Result{}, &PreconditionError{Reason: "setResources parameters are missing"}
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
	if apiequality.Semantic.DeepEqual(container.Resources, p.Resources) {
		return Result{Message: fmt.Sprintf("container %s of deployment %s already has %s", p.Container, deploy.Name, formatResources(p.Resources))}, nil
	}
	container.Resources = p.Resources

	if err := c.Patch(ctx, &deploy, lockedPatch(original), patchOptions(in)...); err != nil {
		return Result{}, wrapWrite(fmt.Sprintf("patch deployment %s", key), err)
	}
	return Result{Message: fmt.Sprintf("set resources of container %s on deployment %s to %s", p.Container, deploy.Name, formatResources(p.Resources))}, nil
}

// formatResources renders requirements as "requests cpu=100m,memory=128Mi;
// limits memory=256Mi", with keys sorted so the message is stable.
func formatResources(r corev1.ResourceRequirements) string {
	part := func(label string, list corev1.ResourceList) string {
		if len(list) == 0 {
			return ""
		}
		keys := make([]string, 0, len(list))
		for k := range list {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		items := make([]string, 0, len(keys))
		for _, k := range keys {
			q := list[corev1.ResourceName(k)]
			items = append(items, k+"="+q.String())
		}
		return label + " " + strings.Join(items, ",")
	}
	var parts []string
	if s := part("requests", r.Requests); s != "" {
		parts = append(parts, s)
	}
	if s := part("limits", r.Limits); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "no requests or limits"
	}
	return strings.Join(parts, "; ")
}
