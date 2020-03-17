package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	admiv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const (
	jsonContentType = `application/json`
)

var (
	deserializer = serializer.NewCodecFactory(runtime.NewScheme()).UniversalDeserializer()
)

func main() {
	certPath, keyPath, err := generateCertificate([]string{"node-ip-webhook.default.svc"})
	if err != nil {
		log.Fatalf("Failed to create the self-signed TLS certificate: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", MutateFunc)
	server := &http.Server{
		Addr:    ":10250",
		Handler: mux,
	}
	log.Fatal(server.ListenAndServeTLS(certPath, keyPath))
}

func MutateFunc(w http.ResponseWriter, r *http.Request) {
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

	var admissionReviewReq admiv1.AdmissionReview
	if _, _, err := deserializer.Decode(body, nil, &admissionReviewReq); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("could not deserialize request: %v", err)
		return
	} else if admissionReviewReq.Request == nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("malformed admission review: request is nil")
		return
	}

	//	Response: &admiv1.AdmissionResponse{
	//		UID: admissionReviewReq.Request.UID,
	//	},
	//}

	var admissionReviewResp admiv1.AdmissionReview
	resp, err := Mutate(admissionReviewReq.Request)
	if err != nil {
		log.Printf("Failed to mutate: %v", err) // TODO(bancel): better message
		admissionReviewResp = admiv1.AdmissionReview{
			Response: &admiv1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
				},
				Allowed: false,
			},
		}
	} else {
		admissionReviewResp = admiv1.AdmissionReview{
			Response: resp,
		}
	}
	admissionReviewResp.Response.UID = admissionReviewReq.Request.UID

	encoder := json.NewEncoder(w)
	encoder.Encode(&admissionReviewResp)
	if err != nil {
		log.Printf("failed to encode the response: %v", err)
		return
	}
}

func Mutate(req *admiv1.AdmissionRequest) (*admiv1.AdmissionResponse, error) {
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return nil, fmt.Errorf("failed to decode raw object: %v", err)
	}

	if !shouldMutate(pod) {
		log.Printf("Allowing Pod %s/%s because it is not a Knative Pod", pod.Namespace, pod.Name)
		return &admiv1.AdmissionResponse{
			Allowed:          true,
		}, nil
	}

	log.Printf("Mutating Pod %s/%s because it is a Knative Pod", pod.Namespace, pod.Name)
	// var patchOps []patchOperation
	// // Apply the admit() function only for non-Kubernetes namespaces. For objects in Kubernetes namespaces, return
	// // an empty set of patch operations.
	// if !isKubeNamespace(admissionReviewReq.Request.Namespace) {
	// 	patchOps, err = admit(admissionReviewReq.Request)
	// }

	// if err != nil {
	// 	// If the handler returned an error, incorporate the error message into the response and deny the object
	// 	// creation.
	// 	admissionReviewResponse.Response.Allowed = false
	// 	admissionReviewResponse.Response.Result = &metav1.Status{
	// 		Message: err.Error(),
	// 	}
	// } else {
	// 	// Otherwise, encode the patch operations to JSON and return a positive response.
	// 	patchBytes, err := json.Marshal(patchOps)
	// 	if err != nil {
	// 		w.WriteHeader(http.StatusInternalServerError)
	// 		return nil, fmt.Errorf("could not marshal JSON patch: %v", err)
	// 	}
	// 	admissionReviewResponse.Response.Allowed = true
	// 	admissionReviewResponse.Response.Patch = patchBytes
	// }

	// // Return the AdmissionReview with a response as JSON.
	return nil, nil
}

func shouldMutate(pod corev1.Pod) bool {
	for k, _ := range pod.ObjectMeta.Labels {
		if strings.HasPrefix(k, "serving.knative.dev/") {
			return true
		}
	}
	return false
}

// generateCertificate generates a self-signed certificate for the provided hosts and returns
// the PEM encoded certificate and private key.
func generateCertificate(hosts []string) (string, string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	notBefore := time.Now().Add(-5 * time.Minute)
	notAfter := notBefore.Add(2 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Knative Serving"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", fmt.Errorf("failed to create the certificate: %w", err)
	}

	certFile, err := ioutil.TempFile("", "cert-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary file for the certificate: %v", err)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return "", "", fmt.Errorf("failed to encode the certificate: %w", err)
	}

	keyFile, err := ioutil.TempFile("", "key-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temporary file for the private key: %v", err)
	}
	defer keyFile.Close()
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return "", "", fmt.Errorf("failed to encode the private key: %w", err)
	}

	return certFile.Name(), keyFile.Name(), nil
}
