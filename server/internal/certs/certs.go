// Package certs creates the self-signed certificate used for SIP over TLS and for HTTPS on the
// portal and provisioning port, until a real certificate is configured (internet mode, phase 3).
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// mu serialises Ensure: the portal and the Asterisk config generator ask for the certificate at the
// same moment on first start, and two writers would leave a key that does not match the certificate.
var mu sync.Mutex

// Ensure returns the certificate and key paths in dir, creating them when missing or expiring.
func Ensure(dir string) (certPath, keyPath string, err error) {
	mu.Lock()
	defer mu.Unlock()
	certPath, keyPath = filepath.Join(dir, "tls", "kiestoon.crt"), filepath.Join(dir, "tls", "kiestoon.key")
	if b, err := os.ReadFile(certPath); err == nil {
		if blk, _ := pem.Decode(b); blk != nil {
			if c, err := x509.ParseCertificate(blk.Bytes); err == nil && time.Until(c.NotAfter) > 30*24*time.Hour {
				if _, err := os.Stat(keyPath); err == nil {
					return certPath, keyPath, nil
				}
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0o750); err != nil {
		return "", "", err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "kiestoon"
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host, Organization: []string{"Kiestoon"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(5, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{host, "localhost"},
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				tpl.IPAddresses = append(tpl.IPAddresses, n.IP)
			}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	// The key first and the certificate last: a valid certificate on disk means the pair is complete.
	if err := writeAtomic(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0o640); err != nil {
		return "", "", err
	}
	if err := writeAtomic(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

func writeAtomic(path string, b []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
