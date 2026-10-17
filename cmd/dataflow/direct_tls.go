package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"time"
)

// directTLSFromEnv gates an operator-provisioned direct HTTPS listener.
// No reverse-proxy header or browser value is trusted for TLS authentication.
// This is an isolated TEST deployment option, NOT internet production hardening.
func directTLSFromEnv() (string, string, *tls.Config, error) {
	enabled := os.Getenv("DATAFLOW_ENABLE_DIRECT_TLS_TEST") == "true"
	secureView := os.Getenv("DATAFLOW_ENABLE_SECURE_CONNECTION_VIEW_TEST") == "true"
	if !enabled {
		if secureView {
			return "", "", nil, errors.New("secure OIDC connection view requires direct TLS listener")
		}
		return "", "", nil, nil
	}
	if os.Getenv("DATAFLOW_SECURE_CONNECTIONS_ISOLATED_LAB") != "true" {
		return "", "", nil, errors.New("direct TLS is restricted to reviewed isolated lab")
	}
	certFile := os.Getenv("DATAFLOW_DIRECT_TLS_CERT_FILE")
	keyFile := os.Getenv("DATAFLOW_DIRECT_TLS_KEY_FILE")
	if certFile == "" || keyFile == "" || certFile == keyFile {
		return "", "", nil, errors.New("direct TLS requires distinct certificate and key paths")
	}
	keyInfo, err := os.Stat(keyFile)
	if err != nil || !keyInfo.Mode().IsRegular() || keyInfo.Mode().Perm()&0077 != 0 {
		return "", "", nil, errors.New("direct TLS private key must be a private regular file")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil || len(cert.Certificate) == 0 {
		return "", "", nil, errors.New("direct TLS requires a valid matching certificate and key")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return "", "", nil, errors.New("direct TLS certificate invalid")
	}
	now := time.Now().UTC()
	if !now.After(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return "", "", nil, errors.New("direct TLS certificate outside validity period")
	}
	// TLS 1.3 minimum protects isolated credential-bearing JWT metadata traffic.
	// Client certificate name/chain trust is independently verified by clients.
	config := &tls.Config{
		MinVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
	}
	return certFile, keyFile, config, nil
}
