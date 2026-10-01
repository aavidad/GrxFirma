// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ports

import (
	"encoding/base64"
	"net/url"
	"strings"
	"unicode/utf8"
)

const facturaePolicyHashSHA1Bytes = 20

var (
	clavesPoliticaFacturaE = []string{
		"facturaePolicyVersion",
		"policyIdentifier",
		"policyIdentifierHash",
		"policyQualifier",
	}
	clavesMetadatosFacturaE = []string{
		"signerClaimedRole",
		"signatureProductionCity",
		"signatureProductionProvince",
		"signatureProductionPostalCode",
		"signatureProductionCountry",
	}
	clavesPoliticaXAdES = []string{
		"policyIdentifier",
		"policyIdentifierHash",
		"policyIdentifierHashAlgorithm",
		"policyQualifier",
		"xadesPolicyIdentifier",
		"xadesPolicyIdentifierHash",
		"xadesPolicyIdentifierHashAlgorithm",
		"xadesPolicyQualifier",
	}
)

// NecesitaOpcionesFirmaPredeterminadas indica si la operación puede obtener
// algún valor útil del documento persistido. Evita que una preferencia
// opcional convierta en dependencia de disponibilidad a otros formatos o a
// una petición que ya declaró su subfiltro.
func NecesitaOpcionesFirmaPredeterminadas(formato string, explicitas map[string]string) bool {
	switch strings.ToLower(strings.TrimSpace(formato)) {
	case "pades":
		return !contieneClaveFirma(explicitas, "subfilter") &&
			!contieneClaveFirma(explicitas, "pdfsubfilter")
	case "facturae":
		// La política es una unidad: declarar cualquiera de sus campos impide
		// mezclar un identificador o hash explícito con otra política guardada.
		if !contieneAlgunaClaveFirma(explicitas, clavesPoliticaFacturaE...) {
			return true
		}
		for _, clave := range clavesMetadatosFacturaE {
			if !contieneClaveFirma(explicitas, clave) {
				return true
			}
		}
		return false
	case "xades":
		// Al igual que FacturaE, una política XAdES es indivisible. Si la
		// petición aporta cualquier campo no debe completarse con otro
		// documento persistido.
		return !contieneAlgunaClaveFirma(explicitas, clavesPoliticaXAdES...)
	default:
		return false
	}
}

// AplicarOpcionesFirmaPredeterminadas proyecta sobre una operación de firma
// únicamente las preferencias tipadas que ya tienen un consumidor real en el
// motor. Las opciones explícitas de la petición siempre prevalecen, también
// cuando usan una capitalización distinta.
//
// La política FacturaE se trata de forma atómica para no combinar un
// identificador explícito con el hash o la versión de otra política.
func AplicarOpcionesFirmaPredeterminadas(doc DocumentoConfiguracionUsuario, formato string, explicitas map[string]string) map[string]string {
	resultado := copiarOpcionesFirma(explicitas)
	switch strings.ToLower(strings.TrimSpace(formato)) {
	case "pades":
		return aplicarSubfiltroPAdESPredeterminado(doc.PAdES, resultado)
	case "facturae":
		return aplicarFacturaEPredeterminada(doc.FacturaE, resultado)
	case "xades":
		return aplicarPoliticaXAdESPredeterminada(doc.XAdES, resultado)
	default:
		return resultado
	}
}

func aplicarSubfiltroPAdESPredeterminado(config ConfiguracionUsuarioPAdES, resultado map[string]string) map[string]string {
	if contieneClaveFirma(resultado, "subfilter") ||
		contieneClaveFirma(resultado, "pdfsubfilter") ||
		config.SubFilter == nil {
		return resultado
	}
	subfilter, ok := subfiltroPAdESRuntime(*config.SubFilter)
	if !ok {
		return resultado
	}
	return agregarOpcionFirma(resultado, "subfilter", subfilter)
}

func aplicarFacturaEPredeterminada(config ConfiguracionUsuarioFacturaE, resultado map[string]string) map[string]string {
	if !contieneAlgunaClaveFirma(resultado, clavesPoliticaFacturaE...) {
		resultado = agregarOpcionFacturaE(resultado, "facturaePolicyVersion", config.PolicyVersion, normalizarVersionPoliticaFacturaE)
		resultado = agregarOpcionFacturaE(resultado, "policyIdentifier", config.PolicyID, normalizarURIPoliticaFirma)
		resultado = agregarOpcionFacturaE(resultado, "policyIdentifierHash", config.PolicyHash, normalizarHashPoliticaFacturaE)
		resultado = agregarOpcionFacturaE(resultado, "policyQualifier", config.PolicyQualifier, normalizarURIPoliticaFirma)
	}
	resultado = agregarOpcionFacturaE(resultado, "signerClaimedRole", config.SignerRole, normalizarRolFacturaE)
	resultado = agregarOpcionFacturaE(resultado, "signatureProductionCity", config.City, normalizarTextoXMLFacturaE)
	resultado = agregarOpcionFacturaE(resultado, "signatureProductionProvince", config.Province, normalizarTextoXMLFacturaE)
	resultado = agregarOpcionFacturaE(resultado, "signatureProductionPostalCode", config.PostalCode, normalizarTextoXMLFacturaE)
	resultado = agregarOpcionFacturaE(resultado, "signatureProductionCountry", config.Country, normalizarTextoXMLFacturaE)
	return resultado
}

func aplicarPoliticaXAdESPredeterminada(config ConfiguracionUsuarioXAdES, resultado map[string]string) map[string]string {
	if contieneAlgunaClaveFirma(resultado, clavesPoliticaXAdES...) {
		return resultado
	}
	// El motor solo debe recibir una política completa. Una configuración
	// parcial o dañada se ignora entera para no emitir XML con un digest
	// ambiguo ni mezclar campos de distinto origen.
	if config.PolicyID == nil ||
		config.PolicyHash == nil ||
		config.PolicyHashAlgorithm == nil {
		return resultado
	}
	policyID, ok := normalizarURIPoliticaFirma(*config.PolicyID)
	if !ok {
		return resultado
	}
	hashAlgorithm, hashSize, ok := normalizarAlgoritmoHashPoliticaXAdES(
		*config.PolicyHashAlgorithm,
	)
	if !ok {
		return resultado
	}
	policyHash, ok := normalizarHashPoliticaXAdES(*config.PolicyHash, hashSize)
	if !ok {
		return resultado
	}
	policyQualifier := ""
	if config.PolicyQualifier != nil {
		policyQualifier, ok = normalizarURIPoliticaFirma(*config.PolicyQualifier)
		if !ok {
			return resultado
		}
	}

	resultado = agregarOpcionFirma(resultado, "policyIdentifier", policyID)
	resultado = agregarOpcionFirma(resultado, "policyIdentifierHash", policyHash)
	resultado = agregarOpcionFirma(
		resultado,
		"policyIdentifierHashAlgorithm",
		hashAlgorithm,
	)
	if policyQualifier != "" {
		resultado = agregarOpcionFirma(
			resultado,
			"policyQualifier",
			policyQualifier,
		)
	}
	return resultado
}

func agregarOpcionFacturaE(resultado map[string]string, clave string, valor *string, normalizar func(string) (string, bool)) map[string]string {
	if valor == nil || contieneClaveFirma(resultado, clave) {
		return resultado
	}
	normalizado, ok := normalizar(*valor)
	if !ok {
		return resultado
	}
	return agregarOpcionFirma(resultado, clave, normalizado)
}

func agregarOpcionFirma(resultado map[string]string, clave, valor string) map[string]string {
	if resultado == nil {
		resultado = make(map[string]string, 1)
	}
	resultado[clave] = valor
	return resultado
}

func normalizarVersionPoliticaFacturaE(valor string) (string, bool) {
	switch strings.TrimSpace(valor) {
	case "3.0":
		return "3.0", true
	case "3.1":
		return "3.1", true
	default:
		return "", false
	}
}

func normalizarURIPoliticaFirma(valor string) (string, bool) {
	valor = strings.TrimSpace(valor)
	u, err := url.Parse(valor)
	if err != nil || u == nil || !u.IsAbs() {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "urn", "oid":
	default:
		return "", false
	}
	if u.Host == "" && u.Opaque == "" {
		return "", false
	}
	return valor, true
}

func normalizarAlgoritmoHashPoliticaXAdES(valor string) (string, int, bool) {
	switch strings.ToUpper(strings.TrimSpace(valor)) {
	case "SHA-1", "HTTP://WWW.W3.ORG/2000/09/XMLDSIG#SHA1":
		return "http://www.w3.org/2000/09/xmldsig#sha1", 20, true
	case "SHA-256", "HTTP://WWW.W3.ORG/2001/04/XMLENC#SHA256":
		return "http://www.w3.org/2001/04/xmlenc#sha256", 32, true
	case "SHA-384", "HTTP://WWW.W3.ORG/2001/04/XMLDSIG-MORE#SHA384":
		return "http://www.w3.org/2001/04/xmldsig-more#sha384", 48, true
	case "SHA-512", "HTTP://WWW.W3.ORG/2001/04/XMLENC#SHA512":
		return "http://www.w3.org/2001/04/xmlenc#sha512", 64, true
	default:
		return "", 0, false
	}
}

func normalizarHashPoliticaXAdES(valor string, expectedSize int) (string, bool) {
	valor = strings.TrimSpace(valor)
	digest, err := base64.StdEncoding.DecodeString(valor)
	if err != nil ||
		expectedSize <= 0 ||
		len(digest) != expectedSize ||
		base64.StdEncoding.EncodeToString(digest) != valor {
		return "", false
	}
	return valor, true
}

func normalizarHashPoliticaFacturaE(valor string) (string, bool) {
	valor = strings.TrimSpace(valor)
	digest, err := base64.StdEncoding.DecodeString(valor)
	if err != nil || len(digest) != facturaePolicyHashSHA1Bytes {
		return "", false
	}
	return valor, true
}

func normalizarRolFacturaE(valor string) (string, bool) {
	valor = strings.ToLower(strings.TrimSpace(valor))
	switch valor {
	case "emisor", "receptor", "tercero", "supplier", "customer", "third party":
		return valor, true
	default:
		return "", false
	}
}

func normalizarTextoXMLFacturaE(valor string) (string, bool) {
	valor = strings.TrimSpace(valor)
	if valor == "" || !utf8.ValidString(valor) {
		return "", false
	}
	for _, r := range valor {
		if r != '\t' && r != '\n' && r != '\r' &&
			!(r >= 0x20 && r <= 0xD7FF) &&
			!(r >= 0xE000 && r <= 0xFFFD) &&
			!(r >= 0x10000 && r <= 0x10FFFF) {
			return "", false
		}
	}
	return valor, true
}

func subfiltroPAdESRuntime(valor string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(valor)) {
	case "etsi", "etsi.cades.detached":
		return "etsi", true
	case "adobe", "adbe.pkcs7.detached":
		return "adobe", true
	default:
		return "", false
	}
}

func copiarOpcionesFirma(origen map[string]string) map[string]string {
	if len(origen) == 0 {
		return nil
	}
	resultado := make(map[string]string, len(origen))
	for clave, valor := range origen {
		resultado[clave] = valor
	}
	return resultado
}

func contieneClaveFirma(options map[string]string, buscada string) bool {
	for clave := range options {
		if strings.EqualFold(strings.TrimSpace(clave), buscada) {
			return true
		}
	}
	return false
}

func contieneAlgunaClaveFirma(options map[string]string, buscadas ...string) bool {
	for _, buscada := range buscadas {
		if contieneClaveFirma(options, buscada) {
			return true
		}
	}
	return false
}
