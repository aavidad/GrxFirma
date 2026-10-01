// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs12importer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"grxfirma/internal/adapters/outbound/common/certutil"
	"grxfirma/internal/adapters/outbound/common/components"
	"grxfirma/internal/adapters/outbound/common/secmem"
	"grxfirma/internal/adapters/outbound/common/securefile"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// Importador implementa ports.CertificateImporter y expone helpers adicionales
// para los entry points que necesitan la clave firmante ademas de la referencia.
type Importador struct{}

const (
	opensslImportTimeout   = 15 * time.Second
	maxCredentialFileBytes = 16 * 1024 * 1024
)

// IdentidadImportada representa una credencial completa cargada localmente.
type IdentidadImportada struct {
	Reference   domain.CertificateRef
	Signer      crypto.Signer
	Certificate *x509.Certificate
	// Chain contiene emisores enlazados criptográficamente, sin repetir la hoja.
	// No convierte sus certificados en anclas confiables del sistema.
	Chain []*x509.Certificate
}

// New construye un importador reutilizable.
func New() *Importador {
	return &Importador{}
}

// Import implementa ports.CertificateImporter devolviendo solo la referencia.
func (i *Importador) Import(ctx context.Context, data []byte, password string) (domain.CertificateRef, error) {
	identidad, err := i.ImportIdentity(ctx, data, password)
	if err != nil {
		return domain.CertificateRef{}, err
	}
	return identidad.Reference, nil
}

// ImportIdentity carga una identidad desde bytes PKCS#12 o desde un bundle PEM.
func (i *Importador) ImportIdentity(ctx context.Context, data []byte, password string) (IdentidadImportada, error) {
	if err := ctx.Err(); err != nil {
		return IdentidadImportada{}, err
	}
	if len(data) == 0 {
		return IdentidadImportada{}, errors.New("los datos de entrada no pueden estar vacios")
	}
	if len(data) > maxCredentialFileBytes {
		return IdentidadImportada{}, fmt.Errorf("la credencial supera el tamaño máximo de %d MiB", maxCredentialFileBytes/(1024*1024))
	}

	if parecePEM(data) {
		return importarDesdePEMBundle(data)
	}
	return importarDesdePKCS12(ctx, data, password)
}

// ImportP12File carga una identidad desde un fichero .p12/.pfx.
func (i *Importador) ImportP12File(ctx context.Context, path string, password string) (IdentidadImportada, error) {
	if err := ctx.Err(); err != nil {
		return IdentidadImportada{}, err
	}
	data, err := securefile.ReadFileLimit(path, maxCredentialFileBytes)
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("no se pudo leer %s: %w", path, err)
	}
	defer secmem.Zeroize(data)
	return i.ImportIdentity(ctx, data, password)
}

// ImportPEMFiles carga una identidad desde dos ficheros PEM separados:
// uno con el certificado y otro con la clave privada.
func (i *Importador) ImportPEMFiles(ctx context.Context, certPath, keyPath string) (IdentidadImportada, error) {
	if err := ctx.Err(); err != nil {
		return IdentidadImportada{}, err
	}
	certData, err := securefile.ReadFileLimit(certPath, maxCredentialFileBytes)
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("no se pudo leer %s: %w", certPath, err)
	}
	keyData, err := securefile.ReadFileLimit(keyPath, maxCredentialFileBytes)
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("no se pudo leer %s: %w", keyPath, err)
	}
	// keyData es la clave privada en claro (T057): zeroizar tras el parseo.
	defer secmem.Zeroize(keyData)
	bundle := append(append([]byte(nil), certData...), keyData...)
	defer secmem.Zeroize(bundle)
	return importarDesdePEMBundle(bundle)
}

func importarDesdePKCS12(ctx context.Context, data []byte, password string) (IdentidadImportada, error) {
	priv, cert, chain, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		fallback, fallbackErr := importarDesdePKCS12ConOpenSSL(ctx, data, password)
		if fallbackErr == nil {
			return fallback, nil
		}
		return IdentidadImportada{}, fmt.Errorf("no se pudo abrir el P12 (contrasena incorrecta o formato no soportado): %w; fallback openssl: %v", err, fallbackErr)
	}

	signer, ok := priv.(crypto.Signer)
	if !ok {
		return IdentidadImportada{}, fmt.Errorf("la clave privada del P12 no implementa crypto.Signer: %T", priv)
	}

	return construirIdentidadConCadena(signer, cert, chain)
}

func importarDesdePKCS12ConOpenSSL(ctx context.Context, data []byte, password string) (IdentidadImportada, error) {
	opensslPath, err := exec.LookPath("openssl")
	if err != nil {
		return IdentidadImportada{}, components.ErrorFalta("openssl")
	}
	opensslCtx, cancel := context.WithTimeout(ctx, opensslImportTimeout)
	defer cancel()

	p12File, err := os.CreateTemp("", "grxfirma-import-*.p12")
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("crear temporal p12: %w", err)
	}
	p12Path := p12File.Name()
	defer os.Remove(p12Path)
	if _, err := p12File.Write(data); err != nil {
		_ = p12File.Close()
		return IdentidadImportada{}, fmt.Errorf("escribir temporal p12: %w", err)
	}
	_ = p12File.Close()

	pwFile, err := os.CreateTemp("", "grxfirma-import-pw-*")
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("crear temporal password: %w", err)
	}
	pwPath := pwFile.Name()
	defer os.Remove(pwPath)
	if err := os.Chmod(pwPath, 0o600); err != nil {
		_ = pwFile.Close()
		return IdentidadImportada{}, fmt.Errorf("proteger temporal password: %w", err)
	}
	if _, err := pwFile.WriteString(password); err != nil {
		_ = pwFile.Close()
		return IdentidadImportada{}, fmt.Errorf("escribir temporal password: %w", err)
	}
	_ = pwFile.Close()

	keyPEM, err := ejecutarOpenSSLPKCS12(
		opensslCtx,
		opensslPath,
		p12Path,
		pwPath,
		"-nocerts",
		"-nodes",
	)
	// OpenSSL can emit partial private-key material before returning an error.
	defer secmem.Zeroize(keyPEM)
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("openssl clave: %w", err)
	}
	// keyPEM contiene la clave privada en claro; zeroizar en cuanto se haya
	// parseado (T057). El crypto.Signer resultante conserva su propia copia
	// gestionada por el runtime, pero este buffer intermedio no debe sobrevivir.
	certPEM, err := ejecutarOpenSSLPKCS12(
		opensslCtx,
		opensslPath,
		p12Path,
		pwPath,
		"-nokeys",
	)
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("openssl certificado: %w", err)
	}

	bundle := make([]byte, 0, len(certPEM)+len(keyPEM))
	bundle = append(bundle, certPEM...)
	bundle = append(bundle, keyPEM...)
	defer secmem.Zeroize(bundle)
	return importarDesdePEMBundle(bundle)
}

func ejecutarOpenSSLPKCS12(
	ctx context.Context,
	opensslPath string,
	p12Path string,
	passwordPath string,
	outputArgs ...string,
) ([]byte, error) {
	baseArgs := []string{
		"pkcs12",
		"-in", p12Path,
		"-passin", "file:" + passwordPath,
	}
	baseArgs = append(baseArgs, outputArgs...)

	// #nosec G204 -- opensslPath is the resolved executable; every dynamic value
	// is a separate argument and both input files are private temporaries.
	output, err := exec.CommandContext(ctx, opensslPath, baseArgs...).CombinedOutput()
	if err == nil {
		return output, nil
	}
	// A failed extraction may still emit private-key bytes. Destroy them before
	// trying the compatibility provider.
	secmem.Zeroize(output)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	// OpenSSL 3 disables RC2 and other old PKCS#12 transport algorithms by
	// default. Official Cliente @firma QA containers still use that encoding.
	// `-legacy` only decodes the local container: document signatures continue
	// to use the application's current cryptographic policy.
	legacyArgs := []string{"pkcs12", "-legacy"}
	legacyArgs = append(legacyArgs, baseArgs[1:]...)
	// #nosec G204 -- same resolved executable and private files as above.
	legacyOutput, legacyErr := exec.CommandContext(
		ctx,
		opensslPath,
		legacyArgs...,
	).CombinedOutput()
	if legacyErr != nil {
		secmem.Zeroize(legacyOutput)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf(
			"modo predeterminado: %w; modo de compatibilidad: %v",
			err,
			legacyErr,
		)
	}
	return legacyOutput, nil
}

func importarDesdePEMBundle(data []byte) (IdentidadImportada, error) {
	certs, err := parseCertificatesPEM(data)
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("certificado: %w", err)
	}
	signer, err := parsePrivateKeyPEM(data)
	if err != nil {
		return IdentidadImportada{}, fmt.Errorf("clave privada: %w", err)
	}

	for _, cert := range certs {
		if certificadoCorrespondeAClave(cert, signer) {
			return construirIdentidadConCadena(signer, cert, certs)
		}
	}
	return IdentidadImportada{}, errors.New("ningún certificado corresponde a la clave privada del bundle")
}

func parsePrivateKeyPEM(data []byte) (crypto.Signer, error) {
	rest := data
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		switch block.Type {
		case "RSA PRIVATE KEY":
			priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("clave RSA PKCS#1 invalida: %w", err)
			}
			return priv, nil
		case "EC PRIVATE KEY":
			priv, err := x509.ParseECPrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("clave EC invalida: %w", err)
			}
			return priv, nil
		case "PRIVATE KEY":
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("clave PKCS#8 invalida: %w", err)
			}
			switch k := key.(type) {
			case *rsa.PrivateKey:
				return k, nil
			case *ecdsa.PrivateKey:
				return k, nil
			default:
				return nil, fmt.Errorf("tipo de clave PKCS#8 no soportado: %T", key)
			}
		}
	}
	return nil, errors.New("no se encontro ningun bloque de clave privada")
}

func construirReference(cert *x509.Certificate) domain.CertificateRef {
	huella := sha256.Sum256(cert.Raw)
	subject := cert.Subject.CommonName
	if subject == "" {
		subject = cert.Subject.String()
	}
	issuer := cert.Issuer.CommonName
	if issuer == "" {
		issuer = cert.Issuer.String()
	}
	tipo, org, nif := certutil.ClasificarCertificado(cert)
	// El ID usa la huella SHA-256 completa (no truncada) para evitar colisiones de
	// 64 bits en catalogos grandes (CWE-840). El consumidor decide si abrevia para mostrar.
	fingerprint := hex.EncodeToString(huella[:])
	return domain.CertificateRef{
		ID:            fingerprint,
		Subject:       subject,
		Issuer:        issuer,
		NotAfter:      cert.NotAfter,
		Fingerprint:   fingerprint,
		DER:           cert.Raw,
		HasSigningKey: true,
		Tipo:          tipo,
		Organizacion:  org,
		NIF:           nif,
	}
}

func parecePEM(data []byte) bool {
	return len(data) >= len("-----BEGIN ") && string(data[:11]) == "-----BEGIN "
}

var _ ports.CertificateImporter = (*Importador)(nil)
