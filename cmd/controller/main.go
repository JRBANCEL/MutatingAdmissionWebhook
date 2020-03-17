package main

import (
	"log"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func main() {

	// set up signals so we handle the first shutdown signal gracefully
	//stopCh := signals.SetupSignalHandler()
	stopCh := make(<-chan struct{})

	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Fatalf("Error building Kubernetes config: %v", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error building Kubernetes client: %v", err)
	}

	//secret, err := client.CoreV1().Secrets("knative-serving").Get("webhook-certs", metav1.GetOptions{})
	//if err != nil {
	//	log.Printf("Failed to get Secret knative-serving/webhook-certs: %v", err)
	//	return
	//}

	//keyPEM, ok := secret.Data["server-key.pem"]
	//if !ok {
	//	return
	//}
	//certPEM, ok := secret.Data["server-cert.pem"]
	//if !ok {
	//	return
	//}
	//cert, err := tls.X509KeyPair(certPEM, keyPEM)
	//if err != nil {
	//	return
	//}

	//if cert.Leaf == nil {
	//	return
	//}
	//log.Println(cert.Leaf.NotAfter.String())

	//time.Sleep(1 * time.Hour)

	// Create an informer factory scoped to the 'node-ip-webhook' namespace
	// because it is the only namespace accessible by the service account.
	informerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		client,
		30 * time.Second,
		kubeinformers.WithNamespace("node-ip-webhook"))

	controller := NewController(client, informerFactory.Core().V1().Secrets())

	informerFactory.Start(stopCh)

	if err = controller.Run(stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}
