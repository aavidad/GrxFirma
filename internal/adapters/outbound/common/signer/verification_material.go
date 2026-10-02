// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"

	"grxfirma/internal/domain"
)

type offlineVerificationContextKey struct{}

func offlineVerification(ctx context.Context) bool {
	offline, _ := ctx.Value(offlineVerificationContextKey{}).(bool)
	return offline
}

// NewRevocationCheckerOffline crea un comprobador que nunca accede a la red.
// Toda consulta devuelve estado desconocido, de modo que la verificación
// queda indeterminada en lugar de depender de servicios externos.
func NewRevocationCheckerOffline() *RevocationChecker {
	return &RevocationChecker{}
}

// NewMultiVerifierOffline compone los verificadores de formato sin ninguna
// conexión de red. La revocación se evalúa después con fuentes locales.
func NewMultiVerifierOffline() *MultiVerifier {
	cades := NewCAdESVerifierWithChecker(NewRevocationCheckerOffline())
	verifier := NewMultiVerifierWithEngines(cades, nil, nil, NewPAdESVerifierWithCAdES(cades), nil, nil, nil, nil)
	verifier.offline = true
	return verifier
}

// materialFirmantes crea el material base (certificado y certificados
// embebidos) de cada firmante con copias defensivas.
func materialFirmantes(signerCerts, embeddedCerts []*x509.Certificate) []domain.MaterialFirma {
	embebidos := make([][]byte, 0, len(embeddedCerts))
	for _, cert := range embeddedCerts {
		if cert != nil {
			embebidos = append(embebidos, bytes.Clone(cert.Raw))
		}
	}
	out := make([]domain.MaterialFirma, 0, len(signerCerts))
	for _, cert := range signerCerts {
		if cert == nil {
			continue
		}
		copia := make([][]byte, len(embebidos))
		for i := range embebidos {
			copia[i] = bytes.Clone(embebidos[i])
		}
		out = append(out, domain.MaterialFirma{
			CertificadoDER:           bytes.Clone(cert.Raw),
			CertificadosEmbebidosDER: copia,
		})
	}
	return out
}

// huellaContenido devuelve la SHA-256 hexadecimal de un contenido verificado.
func huellaContenido(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// marcarSelloXMLNoEvaluable señala que una firma XML contiene un sello de
// tiempo XAdES que este verificador no evalúa todavía: el dictamen lo
// declarará no comprobado en lugar de ausente.
func marcarSelloXMLNoEvaluable(result domain.VerificationResult, xmlData []byte) domain.VerificationResult {
	if !bytes.Contains(xmlData, []byte("SignatureTimeStamp")) {
		return result
	}
	for i := range result.Material.Firmas {
		result.Material.Firmas[i].SelloNoEvaluable = true
	}
	return result
}

// enriquecerMaterialCMS completa el material de cada firmante CMS con el
// valor de firma, los sellos de tiempo y las evidencias de revocación
// embebidas en sus atributos no firmados.
func enriquecerMaterialCMS(material []domain.MaterialFirma, signerCerts []*x509.Certificate, signerInfos []signerInfoRaw) {
	for i, signerInfo := range signerInfos {
		if i >= len(signerCerts) || signerCerts[i] == nil {
			continue
		}
		huella := certificateFingerprint(signerCerts[i])
		for j := range material {
			if certificateFingerprintDER(material[j].CertificadoDER) != huella || material[j].ValorFirma != nil {
				continue
			}
			material[j].ValorFirma = bytes.Clone(signerInfo.Signature)
			material[j].SellosTiempoDER = extraerSellosTiempoCMS(signerInfo)
			if evidence, err := extractEmbeddedRevocationEvidence(signerInfo); err == nil {
				material[j].CRLsEmbebidasDER = evidence.CRLs
				material[j].OCSPEmbebidasDER = evidence.OCSPResponses
			}
			break
		}
	}
}

func extraerSellosTiempoCMS(signerInfo signerInfoRaw) [][]byte {
	attrs, err := parseUnsignedAttributes(signerInfo.UnsignedAttributes)
	if err != nil {
		return nil
	}
	var out [][]byte
	for _, attr := range attrs {
		if !attr.Type.Equal(oidSignatureTimeStamp) {
			continue
		}
		for _, value := range attr.Values {
			out = append(out, bytes.Clone(value.FullBytes))
		}
	}
	return out
}

func certificateFingerprintDER(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}
