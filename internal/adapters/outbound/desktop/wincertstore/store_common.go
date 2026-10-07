// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package wincertstore

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"syscall"

	"grxfirma/internal/adapters/outbound/common/certutil"
	"grxfirma/internal/domain"
)

// ErrNoDisponibleEnEstaPlataforma permite a agregadores ignorar este proveedor
// de forma explícita fuera de Windows.
var ErrNoDisponibleEnEstaPlataforma = errors.New("wincertstore: solo disponible en windows")

// Códigos de Windows (HRESULT o Win32) que indican que la tarjeta o el
// lector no están o que el usuario canceló el diálogo de la clave. Repetir la
// apertura solo volvería a mostrar el mismo diálogo.
const (
	scardECancelled          = 0x80100002
	scardENoSmartcard        = 0x8010000C
	scardENoReadersAvailable = 0x8010002E
	scardEReaderUnavailable  = 0x80100017
	scardWRemovedCard        = 0x80100069
	scardWCancelledByUser    = 0x8010006E
	hresultErrorCancelled    = 0x800704C7
	win32ErrorCancelled      = 1223
)

// accesoClaveNoReintentable indica si el fallo al abrir una clave privada se
// debe a la ausencia de tarjeta o lector, o a una cancelación del usuario.
func accesoClaveNoReintentable(err error) bool {
	var codigo syscall.Errno
	if !errors.As(err, &codigo) {
		return false
	}
	switch uint32(codigo) {
	case scardECancelled, scardENoSmartcard, scardENoReadersAvailable,
		scardEReaderUnavailable, scardWRemovedCard, scardWCancelledByUser,
		hresultErrorCancelled, win32ErrorCancelled:
		return true
	default:
		return false
	}
}

type parametrosHash struct {
	nombreCNG string
	algIDCAPI uint32
}

const (
	calgSHA1   = 0x00008004
	calgSHA256 = 0x0000800c
	calgSHA384 = 0x0000800d
	calgSHA512 = 0x0000800e
)

func construirRefDesdeDER(der []byte) (domain.CertificateRef, *x509.Certificate, error) {
	if len(der) == 0 {
		return domain.CertificateRef{}, nil, errors.New("wincertstore: certificado DER vacio")
	}
	cert, err := x509.ParseCertificate(append([]byte(nil), der...))
	if err != nil {
		return domain.CertificateRef{}, nil, fmt.Errorf("wincertstore: ParseCertificate: %w", err)
	}

	fingerprint := huellaCertificado(cert)
	subject := strings.TrimSpace(cert.Subject.CommonName)
	if subject == "" {
		subject = cert.Subject.String()
	}
	issuer := strings.TrimSpace(cert.Issuer.CommonName)
	if issuer == "" {
		issuer = cert.Issuer.String()
	}
	tipo, organizacion, nif := certutil.ClasificarCertificado(cert)

	return domain.CertificateRef{
		ID:           fingerprint,
		Subject:      subject,
		Issuer:       issuer,
		NotAfter:     cert.NotAfter,
		Fingerprint:  fingerprint,
		DER:          cert.Raw,
		Tipo:         tipo,
		Organizacion: organizacion,
		NIF:          nif,
	}, cert, nil
}

func huellaCertificado(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	suma := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(suma[:])
}

func huellaBuscada(ref domain.CertificateRef) (string, error) {
	raw := strings.TrimSpace(ref.Fingerprint)
	if raw == "" {
		raw = strings.TrimSpace(ref.ID)
	}
	huella, err := normalizarHuella(raw)
	if err != nil {
		return "", fmt.Errorf("wincertstore: referencia de certificado invalida: %w", err)
	}
	return huella, nil
}

func normalizarHuella(raw string) (string, error) {
	normalizada := strings.ToLower(strings.NewReplacer(
		":", "",
		"-", "",
		" ", "",
		"\t", "",
		"\r", "",
		"\n", "",
	).Replace(strings.TrimSpace(raw)))
	if len(normalizada) != sha256.Size*2 {
		return "", fmt.Errorf("la huella SHA-256 debe tener %d caracteres hexadecimales", sha256.Size*2)
	}
	if _, err := hex.DecodeString(normalizada); err != nil {
		return "", fmt.Errorf("la huella SHA-256 no es hexadecimal: %w", err)
	}
	return normalizada, nil
}

func validarClavePublica(publica crypto.PublicKey) error {
	switch publica.(type) {
	case *rsa.PublicKey, *ecdsa.PublicKey:
		return nil
	default:
		return fmt.Errorf("wincertstore: tipo de clave publica no soportado: %T", publica)
	}
}

func certificadoAptoParaCatalogo(cert *x509.Certificate) bool {
	if cert == nil || cert.IsCA || cert.KeyUsage&x509.KeyUsageCertSign != 0 {
		return false
	}

	// Si KeyUsage no está presente, crypto/x509 deja el campo a cero. En ese
	// caso la clave no tiene una restricción de uso explícita y se conserva la
	// compatibilidad con certificados de firma antiguos.
	if cert.KeyUsage == 0 {
		return true
	}
	return cert.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) != 0
}

func resolverParametrosHash(hash crypto.Hash) (parametrosHash, error) {
	switch hash {
	case crypto.SHA1:
		return parametrosHash{nombreCNG: "SHA1", algIDCAPI: calgSHA1}, nil
	case crypto.SHA256:
		return parametrosHash{nombreCNG: "SHA256", algIDCAPI: calgSHA256}, nil
	case crypto.SHA384:
		return parametrosHash{nombreCNG: "SHA384", algIDCAPI: calgSHA384}, nil
	case crypto.SHA512:
		return parametrosHash{nombreCNG: "SHA512", algIDCAPI: calgSHA512}, nil
	default:
		return parametrosHash{}, fmt.Errorf("wincertstore: algoritmo hash no soportado: %v", hash)
	}
}

func longitudSalPSS(publica *rsa.PublicKey, hash crypto.Hash, opciones *rsa.PSSOptions) (uint32, error) {
	if publica == nil || publica.N == nil || opciones == nil {
		return 0, errors.New("wincertstore: parametros RSA-PSS incompletos")
	}
	if _, err := resolverParametrosHash(hash); err != nil {
		return 0, err
	}

	emLen := (publica.N.BitLen() - 1 + 7) / 8
	maxima := emLen - hash.Size() - 2
	if maxima < 0 {
		return 0, errors.New("wincertstore: clave RSA demasiado corta para el hash y padding PSS solicitados")
	}

	longitud := opciones.SaltLength
	switch longitud {
	case rsa.PSSSaltLengthAuto:
		longitud = maxima
	case rsa.PSSSaltLengthEqualsHash:
		longitud = hash.Size()
	}
	if longitud < 0 || longitud > maxima {
		return 0, fmt.Errorf("wincertstore: longitud de sal RSA-PSS invalida: %d (maximo %d)", longitud, maxima)
	}
	longitud64 := int64(longitud)
	if longitud64 > math.MaxUint32 {
		return 0, fmt.Errorf("wincertstore: longitud de sal RSA-PSS fuera de rango CNG: %d", longitud)
	}
	return uint32(longitud64), nil
}

func firmaECDSADesdeCNG(raw []byte, publica *ecdsa.PublicKey) ([]byte, error) {
	if publica == nil || publica.Curve == nil {
		return nil, errors.New("wincertstore: clave publica ECDSA invalida")
	}
	tamano := (publica.Curve.Params().BitSize + 7) / 8
	if len(raw) != 2*tamano {
		return nil, fmt.Errorf("wincertstore: firma ECDSA CNG de longitud invalida: %d (esperada %d)", len(raw), 2*tamano)
	}
	r := new(big.Int).SetBytes(raw[:tamano])
	s := new(big.Int).SetBytes(raw[tamano:])
	if r.Sign() <= 0 || s.Sign() <= 0 {
		return nil, errors.New("wincertstore: firma ECDSA CNG contiene valores no positivos")
	}
	firma, err := asn1.Marshal(struct {
		R *big.Int
		S *big.Int
	}{R: r, S: s})
	if err != nil {
		return nil, fmt.Errorf("wincertstore: codificando firma ECDSA: %w", err)
	}
	return firma, nil
}

func invertirCopia(data []byte) []byte {
	resultado := make([]byte, len(data))
	for i := range data {
		resultado[len(data)-1-i] = data[i]
	}
	return resultado
}

func filtrarCadena(hoja *x509.Certificate, certificados []*x509.Certificate) []*x509.Certificate {
	resultado := make([]*x509.Certificate, 0, len(certificados))
	vistos := make(map[string]struct{}, len(certificados)+1)
	if hoja != nil {
		vistos[string(hoja.Raw)] = struct{}{}
	}
	for _, cert := range certificados {
		if cert == nil || len(cert.Raw) == 0 {
			continue
		}
		clave := string(cert.Raw)
		if _, existe := vistos[clave]; existe {
			continue
		}
		vistos[clave] = struct{}{}
		// El ancla ya debe estar en el almacén de confianza del verificador y no
		// se incluye en los contenedores de firma.
		if bytes.Equal(cert.RawSubject, cert.RawIssuer) && cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) == nil {
			continue
		}
		resultado = append(resultado, cert)
	}
	return resultado
}
