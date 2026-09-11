package actions

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// restartedAtAnnotation is the pod template annotation `kubectl rollout
// restart` sets. Changing it changes the template, which starts a rollout.
const restartedAtAnnotation = "kubectl.kubernetes.io/restartedAt"

// now is the clock RestartRollout stamps into the template. A variable so
// tests can pin it.
var now = time.Now

// RestartRollout starts a rolling restart of a Deployment without changing
// anything else about it.
//
// It is the one executor that is not idempotent, and neither is kubectl:
// every Apply starts a new rollout. The reconciler must not re-apply an
// action it has already recorded as succeeded.
type RestartRollout struct{}

var _ Executor = RestartRollout{}

func (RestartRollout) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionRestartRollout }

func (RestartRollout) Targets() []schema.GroupKind { return []schema.GroupKind{deploymentGK} }

func (RestartRollout) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	key := in.TargetKey()
	var deploy appsv1.Deployment
	if err := fetch(ctx, c, key, &deploy, "deployment"); err != nil {
		return Result{}, err
	}

	original := deploy.DeepCopy()
	if deploy.Spec.Template.Annotations == nil {
		deploy.Spec.Template.Annotations = map[string]string{}
	}
	deploy.Spec.Template.Annotations[restartedAtAnnotation] = now().UTC().Format(time.RFC3339)

	if err := c.Patch(ctx, &deploy, lockedPatch(original), patchOptions(in)...); err != nil {
		return Result{}, wrapWrite(fmt.Sprintf("patch deployment %s", key), err)
	}
	return Result{Message: fmt.Sprintf("restarted rollout of deployment %s", deploy.Name)}, nil
}
