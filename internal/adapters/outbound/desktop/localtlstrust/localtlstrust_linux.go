// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package localtlstrust

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

const (
	trustNSSWebTLSRoot = "C,,"
	trustNSSWebTLSPeer = "P,,"
)

const trustCacheTTL = 12 * time.Hour

type instaladorNSS struct {
	certutil      string
	rutas         []string
	initMissingDB bool
	inspectLegacy bool
}

type trustCacheRecord struct {
	Fingerprint string    `json:"fingerprint"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func ensureTrustedPlatform(ctx context.Context, certFile, validatedCertFile string, cert *x509.Certificate) error {
	return newInstaladorNSS().EnsureTrusted(ctx, certFile, validatedCertFile, cert)
}

func newInstaladorNSS() *instaladorNSS {
	return &instaladorNSS{
		certutil: "certutil",
		rutas:    rutasNSS(),
	}
}

func (i *instaladorNSS) EnsureTrusted(ctx context.Context, certFile, validatedCertFile string, cert *x509.Certificate) error {
	start := time.Now()
	if ok, err := trustCacheHit(certFile, cert); err == nil && ok {
		slog.Info("localtlstrust_nss_cache_hit", "elapsed_ms", time.Since(start).Milliseconds())
		return nil
	}
	if len(i.rutas) == 0 {
		slog.Info("localtlstrust_nss_skip_no_paths", "elapsed_ms", time.Since(start).Milliseconds())
		return nil
	}
	var encontradoAlmacen bool
	var instaladoEnAlguno bool
	var ultimoErr error

	for _, ruta := range i.rutas {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !esDirectorio(ruta) {
			continue
		}
		encontradoAlmacen = true

		nickname := nicknameParaCertificado(cert)
		instalado, err := i.certificadoInstalado(ctx, ruta, nickname, cert)
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return ErrHerramientaNoDisponible
			}
			ultimoErr = err
			continue
		}
		if instalado {
			instaladoEnAlguno = true
			continue
		}

		if err := i.reemplazarCertificado(ctx, ruta, nickname, trustParaCertificado(cert), validatedCertFile); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return ErrHerramientaNoDisponible
			}
			ultimoErr = err
			continue
		}
		instaladoEnAlguno = true
	}

	if !encontradoAlmacen {
		slog.Info("localtlstrust_nss_skip_no_store", "elapsed_ms", time.Since(start).Milliseconds(), "paths", len(i.rutas))
		return nil
	}
	if instaladoEnAlguno {
		_ = writeTrustCache(certFile, cert)
		slog.Info("localtlstrust_nss_ready", "elapsed_ms", time.Since(start).Milliseconds(), "paths", len(i.rutas))
		return nil
	}
	if ultimoErr != nil {
		slog.Warn("localtlstrust_nss_error", "elapsed_ms", time.Since(start).Milliseconds(), "paths", len(i.rutas), "error", ultimoErr)
		return ultimoErr
	}
	slog.Info("localtlstrust_nss_noop", "elapsed_ms", time.Since(start).Milliseconds(), "paths", len(i.rutas))
	return nil
}

func trustCachePath(certFile string) string {
	return certFile + ".trustcache.json"
}

func trustCacheHit(certFile string, cert *x509.Certificate) (bool, error) {
	certHandle, err := securefile.OpenRead(certFile)
	if err != nil {
		return false, err
	}
	defer certHandle.Close()
	info, err := certHandle.Stat()
	if err != nil {
		return false, err
	}
	raw, err := securefile.ReadFileLimit(trustCachePath(certFile), 64*1024)
	if err != nil {
		return false, err
	}
	var rec trustCacheRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return false, err
	}
	if rec.Fingerprint != fingerprintSHA256(cert) {
		return false, nil
	}
	if rec.UpdatedAt.Before(info.ModTime()) {
		return false, nil
	}
	if time.Since(rec.UpdatedAt) > trustCacheTTL {
		return false, nil
	}
	return true, nil
}

func writeTrustCache(certFile string, cert *x509.Certificate) error {
	rec := trustCacheRecord{
		Fingerprint: fingerprintSHA256(cert),
		UpdatedAt:   time.Now(),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return securefile.WriteFileAtomic(trustCachePath(certFile), data, 0o600)
}

func (i *instaladorNSS) certificadoInstalado(ctx context.Context, ruta, nickname string, cert *x509.Certificate) (bool, error) {
	instalado, err := i.certificadoPorNickname(ctx, ruta, nickname)
	if err != nil || instalado == nil {
		return false, err
	}
	return fingerprintSHA256(instalado) == fingerprintSHA256(cert) && certVigente(instalado, time.Now()), nil
}

func (i *instaladorNSS) certificadoPorNickname(ctx context.Context, ruta, nickname string) (*x509.Certificate, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// #nosec G204 -- certutil is a fixed platform tool (or a test fixture);
	// NSS path and nickname are separate arguments.
	cmd := exec.CommandContext(ctx, i.certutil, "-L", "-d", ruta, "-n", nickname, "-a")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("localtlstrust: certutil -L -n en %s: %w", ruta, err)
	}
	instalado, err := parsearCertificadoPEM(out)
	if err != nil {
		return nil, err
	}
	return instalado, nil
}

func (i *instaladorNSS) reemplazarCertificado(ctx context.Context, ruta, nickname, trust, certFile string) error {
	_ = i.ejecutar(ctx, "-D", "-d", ruta, "-n", nickname)
	return i.ejecutar(ctx, "-A", "-d", ruta, "-n", nickname, "-t", trust, "-a", "-i", certFile)
}

func nicknameParaCertificado(cert *x509.Certificate) string {
	if cert != nil && cert.IsCA {
		return nicknameLocalhostRoot
	}
	return nicknameLocalhost
}

func trustParaCertificado(cert *x509.Certificate) string {
	if cert != nil && cert.IsCA {
		return trustNSSWebTLSRoot
	}
	return trustNSSWebTLSPeer
}

func (i *instaladorNSS) ejecutar(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// #nosec G204 -- this helper receives only the fixed -D/-A certutil
	// operations constructed by reemplazarCertificado.
	cmd := exec.CommandContext(ctx, i.certutil, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("localtlstrust: %s %v: %w: %s", i.certutil, args, err, string(bytes.TrimSpace(out)))
	}
	return nil
}

func rutasNSS() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	rutas := []string{
		filepath.Join(home, ".pki", "nssdb"),
		"/etc/chromium/nssdb",
	}
	mozDir := filepath.Join(home, ".mozilla", "firefox")
	entries, err := os.ReadDir(mozDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				rutas = append(rutas, filepath.Join(mozDir, entry.Name()))
			}
		}
	}
	return rutas
}

func esDirectorio(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func parsearCertificadoPEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("localtlstrust: salida PEM NSS inválida")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("localtlstrust: parsear certificado NSS: %w", err)
	}
	return cert, nil
}
