package actions

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// revisionAnnotation is set by the Deployment controller on every ReplicaSet
// it creates, and on the Deployment itself for the current revision.
const revisionAnnotation = "deployment.kubernetes.io/revision"

// RollbackDeployment restores the pod template of an earlier ReplicaSet
// revision into the Deployment. It is the executor behind what
// `kubectl rollout undo --to-revision` does.
//
// The revision must still exist as a ReplicaSet. Kubernetes keeps
// revisionHistoryLimit of them, ten by default, so a rollback to something
// older than that is a precondition failure, not a search.
type RollbackDeployment struct{}

var _ Executor = RollbackDeployment{}

func (RollbackDeployment) Type() aiv1alpha1.ActionType { return aiv1alpha1.ActionRollbackDeployment }

func (RollbackDeployment) Targets() []schema.GroupKind { return []schema.GroupKind{deploymentGK} }

func (RollbackDeployment) Apply(ctx context.Context, c client.Client, in Input) (Result, error) {
	p := in.Action.Rollback
	if p == nil {
		return Result{}, &PreconditionError{Reason: "rollback parameters are missing"}
	}

	key := in.TargetKey()
	var deploy appsv1.Deployment
	if err := fetch(ctx, c, key, &deploy, "deployment"); err != nil {
		return Result{}, err
	}

	var rsList appsv1.ReplicaSetList
	if err := c.List(ctx, &rsList, client.InNamespace(in.Namespace)); err != nil {
		return Result{}, fmt.Errorf("list replicasets in %s: %w", in.Namespace, err)
	}

	wanted := strconv.FormatInt(p.ToRevision, 10)
	var target *appsv1.ReplicaSet
	var available []string
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		owner := metav1.GetControllerOf(rs)
		if owner == nil || owner.UID != deploy.UID {
			continue
		}
		rev := rs.Annotations[revisionAnnotation]
		available = append(available, rev)
		if rev == wanted {
			target = rs
		}
	}
	if target == nil {
		sort.Strings(available)
		return Result{}, &PreconditionError{Reason: fmt.Sprintf(
			"deployment %s has no revision %d (available: %s)", key, p.ToRevision, strings.Join(available, ", "))}
	}

	// The ReplicaSet template is the Deployment template of that revision
	// plus the pod-template-hash label the controller added. Strip it, or the
	// Deployment would carry a stale hash forward.
	template := *target.Spec.Template.DeepCopy()
	delete(template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)

	if apiequality.Semantic.DeepEqual(deploy.Spec.Template, template) {
		return Result{Message: fmt.Sprintf("deployment %s is already at revision %d", deploy.Name, p.ToRevision)}, nil
	}

	original := deploy.DeepCopy()
	deploy.Spec.Template = template

	if err := c.Patch(ctx, &deploy, lockedPatch(original), patchOptions(in)...); err != nil {
		return Result{}, wrapWrite(fmt.Sprintf("patch deployment %s", key), err)
	}
	return Result{Message: fmt.Sprintf("rolled back deployment %s to revision %d (%s)", deploy.Name, p.ToRevision, imagesOf(template.Spec))}, nil
}

// imagesOf renders the container images of a pod spec as "name=image, ...".
func imagesOf(spec corev1.PodSpec) string {
	parts := make([]string, 0, len(spec.Containers))
	for _, ct := range spec.Containers {
		parts = append(parts, ct.Name+"="+ct.Image)
	}
	return strings.Join(parts, ", ")
}
