package main

import (
	"bytes"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	log "github.com/sirupsen/logrus"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/idgenchev/namespace-node-affinity/pkg/webhookconfig"
	"github.com/jessevdk/go-flags"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var opts struct {
	Namespace   string `long:"namespace" short:"n" env:"NAMESPACE" default:"namespace-node-affinity" description:"The namespace where the namespace-node-affinity webhook is deployed"`
	ServiceName string `long:"service-name" short:"s" env:"SERVICE_NAME" default:"namespace-node-affinity" description:"Name of the service object for the namespace-node-affinity"`
	CertFile    string `lond:"cert" short:"c" env:"CERT" default:"/etc/webhook/certs/tls.crt" description:"Path to the cert file"`
	KeyFile     string `lond:"key" short:"k" env:"KEY" default:"/etc/webhook/certs/tls.key" description:"Path to the key file"`
}

const (
	webhookConfigName = "namespace-node-affinity"
)

func main() {
	flags.Parse(&opts)

	var caPEM, serverCertPEM, serverPrivKeyPEM *bytes.Buffer
	// CA config
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2020),
		Subject: pkix.Name{
			Organization: []string{"idgenchev"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// CA private key
	caPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		log.Fatalf("Failed to generate CA private key: %s", err)
	}

	// Self signed CA certificate
	caBytes, err := x509.CreateCertificate(cryptorand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Fatalf("Failed to create self-signed CA cert: %s", err)
	}

	// PEM encode CA cert
	caPEM = new(bytes.Buffer)
	_ = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create k8s config: %s", err)
	}

	clientset, err := k8sclient.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create k8s client: %s", err)
	}

	if err = webhookconfig.CreateOrUpdateMutatingWebhookConfig(clientset, caPEM, opts.Namespace, webhookConfigName, opts.ServiceName); err != nil {
		log.Fatalf("Failed to create mutating webhook config: %s", err)
	}

	commonName := fmt.Sprintf("%s.%s.svc", opts.ServiceName, opts.Namespace)
	dnsNames := []string{
		opts.ServiceName,
		fmt.Sprintf("%s.%s", opts.ServiceName, opts.Namespace),
		commonName,
		fmt.Sprintf("%s.%s.svc.cluster.local", opts.ServiceName, opts.Namespace),
	}

	// server cert config
	cert := &x509.Certificate{
		DNSNames:     dnsNames,
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"idgenchev"},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// server private key
	serverPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		log.Fatalf("Failed to generate server private key: %s", err)
	}

	// sign the server cert
	serverCertBytes, err := x509.CreateCertificate(cryptorand.Reader, cert, ca, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		log.Fatalf("Failed to create server cert: %s", err)
	}

	// PEM encode the  server cert and key
	serverCertPEM = new(bytes.Buffer)
	_ = pem.Encode(serverCertPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertBytes,
	})

	serverPrivKeyPEM = new(bytes.Buffer)
	_ = pem.Encode(serverPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey),
	})

	err = writeFile(opts.CertFile, serverCertPEM)
	if err != nil {
		log.Fatalf("Failed to write certificate: %s", err)
	}

	err = writeFile(opts.KeyFile, serverPrivKeyPEM)
	if err != nil {
		log.Fatalf("Failed to write key: %s", err)
	}
}

func writeFile(path string, sCert *bytes.Buffer) error {
	err := os.MkdirAll(filepath.Dir(path), 0666)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(sCert.Bytes())
	if err != nil {
		return err
	}
	return nil
}
