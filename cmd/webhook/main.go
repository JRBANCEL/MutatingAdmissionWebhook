package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	admiv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/JRBANCEL/MutatingAdmissionWebhook/pkg/certificate"
	"github.com/JRBANCEL/MutatingAdmissionWebhook/pkg/constants"
	"gomodules.xyz/jsonpatch/v3"
)

const (
	jsonContentType = `application/json`
	envVarName      = "DD_AGENT_HOST"
)

var (
	deserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
)

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		log.Fatalf("Error building Kubernetes config: %v", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error building Kubernetes client: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", mutateFunc)
	server := &http.Server{
		Addr:    ":10250",
		Handler: mux,
		TLSConfig: &tls.Config{
			GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
				secret, err := client.CoreV1().Secrets(constants.Namespace).Get(constants.SecretName, metav1.GetOptions{})
				if err != nil {
					log.Fatalf("Failed to get Secret %s/%s : %v", constants.Namespace, constants.SecretName, err)
				}

				cert, err := certificate.ParseSecretData(secret.Data)
				if err != nil {
					log.Fatalf("Failed to parse Secret %s/%s : %v", constants.Namespace, constants.SecretName, err)
				}
				return &cert, nil
			},
		},
	}
	log.Fatal(server.ListenAndServeTLS("", ""))
}

func mutateFunc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		log.Printf("invalid method %s, only POST requests are allowed", r.Method)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("could not read request body: %v", err)
		return
	}
	defer r.Body.Close()

	if contentType := r.Header.Get("Content-Type"); contentType != jsonContentType {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("unsupported content type %s, only %s is supported", contentType, jsonContentType)
		return
	}

	var admissionReviewReq admiv1beta1.AdmissionReview
	if _, _, err := deserializer.Decode(body, nil, &admissionReviewReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("could not deserialize request: %v", err)
		return
	} else if admissionReviewReq.Request == nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("malformed admission review: request is nil")
		return
	}

	var admissionReviewResp admiv1beta1.AdmissionReview
	resp, err := mutate(admissionReviewReq.Request)
	if err != nil {
		log.Printf("Failed to mutate: %v", err) // TODO(bancel): better message
		admissionReviewResp = admiv1beta1.AdmissionReview{
			Response: &admiv1beta1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
				},
				Allowed: false,
			},
		}
	} else {
		admissionReviewResp = admiv1beta1.AdmissionReview{
			Response: resp,
		}
	}
	admissionReviewResp.Response.UID = admissionReviewReq.Request.UID

	encoder := json.NewEncoder(w)
	err = encoder.Encode(&admissionReviewResp)
	if err != nil {
		klog.Errorf("failed to encode the response: %v", err)
		return
	}
}

func mutate(req *admiv1beta1.AdmissionRequest) (*admiv1beta1.AdmissionResponse, error) {
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return nil, fmt.Errorf("failed to decode raw object: %w", err)
	}

	resp := &admiv1beta1.AdmissionResponse{Allowed: true}

	if !shouldMutate(pod) {
		log.Printf("Skipping Pod %s/%s because it is not a Knative Pod", req.Namespace, req.Name)
		return resp, nil
	}
	log.Printf("Mutating Pod %s/%s because it is a Knative Pod", req.Namespace, req.Name)

	for i, container := range pod.Spec.Containers {
		// Skip the Knative Queue Proxy container
		if container.Name == "queue-proxy" {
			continue
		}

		// Find out if there is already an environment variable defined where we want to add one
		found := false
		for _, env := range container.Env {
			if env.Name == envVarName {
				klog.Warningf("Container %q already contains an environment variable entry for %q. Keeping the original value.", container, envVarName)
				found = true
				break
			}
		}

		// Add the environment variable definition
		if !found {
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, corev1.EnvVar{
				Name:  envVarName,
				Value: "",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.hostIP",
					},
				},
			})
		}
	}

	bytes, err := json.Marshal(pod)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the mutated Pod object: %w", err)
	}
	patch, err := jsonpatch.CreatePatch(req.Object.Raw, bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compute the JSON patch: %w", err)
	}
	resp.Patch, err = json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the JSON patch: %w", err)
	}
	return resp, nil
}

// shouldMutate returns whether the provided Pod should be mutated.
func shouldMutate(pod corev1.Pod) bool {
	for label := range pod.ObjectMeta.Labels {
		if strings.HasPrefix(label, "serving.knative.dev/") {
			return true
		}
	}
	return false
}
