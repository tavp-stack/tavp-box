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
	"strings"
	"sync"
	"time"
)

const caCommonName = "TAVPBox Local CA"

// Dir returns the directory holding the TAVPBox CA material.
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tavpbox", "ca")
}

// dirOverride lets tests redirect CA storage (empty = default).
var dirOverride string

func caDir() string {
	if dirOverride != "" {
		return dirOverride
	}
	return Dir()
}

type keypair struct {
	cert *x509.Certificate
	tls  *tls.Certificate
}

var (
	cacheMu   sync.Mutex
	cache     = map[string]*keypair{} // issued certs by name ("*" or full host)
	trustedMu sync.Mutex
	trustedOK bool // whether CA presence in the store has been verified this run
)

// EnsureWildcard makes sure a private CA and a wildcard leaf for "*.suffix"
// exist and are trusted by the current user, then returns the leaf ready for
// an HTTPS server. The CA is name-constrained to the suffix so a leaked key
// cannot be used to spoof unrelated domains.
func EnsureWildcard(suffix string) (tls.Certificate, error) {
	kp, err := wildcardKeypair(suffix)
	if err != nil {
		return tls.Certificate{}, err
	}
	if !trusted(kp.cert, "check."+suffix) {
		if err := installCA(filepath.Join(caDir(), "ca.crt")); err != nil {
			return tls.Certificate{}, fmt.Errorf("installing CA into trust store: %w", err)
		}
		if !trusted(kp.cert, "check."+suffix) {
			return tls.Certificate{}, errors.New("certs: CA installed but chain is still untrusted")
		}
		trustedMu.Lock()
		trustedOK = true
		trustedMu.Unlock()
		fmt.Printf("  TAVPBox local CA installed into your trust store (one-time)\n")
	}
	return *kp.tls, nil
}

// ForHost returns a trusted certificate for exactly host (any subdomain
// depth under suffix), issuing it on demand from the local CA. Unknown or
// out-of-suffix names fall back to the wildcard certificate. Call
// EnsureWildcard first so the CA is present and trusted.
func ForHost(host, suffix string) (*tls.Certificate, error) {
	host = NormalizeHost(host)
	suffix = strings.ToLower(strings.Trim(suffix, "."))

	if !strings.HasSuffix(host, "."+suffix) || strings.Contains(host, "*") {
		kp, err := wildcardKeypair(suffix)
		if err != nil {
			return nil, err
		}
		return kp.tls, nil
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()
	if kp := cache[host]; kp != nil && time.Until(kp.cert.NotAfter) > 30*24*time.Hour {
		return kp.tls, nil
	}
	kp, err := issueHost(host, suffix)
	if err != nil {
		return nil, err
	}
	cache[host] = kp
	return kp.tls, nil
}

// NormalizeHost lowercases and strips any trailing port.
func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}

func caPaths() (string, string) {
	return filepath.Join(caDir(), "ca.crt"), filepath.Join(caDir(), "ca.key")
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

	if err := os.MkdirAll(caDir(), 0755); err != nil {
		return nil, nil, err
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

// wildcardKeypair loads or issues the "*.suffix" leaf (disk-cached).
func wildcardKeypair(suffix string) (*keypair, error) {
	if suffix == "" {
		return nil, errors.New("certs: empty domain suffix")
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if kp := cache["*"]; kp != nil && time.Until(kp.cert.NotAfter) > 30*24*time.Hour {
		return kp, nil
	}

	caCert, caKey, err := ensureCA(suffix)
	if err != nil {
		return nil, err
	}
	certPath := filepath.Join(caDir(), "wildcard-"+suffix+".crt")
	keyPath := filepath.Join(caDir(), "wildcard-"+suffix+".key")

	kp := loadKeypair(certPath, keyPath)
	if needsIssue(kp) {
		if err := writeLeaf(certPath, keyPath, []string{"*." + suffix, suffix}, caCert, caKey); err != nil {
			return nil, err
		}
		kp = loadKeypair(certPath, keyPath)
	}
	if kp == nil {
		return nil, errors.New("certs: failed to load wildcard certificate")
	}
	cache["*"] = kp
	return kp, nil
}

// issueHost creates an in-memory certificate for a concrete host.
func issueHost(host, suffix string) (*keypair, error) {
	caCert, caKey, err := ensureCA(suffix)
	if err != nil {
		return nil, err
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: host, Organization: []string{"tavp-box"}},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 3, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	tlsc := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv, Leaf: cert}
	return &keypair{cert: cert, tls: &tlsc}, nil
}

func writeLeaf(certPath, keyPath string, dnsNames []string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: dnsNames[0], Organization: []string{"tavp-box"}},
		DNSNames:     dnsNames,
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
