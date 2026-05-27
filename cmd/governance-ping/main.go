// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/pkg/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

func main() {
	// 1. Generate self-signed CA and Certs for mTLS demo
	caCert, caPriv, err := generateCA()
	if err != nil {
		log.Fatal(err)
	}

	serverCert, serverPriv, err := generateCert(caCert, caPriv, "Actuator Server")
	if err != nil {
		log.Fatal(err)
	}

	clientCert, clientPriv, err := generateCert(caCert, caPriv, "Sage Client")
	if err != nil {
		log.Fatal(err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	// 2. Start Actuator Server
	go func() {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS13,
			ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs:  caPool,
			Certificates: []tls.Certificate{{
				Certificate: [][]byte{serverCert.Raw},
				PrivateKey:  serverPriv,
			}},
		}

		server := &http.Server{
			Addr:              fmt.Sprintf(":%d", constants.Ports.G8eeHttps),
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 10 * time.Second,
		}

		http.HandleFunc("/uap", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var env governance.GovernanceEnvelope
			if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
				log.Printf("Actuator: Failed to decode envelope: %v", err)
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}

			fmt.Println("\n--- Actuator Received Governance Envelope ---")
			fmt.Printf("ID:      %s\n", env.Id)
			fmt.Printf("Sender:  %s\n", env.OperatorId)
			fmt.Printf("Action:  %s\n", env.ActionType)
			fmt.Printf("Payload: %s\n", string(env.Payload))
			fmt.Println("-----------------------------------")

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Actuator: Envelope authorized and logged."))
		})

		log.Printf("Actuator Server listening on https://localhost:%d/uap (mTLS required)", constants.Ports.G8eeHttps)
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Actuator Server failed: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(1 * time.Second)

	// 3. Sage Client sends Ping
	env := &governance.GovernanceEnvelope{
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

	id, _ := governance.GenerateMessageID(env)
	env.Id = id

	payload, _ := json.Marshal(env)

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    caPool,
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

	log.Printf("Sage Client: Sending Governance envelope to Actuator...")
	resp, err := client.Post(fmt.Sprintf("https://localhost:%d/uap", constants.Ports.G8eeHttps), "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Fatalf("Sage Client: Request failed: %v", err)
	}

	status := resp.StatusCode
	_ = resp.Body.Close()

	if status == http.StatusOK {
		log.Printf("Sage Client: Ping SUCCESS. Server responded: 200 OK")
	} else {
		log.Fatalf("Sage Client: Ping FAILED. Status: %d", status)
	}

	// Wait briefly to see Actuator output
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
