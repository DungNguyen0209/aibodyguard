package mitm

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestGenerateCA(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	if ca.cert == nil {
		t.Fatal("expected non-nil cert")
	}
	if ca.key == nil {
		t.Fatal("expected non-nil key")
	}
	if ca.cert.IsCA != true {
		t.Error("cert should be a CA")
	}
}

func TestSignLeaf(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}

	tlsCert, err := ca.signLeaf("api.githubcopilot.com")
	if err != nil {
		t.Fatalf("signLeaf: %v", err)
	}

	// Parse and verify the leaf cert is signed by our CA
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	_, err = leaf.Verify(x509.VerifyOptions{
		DNSName:     "api.githubcopilot.com",
		Roots:       pool,
		CurrentTime: time.Now(),
	})
	if err != nil {
		t.Errorf("leaf cert not valid for host: %v", err)
	}

	// Verify TLS config works
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	if len(tlsCfg.Certificates) != 1 {
		t.Error("expected 1 certificate in TLS config")
	}
}

func TestCAPEMBytes(t *testing.T) {
	ca, err := generateCA()
	if err != nil {
		t.Fatalf("generateCA: %v", err)
	}
	pem := ca.pemBytes()
	if len(pem) == 0 {
		t.Error("expected non-empty PEM bytes")
	}
	// Should be parseable
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Error("PEM bytes could not be parsed into cert pool")
	}
}
