// Package v1alpha1 contains the API types for the ai.dncp.io group.
//
// AIAgent is the cluster-scoped agent definition. AgentRun is the namespaced
// record of one bounded investigation.
//
// +kubebuilder:object:generate=true
// +groupName=ai.dncp.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const GroupName = "ai.dncp.io"
const APIVersion = "v1alpha1"

var (
	// GroupVersion is the group and version of every type in this package.
	GroupVersion = schema.GroupVersion{Group: GroupName, Version: APIVersion}

	// SchemeBuilder collects the types registered in this package.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds this group's types to a runtime.Scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&AgentRun{},
		&AgentRunList{},
		&AIAgent{},
		&AIAgentList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
