package panel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func certDir() string { return filepath.Join(skiffDir(), "certs") }

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ensureServerCert writes (or reuses) server.crt/.key/.pem in ~/.skiff/certs; server.pem is cert+key concatenated, which MongoDB wants as one file.
func ensureServerCert() (string, error) {
	dir := certDir()
	crtPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	pemPath := filepath.Join(dir, "server.pem")
	if fileExists(crtPath) && fileExists(keyPath) && fileExists(pemPath) {
		return dir, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", err
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "skiff-db", Organization: []string{"Skiff"}},
		DNSNames:     []string{"skiff-db"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		// Self-signed but marked IsCA so it's its own chain of trust — MongoDB refuses to serve TLS otherwise (SERVER-72839).
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", err
	}
	crtPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(crtPath, crtPEM, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(pemPath, append(append([]byte{}, crtPEM...), keyPEM...), 0o600); err != nil {
		return "", err
	}
	return dir, nil
}
