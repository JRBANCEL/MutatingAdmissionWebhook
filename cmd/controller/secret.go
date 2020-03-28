package main

import (
	"bytes"
	"fmt"
	"net"

	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	corev1 "k8s.io/api/core/v1"
	"math/big"
	"time"
)

const (
	certKey = "cert.pem"
	keyKey = "key.pem"
)

func generateCertificate(hosts []string, notBefore, notAfter time.Time) ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

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

func generateSecretData(notBefore, notAfter time.Time) (map[string][]byte, error) {
	certPEM, keyPEM, err := generateCertificate(
		[]string{"webhook", "webhook.node-ip-webhook", "webhook.node-ip-webhook.svc", ".webhook.node-ip-webhook.svc.cluster.local"},
		notBefore,
		notAfter)
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
