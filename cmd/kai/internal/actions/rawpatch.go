package actions

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// rawPatchTargets is the allow-list of kinds RawPatch may touch. It is the
// executor's own safety property, so Apply checks it even though validation
// checks it too.
var rawPatchTargets = []schema.GroupKind{deploymentGK, serviceGK, configMapGK}

var patchTypes = map[aiv1alpha1.PatchType]types.PatchType{
	aiv1alpha1.PatchTypeJSON:      types.JSONPatchType,
	aiv1alpha1.PatchTypeMerge:     types.MergePatchType,
	aiv1alpha1.PatchTypeStrategic: types.StrategicMergePatchType,
}

// RawPatch applies an arbitrary patch body to an allow-listed kind. It is the
// escape hatch: never auto-applied, and only for kinds in rawPatchTargets.
//
// Unlike the other executors it does no read-modify-write, so it carries no
// optimistic lock. The patch is declarative; approval means "apply exactly
// this", and the digest covers the body.
type RawPatch struct{}

var _ Executor = RawPatch{}

func (RawPatch) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionRawPatch }

func (RawPatch) Targets() []schema.GroupKind { return rawPatchTargets }

func (RawPatch) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	p := in.Action.RawPatch
	if p == nil {
		return Result{}, &PreconditionError{Reason: "rawPatch parameters are missing"}
	}
	if len(p.Patch.Raw) == 0 {
		return Result{}, &PreconditionError{Reason: "rawPatch body is empty"}
	}
	patchType, ok := patchTypes[p.PatchType]
	if !ok {
		return Result{}, &PreconditionError{Reason: fmt.Sprintf("unsupported patch type %q", p.PatchType)}
	}

	gv, err := schema.ParseGroupVersion(in.Action.Target.APIVersion)
	if err != nil {
		return Result{}, &PreconditionError{Reason: fmt.Sprintf("target apiVersion %q: %v", in.Action.Target.APIVersion, err)}
	}
	gvk := gv.WithKind(in.Action.Target.Kind)
	if !allowedRawPatchTarget(gvk.GroupKind()) {
		return Result{}, &PreconditionError{Reason: fmt.Sprintf(
			"kind %s is not allow-listed for RawPatch (allowed: %s)", gvk.GroupKind(), formatKinds(rawPatchTargets))}
	}

	key := in.TargetKey()
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	if err := fetch(ctx, c, key, obj, strings.ToLower(gvk.Kind)); err != nil {
		return Result{}, err
	}

	if err := c.Patch(ctx, obj, client.RawPatch(patchType, p.Patch.Raw), patchOptions(in)...); err != nil {
		return Result{}, wrapWrite(fmt.Sprintf("patch %s %s", strings.ToLower(gvk.Kind), key), err)
	}
	return Result{Message: fmt.Sprintf("applied %s patch to %s %s", p.PatchType, strings.ToLower(gvk.Kind), key.Name)}, nil
}

func allowedRawPatchTarget(gk schema.GroupKind) bool {
	for _, a := range rawPatchTargets {
		if a == gk {
			return true
		}
	}
	return false
}

func formatKinds(gks []schema.GroupKind) string {
	parts := make([]string, 0, len(gks))
	for _, gk := range gks {
		parts = append(parts, gk.String())
	}
	return strings.Join(parts, ", ")
}
