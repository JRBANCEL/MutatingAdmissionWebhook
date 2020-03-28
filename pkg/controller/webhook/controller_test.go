package webhook

import (
	"reflect"
	"testing"
	"time"

	admiv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/JRBANCEL/MutatingAdmissionWebhook/pkg/certificate"
)

const (
	secretNamespace = "foo"
	secretName = "bar"
	webhookName = "whatever"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

func TestCreateWebhookIfItDoesntExist(t *testing.T) {
	f := newFixture(t)

	data, err := certificate.GenerateSecretData(time.Now(), time.Now().Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Data: data,
	}
	f.secrets = append(f.secrets, secret)

	c := f.run(t)

	// Validate that the Webhook was created
	webhook, err := c.webhooksLister.Get(webhookName)
	if err != nil {
		t.Fatalf("Failed to get the Webhook: %v", err)
	}
	if len(webhook.Webhooks) != 1 {
		t.Fatalf("Webhook.Webhooks should contain a single entry: %v", webhook)
	}
	if !reflect.DeepEqual(webhook.Webhooks[0].ClientConfig.CABundle, certificate.GetCABundle(secret.Data)) {
		t.Fatalf("The Webhook CABundle doesn't match the Secret: CABundle: %v, Secret: %v", webhook.Webhooks[0].ClientConfig.CABundle, secret)
	}
}

func TestUpdateWebhookIfItExistsButDoesntMatchTheSecret(t *testing.T) {
	f := newFixture(t)

	data, err := certificate.GenerateSecretData(time.Now(), time.Now().Add(365*24*time.Hour))
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Data: data,
	}
	f.secrets = append(f.secrets, secret)
	oldWebhook := &admiv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: webhookName,
		},
	}
	f.webhooks = append(f.webhooks, oldWebhook)

	c := f.run(t)

	// Validate that the Webhook was updated
	newWebhook, err := c.webhooksLister.Get(webhookName)
	if err != nil {
		t.Fatalf("Failed to get the Webhook: %v", err)
	}
	if reflect.DeepEqual(oldWebhook, newWebhook) {
		t.Fatalf("The Webhook hasn't been modified")
	}
	// TODO: abstract to validation method
	if len(newWebhook.Webhooks) != 1 {
		t.Fatalf("Webhook.Webhooks should contain a single entry: %v", newWebhook)
	}
	if !reflect.DeepEqual(newWebhook.Webhooks[0].ClientConfig.CABundle, certificate.GetCABundle(secret.Data)) {
		t.Fatalf("The Webhook CABundle doesn't match the Secret: CABundle: %v, Secret: %v", newWebhook.Webhooks[0].ClientConfig.CABundle, secret)
	}
}

type fixture struct {
	t *testing.T

	kubeClient    *k8sfake.Clientset
	secrets []*corev1.Secret
	webhooks []*admiv1beta1.MutatingWebhookConfiguration
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	return f
}

func (f *fixture) newController() (*Controller, kubeinformers.SharedInformerFactory) {
	f.kubeClient = k8sfake.NewSimpleClientset()

	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeClient, noResyncPeriodFunc())

	c := NewController(f.kubeClient, k8sI.Core().V1().Secrets(), secretNamespace, secretName, k8sI.Admissionregistration().V1beta1().MutatingWebhookConfigurations(), webhookName)
	c.secretsSynced = alwaysReady

	for _, s := range f.secrets {
		_, _ = f.kubeClient.CoreV1().Secrets(s.Namespace).Create(s)
	}
	for _, w := range f.webhooks {
		_, _ = f.kubeClient.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(w)
	}

	return c, k8sI
}

func (f *fixture) run(t *testing.T) *Controller {
	stopCh := make(chan struct{})
	defer close(stopCh)

	c, k8sI := f.newController()
	k8sI.Start(stopCh)

	go func() {
		err := c.Run(stopCh)
		if err != nil {
			t.Fatalf("Failed to run controller: %v", err)
		}
	}()

	// TODO: explain this hack
	lastChange := time.Now()
	lastCount := 0
	for {
		time.Sleep(1 * time.Second)

		count := len(f.kubeClient.Actions())
		if count > lastCount {
			lastChange = time.Now()
			lastCount = count
		} else if time.Since(lastChange) > 2*time.Second {
			return c
		}
	}
}
