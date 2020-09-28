[![Go Report Card](https://goreportcard.com/badge/github.com/JRBANCEL/MutatingAdmissionWebhook)](https://goreportcard.com/report/github.com/JRBANCEL/MutatingAdmissionWebhook)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

# What?
A [Mutating Admission Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) adding an environment variable containing the Node IP to [Knative](https://knative.dev) Pods using the [Downward API](https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/):

```
spec:
  containers:
  - env:    
    - name: DD_AGENT_HOST
      valueFrom:
        fieldRef:
          fieldPath: status.hostIP
```

# Why?
[Datadog](https://www.datadoghq.com/) instrumentation library needs to communicate with the Datadog Agent located on the same node, [see documentation](https://docs.datadoghq.com/developers/dogstatsd/?tab=kubernetes#send-statsd-metrics-to-the-agent). ~~Unfortunately, Knative doesn't support the Downward API ([yet](https://github.com/knative/serving/issues/4190))~~. Dynamically injecting the environment variable is a workaround.

[Update] Datadog now provides a Webhook doing exactly this: https://docs.datadoghq.com/agent/cluster_agent/admission_controller/ 

# How?
The Webhook intercepts Pod `CREATE` calls to the Kubernetes API Server and inserts the environment variable in the Pod Spec. This is the easy part and is defined in [cmd/webhook/main.go](https://github.com/JRBANCEL/MutatingAdmissionWebhook/blob/master/cmd/webhook/main.go).

Webhooks must expose an HTTPS endpoint, therefore a TLS certificate must be used. Manual provisionning is possible but not recommended. This projects contains different components automating the process:
* [pkg/controller/secret/controller.go](https://github.com/JRBANCEL/MutatingAdmissionWebhook/blob/master/pkg/controller/secret/controller.go): a controller ensuring that there is a Kubernetes Secret containing a valid self-signed TLS certficate at all time: creates it if it doesn't exist, refreshes it when it is about to expire, etc...
* [pkg/controller/webhook/controller.go](https://github.com/JRBANCEL/MutatingAdmissionWebhook/blob/master/pkg/controller/webhook/controller.go): a controller ensuring that there is a `mutatingwebhookconfigurations.admissionregistration.k8s.io` configured such that its `webhooks.admissionReviewVersions.clientConfig.caBundle` matches the Kubernetes Secret described above.
* [cmd/webhook/main.go](https://github.com/JRBANCEL/MutatingAdmissionWebhook/blob/master/cmd/webhook/main.go): exposes an HTTPS endpoints with a TLS certificate matching the Kubernetes Secret described above.

# Installation
Using [ko](https://github.com/google/ko):

```
ko apply -f config
```

Everything (except the MutatingWebhookConfiguration which is cluster scoped) is installed under the `node-ip-webhook` namespace and can be uninstalled via:

```
kubectl delete mutatingwebhookconfigurations.admissionregistration.k8s.io node-ip-webhook
kubectl delete namespace node-ip-webhook
```
