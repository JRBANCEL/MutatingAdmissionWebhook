package main

import (
	"context"
	"golang.org/x/sync/errgroup"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/JRBANCEL/MutatingAdmissionWebhook/pkg/constants"
	"github.com/JRBANCEL/MutatingAdmissionWebhook/pkg/controller/secret"
	"github.com/JRBANCEL/MutatingAdmissionWebhook/pkg/controller/webhook"
)

func main() {
	// TODO: use signals to close this channel
	stopCh := make(chan struct{})
	defer close(stopCh)

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
		24*time.Hour,
		kubeinformers.WithNamespace(constants.Namespace))

	secretController := secret.NewController(
		client,
		informerFactory.Core().V1().Secrets(),
		constants.Namespace,
		constants.SecretName)

	webhookController := webhook.NewController(
		client,
		informerFactory.Core().V1().Secrets(),
		constants.Namespace,
		constants.SecretName,
		informerFactory.Admissionregistration().V1beta1().MutatingWebhookConfigurations(),
		constants.WebhookName)

	informerFactory.Start(stopCh)

	eg, _ := errgroup.WithContext(context.Background())
	eg.Go(func() error { return webhookController.Run(stopCh) })
	eg.Go(func() error { return secretController.Run(stopCh) })
	if err = eg.Wait(); err != nil {
		klog.Fatalf("Error running a controller: %v", err)
	}
}
