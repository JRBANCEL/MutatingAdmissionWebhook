package constants

const (
	// Namespace is the Kubernetes namespace where the Webhook controllers and implementation live
	Namespace   = "node-ip-webhook"

	// SecretName is the name of the Kubernetes Secret containing the Webhook TLS certificate inside `Namespace`
	SecretName  = "webhook-cert"

	// WebhookName is the name of the Kubernetes Webhook (cluster-scoped)
	WebhookName = "node-ip-webhook"

	// ServiceName is the name of the Kubernetes Service pointing to the Webhook implementation inside `Namespace`
	ServiceName = "webhook"
)
