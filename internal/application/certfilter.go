// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- huella SHA-1 solo para localizar un certificado indicado por la web (filtro thumbprint de AutoFirma Java), no para firmar.
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"grxfirma/internal/domain"
)

// Filtros de certificados de AutoFirma Java (CertFilterManager). La web los
// envía en las propiedades "filters", "filter" o "filters.1", "filters.2"...
// Cada grupo es una conjunción de filtros separados por ";" y los grupos se
// combinan en disyunción. Si ningún certificado los cumple no se ofrecen los
// demás: la web ha pedido expresamente acotar la selección.

// ResultadoFiltroCertificados describe la aplicación de los filtros.
type ResultadoFiltroCertificados struct {
	Certificados []domain.CertificateRef
	Descripcion  string
	Aplicado     bool
	Ignorados    []string
}

var (
	oidPseudonym    = asn1.ObjectIdentifier{2, 5, 4, 65}
	oidQCStatements = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}
	oidQcSSCD       = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}
)

// GruposFiltroCertificados extrae los grupos de filtros de las propiedades.
func GruposFiltroCertificados(options map[string]string) []string {
	valor := func(k string) string {
		for clave, v := range options {
			if strings.EqualFold(clave, k) {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	if v := valor("filters"); v != "" {
		return []string{v}
	}
	if v := valor("filter"); v != "" {
		return []string{v}
	}
	var grupos []string
	for i := 1; i <= 64; i++ {
		v := valor("filters." + strconv.Itoa(i))
		if v == "" {
			break
		}
		grupos = append(grupos, v)
	}
	return grupos
}

// FiltrarCertificados aplica los filtros pedidos por la web. Sin filtros,
// como Java, solo se ocultan los certificados caducados.
func FiltrarCertificados(certs []domain.CertificateRef, options map[string]string, ahora time.Time) ResultadoFiltroCertificados {
	grupos := GruposFiltroCertificados(options)
	if len(grupos) == 0 {
		var vigentes []domain.CertificateRef
		for _, c := range certs {
			if !c.IsExpired(ahora) {
				vigentes = append(vigentes, c)
			}
		}
		return ResultadoFiltroCertificados{Certificados: vigentes, Descripcion: "nonexpired (por defecto)"}
	}
	res := ResultadoFiltroCertificados{Descripcion: strings.Join(grupos, " || "), Aplicado: true}
	for _, c := range certs {
		cert, _ := x509.ParseCertificate(c.DER)
		for _, grupo := range grupos {
			ok, ignorados := cumpleGrupo(c, cert, grupo, ahora)
			res.Ignorados = append(res.Ignorados, ignorados...)
			if ok {
				res.Certificados = append(res.Certificados, c)
				break
			}
		}
	}
	return res
}

func cumpleGrupo(ref domain.CertificateRef, cert *x509.Certificate, grupo string, ahora time.Time) (bool, []string) {
	var ignorados []string
	for _, filtro := range strings.Split(grupo, ";") {
		filtro = strings.TrimSpace(filtro)
		if filtro == "" {
			continue
		}
		nombre, arg, _ := strings.Cut(filtro, ":")
		nombre = strings.ToLower(strings.TrimSpace(nombre))
		arg = strings.TrimSpace(arg)
		ok, conocido := aplicarFiltro(ref, cert, nombre, arg, ahora)
		if !conocido {
			ignorados = append(ignorados, nombre)
			continue
		}
		if !ok {
			return false, ignorados
		}
	}
	return true, ignorados
}

func aplicarFiltro(ref domain.CertificateRef, cert *x509.Certificate, nombre, arg string, ahora time.Time) (cumple, conocido bool) {
	switch nombre {
	case "nonexpired":
		return strings.EqualFold(arg, "false") || !ref.IsExpired(ahora), true
	case "subject.contains":
		return strings.Contains(strings.ToLower(ref.Subject), strings.ToLower(arg)), true
	case "issuer.contains":
		return strings.Contains(strings.ToLower(ref.Issuer), strings.ToLower(arg)), true
	}
	if cert == nil {
		// Sin el certificado no se puede comprobar un filtro que lo exige:
		// se excluye, que es lo prudente cuando la web acota la selección.
		return false, true
	}
	switch nombre {
	case "subject.rfc2254":
		return cumpleLDAP(arg, cert.Subject.Names), true
	case "issuer.rfc2254", "issuer.rfc2254.recurse":
		return cumpleLDAP(arg, cert.Issuer.Names), true
	case "thumbprint":
		return cumpleHuella(cert, arg), true
	case "encodedcert":
		der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(arg), ""))
		return err == nil && bytes.Equal(der, cert.Raw), true
	case "ssl", "qualified":
		serie, ok := new(big.Int).SetString(strings.TrimPrefix(strings.ToLower(arg), "0x"), 16)
		return ok && serie.Cmp(cert.SerialNumber) == 0, true
	case "dnie":
		return esDNIe(cert) && !esAutenticacion(cert), true
	case "signingcert":
		return !esAutenticacion(cert), true
	case "authcert":
		return esAutenticacion(cert), true
	case "policyid":
		for _, oid := range strings.Split(arg, ",") {
			for _, p := range cert.PolicyIdentifiers {
				if p.String() == strings.TrimSpace(oid) {
					return true, true
				}
			}
		}
		return false, true
	case "pseudonym":
		if strings.EqualFold(arg, "only") {
			return tieneAtributo(cert.Subject.Names, oidPseudonym), true
		}
		return true, true
	case "sscd":
		return esSSCD(cert), true
	}
	if strings.HasPrefix(nombre, "keyusage.") {
		return cumpleKeyUsage(cert, strings.TrimPrefix(nombre, "keyusage."), arg)
	}
	return false, false
}

func cumpleHuella(cert *x509.Certificate, arg string) bool {
	alg, valor, ok := strings.Cut(arg, ":")
	if !ok {
		return false
	}
	var huella []byte
	switch strings.ToUpper(strings.ReplaceAll(alg, "-", "")) {
	case "SHA1":
		h := sha1.Sum(cert.Raw) // #nosec G401 -- identificación del certificado pedida por la web.
		huella = h[:]
	case "SHA256":
		h := sha256.Sum256(cert.Raw)
		huella = h[:]
	case "SHA512":
		h := sha512.Sum512(cert.Raw)
		huella = h[:]
	default:
		return false
	}
	esperado, err := hex.DecodeString(strings.ReplaceAll(strings.ReplaceAll(valor, ":", ""), " ", ""))
	return err == nil && bytes.Equal(esperado, huella)
}

func cumpleKeyUsage(cert *x509.Certificate, uso, arg string) (bool, bool) {
	bits := map[string]x509.KeyUsage{
		"digitalsignature": x509.KeyUsageDigitalSignature,
		"nonrepudiation":   x509.KeyUsageContentCommitment,
		"keyencipherment":  x509.KeyUsageKeyEncipherment,
		"dataencipherment": x509.KeyUsageDataEncipherment,
		"keyagreement":     x509.KeyUsageKeyAgreement,
		"keycertsign":      x509.KeyUsageCertSign,
		"crlsign":          x509.KeyUsageCRLSign,
		"encipheronly":     x509.KeyUsageEncipherOnly,
		"decipheronly":     x509.KeyUsageDecipherOnly,
	}
	bit, ok := bits[strings.ToLower(uso)]
	if !ok {
		return false, false
	}
	switch strings.ToLower(arg) {
	case "true":
		return cert.KeyUsage&bit != 0, true
	case "false":
		return cert.KeyUsage&bit == 0, true
	default: // "null" o vacío: indiferente
		return true, true
	}
}

func esDNIe(cert *x509.Certificate) bool {
	emisor := strings.ToUpper(cert.Issuer.String())
	return strings.Contains(emisor, "DIRECCION GENERAL DE LA POLICIA") || strings.Contains(emisor, "DNIE")
}

// esAutenticacion reconoce certificados solo de autenticación (DNIe y
// tarjetas CERES "AUTENTICACIÓN", o sin no repudio y con uso de cliente TLS).
func esAutenticacion(cert *x509.Certificate) bool {
	cn := strings.ToUpper(cert.Subject.CommonName)
	if strings.Contains(cn, "AUTENTICACI") {
		return true
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageContentCommitment == 0 {
		soloCliente := len(cert.ExtKeyUsage) > 0
		for _, u := range cert.ExtKeyUsage {
			if u != x509.ExtKeyUsageClientAuth {
				soloCliente = false
			}
		}
		return soloCliente
	}
	return false
}

func esSSCD(cert *x509.Certificate) bool {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oidQCStatements) {
			der, _ := asn1.Marshal(oidQcSSCD)
			return bytes.Contains(ext.Value, der)
		}
	}
	return false
}

func tieneAtributo(names []pkixAttribute, oid asn1.ObjectIdentifier) bool {
	for _, n := range names {
		if n.Type.Equal(oid) {
			return true
		}
	}
	return false
}

// DescribirFiltro resume un filtro para los avisos al usuario.
func DescribirFiltro(r ResultadoFiltroCertificados) string {
	return fmt.Sprintf("la web solo admite certificados que cumplan: %s", r.Descripcion)
}
