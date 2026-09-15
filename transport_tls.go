package main

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"strings"
)

const (
	transportCertFileEnv = "FLARETUNNEL_TRANSPORT_CERT"
	transportKeyFileEnv  = "FLARETUNNEL_TRANSPORT_KEY"
)

// parseTransportSANs validates the configured public identities. Listen
// addresses such as 0.0.0.0 are deliberately not accepted as identities.
func parseTransportSANs(raw string) ([]string, []net.IP, error) {
	var dnsNames []string
	var ipAddresses []net.IP
	values := strings.Fields(raw)
	if len(values) == 0 {
		return nil, nil, fmt.Errorf("FLARETUNNEL_TLS_SAN must contain at least one DNS name or IP address")
	}
	for _, value := range values {
		if ip := net.ParseIP(value); ip != nil {
			if ip.IsUnspecified() {
				return nil, nil, fmt.Errorf("FLARETUNNEL_TLS_SAN contains unspecified IP %q", value)
			}
			ipAddresses = append(ipAddresses, ip)
			continue
		}
		if strings.ContainsAny(value, "/,:[] ") || strings.TrimSpace(value) != value || value == "." {
			return nil, nil, fmt.Errorf("FLARETUNNEL_TLS_SAN contains invalid identity %q", value)
		}
		dnsNames = append(dnsNames, value)
	}
	return dnsNames, ipAddresses, nil
}

func loadTransportCertificate(certPath, keyPath string) (tls.Certificate, error) {
	if strings.TrimSpace(certPath) == "" || strings.TrimSpace(keyPath) == "" {
		return tls.Certificate{}, fmt.Errorf("%s and %s are required for transport TLS", transportCertFileEnv, transportKeyFileEnv)
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("cannot load transport TLS certificate/key: %w", err)
	}
	if len(cert.Certificate) == 0 {
		return tls.Certificate{}, fmt.Errorf("transport TLS certificate is empty")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("transport TLS certificate is invalid: %w", err)
	}
	if leaf.IsCA {
		return tls.Certificate{}, fmt.Errorf("transport TLS certificate must be a server certificate, not a CA")
	}
	if len(leaf.DNSNames) == 0 && len(leaf.IPAddresses) == 0 {
		return tls.Certificate{}, fmt.Errorf("transport TLS certificate must contain at least one SAN")
	}
	if key, ok := cert.PrivateKey.(*rsa.PrivateKey); ok {
		if pub, ok := leaf.PublicKey.(*rsa.PublicKey); !ok || pub.N.Cmp(key.N) != 0 || pub.E != key.E {
			return tls.Certificate{}, fmt.Errorf("transport TLS certificate does not match its private key")
		}
	}
	return cert, nil
}

func validateTransportCertificateSAN(certPath string, expected string) error {
	dnsNames, ips, err := parseTransportSANs(expected)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("cannot read transport TLS certificate: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("transport TLS certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("transport TLS certificate is invalid: %w", err)
	}
	for _, name := range dnsNames {
		found := false
		for _, actual := range cert.DNSNames {
			if strings.EqualFold(name, actual) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("transport TLS certificate is missing DNS SAN %q", name)
		}
	}
	for _, ip := range ips {
		found := false
		for _, actual := range cert.IPAddresses {
			if ip.Equal(actual) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("transport TLS certificate is missing IP SAN %q", ip.String())
		}
	}
	return nil
}
