// Command kai runs the kubernetes-ai operator.
//
// kai owns both CRDs of the ai.dncp.io group. It provisions a runtime for
// every AIAgent and drives every AgentRun from admission to a terminal phase.
// It never calls a model.
//
// The name is short for kubernetes-ai. It is also the author's dog.
package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
	"github.com/muraduiurie/dcnd-kubernetes-ai-agent/cmd/kai/internal/controllers"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(aiv1alpha1.AddToScheme(scheme))
}

func main() {
	// Production defaults: JSON, info level. Pass --zap-devel for a console
	// encoder and debug level on stage, --zap-log-level to tune either.
	zapOpts := zap.Options{}
	zapOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zapOpts)))
	log := ctrl.Log.WithName("kai")

	// One context for the whole process. It is cancelled on SIGTERM or SIGINT
	// and reaches every Reconcile call through the manager.
	ctx := ctrl.SetupSignalHandler()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	for _, c := range controllers.All(mgr) {
		if err := c.SetupWithManager(mgr); err != nil {
			log.Error(err, "unable to register controller", "controller", fmt.Sprintf("%T", c))
			os.Exit(1)
		}
	}

	log.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		log.Error(err, "manager exited with error")
		os.Exit(1)
	}
}
