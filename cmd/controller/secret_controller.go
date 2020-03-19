package main

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const controllerAgentName = "sample-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Foo is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Foo fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Foo"
	// MessageResourceSynced is the message used for an Event fired when a Foo
	// is synced successfully
	MessageResourceSynced = "Foo synced successfully"
)

var (
	expirationThreshold = 30 * 24 * time.Hour
)

// Controller is the controller implementation for Foo resources
type SecretController struct {
	kubeClient kubernetes.Interface

	secretNamespace string
	secretName      string

	secretsLister corelisters.SecretLister
	secretsSynced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewController(
	kubeclientset kubernetes.Interface,
	secretInformer coreinformers.SecretInformer,
	secretNamespace string,
	secretName string) *SecretController {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	//utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	//eventBroadcaster := record.NewBroadcaster()
	//eventBroadcaster.StartLogging(klog.Infof)
	//eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	//recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &SecretController{
		kubeClient:      kubeclientset,
		secretNamespace: secretNamespace,
		secretName:      secretName,
		secretsLister:   secretInformer.Lister(),
		secretsSynced:   secretInformer.Informer().HasSynced,
		workqueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Webhook-Secret"),
		//recorder:          recorder,
	}

	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a Foo resource will enqueue that Foo resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			//newDepl := new.(*corev1.Secret)
			//oldDepl := old.(*corev1.Secret)
			//if newDepl.ResourceVersion == oldDepl.ResourceVersion {
			//	// Periodic resync will send update events for all known Deployments.
			//	// Two different versions of the same Deployment will always have different RVs.
			//	return
			//}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *SecretController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Foo controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.secretsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting worker")
	// Launch two workers to process Foo resources
	go wait.Until(c.runWorker, time.Second, stopCh)

	klog.Info("Started workers")
	go func() {
		wait.PollImmediateUntil(1*time.Hour, func() (bool, error) {
			c.triggerReconciliation()
			return false, nil
		}, stopCh)
	}()
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

func (c *SecretController) triggerReconciliation() {
	c.workqueue.Add(struct{}{})
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *SecretController) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *SecretController) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		//var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		//if key, ok = obj.(string); !ok {
		//	// As the item in the workqueue is actually invalid, we call
		//	// Forget here else we'd go into a loop of attempting to
		//	// process a work item that is invalid.
		//	c.workqueue.Forget(obj)
		//	utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
		//	return nil
		//}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
	}

	return true
}

func (c *SecretController) createSecret() error {
	data, err := generateSecretData()
	if err != nil {
		return fmt.Errorf("failed to generate the Secret data: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:                  c.secretNamespace,
			Name:                       c.secretName,
		},
		Data:data,
	}
	_, err = c.kubeClient.CoreV1().Secrets(c.secretNamespace).Create(secret)
	return err
}

func (c *SecretController) updateSecret(secret *corev1.Secret) error {
	data, err := generateSecretData()
	if err != nil {
		return fmt.Errorf("failed to generate the Secret data: %w", err)
	}

	secret = secret.DeepCopy()
	secret.Data = data
	_, err = c.kubeClient.CoreV1().Secrets(c.secretNamespace).Update(secret)
	return err
}


// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Foo resource
// with the current status of the resource.
func (c *SecretController) syncHandler(key string) error {
	secret, err := c.secretsLister.Secrets(c.secretNamespace).Get(c.secretName)
	if err != nil {
		if errors.IsNotFound(err) {
			// If the Secret doesn't exist, it needs to be created.
			klog.Infof("The Secret %s/%s was not found, creating it.", c.secretNamespace, c.secretName)
			return c.createSecret()
		}
		return err
	}

	// If the Secret is close to expiration, it needs to be refreshed
	durationBeforeExpiration, err := getDurationBeforeExpiration(secret)
	if err != nil {
		return err
	}
	if durationBeforeExpiration < expirationThreshold {
		klog.Infof("The certificate is expiring soon (%v), refreshing it.", durationBeforeExpiration)
		return c.updateSecret(secret)
	}

	klog.Infof("The certificate is not expiring soon (%v), doing nothing.", durationBeforeExpiration)

	//deploymentName := foo.Spec.DeploymentName
	//if deploymentName == "" {
	//	// We choose to absorb the error here as the worker would requeue the
	//	// resource otherwise. Instead, the next time the resource is updated
	//	// the resource will be queued again.
	//	utilruntime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
	//	return nil
	//}

	//// Get the deployment with the name specified in Foo.spec
	//deployment, err := c.deploymentsLister.Deployments(foo.Namespace).Get(deploymentName)
	//// If the resource doesn't exist, we'll create it
	//if errors.IsNotFound(err) {
	//	deployment, err = c.kubeclientset.AppsV1().Deployments(foo.Namespace).Create(context.TODO(), newDeployment(foo), metav1.CreateOptions{})
	//}

	//// If an error occurs during Get/Create, we'll requeue the item so we can
	//// attempt processing again later. This could have been caused by a
	//// temporary network failure, or any other transient reason.
	//if err != nil {
	//	return err
	//}

	//// If the Deployment is not controlled by this Foo resource, we should log
	//// a warning to the event recorder and return error msg.
	//if !metav1.IsControlledBy(deployment, foo) {
	//	msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
	//	c.recorder.Event(foo, corev1.EventTypeWarning, ErrResourceExists, msg)
	//	return fmt.Errorf(msg)
	//}

	//// If this number of the replicas on the Foo resource is specified, and the
	//// number does not equal the current desired replicas on the Deployment, we
	//// should update the Deployment resource.
	//if foo.Spec.Replicas != nil && *foo.Spec.Replicas != *deployment.Spec.Replicas {
	//	klog.V(4).Infof("Foo %s replicas: %d, deployment replicas: %d", name, *foo.Spec.Replicas, *deployment.Spec.Replicas)
	//	deployment, err = c.kubeclientset.AppsV1().Deployments(foo.Namespace).Update(context.TODO(), newDeployment(foo), metav1.UpdateOptions{})
	//}

	//// If an error occurs during Update, we'll requeue the item so we can
	//// attempt processing again later. This could have been caused by a
	//// temporary network failure, or any other transient reason.
	//if err != nil {
	//	return err
	//}

	//// Finally, we update the status block of the Foo resource to reflect the
	//// current state of the world
	//err = c.updateFooStatus(foo, deployment)
	//if err != nil {
	//	return err
	//}

	//c.recorder.Event(foo, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

//func (c *Controller) updateFooStatus(foo *samplev1alpha1.Foo, deployment *appsv1.Deployment) error {
//	// NEVER modify objects from the store. It's a read-only, local cache.
//	// You can use DeepCopy() to make a deep copy of original object and modify this copy
//	// Or create a copy manually for better performance
//	fooCopy := foo.DeepCopy()
//	fooCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas
//	// If the CustomResourceSubresources feature gate is not enabled,
//	// we must use Update instead of UpdateStatus to update the Status block of the Foo resource.
//	// UpdateStatus will not allow changes to the Spec of the resource,
//	// which is ideal for ensuring nothing other than resource status has been updated.
//	_, err := c.sampleclientset.SamplecontrollerV1alpha1().Foos(foo.Namespace).Update(context.TODO(), fooCopy, metav1.UpdateOptions{})
//	return err
//}

// enqueueFoo takes a Foo resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Foo.
//func (c *Controller) enqueueFoo(obj interface{}) {
//	var key string
//	var err error
//	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
//		utilruntime.HandleError(err)
//		return
//	}
//	c.workqueue.Add(key)
//}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Foo resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Foo resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *SecretController) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	klog.V(4).Infof("Processing object: %s/%s", object.GetNamespace(), object.GetName())
	if object.GetNamespace() == "node-ip-webhook" &&
		object.GetName() == "node-ip-webhook-certs" {
		c.workqueue.Add(struct{}{})
	}
	//if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
	//	// If this object is not owned by a Foo, we should not do anything more
	//	// with it.
	//	if ownerRef.Kind != "Foo" {
	//		return
	//	}

	//	foo, err := c.foosLister.Foos(object.GetNamespace()).Get(ownerRef.Name)
	//	if err != nil {
	//		klog.V(4).Infof("ignoring orphaned object '%s' of foo '%s'", object.GetSelfLink(), ownerRef.Name)
	//		return
	//	}

	//	c.enqueueFoo(foo)
	//	return
	//}
}

// newDeployment creates a new Deployment for a Foo resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the Foo resource that 'owns' it.
//func newDeployment(foo *samplev1alpha1.Foo) *appsv1.Deployment {
//	labels := map[string]string{
//		"app":        "nginx",
//		"controller": foo.Name,
//	}
//	return &appsv1.Deployment{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      foo.Spec.DeploymentName,
//			Namespace: foo.Namespace,
//			OwnerReferences: []metav1.OwnerReference{
//				*metav1.NewControllerRef(foo, samplev1alpha1.SchemeGroupVersion.WithKind("Foo")),
//			},
//		},
//		Spec: appsv1.DeploymentSpec{
//			Replicas: foo.Spec.Replicas,
//			Selector: &metav1.LabelSelector{
//				MatchLabels: labels,
//			},
//			Template: corev1.PodTemplateSpec{
//				ObjectMeta: metav1.ObjectMeta{
//					Labels: labels,
//				},
//				Spec: corev1.PodSpec{
//					Containers: []corev1.Container{
//						{
//							Name:  "nginx",
//							Image: "nginx:latest",
//						},
//					},
//				},
//			},
//		},
//	}
//}

