package main

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	admissioninformers "k8s.io/client-go/informers/admissionregistration/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	admissionlisters "k8s.io/client-go/listers/admissionregistration/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

// WebhookController is the controller in charge of watching the TLS certificate stored in the Secret
// secretNamespace/secretName and deriving the Webhook webhookNamespace/webhookName from it.
type WebhookController struct {
	kubeClient kubernetes.Interface

	secretNamespace string
	secretName      string
	webhookName      string

	secretsLister corelisters.SecretLister
	secretsSynced cache.InformerSynced

	webhooksLister admissionlisters.MutatingWebhookConfigurationLister
	webhooksSynced cache.InformerSynced

	workQueue workqueue.RateLimitingInterface
}

// NewWebhookController returns a new WebhookController.
func NewWebhookController(
	kubeClient kubernetes.Interface,
	secretInformer coreinformers.SecretInformer,
	secretNamespace string,
	secretName string,
	webhookInformer admissioninformers.MutatingWebhookConfigurationInformer,
	webhookName string) *WebhookController {
	controller := &WebhookController{
		kubeClient:       kubeClient,
		secretNamespace:  secretNamespace,
		secretName:       secretName,
		secretsLister:    secretInformer.Lister(),
		secretsSynced:    secretInformer.Informer().HasSynced,
		webhookName:      webhookName,
		webhooksLister:   webhookInformer.Lister(),
		webhooksSynced:   webhookInformer.Informer().HasSynced,
		workQueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "WebhookController"),
	}

	secretInformer.Informer().AddEventHandler(createSecretEventHandler(controller))
	webhookInformer.Informer().AddEventHandler(createWebhookEventHandler(controller))

	return controller
}

func createSecretEventHandler(c *WebhookController) cache.ResourceEventHandler {
	handleObject := func(obj interface{}) {
		if object, ok := obj.(metav1.Object); ok {
			// Ignore everything except the Secret being watched
			if object.GetNamespace() == c.secretNamespace &&
				object.GetName() == c.secretName {
				c.workQueue.Add(struct{}{})
			}
		}
	}
	return &cache.ResourceEventHandlerFuncs{
		// If the Secret is added, the Webhook must be updated.
		AddFunc:    handleObject,
		// If the Secret is updated, the Webhook must be updated.
		UpdateFunc: func(oldObj, newObj interface{}) {
			newSecret:= newObj.(*corev1.Secret)
			oldSecret := oldObj.(*corev1.Secret)
			if newSecret.ResourceVersion == oldSecret.ResourceVersion {
				return
			}
			handleObject(newObj)
		},
		// If the Secret is deleted, it will break the Webhook but there is nothing
		// that this controller can do except wait for a new Secret to be created.
		DeleteFunc: nil,
	}
}

func createWebhookEventHandler(c *WebhookController) cache.ResourceEventHandler {
	handleObject := func(obj interface{}) {
		if object, ok := obj.(metav1.Object); ok {
			// Ignore everything except the Webhook being watched
			if object.GetName() == c.webhookName {
				c.workQueue.Add(struct{}{})
			}
		}
	}
	return &cache.ResourceEventHandlerFuncs{
		// If the Webhook is created or updated, we must make sure that its definition
		// matches our expectation.
		AddFunc:    handleObject,
		UpdateFunc: func(oldObj, newObj interface{}) {
			newWebhook := newObj.(*admissionv1.MutatingWebhookConfiguration)
			oldWebhook := oldObj.(*admissionv1.MutatingWebhookConfiguration)
			if newWebhook.ResourceVersion == oldWebhook.ResourceVersion {
				return
			}
			handleObject(newObj)
		},
		// If the Webhook is deleted, it must be re-created.
		DeleteFunc: handleObject,
	}
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workQueue and wait for
// workers to finish processing their current work items.
func (c *WebhookController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workQueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Infof("Starting the Webhook controller for Secret '%s/%s' and Webhook '%s'",
		c.secretNamespace,
		c.secretName,
		c.webhookName)

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync...")
	if ok := cache.WaitForCacheSync(stopCh, c.secretsSynced, c.webhooksSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	go wait.Until(c.runWorker, time.Second, stopCh)

	// Trigger a reconciliation to create the Secret if it doesn't exist
	c.workQueue.Add(struct{}{})

	klog.Info("Successfully started!")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker processes the workQueue.
func (c *WebhookController) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workQueue and
// attempt to process it, by calling reconcileSecret.
func (c *WebhookController) processNextWorkItem() bool {
	obj, shutdown := c.workQueue.Get()

	if shutdown {
		return false
	}

	func() {
		// Done() must always be called
		defer c.workQueue.Done(obj)
		if err := c.reconcileWebhook(); err != nil {
			// Requeue for retry
			c.workQueue.AddRateLimited(struct{}{})
			klog.Errorf("Failed to reconcile '%s/%s': %v", c.secretNamespace, c.secretName, err)
		}
		// Remove from the queue
		c.workQueue.Forget(obj)
		klog.Infof("Successfully reconciled '%s/%s'", c.secretNamespace, c.secretName)
	}()

	return true
}

// reconcileSecret reconcile the current state of the Webhook with its desired state.
func (c *WebhookController) reconcileWebhook() error {
	secret, err := c.secretsLister.Secrets(c.secretNamespace).Get(c.secretName)
	if err != nil {
		//if errors.IsNotFound(err) {
		//	return fmt.Errorf("the Secret %s/%s was not found, aborting the reconciliation", c.secretNamespace, c.secretName)
		//}
		return err
	}

	webhook, err := c.webhooksLister.Get(c.webhookName)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("The Webhook %q was not found, creating it.", c.webhookName)
			return c.createWebhook()
		}
		return err
	}
	klog.Infof("The Webhook %q was found, updating it.", c.webhookName)
	return nil
}

func (c *WebhookController) createWebhook() error {
	data, err := generateSecretData()
	if err != nil {
		return fmt.Errorf("failed to generate the Secret data: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.secretNamespace,
			Name:      c.secretName,
		},
		Data: data,
	}
	_, err = c.kubeClient.CoreV1().Secrets(c.secretNamespace).Create(secret)
	return err
}

func (c *WebhookController) updateWebhook(secret *corev1.Secret) error {
	data, err := generateSecretData()
	if err != nil {
		return fmt.Errorf("failed to generate the Secret data: %w", err)
	}

	secret = secret.DeepCopy()
	secret.Data = data
	_, err = c.kubeClient.CoreV1().Secrets(c.secretNamespace).Update(secret)
	return err
}