// Package controllers holds kai's reconcilers, one per CRD of the ai.dncp.io
// group. main registers everything returned by All with the manager.
package controllers

import (
	ctrl "sigs.k8s.io/controller-runtime"
)

// Controller is a reconciler that knows how to register itself with a
// manager.
type Controller interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// All returns every controller kai runs, wired to the manager's client and
// scheme. New reconcilers are added here and nowhere else.
func All(mgr ctrl.Manager) []Controller {
	return []Controller{
		&AIAgentReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
		&AgentRunReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()},
	}
}
