// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/domain"
)

const maxPublicCertificateBytes = 64 * 1024
const maxPublicRecipients = 128

// LocalPublicRecipientBook guarda únicamente certificados públicos para cifrado.
type LocalPublicRecipientBook struct {
	ConfigDir string
	mu        sync.Mutex
}

func NuevoLocalPublicRecipientBook(configDir string) *LocalPublicRecipientBook {
	return &LocalPublicRecipientBook{ConfigDir: strings.TrimSpace(configDir)}
}

func (b *LocalPublicRecipientBook) dir() string {
	return filepath.Join(b.ConfigDir, "protection", "public-recipients")
}

func publicRecipient(data []byte, origin string) (domain.ProtectionRecipient, error) {
	if len(data) == 0 || len(data) > maxPublicCertificateBytes {
		return domain.ProtectionRecipient{}, fmt.Errorf("el certificado supera el límite de 64 KiB o está vacío")
	}
	if strings.Contains(strings.ToUpper(string(data)), "PRIVATE KEY") {
		return domain.ProtectionRecipient{}, fmt.Errorf("el fichero contiene una clave privada; seleccione solo un certificado público")
	}
	der := data
	if strings.HasPrefix(strings.TrimSpace(string(data)), "-----BEGIN") {
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) != 0 || len(block.Headers) != 0 {
			return domain.ProtectionRecipient{}, fmt.Errorf("se requiere un único certificado X.509 público en PEM")
		}
		der = block.Bytes
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("se requiere un certificado X.509 público DER o PEM; no se admiten P12/PFX ni claves privadas")
	}
	if cert.IsCA {
		return domain.ProtectionRecipient{}, fmt.Errorf("el certificado es de una autoridad, no de un destinatario")
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return domain.ProtectionRecipient{}, fmt.Errorf("el certificado está caducado o aún no es válido")
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		return domain.ProtectionRecipient{}, fmt.Errorf("KeyUsage no permite keyEncipherment para RSA-OAEP")
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || pub.N == nil || pub.N.BitLen() < 2048 {
		return domain.ProtectionRecipient{}, fmt.Errorf("el perfil compatible requiere una clave pública RSA de al menos 2048 bits apta para cifrado")
	}
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return domain.ProtectionRecipient{}, fmt.Errorf("clave pública no válida: %w", err)
	}
	sum := sha256.Sum256(cert.Raw)
	label := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, cert.Subject.String()))
	if runes := []rune(label); len(runes) > 256 {
		label = string(runes[:256])
	}
	if label == "" {
		label = hex.EncodeToString(sum[:8])
	}
	return domain.ProtectionRecipient{ID: "x509-" + hex.EncodeToString(sum[:]), Label: label, Origin: origin, RSAOAEP256PublicKeyDER: pubDER, CertificateDER: append([]byte(nil), cert.Raw...)}, nil
}

// ValidatePublicRecipientCertificate aplica al certificado propio las mismas
// restricciones que se aplican al importar un destinatario público.
func ValidatePublicRecipientCertificate(der []byte) (domain.ProtectionRecipient, error) {
	return publicRecipient(der, "propio")
}

func (b *LocalPublicRecipientBook) Import(ctx context.Context, data []byte) (domain.ProtectionRecipient, error) {
	if err := ctx.Err(); err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if b == nil || b.ConfigDir == "" {
		return domain.ProtectionRecipient{}, fmt.Errorf("libro de destinatarios públicos no configurado")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	recipient, err := publicRecipient(data, "importado")
	if err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if err := os.MkdirAll(b.dir(), 0o700); err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if err := securefile.ProtectDirectory(filepath.Dir(b.dir()), 0o700); err != nil {
		return domain.ProtectionRecipient{}, err
	}
	if err := securefile.ProtectDirectory(b.dir(), 0o700); err != nil {
		return domain.ProtectionRecipient{}, err
	}
	entries, err := os.ReadDir(b.dir())
	if err != nil {
		return domain.ProtectionRecipient{}, err
	}
	path := filepath.Join(b.dir(), recipient.ID+".der")
	if existing, err := securefile.ReadFileLimit(path, maxPublicCertificateBytes); err == nil {
		if string(existing) == string(recipient.CertificateDER) {
			return recipient, nil
		}
		return domain.ProtectionRecipient{}, fmt.Errorf("el identificador ya existe con otro certificado")
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.ProtectionRecipient{}, err
	}
	if len(entries) >= maxPublicRecipients {
		return domain.ProtectionRecipient{}, fmt.Errorf("el libro admite como máximo 128 destinatarios")
	}
	if err := securefile.WriteFileAtomic(path, recipient.CertificateDER, 0o600); err != nil {
		return domain.ProtectionRecipient{}, err
	}
	return recipient, nil
}

func (b *LocalPublicRecipientBook) Remove(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil || b.ConfigDir == "" || !strings.HasPrefix(id, "x509-") || len(id) != 69 {
		return fmt.Errorf("identificador de destinatario importado no válido")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := hex.DecodeString(id[5:]); err != nil {
		return fmt.Errorf("identificador de destinatario importado no válido")
	}
	path := filepath.Join(b.dir(), id+".der")
	raw, err := securefile.ReadFileLimit(path, maxPublicCertificateBytes)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if "x509-"+hex.EncodeToString(sum[:]) != id {
		return fmt.Errorf("certificado importado no válido")
	}
	return os.Remove(path)
}

func (b *LocalPublicRecipientBook) List(ctx context.Context) ([]domain.ProtectionRecipient, error) {
	return b.Resolve(ctx, nil)
}

func (b *LocalPublicRecipientBook) Resolve(ctx context.Context, ids []string) ([]domain.ProtectionRecipient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil || b.ConfigDir == "" {
		return nil, fmt.Errorf("libro de destinatarios públicos no configurado")
	}
	entries, err := os.ReadDir(b.dir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > maxPublicRecipients {
		return nil, fmt.Errorf("el libro supera el límite de 128 destinatarios")
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	out := make([]domain.ProtectionRecipient, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".der") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".der")
		if ids != nil && !wanted[id] {
			continue
		}
		raw, err := securefile.ReadFileLimit(filepath.Join(b.dir(), entry.Name()), maxPublicCertificateBytes)
		if err != nil {
			return nil, err
		}
		recipient, err := publicRecipient(raw, "importado")
		if err != nil && strings.Contains(err.Error(), "caducado") {
			continue
		}
		if err != nil || recipient.ID != id {
			return nil, fmt.Errorf("certificado importado alterado: %s", entry.Name())
		}
		out = append(out, recipient)
	}
	return out, nil
}
