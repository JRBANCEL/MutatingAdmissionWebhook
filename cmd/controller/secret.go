package main

import (
	"bytes"
	"fmt"
	"net"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

const (
	certKey = "cert.pem"
	keyKey = "key.pem"
)

//// Create the common parts of the cert. These don't change between
//// the root/CA cert and the server cert.
//func createCertTemplate(name, namespace string, notAfter time.Time) (*x509.Certificate, error) {
//	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
//	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
//	if err != nil {
//		return nil, errors.New("failed to generate serial number: " + err.Error())
//	}
//
//	serviceName := name + "." + namespace
//	serviceNames := []string{
//		name,
//		serviceName,
//		serviceName + ".svc",
//		serviceName + ".svc.cluster.local",
//	}
//
//	tmpl := x509.Certificate{
//		SerialNumber:          serialNumber,
//		Subject:               pkix.Name{Organization: []string{organization}},
//		SignatureAlgorithm:    x509.SHA256WithRSA,
//		NotBefore:             time.Now(),
//		NotAfter:              notAfter,
//		BasicConstraintsValid: true,
//		DNSNames:              serviceNames,
//	}
//	return &tmpl, nil
//}
//
//// Create cert template suitable for CA and hence signing
//func createCACertTemplate(name, namespace string, notAfter time.Time) (*x509.Certificate, error) {
//	rootCert, err := createCertTemplate(name, namespace, notAfter)
//	if err != nil {
//		return nil, err
//	}
//	// Make it into a CA cert and change it so we can use it to sign certs
//	rootCert.IsCA = true
//	rootCert.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
//	rootCert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
//	return rootCert, nil
//}
//
//// Create cert template that we can use on the server for TLS
//func createServerCertTemplate(name, namespace string, notAfter time.Time) (*x509.Certificate, error) {
//	serverCert, err := createCertTemplate(name, namespace, notAfter)
//	if err != nil {
//		return nil, err
//	}
//	serverCert.KeyUsage = x509.KeyUsageDigitalSignature
//	serverCert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
//	return serverCert, err
//}
//
//// Actually sign the cert and return things in a form that we can use later on
//func createCert(template, parent *x509.Certificate, pub interface{}, parentPriv interface{}) (
//		cert *x509.Certificate, certPEM []byte, err error) {
//
//	certDER, err := x509.CreateCertificate(rand.Reader, template, parent, pub, parentPriv)
//	if err != nil {
//		return
//	}
//	cert, err = x509.ParseCertificate(certDER)
//	if err != nil {
//		return
//	}
//	b := pem.Block{Type: "CERTIFICATE", Bytes: certDER}
//	certPEM = pem.EncodeToMemory(&b)
//	return
//}
//
//func createCA(ctx context.Context, name, namespace string, notAfter time.Time) (*rsa.PrivateKey, *x509.Certificate, []byte, error) {
//	rootKey, err := rsa.GenerateKey(rand.Reader, 2048)
//	if err != nil {
//		return nil, nil, nil, fmt.Errorf("failed to generate random key: %w", err)
//	}
//
//	rootCertTmpl, err := createCACertTemplate(name, namespace, notAfter)
//	if err != nil {
//		return nil, nil, nil, fmt.Errorf("failed to generate CA certificate: %w", err)
//	}
//
//	rootCert, rootCertPEM, err := createCert(rootCertTmpl, rootCertTmpl, &rootKey.PublicKey, rootKey)
//	if err != nil {
//		logger.Errorw("error signing the CA cert", zap.Error(err))
//		return nil, nil, nil, fmt.Errorf("failed to sign")
//	}
//	return rootKey, rootCert, rootCertPEM, nil
//}

//// CreateCerts creates and returns a CA certificate and certificate and
//// key for the server. serverKey and serverCert are used by the server
//// to establish trust for clients, CA certificate is used by the
//// client to verify the server authentication chain. notAfter specifies
//// the expiration date.
//func CreateCerts(ctx context.Context, name, namespace string, notAfter time.Time) (serverKey, serverCert, caCert []byte, err error) {
//	logger := logging.FromContext(ctx)
//	// First create a CA certificate and private key
//	caKey, caCertificate, caCertificatePEM, err := createCA(ctx, name, namespace, notAfter)
//	if err != nil {
//		return nil, nil, nil, err
//	}
//
//	// Then create the private key for the serving cert
//	servKey, err := rsa.GenerateKey(rand.Reader, 2048)
//	if err != nil {
//		logger.Errorw("error generating random key", zap.Error(err))
//		return nil, nil, nil, err
//	}
//	servCertTemplate, err := createServerCertTemplate(name, namespace, notAfter)
//	if err != nil {
//		logger.Errorw("failed to create the server certificate template", zap.Error(err))
//		return nil, nil, nil, err
//	}
//
//	// create a certificate which wraps the server's public key, sign it with the CA private key
//	_, servCertPEM, err := createCert(servCertTemplate, caCertificate, &servKey.PublicKey, caKey)
//	if err != nil {
//		logger.Errorw("error signing server certificate template", zap.Error(err))
//		return nil, nil, nil, err
//	}
//	servKeyPEM := pem.EncodeToMemory(&pem.Block{
//		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(servKey),
//	})
//	return servKeyPEM, servCertPEM, caCertificatePEM, nil
//}

func generateCertificate(hosts []string) ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	notBefore := time.Now().Add(-5 * time.Minute)
	notAfter := notBefore.Add(2 * 30 * 24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Node IP Webhook"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
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

	klog.Infof("DNS Names: %v", template.DNSNames)
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the certificate: %w", err)
	}

	var certBuf bytes.Buffer
	if err := pem.Encode(&certBuf, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, nil, fmt.Errorf("failed to encode the certificate: %w", err)
	}

	var keyBuf bytes.Buffer
	if err := pem.Encode(&keyBuf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return nil, nil, fmt.Errorf("failed to encode the private key: %w", err)
	}

	return certBuf.Bytes(), keyBuf.Bytes(), nil
}

func generateSecretData() (map[string][]byte, error) {
	certPEM, keyPEM, err := generateCertificate([]string{"webhook", "webhook.node-ip-webhook", "webhook.node-ip-webhook.svc", ".webhook.node-ip-webhook.svc.cluster.local"})
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate: %w", err)
	}
	data := map[string][]byte{
		certKey: certPEM,
		keyKey:  keyPEM,
	}
	return data, nil
}

func getDurationBeforeExpiration(secret *corev1.Secret) (time.Duration, error) {
	certPEM, ok := secret.Data[certKey]
		if !ok {
			return 0, fmt.Errorf("the Secret doesn't contain an entry for %q", certKey)
		}
		certAsn1, _ := pem.Decode(certPEM)
		if certAsn1 == nil {
			return 0, fmt.Errorf("failed to parse certificate PEM")
		}
		cert, err := x509.ParseCertificate(certAsn1.Bytes)
		if err != nil {
			return 0, fmt.Errorf("failed to parse the certificate ASN.1: %w", err)
		}

		return -time.Since(cert.NotAfter), nil
}
