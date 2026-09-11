package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// AgentRunReconciler drives a run from admission to a terminal phase:
// admit, digest, dry-run, apply on approval, verify.
type AgentRunReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is invoked for every AgentRun event.
func (r *AgentRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconcile loop for AgentRun.
func (r *AgentRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1alpha1.AgentRun{}).
		Complete(r)
}
