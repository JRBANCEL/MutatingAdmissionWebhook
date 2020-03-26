package main

import (
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

const (
	secretNamespace = "node-ip-webhook"
	secretName = "webhook-cert"
)

func main() {

	// set up signals so we handle the first shutdown signal gracefully
	//stopCh := signals.SetupSignalHandler()
	stopCh := make(<-chan struct{})

	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		klog.Fatalf("Error building the Kubernetes config: %v", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error building the Kubernetes client: %v", err)
	}

	// Create an informer factory scoped to secretNamespace
	// because it is the only namespace accessible by the service account.
	secretInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		client,
		1 * time.Hour,
		kubeinformers.WithNamespace(secretNamespace))

	secretController := NewController(
		client,
		secretInformerFactory.Core().V1().Secrets(),
		secretNamespace,
		secretName)

	webhookInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		client,
		1 * time.Hour,
		kubeinformers.WithNamespace(secretNamespace))
	webhookController := NewWebhookController(
		client,
		secretInformerFactory.Core().V1().Secrets(),
		secretNamespace,
		secretName,
		webhookInformerFactory.Admissionregistration().V1beta1().MutatingWebhookConfigurations(),
		"node-ip-webhook")

	secretInformerFactory.Start(stopCh)
	webhookInformerFactory.Start(stopCh)

	// TODO: clean this up
	go func() {
		webhookController.Run(stopCh)
	}()
	if err = secretController.Run(stopCh); err != nil {
		klog.Fatalf("Error running the controller: %s", err.Error())
	}
}
