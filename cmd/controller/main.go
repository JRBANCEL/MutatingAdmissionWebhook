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
	informerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		client,
		1 * time.Hour,
		kubeinformers.WithNamespace(secretNamespace))

	controller := NewController(
		client,
		informerFactory.Core().V1().Secrets(),
		secretNamespace,
		secretName)

	informerFactory.Start(stopCh)

	if err = controller.Run(stopCh); err != nil {
		klog.Fatalf("Error running the controller: %s", err.Error())
	}
}
