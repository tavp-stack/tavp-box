package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const caCommonName = "TAVPBox Local CA"

// Dir returns the directory holding the TAVPBox CA material.
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tavpbox", "ca")
}

type keypair struct {
	cert *x509.Certificate
	tls  *tls.Certificate
}

// EnsureWildcard makes sure a private CA and a wildcard leaf for "*.suffix"
// exist and are trusted by the current user, then returns the leaf ready for
// an HTTPS server. The CA is name-constrained to the suffix so a leaked key
// cannot be used to spoof unrelated domains. The leaf is re-issued when it
// expires within 30 days or when the suffix changes.
func EnsureWildcard(suffix string) (tls.Certificate, error) {
	if suffix == "" {
		return tls.Certificate{}, errors.New("certs: empty domain suffix")
	}
	if err := os.MkdirAll(Dir(), 0755); err != nil {
		return tls.Certificate{}, err
	}

	caCert, caKey, err := ensureCA(suffix)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPath := filepath.Join(Dir(), "wildcard-"+suffix+".crt")
	keyPath := filepath.Join(Dir(), "wildcard-"+suffix+".key")

	kp := loadKeypair(certPath, keyPath)
	if needsIssue(kp) {
		if err := issueLeaf(certPath, keyPath, suffix, caCert, caKey); err != nil {
			return tls.Certificate{}, err
		}
		kp = loadKeypair(certPath, keyPath)
	}
	if kp == nil || kp.cert == nil || kp.tls == nil {
		return tls.Certificate{}, errors.New("certs: failed to load wildcard certificate")
	}

	if !trusted(kp.cert, "check."+suffix) {
		if err := installCA(filepath.Join(Dir(), "ca.crt")); err != nil {
			return tls.Certificate{}, fmt.Errorf("installing CA into trust store: %w", err)
		}
		if !trusted(kp.cert, "check."+suffix) {
			return tls.Certificate{}, errors.New("certs: CA installed but chain is still untrusted")
		}
		fmt.Printf("  TAVPBox local CA installed into your trust store (one-time)\n")
	}
	return *kp.tls, nil
}

func caPaths() (string, string) {
	return filepath.Join(Dir(), "ca.crt"), filepath.Join(Dir(), "ca.key")
}

func ensureCA(suffix string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPath, keyPath := caPaths()
	cb, err1 := os.ReadFile(certPath)
	kb, err2 := os.ReadFile(keyPath)
	if err1 == nil && err2 == nil {
		cblk, _ := pem.Decode(cb)
		kblk, _ := pem.Decode(kb)
		if cblk != nil && kblk != nil {
			c, errC := x509.ParseCertificate(cblk.Bytes)
			k, errK := x509.ParseECPrivateKey(kblk.Bytes)
			if errC == nil && errK == nil {
				return c, k, nil
			}
		}
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	tpl := &x509.Certificate{
		SerialNumber:                big.NewInt(time.Now().UnixNano()),
		Subject:                     pkix.Name{CommonName: caCommonName, Organization: []string{"tavp-box"}},
		NotBefore:                   time.Now().Add(-time.Hour),
		NotAfter:                    time.Now().AddDate(10, 0, 0),
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid:       true,
		IsCA:                        true,
		MaxPathLen:                  1,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         []string{suffix, "." + suffix},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, priv, nil
}

func issueLeaf(certPath, keyPath, suffix string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "*." + suffix, Organization: []string{"tavp-box"}},
		DNSNames:     []string{"*." + suffix, suffix},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 3, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		return err
	}
	return os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600)
}

func loadKeypair(certPath, keyPath string) *keypair {
	cb, err1 := os.ReadFile(certPath)
	kb, err2 := os.ReadFile(keyPath)
	if err1 != nil || err2 != nil {
		return nil
	}
	cblk, _ := pem.Decode(cb)
	kblk, _ := pem.Decode(kb)
	if cblk == nil || kblk == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(cblk.Bytes)
	if err != nil {
		return nil
	}
	key, err := x509.ParseECPrivateKey(kblk.Bytes)
	if err != nil {
		return nil
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil
	}
	tlsc, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cblk.Bytes}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	)
	if err != nil {
		return nil
	}
	return &keypair{cert: cert, tls: &tlsc}
}

func needsIssue(kp *keypair) bool {
	if kp == nil || kp.cert == nil || kp.tls == nil {
		return true
	}
	return time.Until(kp.cert.NotAfter) < 30*24*time.Hour
}

// trusted reports whether the leaf chains to a system-trusted root for the
// given DNS name.
func trusted(leaf *x509.Certificate, dnsName string) bool {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	_, err = leaf.Verify(x509.VerifyOptions{DNSName: dnsName, Roots: pool})
	return err == nil
}

// installCA adds the TAVPBox CA to the current user's trust store.
func installCA(caPath string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("certutil", "-user", "-addstore", "Root", caPath).Run()
	case "linux":
		dest := "/usr/local/share/ca-certificates/tavpbox-local-ca.crt"
		data, err := os.ReadFile(caPath)
		if err != nil {
			return err
		}
		cp := exec.Command("cp", caPath, dest)
		cp.Stdout, cp.Stderr = os.Stdout, os.Stderr
		if err := cp.Run(); err != nil {
			sudo := exec.Command("sudo", "-n", "cp", caPath, dest)
			if err := sudo.Run(); err != nil {
				return errors.New("need root: copy CA to " + dest + " manually, then run update-ca-certificates")
			}
			if err := os.WriteFile("/tmp/tavpbox-ca.tmp", data, 0644); err != nil {
				return err
			}
		}
		upd := exec.Command("update-ca-certificates")
		if upd.Run() != nil {
			exec.Command("sudo", "-n", "update-ca-certificates").Run()
		}
		return nil
	case "darwin":
		home, _ := os.UserHomeDir()
		cmd := exec.Command("security", "add-trusted-cert", "-r", "trustRoot",
			"-k", filepath.Join(home, "Library", "Keychains", "login.keychain-db"), caPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, string(out))
		}
		return nil
	default:
		return errors.New("unsupported platform: install " + caPath + " into your trust store manually")
	}
}
