package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// AIAgentReconciler provisions the runtime Deployment and the read-only
// agent-reader identities for an agent definition.
type AIAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile is invoked for every AIAgent event.
func (r *AIAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconcile loop for AIAgent.
func (r *AIAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1alpha1.AIAgent{}).
		Complete(r)
}
