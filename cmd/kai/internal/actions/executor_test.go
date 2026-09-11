package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aiv1alpha1 "github.com/muraduiurie/dcnd-kubernetes-ai-agent/api/v1alpha1"
)

// The fixture cluster every test starts from: one namespace holding a
// Deployment on a bad image with two owned ReplicaSet revisions and one
// foreign ReplicaSet, an owned Pod, an orphan Pod, a terminating Pod, a
// ConfigMap with text and binary keys, an immutable ConfigMap, and a Service
// whose selector matches nothing.

const (
	ns         = "checkout"
	deployName = "checkout"
	deployUID  = types.UID("dep-uid")
	goodImage  = "busybox:1.36"
	badImage   = "non-existing-registry/busybox:latest"
)

func podSpec(appImage string) corev1.PodSpec {
	return corev1.PodSpec{
		InitContainers: []corev1.Container{{Name: "init", Image: "busybox:1.36"}},
		Containers: []corev1.Container{
			{
				Name: "app", Image: appImage,
				Env: []corev1.EnvVar{
					{Name: "DATABASE_URL", Value: "postgres://old"},
					{Name: "SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "k"}}},
				},
				Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				}},
			},
			{Name: "sidecar", Image: "envoy:1"},
		},
	}
}

func ownedBy(kind, name string, uid types.UID) []metav1.OwnerReference {
	isController := true
	return []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: kind, Name: name, UID: uid, Controller: &isController}}
}

func replicaSet(name, revision, image string, ownerUID types.UID) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns, UID: types.UID(name + "-uid"),
			Annotations:     map[string]string{revisionAnnotation: revision},
			OwnerReferences: ownedBy("Deployment", deployName, ownerUID),
		},
		Spec: appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "checkout", appsv1.DefaultDeploymentUniqueLabelKey: name}},
			Spec:       podSpec(image),
		}},
	}
}

func fixtures() []client.Object {
	three := int32(3)
	immutable := true
	deletionTime := metav1.NewTime(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	return []client.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: ns, UID: deployUID, Annotations: map[string]string{revisionAnnotation: "3"}},
			Spec: appsv1.DeploymentSpec{Replicas: &three, Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "checkout"}},
				Spec:       podSpec(badImage),
			}},
		},
		replicaSet("checkout-aaa", "2", goodImage, deployUID),
		replicaSet("checkout-bbb", "3", badImage, deployUID),
		replicaSet("other-ccc", "1", "other:1", "other-uid"),
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "checkout-bbb-x2k1", Namespace: ns, UID: "pod-uid",
			OwnerReferences: ownedBy("ReplicaSet", "checkout-bbb", "checkout-bbb-uid")}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: ns, UID: "orphan-uid"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "terminating", Namespace: ns, UID: "term-uid",
			DeletionTimestamp: &deletionTime, Finalizers: []string{"test/keep"},
			OwnerReferences: ownedBy("ReplicaSet", "checkout-bbb", "checkout-bbb-uid")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "checkout-config", Namespace: ns},
			Data: map[string]string{"DATABASE_URL": "postgres://old"}, BinaryData: map[string][]byte{"blob": {1, 2, 3}}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "frozen", Namespace: ns}, Immutable: &immutable,
			Data: map[string]string{"k": "v"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: ns},
			Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "checkout-v2"}}},
	}
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if objs == nil {
		objs = fixtures()
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

// Helpers.

func key(name string) client.ObjectKey { return client.ObjectKey{Namespace: ns, Name: name} }

func getDeploy(t *testing.T, c client.Client) *appsv1.Deployment {
	t.Helper()
	var d appsv1.Deployment
	if err := c.Get(context.Background(), key(deployName), &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

func appContainer(t *testing.T, c client.Client) *corev1.Container {
	t.Helper()
	ct := findContainer(&getDeploy(t, c).Spec.Template.Spec, "app")
	if ct == nil {
		t.Fatal("app container missing")
	}
	return ct
}

// action builds a proposal for kind/name in the fixture namespace. The
// parameter block is set by the caller.
func action(typ aiv1alpha1.ActionType, kind, name string) aiv1alpha1.ProposedAction {
	apiVersion := "v1"
	if kind == "Deployment" {
		apiVersion = "apps/v1"
	}
	return aiv1alpha1.ProposedAction{
		ID: "a1", Type: typ,
		Target:       aiv1alpha1.TargetRef{APIVersion: apiVersion, Kind: kind, Name: name},
		EvidenceRefs: []string{"ev-1"},
	}
}

func apply(t *testing.T, ex Executor, c client.Client, a aiv1alpha1.ProposedAction) Result {
	t.Helper()
	res, err := ex.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", ex.Type(), err)
	}
	return res
}

func mustPrecondition(t *testing.T, what string, err error) {
	t.Helper()
	if !IsPrecondition(err) {
		t.Fatalf("%s: expected a PreconditionError, got %v", what, err)
	}
}

// Contract.

type stubExecutor struct{ typ aiv1alpha1.ActionType }

func (s stubExecutor) Type() aiv1alpha1.ActionType { return s.typ }
func (s stubExecutor) Targets() []schema.GroupKind { return nil }
func (s stubExecutor) Apply(context.Context, client.Client, Input) (Result, error) {
	return Result{}, nil
}

func TestRegistry(t *testing.T) {
	if _, err := NewRegistry(stubExecutor{aiv1alpha1.ActionScale}, stubExecutor{aiv1alpha1.ActionScale}); err == nil {
		t.Fatal("a duplicate action type must be rejected")
	}

	r, err := NewRegistry(stubExecutor{aiv1alpha1.ActionSetImage}, stubExecutor{aiv1alpha1.ActionScale})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Types(); len(got) != 2 || got[0] != aiv1alpha1.ActionScale || got[1] != aiv1alpha1.ActionSetImage {
		t.Fatalf("Types() must be sorted, got %v", got)
	}
	if _, err := r.Lookup(aiv1alpha1.ActionRawPatch); !errors.Is(err, ErrUnknownActionType) {
		t.Fatalf("unknown type must wrap ErrUnknownActionType, got %v", err)
	}
}

func TestRegistryHoldsEveryExecutor(t *testing.T) {
	r, err := NewRegistry(
		RollbackDeployment{}, SetImage{}, Scale{}, RestartRollout{}, SetEnv{},
		SetResources{}, SetConfigMapKey{}, DeletePod{}, RawPatch{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Types(); len(got) != 9 {
		t.Fatalf("expected all nine action types, got %v", got)
	}
}

func TestIsPreconditionSeesThroughWrapping(t *testing.T) {
	inner := &PreconditionError{Reason: "no such container"}
	wrapped := errors.Join(errors.New("context"), inner)
	if !IsPrecondition(wrapped) || IsPrecondition(errors.New("plain")) {
		t.Fatal("IsPrecondition must match wrapped PreconditionErrors and nothing else")
	}
}

func TestWrapWriteClassifiesAPIErrors(t *testing.T) {
	gk := schema.GroupKind{Group: "apps", Kind: "Deployment"}
	if !IsPrecondition(wrapWrite("op", apierrors.NewInvalid(gk, "checkout", nil))) {
		t.Fatal("422 Invalid must be permanent")
	}
	if !IsPrecondition(wrapWrite("op", apierrors.NewBadRequest("malformed"))) {
		t.Fatal("400 BadRequest must be permanent")
	}
	conflict := apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "checkout", errors.New("modified"))
	wrapped := wrapWrite("op", conflict)
	if IsPrecondition(wrapped) || !apierrors.IsConflict(wrapped) {
		t.Fatal("409 Conflict must stay transient and remain recognisable")
	}
	if wrapWrite("op", nil) != nil {
		t.Fatal("nil in, nil out")
	}
}

// Executors, in the order of their source files.

func TestSetImage(t *testing.T) {
	c := newFakeClient(t)
	a := action(aiv1alpha1.ActionSetImage, "Deployment", deployName)
	a.SetImage = &aiv1alpha1.SetImageParams{Container: "app", Image: goodImage}

	res := apply(t, SetImage{}, c, a)
	if got := appContainer(t, c).Image; got != goodImage {
		t.Fatalf("image = %q", got)
	}
	if got := getDeploy(t, c).Spec.Template.Spec.Containers[1].Image; got != "envoy:1" {
		t.Fatalf("sidecar must be untouched, got %q", got)
	}
	if want := "set image of container app on deployment checkout from " + badImage + " to " + goodImage; res.Message != want {
		t.Fatalf("message = %q", res.Message)
	}

	if res := apply(t, SetImage{}, c, a); !strings.Contains(res.Message, "already runs") {
		t.Fatalf("second apply must be a no-op, got %q", res.Message)
	}

	a.SetImage = &aiv1alpha1.SetImageParams{Container: "init", Image: "busybox:1.37"}
	apply(t, SetImage{}, c, a)
	if got := getDeploy(t, c).Spec.Template.Spec.InitContainers[0].Image; got != "busybox:1.37" {
		t.Fatalf("init container must be reachable, got %q", got)
	}

	a.SetImage = &aiv1alpha1.SetImageParams{Container: "nope", Image: "x"}
	_, err := SetImage{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "unknown container", err)

	_, err = SetImage{}.Apply(context.Background(), c, Input{Namespace: "elsewhere", Action: a})
	mustPrecondition(t, "missing deployment", err)

	a.SetImage = nil
	_, err = SetImage{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "nil parameters", err)
}

func TestScale(t *testing.T) {
	c := newFakeClient(t)
	a := action(aiv1alpha1.ActionScale, "Deployment", deployName)
	a.Scale = &aiv1alpha1.ScaleParams{Replicas: 0}

	res := apply(t, Scale{}, c, a)
	if got := *getDeploy(t, c).Spec.Replicas; got != 0 {
		t.Fatalf("replicas = %d", got)
	}
	if res.Message != "scaled deployment checkout from 3 to 0 replicas" {
		t.Fatalf("message = %q", res.Message)
	}
	if res := apply(t, Scale{}, c, a); res.Message != "deployment checkout already has 0 replicas" {
		t.Fatalf("second apply must be a no-op, got %q", res.Message)
	}

	a.Scale = nil
	_, err := Scale{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "nil parameters", err)
}

func TestScaleTreatsUnsetReplicasAsOne(t *testing.T) {
	c := newFakeClient(t, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: ns}})
	a := action(aiv1alpha1.ActionScale, "Deployment", deployName)

	a.Scale = &aiv1alpha1.ScaleParams{Replicas: 1}
	if res := apply(t, Scale{}, c, a); !strings.Contains(res.Message, "already has 1") {
		t.Fatalf("unset replicas must count as one, got %q", res.Message)
	}
	a.Scale = &aiv1alpha1.ScaleParams{Replicas: 2}
	if res := apply(t, Scale{}, c, a); res.Message != "scaled deployment checkout from 1 to 2 replicas" {
		t.Fatalf("message = %q", res.Message)
	}
}

func TestRestartRollout(t *testing.T) {
	c := newFakeClient(t)
	a := action(aiv1alpha1.ActionRestartRollout, "Deployment", deployName)
	t.Cleanup(func() { now = time.Now })

	first := time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)
	now = func() time.Time { return first }
	res := apply(t, RestartRollout{}, c, a)
	if got := getDeploy(t, c).Spec.Template.Annotations[restartedAtAnnotation]; got != "2026-10-30T14:00:00Z" {
		t.Fatalf("annotation = %q", got)
	}
	if res.Message != "restarted rollout of deployment checkout" {
		t.Fatalf("message = %q", res.Message)
	}

	// Deliberately not idempotent, like kubectl: a later apply starts a new
	// rollout. The reconciler is what must not re-apply a succeeded action.
	now = func() time.Time { return first.Add(time.Minute) }
	apply(t, RestartRollout{}, c, a)
	if got := getDeploy(t, c).Spec.Template.Annotations[restartedAtAnnotation]; got != "2026-10-30T14:01:00Z" {
		t.Fatalf("second apply must restamp, got %q", got)
	}
}

func TestRollbackDeployment(t *testing.T) {
	c := newFakeClient(t)
	a := action(aiv1alpha1.ActionRollbackDeployment, "Deployment", deployName)
	a.Rollback = &aiv1alpha1.RollbackParams{ToRevision: 2}

	res := apply(t, RollbackDeployment{}, c, a)
	d := getDeploy(t, c)
	if got := d.Spec.Template.Spec.Containers[0].Image; got != goodImage {
		t.Fatalf("image after rollback = %q", got)
	}
	if _, leaked := d.Spec.Template.Labels[appsv1.DefaultDeploymentUniqueLabelKey]; leaked {
		t.Fatal("pod-template-hash must be stripped from the restored template")
	}
	if res.Message != "rolled back deployment checkout to revision 2 (app="+goodImage+", sidecar=envoy:1)" {
		t.Fatalf("message = %q", res.Message)
	}

	if res := apply(t, RollbackDeployment{}, c, a); res.Message != "deployment checkout is already at revision 2" {
		t.Fatalf("second apply must be a no-op, got %q", res.Message)
	}

	a.Rollback = &aiv1alpha1.RollbackParams{ToRevision: 9}
	_, err := RollbackDeployment{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "missing revision", err)
	if !strings.Contains(err.Error(), "available: 2, 3") {
		t.Fatalf("available revisions must list only owned ReplicaSets, got %v", err)
	}

	a.Rollback = nil
	_, err = RollbackDeployment{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "nil parameters", err)
}

func TestSetEnv(t *testing.T) {
	c := newFakeClient(t)
	a := action(aiv1alpha1.ActionSetEnv, "Deployment", deployName)

	cases := []struct{ name, value, wantPrefix string }{
		{"NEW_FLAG", "flag-value-1", "added env NEW_FLAG"},
		{"DATABASE_URL", "postgres://new", "set env DATABASE_URL"},
		{"SECRET", "literal", "replaced sourced env SECRET"},
	}
	for _, tc := range cases {
		a.SetEnv = &aiv1alpha1.SetEnvParams{Container: "app", Name: tc.name, Value: tc.value}
		res := apply(t, SetEnv{}, c, a)
		if !strings.HasPrefix(res.Message, tc.wantPrefix) {
			t.Fatalf("%s: message = %q", tc.name, res.Message)
		}
		if strings.Contains(res.Message, tc.value) {
			t.Fatalf("%s: the value must not be repeated in the message", tc.name)
		}
		env := findEnv(appContainer(t, c), tc.name)
		if env == nil || env.Value != tc.value || env.ValueFrom != nil {
			t.Fatalf("%s: env not set as a literal: %+v", tc.name, env)
		}
	}
	if res := apply(t, SetEnv{}, c, a); !strings.Contains(res.Message, "already set") {
		t.Fatalf("second apply must be a no-op, got %q", res.Message)
	}
	if got := getDeploy(t, c).Spec.Template.Spec.Containers[1]; len(got.Env) != 0 {
		t.Fatal("sidecar env must be untouched")
	}

	a.SetEnv = &aiv1alpha1.SetEnvParams{Container: "nope", Name: "X", Value: "y"}
	_, err := SetEnv{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "unknown container", err)

	a.SetEnv = nil
	_, err = SetEnv{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "nil parameters", err)
}

func TestSetResources(t *testing.T) {
	c := newFakeClient(t)
	a := action(aiv1alpha1.ActionSetResources, "Deployment", deployName)
	a.SetResources = &aiv1alpha1.SetResourcesParams{Container: "app", Resources: corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")},
	}}

	res := apply(t, SetResources{}, c, a)
	got := appContainer(t, c).Resources
	if got.Limits.Memory().String() != "512Mi" || got.Requests.Cpu().String() != "100m" {
		t.Fatalf("resources = %+v", got)
	}
	if _, stillThere := got.Limits[corev1.ResourceCPU]; stillThere {
		t.Fatal("a limit absent from the proposal must be removed, not kept")
	}
	if res.Message != "set resources of container app on deployment checkout to requests cpu=100m,memory=128Mi; limits memory=512Mi" {
		t.Fatalf("message = %q", res.Message)
	}
	if res := apply(t, SetResources{}, c, a); !strings.Contains(res.Message, "already has") {
		t.Fatalf("second apply must be a no-op, got %q", res.Message)
	}

	a.SetResources = nil
	_, err := SetResources{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "nil parameters", err)
}

func TestSetConfigMapKey(t *testing.T) {
	c := newFakeClient(t)
	a := action(aiv1alpha1.ActionSetConfigMapKey, "ConfigMap", "checkout-config")

	a.SetConfigMapKey = &aiv1alpha1.SetConfigMapKeyParams{Key: "DATABASE_URL", Value: "postgres://new"}
	if res := apply(t, SetConfigMapKey{}, c, a); res.Message != "set key DATABASE_URL in configmap checkout-config" {
		t.Fatalf("message = %q", res.Message)
	}
	a.SetConfigMapKey = &aiv1alpha1.SetConfigMapKeyParams{Key: "DB_URL", Value: "postgres://new"}
	if res := apply(t, SetConfigMapKey{}, c, a); res.Message != "added key DB_URL in configmap checkout-config" {
		t.Fatalf("message = %q", res.Message)
	}
	if res := apply(t, SetConfigMapKey{}, c, a); !strings.Contains(res.Message, "already set") {
		t.Fatalf("second apply must be a no-op, got %q", res.Message)
	}

	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), key("checkout-config"), &cm); err != nil {
		t.Fatal(err)
	}
	if cm.Data["DATABASE_URL"] != "postgres://new" || cm.Data["DB_URL"] != "postgres://new" {
		t.Fatalf("data = %v", cm.Data)
	}
	if len(cm.BinaryData["blob"]) != 3 {
		t.Fatal("binaryData must survive a data patch")
	}

	a.SetConfigMapKey = &aiv1alpha1.SetConfigMapKeyParams{Key: "blob", Value: "text"}
	_, err := SetConfigMapKey{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "key in binaryData", err)

	a = action(aiv1alpha1.ActionSetConfigMapKey, "ConfigMap", "frozen")
	a.SetConfigMapKey = &aiv1alpha1.SetConfigMapKeyParams{Key: "k", Value: "v2"}
	_, err = SetConfigMapKey{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "immutable configmap", err)

	a.SetConfigMapKey = nil
	_, err = SetConfigMapKey{}.Apply(context.Background(), c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "nil parameters", err)
}

func TestDeletePod(t *testing.T) {
	c := newFakeClient(t)
	ctx := context.Background()

	_, err := DeletePod{}.Apply(ctx, c, Input{Namespace: ns, Action: action(aiv1alpha1.ActionDeletePod, "Pod", "orphan")})
	mustPrecondition(t, "orphan pod", err)
	if !strings.Contains(err.Error(), "no controller") {
		t.Fatalf("reason must say why, got %v", err)
	}

	res := apply(t, DeletePod{}, c, action(aiv1alpha1.ActionDeletePod, "Pod", "terminating"))
	if !strings.Contains(res.Message, "already terminating") {
		t.Fatalf("terminating pod must be a no-op, got %q", res.Message)
	}

	a := action(aiv1alpha1.ActionDeletePod, "Pod", "checkout-bbb-x2k1")
	res = apply(t, DeletePod{}, c, a)
	if res.Message != "deleted pod checkout-bbb-x2k1 (owned by ReplicaSet checkout-bbb)" {
		t.Fatalf("message = %q", res.Message)
	}
	var pod corev1.Pod
	if err := c.Get(ctx, key("checkout-bbb-x2k1"), &pod); !apierrors.IsNotFound(err) {
		t.Fatalf("pod must be gone, got %v", err)
	}

	_, err = DeletePod{}.Apply(ctx, c, Input{Namespace: ns, Action: a})
	mustPrecondition(t, "already deleted pod", err)
}

func TestRawPatch(t *testing.T) {
	c := newFakeClient(t)
	ctx := context.Background()

	a := action(aiv1alpha1.ActionRawPatch, "Service", "checkout")
	a.RawPatch = &aiv1alpha1.RawPatchParams{PatchType: aiv1alpha1.PatchTypeJSON,
		Patch: runtime.RawExtension{Raw: []byte(`[{"op":"replace","path":"/spec/selector/app","value":"checkout"}]`)}}
	res := apply(t, RawPatch{}, c, a)
	var svc corev1.Service
	if err := c.Get(ctx, key("checkout"), &svc); err != nil {
		t.Fatal(err)
	}
	if svc.Spec.Selector["app"] != "checkout" {
		t.Fatalf("selector = %v", svc.Spec.Selector)
	}
	if res.Message != "applied json patch to service checkout" {
		t.Fatalf("message = %q", res.Message)
	}

	a = action(aiv1alpha1.ActionRawPatch, "ConfigMap", "checkout-config")
	a.RawPatch = &aiv1alpha1.RawPatchParams{PatchType: aiv1alpha1.PatchTypeMerge,
		Patch: runtime.RawExtension{Raw: []byte(`{"data":{"FEATURE":"on"}}`)}}
	apply(t, RawPatch{}, c, a)
	var cm corev1.ConfigMap
	if err := c.Get(ctx, key("checkout-config"), &cm); err != nil || cm.Data["FEATURE"] != "on" {
		t.Fatalf("merge patch not applied: %v %v", err, cm.Data)
	}

	preconditions := []struct {
		name string
		a    aiv1alpha1.ProposedAction
	}{
		{"kind not allow-listed", func() aiv1alpha1.ProposedAction {
			p := action(aiv1alpha1.ActionRawPatch, "Pod", "checkout-bbb-x2k1")
			p.RawPatch = &aiv1alpha1.RawPatchParams{PatchType: aiv1alpha1.PatchTypeMerge, Patch: runtime.RawExtension{Raw: []byte(`{}`)}}
			return p
		}()},
		{"empty body", func() aiv1alpha1.ProposedAction {
			p := action(aiv1alpha1.ActionRawPatch, "Service", "checkout")
			p.RawPatch = &aiv1alpha1.RawPatchParams{PatchType: aiv1alpha1.PatchTypeMerge}
			return p
		}()},
		{"unsupported patch type", func() aiv1alpha1.ProposedAction {
			p := action(aiv1alpha1.ActionRawPatch, "Service", "checkout")
			p.RawPatch = &aiv1alpha1.RawPatchParams{PatchType: "bogus", Patch: runtime.RawExtension{Raw: []byte(`{}`)}}
			return p
		}()},
		{"missing target", func() aiv1alpha1.ProposedAction {
			p := action(aiv1alpha1.ActionRawPatch, "Service", "nope")
			p.RawPatch = &aiv1alpha1.RawPatchParams{PatchType: aiv1alpha1.PatchTypeMerge, Patch: runtime.RawExtension{Raw: []byte(`{}`)}}
			return p
		}()},
		{"nil parameters", action(aiv1alpha1.ActionRawPatch, "Service", "checkout")},
	}
	for _, tc := range preconditions {
		_, err := RawPatch{}.Apply(ctx, c, Input{Namespace: ns, Action: tc.a})
		mustPrecondition(t, tc.name, err)
	}
}

// Cross-cutting: a dry-run never persists, for every executor, and a real
// apply does. The fake client bumps resourceVersion on every write, so
// comparing it before and after is a generic change detector.

func TestDryRunPersistsNothing(t *testing.T) {
	rollback := action(aiv1alpha1.ActionRollbackDeployment, "Deployment", deployName)
	rollback.Rollback = &aiv1alpha1.RollbackParams{ToRevision: 2}
	setImage := action(aiv1alpha1.ActionSetImage, "Deployment", deployName)
	setImage.SetImage = &aiv1alpha1.SetImageParams{Container: "app", Image: goodImage}
	scale := action(aiv1alpha1.ActionScale, "Deployment", deployName)
	scale.Scale = &aiv1alpha1.ScaleParams{Replicas: 0}
	restart := action(aiv1alpha1.ActionRestartRollout, "Deployment", deployName)
	setEnv := action(aiv1alpha1.ActionSetEnv, "Deployment", deployName)
	setEnv.SetEnv = &aiv1alpha1.SetEnvParams{Container: "app", Name: "X", Value: "y"}
	setResources := action(aiv1alpha1.ActionSetResources, "Deployment", deployName)
	setResources.SetResources = &aiv1alpha1.SetResourcesParams{Container: "app",
		Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")}}}
	setKey := action(aiv1alpha1.ActionSetConfigMapKey, "ConfigMap", "checkout-config")
	setKey.SetConfigMapKey = &aiv1alpha1.SetConfigMapKeyParams{Key: "X", Value: "y"}
	deletePod := action(aiv1alpha1.ActionDeletePod, "Pod", "checkout-bbb-x2k1")
	rawPatch := action(aiv1alpha1.ActionRawPatch, "Service", "checkout")
	rawPatch.RawPatch = &aiv1alpha1.RawPatchParams{PatchType: aiv1alpha1.PatchTypeMerge,
		Patch: runtime.RawExtension{Raw: []byte(`{"metadata":{"labels":{"touched":"yes"}}}`)}}

	cases := []struct {
		ex       Executor
		a        aiv1alpha1.ProposedAction
		target   func() client.Object
		wantGone bool
	}{
		{RollbackDeployment{}, rollback, func() client.Object { return &appsv1.Deployment{} }, false},
		{SetImage{}, setImage, func() client.Object { return &appsv1.Deployment{} }, false},
		{Scale{}, scale, func() client.Object { return &appsv1.Deployment{} }, false},
		{RestartRollout{}, restart, func() client.Object { return &appsv1.Deployment{} }, false},
		{SetEnv{}, setEnv, func() client.Object { return &appsv1.Deployment{} }, false},
		{SetResources{}, setResources, func() client.Object { return &appsv1.Deployment{} }, false},
		{SetConfigMapKey{}, setKey, func() client.Object { return &corev1.ConfigMap{} }, false},
		{DeletePod{}, deletePod, func() client.Object { return &corev1.Pod{} }, true},
		{RawPatch{}, rawPatch, func() client.Object { return &corev1.Service{} }, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.ex.Type()), func(t *testing.T) {
			c := newFakeClient(t)
			ctx := context.Background()
			k := key(tc.a.Target.Name)
			version := func() string {
				obj := tc.target()
				if err := c.Get(ctx, k, obj); err != nil {
					t.Fatalf("get target: %v", err)
				}
				return obj.GetResourceVersion()
			}

			before := version()
			if _, err := tc.ex.Apply(ctx, c, Input{Namespace: ns, Action: tc.a, DryRun: true}); err != nil {
				t.Fatalf("dry-run: %v", err)
			}
			if after := version(); after != before {
				t.Fatalf("dry-run changed the target: resourceVersion %s -> %s", before, after)
			}

			if _, err := tc.ex.Apply(ctx, c, Input{Namespace: ns, Action: tc.a}); err != nil {
				t.Fatalf("apply: %v", err)
			}
			if tc.wantGone {
				if err := c.Get(ctx, k, tc.target()); !apierrors.IsNotFound(err) {
					t.Fatalf("target must be gone after apply, got %v", err)
				}
				return
			}
			if after := version(); after == before {
				t.Fatal("real apply must change the target")
			}
		})
	}
}
