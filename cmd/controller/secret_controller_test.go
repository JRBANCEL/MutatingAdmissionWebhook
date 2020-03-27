package main

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	kubeClient *k8sfake.Clientset
	// Objects to put in the store.
	secretsLister []*corev1.Secret
	// Actions expected to happen on the client.
	kubeActions []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.kubeobjects = []runtime.Object{}
	return f
}

func (f *fixture) newController() (*SecretController, kubeinformers.SharedInformerFactory) {
	f.kubeClient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeClient, noResyncPeriodFunc())

	c := NewController(f.kubeClient,
		k8sI.Core().V1().Secrets(), "node-ip-webhook", "webhook-cert")

	c.secretsSynced = alwaysReady
	//c.recorder = &record.FakeRecorder{}


	//for _, d := range f.deploymentLister {
	//	k8sI.Apps().V1().Deployments().Informer().GetIndexer().Add(d)
	//}

	return c, k8sI
}

func (f *fixture) run() {
	f.runController(true, false)
}

func (f *fixture) runExpectError(fooName string) {
	f.runController(true, true)
}

func (f *fixture) runController(startInformers bool, expectError bool) {
	c, k8sI := f.newController()
	stopCh := make(chan struct{})
	defer close(stopCh)
	if startInformers {
		k8sI.Start(stopCh)
	}

	err := c.Run(stopCh)
	if !expectError && err != nil {
		f.t.Errorf("error syncing foo: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing foo, got nil")
	}

	k8sActions := filterInformerActions(f.kubeClient.Actions())
	for i, action := range k8sActions {
		if len(f.kubeActions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeActions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeActions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.kubeActions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeActions)-len(k8sActions), f.kubeActions[len(k8sActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "foos") ||
				action.Matches("watch", "foos") ||
				action.Matches("list", "deployments") ||
				action.Matches("watch", "deployments")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func TestCreateSecretIfItDoesntExist(t *testing.T) {
	f := newFixture(t)
	f.run()
	//f.fooLister = append(f.fooLister, foo)
	//f.objects = append(f.objects, foo)

//	expDeployment := newDeployment(foo)
//	f.expectCreateDeploymentAction(expDeployment)
//	f.expectUpdateFooStatusAction(foo)
//
	//f.run(getKey(foo, t))
}

// func (f *fixture) expectCreateDeploymentAction(d *apps.Deployment) {
// 	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: "deployments"}, d.Namespace, d))
// }
//
// func (f *fixture) expectUpdateDeploymentAction(d *apps.Deployment) {
// 	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "deployments"}, d.Namespace, d))
// }
//
// func (f *fixture) expectUpdateFooStatusAction(foo *samplecontroller.Foo) {
// 	action := core.NewUpdateAction(schema.GroupVersionResource{Resource: "foos"}, foo.Namespace, foo)
// 	// TODO: Until #38113 is merged, we can't use Subresource
// 	//action.Subresource = "status"
// 	f.actions = append(f.actions, action)
// }
//
// func getKey(foo *samplecontroller.Foo, t *testing.T) string {
// 	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(foo)
// 	if err != nil {
// 		t.Errorf("Unexpected error getting key for foo %v: %v", foo.Name, err)
// 		return ""
// 	}
// 	return key
// }
//
// func TestCreatesDeployment(t *testing.T) {
// 	f := newFixture(t)
// 	foo := newFoo("test", int32Ptr(1))
//
// 	f.fooLister = append(f.fooLister, foo)
// 	f.objects = append(f.objects, foo)
//
// 	expDeployment := newDeployment(foo)
// 	f.expectCreateDeploymentAction(expDeployment)
// 	f.expectUpdateFooStatusAction(foo)
//
// 	f.run(getKey(foo, t))
// }
//
// func TestDoNothing(t *testing.T) {
// 	f := newFixture(t)
// 	foo := newFoo("test", int32Ptr(1))
// 	d := newDeployment(foo)
//
// 	f.fooLister = append(f.fooLister, foo)
// 	f.objects = append(f.objects, foo)
// 	f.deploymentLister = append(f.deploymentLister, d)
// 	f.kubeobjects = append(f.kubeobjects, d)
//
// 	f.expectUpdateFooStatusAction(foo)
// 	f.run(getKey(foo, t))
// }
//
// func TestUpdateDeployment(t *testing.T) {
// 	f := newFixture(t)
// 	foo := newFoo("test", int32Ptr(1))
// 	d := newDeployment(foo)
//
// 	// Update replicas
// 	foo.Spec.Replicas = int32Ptr(2)
// 	expDeployment := newDeployment(foo)
//
// 	f.fooLister = append(f.fooLister, foo)
// 	f.objects = append(f.objects, foo)
// 	f.deploymentLister = append(f.deploymentLister, d)
// 	f.kubeobjects = append(f.kubeobjects, d)
//
// 	f.expectUpdateFooStatusAction(foo)
// 	f.expectUpdateDeploymentAction(expDeployment)
// 	f.run(getKey(foo, t))
// }
//
// func TestNotControlledByUs(t *testing.T) {
// 	f := newFixture(t)
// 	foo := newFoo("test", int32Ptr(1))
// 	d := newDeployment(foo)
//
// 	d.ObjectMeta.OwnerReferences = []metav1.OwnerReference{}
//
// 	f.fooLister = append(f.fooLister, foo)
// 	f.objects = append(f.objects, foo)
// 	f.deploymentLister = append(f.deploymentLister, d)
// 	f.kubeobjects = append(f.kubeobjects, d)
//
// 	f.runExpectError(getKey(foo, t))
// }

func int32Ptr(i int32) *int32 { return &i }