// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package localtlstrust

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"grxfirma/internal/adapters/outbound/common/securefile"
)

var (
	ErrSoporteNoDisponible     = errors.New("localtlstrust: soporte no disponible en esta plataforma")
	ErrHerramientaNoDisponible = errors.New("localtlstrust: herramienta del sistema no disponible")
)

const (
	nicknameLocalhost             = "GrxFirma localhost"
	nicknameLocalhostRoot         = "GrxFirma Local Root CA"
	managedLocalCAOwnershipMarker = "GrxFirma Managed Local TLS CA v1"
	managedTrustInventorySuffix   = ".grxfirma-trust.json"
	maxTrustPEMFileBytes          = 1024 * 1024
)

// EnsureTrusted instala el certificado TLS local en el almacén de confianza del sistema actual.
func EnsureTrusted(ctx context.Context, certFile string) error {
	return ensureTrusted(ctx, certFile, false)
}

// EnsureManagedTrusted instala una CA TLS creada y marcada por GrxFirma.
// En Windows y Linux mantiene un inventario de propiedad para poder rotarla y
// retirarla sin inferir propiedad por sujeto, nombre descriptivo ni ruta.
func EnsureManagedTrusted(ctx context.Context, certFile string) error {
	return ensureTrusted(ctx, certFile, true)
}

// EnsureManagedTrustedWithResult indica si este arranque añadió confianza a un
// almacén del usuario. El error puede coexistir con cambios en otros perfiles.
func EnsureManagedTrustedWithResult(ctx context.Context, certFile string) (bool, error) {
	before, err := managedTrustChangeTokenPlatform(certFile)
	if err != nil {
		return false, err
	}
	installErr := EnsureManagedTrusted(ctx, certFile)
	after, tokenErr := managedTrustChangeTokenPlatform(certFile)
	return before != after, errors.Join(installErr, tokenErr)
}

// RemoveManagedTrusted retira únicamente las huellas que el inventario
// protegido acredita que fueron añadidas por GrxFirma. Un inventario
// ausente o una CA no marcada nunca autorizan una eliminación.
func RemoveManagedTrusted(ctx context.Context, certFile string) error {
	return removeManagedTrustedPlatform(ctx, certFile)
}

// ManagedTrustLifecycleSupported indica si la plataforma ofrece el ciclo
// completo de alta y retirada con inventario de propiedad. No basta con poder
// instalar una CA: las interfaces no deben anunciar una acción irreversible.
func ManagedTrustLifecycleSupported() bool {
	return managedTrustLifecycleSupportedPlatform()
}

func ensureTrusted(ctx context.Context, certFile string, managed bool) error {
	cert, certPEM, err := cargarCertificado(certFile)
	if err != nil {
		return err
	}
	if managed && !IsManagedLocalCA(cert) {
		return errors.New("localtlstrust: la CA local no contiene el marcador de propiedad de GrxFirma")
	}
	validatedFile, err := os.CreateTemp("", "grxfirma-trusted-cert-*.pem")
	if err != nil {
		return fmt.Errorf("localtlstrust: crear certificado temporal validado: %w", err)
	}
	validatedPath := validatedFile.Name()
	defer os.Remove(validatedPath)
	if err := validatedFile.Chmod(0o600); err != nil {
		_ = validatedFile.Close()
		return fmt.Errorf("localtlstrust: proteger certificado temporal validado: %w", err)
	}
	if _, err := validatedFile.Write(certPEM); err != nil {
		_ = validatedFile.Close()
		return fmt.Errorf("localtlstrust: escribir certificado temporal validado: %w", err)
	}
	if err := validatedFile.Close(); err != nil {
		return fmt.Errorf("localtlstrust: cerrar certificado temporal validado: %w", err)
	}
	if managed {
		return ensureManagedTrustedPlatform(ctx, certFile, validatedPath, cert)
	}
	return ensureTrustedPlatform(ctx, certFile, validatedPath, cert)
}

// MarkManagedLocalCA añade a una plantilla de CA el marcador que permite
// distinguir CAs nuevas del producto de certificados ajenos con un CN similar.
func MarkManagedLocalCA(template *x509.Certificate) {
	if template == nil {
		return
	}
	if !containsExact(template.Subject.OrganizationalUnit, managedLocalCAOwnershipMarker) {
		template.Subject.OrganizationalUnit = append(
			template.Subject.OrganizationalUnit,
			managedLocalCAOwnershipMarker,
		)
	}
}

// IsManagedLocalCA valida la identidad estructural exigida antes de atribuir o
// retirar confianza. La huella exacta se valida adicionalmente contra el
// inventario.
func IsManagedLocalCA(cert *x509.Certificate) bool {
	if cert == nil ||
		!cert.IsCA ||
		!cert.BasicConstraintsValid ||
		cert.Subject.CommonName != nicknameLocalhostRoot ||
		!containsExact(cert.Subject.Organization, "Diputacion de Granada") ||
		!containsExact(cert.Subject.OrganizationalUnit, managedLocalCAOwnershipMarker) ||
		cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return false
	}
	return cert.CheckSignatureFrom(cert) == nil
}

func containsExact(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func cargarCertificado(certFile string) (*x509.Certificate, []byte, error) {
	data, err := securefile.ReadFileLimit(certFile, maxTrustPEMFileBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("localtlstrust: leer certificado: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, nil, errors.New("localtlstrust: PEM de certificado inválido")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("localtlstrust: parsear certificado: %w", err)
	}
	return cert, data, nil
}

func fingerprintSHA256(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}
