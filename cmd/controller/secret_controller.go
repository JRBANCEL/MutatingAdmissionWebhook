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
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

var (
	// notBefore and notAfter define when a new Secret is valid
	notBefore = func() time.Time { return time.Now().Add(-5 * time.Minute) }
	notAfter = func() time.Time { return time.Now().Add(365 * 24 * time.Hour) }

	// expirationThreshold defines how long before its expiration a Secret
	// should be refreshed.
	expirationThreshold = 30 * 24 * time.Hour
)

// SecretController is the controller in charge of the creating and refreshing
// the Webhook Secret.
type SecretController struct {
	kubeClient kubernetes.Interface

	secretNamespace string
	secretName      string

	secretsLister corelisters.SecretLister
	secretsSynced cache.InformerSynced

	workQueue workqueue.RateLimitingInterface
}

// NewController returns a new SecretController.
func NewController(
	kubeClient kubernetes.Interface,
	secretInformer coreinformers.SecretInformer,
	secretNamespace string,
	secretName string) *SecretController {
	controller := &SecretController{
		kubeClient:      kubeClient,
		secretNamespace: secretNamespace,
		secretName:      secretName,
		secretsLister:   secretInformer.Lister(),
		secretsSynced:   secretInformer.Informer().HasSynced,
		workQueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "SecretController"),
	}

	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			// Even if old and new are the same objects, expiration needs to be handled.
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workQueue and wait for
// workers to finish processing their current work items.
func (c *SecretController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workQueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Infof("Starting the Secret controller for '%s/%s'", c.secretNamespace, c.secretName)

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync...")
	if ok := cache.WaitForCacheSync(stopCh, c.secretsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	go wait.Until(c.runWorker, time.Second, stopCh)

	// Trigger a reconciliation to create the Secret if it doesn't exist
	c.workQueue.Add(struct {}{})

	klog.Info("Successfully started!")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker processes the workQueue.
func (c *SecretController) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workQueue and
// attempt to process it, by calling reconcileSecret.
func (c *SecretController) processNextWorkItem() bool {
	obj, shutdown := c.workQueue.Get()

	if shutdown {
		return false
	}

	func() {
		// Done() must always be called
		defer c.workQueue.Done(obj)
		if err := c.reconcileSecret(); err != nil {
			// Requeue for retry
			c.workQueue.AddRateLimited(struct {}{})
			klog.Errorf("Failed to reconcile '%s/%s': %v", c.secretNamespace, c.secretName, err)
		}
		// Remove from the queue
		c.workQueue.Forget(obj)
		klog.Infof("Successfully reconciled '%s/%s'", c.secretNamespace, c.secretName)
	}()

	return true
}

// handleObject decides what to do with the provided object.
// If the object is the Secret this controller is in charge of then an item is
// enqueued to trigger reconciliation; otherwise, nothing is done.
func (c *SecretController) handleObject(obj interface{}) {
	if object, ok := obj.(metav1.Object); ok {
		// Ignore everything except the Secret
		if object.GetNamespace() == c.secretNamespace &&
				object.GetName() == c.secretName {
			c.workQueue.Add(struct{}{})
		}
	}// else {
	//	klog.Infof("Ignoring: %s/%s", object.GetNamespace(), object.GetName())
	//}
}

// reconcileSecret reconcile the current state of the Secret with its desired state.
func (c *SecretController) reconcileSecret() error {
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
	if err != nil || durationBeforeExpiration < expirationThreshold {
		klog.Infof("The certificate is expiring soon (%v), refreshing it.", durationBeforeExpiration)
		return c.updateSecret(secret)
	}

	klog.Infof("The certificate is not expiring soon (%v), doing nothing.", durationBeforeExpiration)
	return nil
}

func (c *SecretController) createSecret() error {
	data, err := generateSecretData(notBefore(), notAfter())
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
	data, err := generateSecretData(notBefore(), notAfter())
	if err != nil {
		return fmt.Errorf("failed to generate the Secret data: %w", err)
	}

	secret = secret.DeepCopy()
	secret.Data = data
	_, err = c.kubeClient.CoreV1().Secrets(c.secretNamespace).Update(secret)
	return err
}