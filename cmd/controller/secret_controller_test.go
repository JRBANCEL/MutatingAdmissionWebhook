package main

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	kubeClient *k8sfake.Clientset
	secretsLister []*corev1.Secret
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	return f
}

func (f *fixture) newController() (*SecretController, kubeinformers.SharedInformerFactory) {
	f.kubeClient = k8sfake.NewSimpleClientset()

	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeClient, noResyncPeriodFunc())

	c := NewController(f.kubeClient, k8sI.Core().V1().Secrets(), secretNamespace, secretName)
	c.secretsSynced = alwaysReady

	for _, s := range f.secretsLister {
		f.kubeClient.CoreV1().Secrets(s.Namespace).Create(s)
	}

	return c, k8sI
}

func (f *fixture) run(t *testing.T) *SecretController {
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
		} else if time.Since(lastChange) > 2 * time.Second {
			return c
		}
	}
}

func TestCreateSecretIfItDoesntExist(t *testing.T) {
	f := newFixture(t)

	c := f.run(t)

	// Validate that a fresh Secret has been created
	secret, err := c.secretsLister.Secrets(secretNamespace).Get(secretName)
	if err != nil {
		t.Fatalf("Failed to get the Secret: %v", err)
	}
	expiration, err := getDurationBeforeExpiration(secret)
	if err != nil {
		t.Fatalf("Failed to parse the Secret: %v", err)
	}
	if expiration < 364 * 24 * time.Hour {
		t.Fatalf("The Secret expires too soon: %v", expiration)
	}
}

func TestDoNothingIfSecretExistsAndIsNotExpiringSoon(t *testing.T) {
	f := newFixture(t)

	// Create a Secret not expiring soon (as defined by the expiration threshold)
	data, err := generateSecretData(time.Now(), time.Now().Add(365 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Data: data,
	}
	f.secretsLister = append(f.secretsLister, oldSecret)

	c := f.run(t)

	// Validate that the Secret hasn't changed
	newSecret, err := c.secretsLister.Secrets(secretNamespace).Get(secretName)
	if err != nil {
		t.Fatalf("Failed to get the Secret: %v", err)
	}
	if !reflect.DeepEqual(oldSecret, newSecret) {
		t.Fatalf("The Secret has been modified, diff:\n %s", diff.ObjectGoPrintSideBySide(oldSecret, newSecret))
	}
}

func TestRefreshSecretIfExistsAndIsExpiringSoon(t *testing.T) {
	f := newFixture(t)

	// Create a Secret expiring soon (as defined by the expiration threshold)
	data, err := generateSecretData(time.Now(), time.Now().Add(5 * time.Minute))
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Data: data,
	}
	f.secretsLister = append(f.secretsLister, oldSecret)

	c := f.run(t)

	// Validate that the Secret has been refreshed
	newSecret, err := c.kubeClient.CoreV1().Secrets(secretNamespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get the Secret: %v", err)
	}
	if reflect.DeepEqual(oldSecret, newSecret) {
		t.Fatalf("The Secret hasn't been modified, diff:\n %s", diff.ObjectGoPrintSideBySide(oldSecret, newSecret))
	}
	expiration, err := getDurationBeforeExpiration(newSecret)
	if err != nil {
		t.Fatalf("Failed to parse the Secret: %v", err)
	}
	if expiration < 364 * 24 * time.Hour {
		t.Fatalf("The Secret expires too soon: %v", expiration)
	}
}
