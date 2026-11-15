// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- solo para verificar firmas PDF antiguas (adbe.pkcs7.sha1, adbe.x509.rsa_sha1).
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/adapters/outbound/common/certutil"
	"grxfirma/internal/domain"
)

// Subfiltros PDF anteriores a PAdES que AutoFirma verificaba (ISO 32000-1,
// 12.8.3). Solo se leen para verificar: GrxFirma nunca firma con ellos y los
// dos llevan SHA-1, así que como mucho salen con la integridad comprobada y el
// aviso de algoritmo antiguo (ver algoritmo_antiguo.go).
const (
	// subFiltroPKCS7SHA1: el PKCS#7 firma, como eContent, el resumen SHA-1 del
	// rango de bytes.
	subFiltroPKCS7SHA1 = "adbe.pkcs7.sha1"
	// subFiltroX509RSASHA1: /Contents es la firma PKCS#1 (un OCTET STRING) del
	// rango de bytes y /Cert lleva el certificado del firmante y su cadena.
	subFiltroX509RSASHA1 = "adbe.x509.rsa_sha1"

	maxCertificadosPDFHeredado = 16
)

// esSubFiltroPDFHeredado dice si el subfiltro es uno de los antiguos que solo
// el verificador admite.
func esSubFiltroPDFHeredado(subFilter string) bool {
	return subFilter == subFiltroPKCS7SHA1 || subFilter == subFiltroX509RSASHA1
}

// verificarPKCS7SHA1PDF comprueba un adbe.pkcs7.sha1: el CMS encapsula el
// resumen SHA-1 del rango firmado. Se verifica el CMS contra ese resumen como
// contenido aportado (tiene que ser idéntico al eContent y cuadrar con
// messageDigest) y se marca SHA-1 por el resumen del documento.
func (v *CAdESVerifier) verificarPKCS7SHA1PDF(ctx context.Context, cmsDER, signedBytes []byte, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if blocked, yes := bloqueoSHA1Previo(ctx, string(domain.FormatPAdES)); yes {
		return blocked, nil, nil
	}
	resumen := sha1.Sum(signedBytes) // #nosec G401 -- verificación de una firma PDF antigua.
	result, signers, err := v.verifyCMS(ctx, cmsDER, resumen[:], cmsContenidoOriginalAportado, anchors)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}
	result = marcarAlgoritmoAntiguo(domain.RegistroConfianzaForzadaDe(ctx), result, []string{detalleAlgoritmoSHA1}, ctx)
	result.Material.HuellasContenidoFirmado = []string{huellaContenido(signedBytes)}
	return result, signers, nil
}

// verificarPKCS1PDF comprueba un adbe.x509.rsa_sha1. El firmante es el
// certificado de /Cert cuya clave RSA verifica la firma sobre el rango; /Cert
// está dentro de lo firmado, así que también queda protegido.
func (v *CAdESVerifier) verificarPKCS1PDF(ctx context.Context, contents []byte, certsDER [][]byte, signedBytes []byte, anchors domain.CertificateChain) (domain.VerificationResult, []domain.CertificateRef, error) {
	if blocked, yes := bloqueoSHA1Previo(ctx, string(domain.FormatPAdES)); yes {
		return blocked, nil, nil
	}
	var firma []byte
	resto, err := asn1.Unmarshal(contents, &firma)
	if err != nil || len(resto) != 0 || len(firma) == 0 {
		return domain.VerificationResult{}, nil, errors.New("firma PDF adbe.x509.rsa_sha1: /Contents no es un OCTET STRING con la firma")
	}
	if len(certsDER) == 0 {
		return domain.VerificationResult{}, nil, errors.New("firma PDF adbe.x509.rsa_sha1 sin certificado en /Cert")
	}
	certs := make([]*x509.Certificate, 0, len(certsDER))
	for i, der := range certsDER {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return domain.VerificationResult{}, nil, fmt.Errorf("parseando el certificado %d de /Cert: %w", i, err)
		}
		certs = append(certs, cert)
	}
	firmante, hash, err := firmantePKCS1(certs, firma, signedBytes)
	if err != nil {
		return domain.VerificationResult{}, nil, err
	}

	result := domain.NewVerificationSuccess(string(domain.FormatPAdES), "firma PAdES válida", nil)
	detalles := []string{
		fmt.Sprintf("firmante=%s", certutil.NombreLegible(firmante.Subject)),
		fmt.Sprintf("algoritmo=pkcs1-%s", strings.ToLower(strings.ReplaceAll(hash.String(), "-", ""))),
	}
	result.Details = append(result.Details, detalles...)
	result.Integrity.Details = append(result.Integrity.Details, detalles...)
	registro := domain.RegistroConfianzaForzadaDe(ctx)
	result = applySignerVerificationMetadata(registro, result, []*x509.Certificate{firmante}, certs, anchors)
	result.Material.HuellasContenidoFirmado = []string{huellaContenido(signedBytes)}

	candidatos := append(append([]*x509.Certificate(nil), certs...), certificadosDeAnclas(anchors)...)
	chain := chainForRevocation(ctx, v.revChecker, firmante, candidatos)
	if len(chain) < 2 {
		result = markRevocationNotChecked(result, firmante)
	} else {
		result = marcarCadenaRevocacionIncompleta(result, chain)
		if revocation, err := v.verifyRevocationWithEmbedded(ctx, chain, nil); err == nil {
			result = mergeRevocationResult(result, revocation)
		}
	}
	result = exigirRevocacionConcluyente(registro, result)
	// adbe.x509.rsa_sha1 lleva SHA-1 en su propia definición aunque el
	// DigestInfo use otro resumen.
	result = marcarAlgoritmoAntiguo(registro, result, []string{detalleAlgoritmoSHA1}, ctx)
	return result.Normalize(), []domain.CertificateRef{certificateToRef(firmante)}, nil
}

// firmantePKCS1 busca el certificado RSA que verifica la firma PKCS#1 v1.5
// del rango. El DigestInfo puede llevar SHA-1 (lo normal) o, en PDF 1.7, un
// SHA-2.
func firmantePKCS1(certs []*x509.Certificate, firma, signedBytes []byte) (*x509.Certificate, crypto.Hash, error) {
	for _, cert := range certs {
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			continue
		}
		for _, hash := range []crypto.Hash{crypto.SHA1, crypto.SHA256, crypto.SHA384, crypto.SHA512} {
			if rsa.VerifyPKCS1v15(pub, hash, cmsDigest(hash, signedBytes), firma) == nil {
				return cert, hash, nil
			}
		}
	}
	return nil, 0, errors.New("firma PDF adbe.x509.rsa_sha1 inválida: ningún certificado de /Cert la verifica")
}

// extraerCertificadosPDF lee /Cert del diccionario de firma: una cadena o una
// lista de cadenas (literales o hexadecimales) con certificados DER.
func extraerCertificadosPDF(dict string) ([][]byte, error) {
	idx := -1
	for desde := 0; ; {
		i := findPDFName(dict, "/Cert", desde)
		if i < 0 {
			break
		}
		fin := i + len("/Cert")
		if fin >= len(dict) || !isPDFNameChar(rune(dict[fin])) {
			if idx >= 0 {
				return nil, errors.New("el diccionario de firma tiene más de un /Cert")
			}
			idx = fin
		}
		desde = fin
	}
	if idx < 0 {
		return nil, nil
	}
	pos := skipPDFSpaces(dict, idx)
	if pos >= len(dict) {
		return nil, errors.New("/Cert sin valor")
	}
	if dict[pos] != '[' {
		cert, _, err := leerCadenaPDF(dict, pos)
		if err != nil {
			return nil, err
		}
		return [][]byte{cert}, nil
	}
	var out [][]byte
	pos++
	for {
		pos = skipPDFSpaces(dict, pos)
		if pos >= len(dict) {
			return nil, errors.New("/Cert sin cerrar")
		}
		if dict[pos] == ']' {
			return out, nil
		}
		if len(out) >= maxCertificadosPDFHeredado {
			return nil, fmt.Errorf("/Cert con más de %d certificados", maxCertificadosPDFHeredado)
		}
		cert, siguiente, err := leerCadenaPDF(dict, pos)
		if err != nil {
			return nil, err
		}
		out = append(out, cert)
		pos = siguiente
	}
}

// leerCadenaPDF lee una cadena PDF (ISO 32000-1, 7.3.4) desde pos y devuelve
// sus bytes y la posición siguiente.
func leerCadenaPDF(text string, pos int) ([]byte, int, error) {
	switch {
	case pos < len(text) && text[pos] == '<':
		fin := strings.IndexByte(text[pos+1:], '>')
		if fin < 0 {
			return nil, 0, errors.New("cadena hexadecimal PDF sin cerrar")
		}
		digitos, err := compactPDFHex(text[pos+1 : pos+1+fin])
		if err != nil {
			return nil, 0, err
		}
		if len(digitos)%2 == 1 {
			digitos += "0"
		}
		out, err := hex.DecodeString(digitos)
		if err != nil {
			return nil, 0, fmt.Errorf("cadena hexadecimal PDF inválida: %w", err)
		}
		return out, pos + fin + 2, nil
	case pos < len(text) && text[pos] == '(':
		return leerCadenaLiteralPDF(text, pos)
	default:
		return nil, 0, errors.New("se esperaba una cadena PDF")
	}
}

func leerCadenaLiteralPDF(text string, pos int) ([]byte, int, error) {
	var out []byte
	nivel := 0
	for i := pos; i < len(text); i++ {
		c := text[i]
		switch c {
		case '(':
			if nivel > 0 {
				out = append(out, c)
			}
			nivel++
		case ')':
			nivel--
			if nivel == 0 {
				return out, i + 1, nil
			}
			out = append(out, c)
		case '\r':
			// Un fin de línea sin escapar vale como \n (7.3.4.2).
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
			out = append(out, '\n')
		case '\\':
			i++
			if i >= len(text) {
				return nil, 0, errors.New("cadena PDF sin cerrar")
			}
			switch e := text[i]; e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '(', ')', '\\':
				out = append(out, e)
			case '\r':
				// Continuación de línea: no aporta nada.
				if i+1 < len(text) && text[i+1] == '\n' {
					i++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					valor := int(e - '0')
					for n := 1; n < 3 && i+1 < len(text) && text[i+1] >= '0' && text[i+1] <= '7'; n++ {
						i++
						valor = valor*8 + int(text[i]-'0')
					}
					out = append(out, byte(valor))
				} else {
					// Escape desconocido: se ignora la barra.
					out = append(out, e)
				}
			}
		default:
			out = append(out, c)
		}
	}
	return nil, 0, errors.New("cadena PDF sin cerrar")
}
