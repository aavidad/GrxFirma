// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/adapters/outbound/desktop/localtlstrust"
)

const (
	// ManagedLocalhostPrefix identifica la única CA y hoja TLS del navegador.
	ManagedLocalhostPrefix  = "websocket-localhost"
	defaultCertPrefix       = ManagedLocalhostPrefix
	maxTLSPEMFileBytes      = 1024 * 1024
	localServerCertLifetime = 30 * 24 * time.Hour
	localServerRenewBefore  = 7 * 24 * time.Hour
)

// TLSServer expone el servidor y los artefactos del certificado local.
type TLSServer struct {
	Server   *http.Server
	Addr     string
	CertFile string
	KeyFile  string
}

// StartTLSServer arranca HTTPS local con la identidad TLS compartida del navegador.
func StartTLSServer(ctx context.Context, addr string, handler http.Handler, certDir string) (*TLSServer, error) {
	return startTLSServer(ctx, addr, handler, certDir, tls.VersionTLS12)
}

// StartTLS13Server reserva TLS 1.3 para el servicio de solo verificación.
// El servidor REST normal conserva su política de transporte anterior.
func StartTLS13Server(ctx context.Context, addr string, handler http.Handler, certDir string) (*TLSServer, error) {
	return startTLSServer(ctx, addr, handler, certDir, tls.VersionTLS13)
}

// StartTLS13ServerWithKeyPair publica el validador con un certificado y una
// clave del operador, en lugar de la identidad local de los navegadores.
func StartTLS13ServerWithKeyPair(ctx context.Context, addr string, handler http.Handler, certFile, keyFile string) (*TLSServer, error) {
	cert, certPEM, err := loadX509KeyPairSecure(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("rest: no se pudo cargar el par TLS propio: %w", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("rest: el certificado TLS propio no es PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("rest: el certificado TLS propio no es válido: %w", err)
	}
	if now := time.Now(); now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		return nil, errors.New("rest: el certificado TLS propio no está vigente")
	}
	return serveTLS(ctx, addr, handler, cert, certFile, keyFile, tls.VersionTLS13)
}

// ManagedLocalhostCAFile devuelve la CA local persistente que firma la hoja TLS.
func ManagedLocalhostCAFile(certDir string) string {
	return filepath.Join(certDir, ManagedLocalhostPrefix+"-root.crt.pem")
}

func startTLSServer(ctx context.Context, addr string, handler http.Handler, certDir string, minVersion uint16) (*TLSServer, error) {
	certFile, keyFile, _, source, err :=
		EnsureBrowserCompatibleLocalhostCertificate(certDir, defaultCertPrefix)
	if err != nil {
		return nil, err
	}
	if source != "grxfirma-local-ca" {
		return nil, errors.New("rest: la fuente del certificado TLS local no es gestionable")
	}

	cert, _, err := loadX509KeyPairSecure(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("rest: no se pudo cargar el par TLS: %w", err)
	}
	return serveTLS(ctx, addr, handler, cert, certFile, keyFile, minVersion)
}

func serveTLS(ctx context.Context, addr string, handler http.Handler, cert tls.Certificate, certFile, keyFile string, minVersion uint16) (*TLSServer, error) {
	tlsCfg := serverTLSConfig(cert, minVersion)

	ln, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("rest: no se pudo abrir el listener TLS: %w", err)
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	go func() {
		_ = srv.Serve(ln)
	}()

	return &TLSServer{
		Server:   srv,
		Addr:     ln.Addr().String(),
		CertFile: certFile,
		KeyFile:  keyFile,
	}, nil
}

func serverTLSConfig(cert tls.Certificate, minVersion uint16) *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
	if minVersion > tls.VersionTLS12 {
		cfg.MinVersion = minVersion
	}
	return cfg
}

// EnsureLocalhostCertificate crea o reutiliza un certificado válido para localhost y 127.0.0.1.
func EnsureLocalhostCertificate(dir, prefix string) (string, string, error) {
	if prefix == "" {
		prefix = defaultCertPrefix
	}
	if dir == "" {
		dir = os.TempDir()
	}
	if err := prepareTLSDirectory(dir); err != nil {
		return "", "", err
	}

	certFile := filepath.Join(dir, prefix+".crt.pem")
	keyFile := filepath.Join(dir, prefix+".key.pem")
	if certValidoLocalhost(certFile, keyFile) {
		return certFile, keyFile, nil
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("rest: no se pudo generar la clave TLS: %w", err)
	}

	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{localtlstrust.ManagedLocalCAOrganization},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(180 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", fmt.Errorf("rest: no se pudo crear el certificado TLS: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if err := writeTLSFileAtomic(certFile, certPEM, 0o600); err != nil {
		return "", "", fmt.Errorf("rest: no se pudo escribir el certificado TLS: %w", err)
	}
	if err := writeTLSFileAtomic(keyFile, keyPEM, 0o600); err != nil {
		return "", "", fmt.Errorf("rest: no se pudo escribir la clave TLS: %w", err)
	}

	return certFile, keyFile, nil
}

// EnsureLocalhostCertificateWithLocalCA crea o reutiliza una CA local y un
// certificado de servidor para localhost firmado por esa CA.
func EnsureLocalhostCertificateWithLocalCA(dir, prefix string) (string, string, string, error) {
	if prefix == "" {
		prefix = defaultCertPrefix
	}
	if dir == "" {
		dir = os.TempDir()
	}
	if err := prepareTLSDirectory(dir); err != nil {
		return "", "", "", err
	}
	unlock, err := acquireTLSLock(filepath.Join(dir, "."+prefix+"-ca.lock"))
	if err != nil {
		return "", "", "", fmt.Errorf("rest: no se pudo bloquear la renovación TLS: %w", err)
	}
	defer unlock()

	rootCertFile := filepath.Join(dir, prefix+"-root.crt.pem")
	rootKeyFile := filepath.Join(dir, prefix+"-root.key.pem")
	certFile := filepath.Join(dir, prefix+".crt.pem")
	keyFile := filepath.Join(dir, prefix+".key.pem")
	rootCert, rootKey := managedLocalCAForRenewal(rootCertFile, rootKeyFile)
	if rootCert != nil && certValidoLocalhostFirmadoPorCA(certFile, keyFile, rootCertFile) {
		return certFile, keyFile, rootCertFile, nil
	}
	newRoot := rootCert == nil
	var rootDER []byte
	if newRoot {
		var err error
		rootKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", "", "", fmt.Errorf("rest: no se pudo generar la clave CA TLS: %w", err)
		}
		rootTPL := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject: pkix.Name{
				CommonName:   "GrxFirma Local Root CA",
				Organization: []string{localtlstrust.ManagedLocalCAOrganization},
				Country:      []string{"ES"},
			},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().AddDate(10, 0, 0),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: true,
			IsCA:                  true,
			MaxPathLenZero:        true,
			// La extensión no crítica preserva la compatibilidad con verificadores
			// de navegador aún no validados; Go aplica igualmente la restricción.
			PermittedDNSDomainsCritical: false,
			PermittedDNSDomains:         []string{"localhost"},
			PermittedIPRanges: []*net.IPNet{
				{IP: net.IPv4(127, 0, 0, 0).To4(), Mask: net.CIDRMask(8, 32)},
				{IP: net.IPv6loopback, Mask: net.CIDRMask(128, 128)},
			},
		}
		localtlstrust.MarkManagedLocalCA(rootTPL)
		rootDER, err = x509.CreateCertificate(rand.Reader, rootTPL, rootTPL, &rootKey.PublicKey, rootKey)
		if err != nil {
			return "", "", "", fmt.Errorf("rest: no se pudo crear la CA TLS: %w", err)
		}
		rootCert, err = x509.ParseCertificate(rootDER)
		if err != nil {
			return "", "", "", fmt.Errorf("rest: no se pudo parsear la CA TLS: %w", err)
		}
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", "", fmt.Errorf("rest: no se pudo generar la clave TLS: %w", err)
	}
	serverTPL := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() + 1),
		Subject: pkix.Name{
			CommonName:   "127.0.0.1",
			Organization: []string{"GrxFirma Local Server"},
			Country:      []string{"ES"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}
	serverTPL.NotAfter = serverTPL.NotBefore.Add(localServerCertLifetime)
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTPL, rootCert, &serverKey.PublicKey, rootKey)
	if err != nil {
		return "", "", "", fmt.Errorf("rest: no se pudo crear el certificado TLS: %w", err)
	}

	if newRoot {
		rootKeyDER := x509.MarshalPKCS1PrivateKey(rootKey)
		err := localtlstrust.SaveManagedCAKey(rootKeyFile, rootKeyDER)
		secmem.Zeroize(rootKeyDER)
		if err != nil {
			return "", "", "", fmt.Errorf("rest: no se pudo guardar la clave CA TLS: %w", err)
		}
		if err := writePEMCertificate(rootCertFile, rootDER); err != nil {
			return "", "", "", err
		}
	}
	if err := writePEMCertificate(certFile, serverDER); err != nil {
		return "", "", "", err
	}
	if err := writePEMKey(keyFile, serverKey); err != nil {
		return "", "", "", err
	}
	return certFile, keyFile, rootCertFile, nil
}

// EnsureBrowserCompatibleLocalhostCertificate usa siempre la CA local marcada
// e inventariable de V2. No reutiliza material de versiones anteriores: aunque
// el par sea criptográficamente válido, instalar su raíz desde V2 impediría
// distinguir después si la confianza era preexistente o fue añadida por V2.
func EnsureBrowserCompatibleLocalhostCertificate(dir, prefix string) (string, string, string, string, error) {
	if prefix == ManagedLocalhostPrefix {
		if err := retireLegacyRESTCertificate(context.Background(), dir); err != nil {
			return "", "", "", "", err
		}
	}
	certFile, keyFile, rootCertFile, err := EnsureLocalhostCertificateWithLocalCA(dir, prefix)
	return certFile, keyFile, rootCertFile, "grxfirma-local-ca", err
}

func certValidoLocalhost(certFile, keyFile string) bool {
	if !regularTLSFile(certFile) || !securePrivateKeyFile(keyFile) {
		return false
	}
	_, certPEM, err := loadX509KeyPairSecure(certFile, keyFile)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	ahora := time.Now()
	if ahora.Before(cert.NotBefore) || ahora.After(cert.NotAfter) {
		return false
	}
	if err := cert.VerifyHostname("localhost"); err != nil {
		return false
	}
	return true
}

func certValidoLocalhostFirmadoPorCA(certFile, keyFile, rootCertFile string) bool {
	if !regularTLSFile(certFile) || !securePrivateKeyFile(keyFile) || !regularTLSFile(rootCertFile) {
		return false
	}
	pair, _, err := loadX509KeyPairSecure(certFile, keyFile)
	if err != nil || len(pair.Certificate) == 0 {
		return false
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return false
	}
	root, ok := cargarCertificadoLocal(rootCertFile)
	if !ok {
		return false
	}
	ahora := time.Now()
	if ahora.Before(cert.NotBefore) || ahora.Add(localServerRenewBefore).After(cert.NotAfter) ||
		cert.NotAfter.Sub(cert.NotBefore) > localServerCertLifetime ||
		ahora.Before(root.NotBefore) || ahora.After(root.NotAfter) {
		return false
	}
	if !root.IsCA {
		return false
	}
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != "localhost" ||
		len(cert.IPAddresses) != 2 ||
		!cert.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")) ||
		!cert.IPAddresses[1].Equal(net.IPv6loopback) {
		return false
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	_, err = cert.Verify(x509.VerifyOptions{
		DNSName: "127.0.0.1",
		Roots:   pool,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	})
	if err != nil {
		return false
	}
	_, err = cert.Verify(x509.VerifyOptions{
		DNSName:   "localhost",
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err == nil
}

func managedLocalCAForRenewal(certFile, keyFile string) (*x509.Certificate, *rsa.PrivateKey) {
	root, ok := cargarCertificadoLocal(certFile)
	if !ok || !localtlstrust.IsManagedLocalCA(root) || !localCANameConstraintsValid(root) {
		return nil, nil
	}
	now := time.Now()
	if now.Before(root.NotBefore) || now.Add(localServerCertLifetime).After(root.NotAfter) {
		return nil, nil
	}
	keyDER, err := localtlstrust.LoadManagedCAKey(keyFile)
	if err != nil {
		return nil, nil
	}
	defer secmem.Zeroize(keyDER)
	key, err := x509.ParsePKCS1PrivateKey(keyDER)
	if err != nil {
		return nil, nil
	}
	public, ok := root.PublicKey.(*rsa.PublicKey)
	if !ok || key.PublicKey.E != public.E || key.PublicKey.N.Cmp(public.N) != 0 {
		return nil, nil
	}
	return root, key
}

func localCANameConstraintsValid(cert *x509.Certificate) bool {
	if len(cert.PermittedDNSDomains) != 1 || cert.PermittedDNSDomains[0] != "localhost" ||
		len(cert.PermittedIPRanges) != 2 || len(cert.ExcludedDNSDomains) != 0 ||
		len(cert.ExcludedIPRanges) != 0 || len(cert.PermittedEmailAddresses) != 0 ||
		len(cert.PermittedURIDomains) != 0 {
		return false
	}
	want := []string{"127.0.0.0/8", "::1/128"}
	for i, permitted := range cert.PermittedIPRanges {
		if permitted == nil || permitted.String() != want[i] {
			return false
		}
	}
	return true
}

func cargarCertificadoLocal(certFile string) (*x509.Certificate, bool) {
	if !regularTLSFile(certFile) {
		return nil, false
	}
	certPEM, err := securefile.ReadFileLimit(certFile, maxTLSPEMFileBytes)
	if err != nil {
		return nil, false
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	return cert, err == nil
}

func loadX509KeyPairSecure(certFile, keyFile string) (tls.Certificate, []byte, error) {
	certPEM, err := securefile.ReadFileLimit(certFile, maxTLSPEMFileBytes)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	keyPEM, err := securefile.ReadFileLimit(keyFile, maxTLSPEMFileBytes)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	defer secmem.Zeroize(keyPEM)
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return pair, certPEM, nil
}

func writePEMCertificate(path string, der []byte) error {
	if err := writeTLSFileAtomic(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		return fmt.Errorf("rest: no se pudo escribir el certificado TLS: %w", err)
	}
	return nil
}

func writePEMKey(path string, key *rsa.PrivateKey) error {
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := writeTLSFileAtomic(path, keyPEM, 0o600); err != nil {
		return fmt.Errorf("rest: no se pudo escribir la clave TLS: %w", err)
	}
	return nil
}

func prepareTLSDirectory(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("rest: no se pudo crear el directorio TLS: %w", err)
	}
	if err := securefile.ProtectDirectory(dir, 0o700); err != nil {
		return fmt.Errorf("rest: no se pudieron restringir los permisos TLS: %w", err)
	}
	return nil
}

func regularTLSFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func securePrivateKeyFile(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o077 == 0
}

func removeTLSArtifact(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s es un directorio", filepath.Base(path))
	}
	return os.Remove(path)
}

func writeTLSFileAtomic(path string, data []byte, mode os.FileMode) (retErr error) {
	if err := removeTLSArtifact(path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if retErr != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}
