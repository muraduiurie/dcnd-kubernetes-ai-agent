// Package actions is kai's closed write-side vocabulary. Every ActionType has
// exactly one Executor here, and this package is the only place in the
// repository that changes workload objects. It lives under cmd/kai/internal
// so the runtime cannot import it.
//
// An Executor is deterministic: the same Input against the same cluster state
// produces the same API call. Dry-run and apply share one code path, told
// apart only by Input.DryRun, so the dry-run shown to a human before approval
// exercises exactly the write that approval will trigger.
package actions

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// Input is everything an Executor needs for one action. The target is always
// in Namespace; the reconciler enforces that before an Executor sees it.
type Input struct {
	// Namespace is the AgentRun's namespace.
	Namespace string

	// Action is the proposal as stored in AgentRun status, after the digest
	// check has passed.
	Action aiv1alpha1.ProposedAction

	// DryRun sends the write with dryRun=All so nothing persists.
	DryRun bool
}

// TargetKey returns the object key of the action's target.
func (in Input) TargetKey() client.ObjectKey {
	return client.ObjectKey{Namespace: in.Namespace, Name: in.Action.Target.Name}
}

// Result describes what an Executor did, for AgentRun status.
type Result struct {
	// Message is one human-readable line, for example
	// "rolled back checkout to revision 3".
	Message string
}

// Executor applies one ActionType.
type Executor interface {
	// Type is the ActionType this Executor implements. The Registry keys on
	// it.
	Type() aiv1alpha1.ActionType

	// Targets lists the kinds this Executor may change. Validation rejects a
	// proposal whose target kind is not listed.
	Targets() []schema.GroupKind

	// Apply reads the target, builds the change, and writes it. It returns a
	// *PreconditionError when the target's current state rules the action
	// out, for example a container or revision that does not exist. Any
	// other error is treated as transient and retried.
	Apply(ctx context.Context, c client.Client, in Input) (Result, error)
}

// PreconditionError means the action cannot be applied to the target as it
// is, and retrying will not change that. The reconciler records the reason
// and does not requeue.
type PreconditionError struct {
	Reason string
}

func (e *PreconditionError) Error() string {
	return "precondition failed: " + e.Reason
}

// IsPrecondition reports whether err is, or wraps, a *PreconditionError.
func IsPrecondition(err error) bool {
	var pe *PreconditionError
	return errors.As(err, &pe)
}

// ErrUnknownActionType is returned by Registry.Lookup for a type with no
// Executor. The CRD enum should make this unreachable; the check exists so a
// schema and a registry that drift apart fail loudly instead of silently.
var ErrUnknownActionType = errors.New("no executor for action type")

// Registry maps every ActionType to its Executor.
type Registry struct {
	byType map[aiv1alpha1.ActionType]Executor
}

// NewRegistry builds a Registry from the given Executors. It fails on a
// duplicate type, so wiring two Executors to one ActionType is caught at
// startup rather than at apply time.
func NewRegistry(execs ...Executor) (*Registry, error) {
	r := &Registry{byType: make(map[aiv1alpha1.ActionType]Executor, len(execs))}
	for _, e := range execs {
		t := e.Type()
		if _, dup := r.byType[t]; dup {
			return nil, fmt.Errorf("duplicate executor for action type %q", t)
		}
		r.byType[t] = e
	}
	return r, nil
}

// Lookup returns the Executor for t.
func (r *Registry) Lookup(t aiv1alpha1.ActionType) (Executor, error) {
	e, ok := r.byType[t]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownActionType, t)
	}
	return e, nil
}

// Types returns the registered ActionTypes in a stable order.
func (r *Registry) Types() []aiv1alpha1.ActionType {
	out := make([]aiv1alpha1.ActionType, 0, len(r.byType))
	for t := range r.byType {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
