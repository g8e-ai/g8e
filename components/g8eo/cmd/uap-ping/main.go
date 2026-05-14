package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/internal/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
)

func main() {
	// 1. Generate self-signed CA and Certs for mTLS demo
	caCert, caPriv, err := generateCA()
	if err != nil {
		log.Fatal(err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Warden Server")
	if err != nil {
		log.Fatal(err)
	}

	clientCert, clientPriv, err := generateCert(caCert, caPriv, "Sage Client")
	if err != nil {
		log.Fatal(err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// 2. Start Warden Server (8443)
	go func() {
		tlsConfig := &tls.Config{
			ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs:  caPool,
			Certificates: []tls.Certificate{{
				Certificate: [][]byte{serverCert.Raw},
				PrivateKey:  serverPriv,
			}},
		}

		server := &http.Server{
			Addr:      ":8443",
			TLSConfig: tlsConfig,
		}

		http.HandleFunc("/uap", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var env uap.UAPEnvelope
			if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
				log.Printf("Warden: Failed to decode envelope: %v", err)
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}

			fmt.Println("\n--- Warden Received UAP Envelope ---")
			fmt.Printf("ID:      %s\n", env.Id)
			fmt.Printf("Sender:  %s\n", env.OperatorId)
			fmt.Printf("Action:  %s\n", env.ActionType)
			fmt.Printf("Payload: %s\n", string(env.Payload))
			fmt.Println("-----------------------------------")

			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Warden: Envelope authorized and logged."))
		})

		log.Printf("Warden Server listening on https://localhost:8443/uap (mTLS required)")
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Warden Server failed: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(1 * time.Second)

	// 3. Sage Client sends Ping
	env := &uap.UAPEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "sage-agent-alpha",
		Timestamp:       timestamppb.Now(),
		ActionType:      "EXECUTE_BASH",
		TargetResource:  "localhost",
		Payload:         []byte("echo 'UAP mTLS Ping Success'"),
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				KeyId: "demo-key",
			},
		},
	}

	id, _ := uap.GenerateMessageID(env)
	env.Id = id

	payload, _ := json.Marshal(env)

	tlsConfig := &tls.Config{
		RootCAs: caPool,
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{clientCert.Raw},
			PrivateKey:  clientPriv,
		}},
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	log.Printf("Sage Client: Sending UAP envelope to Warden...")
	resp, err := client.Post("https://localhost:8443/uap", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Fatalf("Sage Client: Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Printf("Sage Client: Ping SUCCESS. Server responded: 200 OK")
	} else {
		log.Fatalf("Sage Client: Ping FAILED. Status: %s", resp.Status)
	}

	// Wait briefly to see Warden output
	time.Sleep(500 * time.Millisecond)
	log.Printf("Phase 1: Proof of Life Complete.")
}

func generateCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"g8e Trusted Authority"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certBytes)
	return cert, priv, err
}

func generateCert(caCert *x509.Certificate, caPriv *ecdsa.PrivateKey, commonName string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(1 * time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:    []string{"localhost"},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, &priv.PublicKey, caPriv)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certBytes)
	return cert, priv, err
}
