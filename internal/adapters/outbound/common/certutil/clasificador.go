// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package certutil proporciona utilidades compartidas para clasificar e
// interpretar certificados X.509 en el contexto de la firma electronica espanola.
//
// Los adaptadores de almacen (pkcs12importer, nssstore, pkcs11store) usan este
// paquete para enriquecer domain.CertificateRef con tipo, organizacion y NIF.
package certutil

import (
	"crypto/x509"
	"encoding/asn1"
	"strconv"
	"strings"
	"time"

	"grxfirma/internal/domain"
)

// OIDs de politicas de certificacion espanolas (FNMT-RCM).
// Referencia: FNMT Declaracion de Practicas de Certificacion (DPC) v3.
var (
	oidFNMTFisica         = asn1.ObjectIdentifier{2, 16, 724, 1, 3, 5, 4, 1}
	oidFNMTRepresentacion = asn1.ObjectIdentifier{2, 16, 724, 1, 3, 5, 4, 2}
	oidFNMTEmpleado       = asn1.ObjectIdentifier{2, 16, 724, 1, 3, 5, 4, 3}
	oidFNMTSello          = asn1.ObjectIdentifier{2, 16, 724, 1, 3, 5, 8, 1, 1}

	// Formas en puntos de los OIDs conocidos para comparar contra cert.Policies
	// (x509.OID, introducido en Go 1.22 para reemplazar PolicyIdentifiers).
	strFNMTFisica         = oidAPuntos(oidFNMTFisica)
	strFNMTRepresentacion = oidAPuntos(oidFNMTRepresentacion)
	strFNMTEmpleado       = oidAPuntos(oidFNMTEmpleado)
	strFNMTSello          = oidAPuntos(oidFNMTSello)
)

// oidAPuntos convierte un asn1.ObjectIdentifier a su representacion en puntos.
func oidAPuntos(oid asn1.ObjectIdentifier) string {
	partes := make([]string, len(oid))
	for i, v := range oid {
		partes[i] = strconv.Itoa(v)
	}
	return strings.Join(partes, ".")
}

// ClasificarCertificado determina el tipo juridico del certificado y extrae
// la organizacion y el NIF del Subject segun ETSI EN 319 412-1.
//
// Devuelve los valores para domain.CertificateRef.{Tipo, Organizacion, NIF}.
func ClasificarCertificado(cert *x509.Certificate) (tipo domain.TipoCertificado, org, nif string) {
	if cert == nil || cert.IsCA {
		return domain.TipoCertDesconocido, "", ""
	}

	nif = cert.Subject.SerialNumber
	if len(cert.Subject.Organization) > 0 {
		org = cert.Subject.Organization[0]
	}

	// 1. OIDs de politica (mas fiable; FNMT los incluye siempre).
	if t, ok := clasificarPorOID(cert); ok {
		return t, org, nif
	}

	// 2. Prefijo del serialNumber del Subject (ETSI EN 319 412-1).
	if t, ok := clasificarPorSerialNumber(nif, org); ok {
		return t, org, nif
	}

	// 3. Heuristica: Organization sin numero de persona → sello electronico.
	if org != "" && nif == "" {
		return domain.TipoCertSello, org, nif
	}

	return domain.TipoCertDesconocido, org, nif
}

// clasificarPorOID comprueba los OIDs de politica del certificado.
//
// Comprueba tanto cert.PolicyIdentifiers (campo legacy) como cert.Policies
// (introducido en Go 1.22, activo por defecto desde Go 1.23). Ambos se revisan
// para garantizar compatibilidad con certs creados con distintas versiones de Go.
func clasificarPorOID(cert *x509.Certificate) (domain.TipoCertificado, bool) {
	// Legacy: PolicyIdentifiers (Go < 1.23 o GODEBUG=x509usepolicies=1)
	for _, oid := range cert.PolicyIdentifiers {
		switch {
		case oid.Equal(oidFNMTFisica):
			return domain.TipoCertFisica, true
		case oid.Equal(oidFNMTRepresentacion):
			return domain.TipoCertRepresentacion, true
		case oid.Equal(oidFNMTEmpleado):
			return domain.TipoCertEmpleadoPublico, true
		case oid.Equal(oidFNMTSello):
			return domain.TipoCertSello, true
		}
	}

	// Moderno: cert.Policies (Go 1.22+, activo por defecto desde Go 1.23).
	for _, p := range cert.Policies {
		s := p.String()
		switch s {
		case strFNMTFisica:
			return domain.TipoCertFisica, true
		case strFNMTRepresentacion:
			return domain.TipoCertRepresentacion, true
		case strFNMTEmpleado:
			return domain.TipoCertEmpleadoPublico, true
		case strFNMTSello:
			return domain.TipoCertSello, true
		}
	}

	return "", false
}

// clasificarPorSerialNumber clasifica segun el prefijo ETSI EN 319 412-1.
//
//	IDCES-12345678A  → persona fisica (DNI)
//	NIFES-12345678A  → persona fisica (NIF)
//	NIEES-X1234567A  → persona fisica (NIE)
//	VATES-A12345678  → juridica/representacion (CIF empresa) si tiene Org
//	VATES-B12345678  → sello electronico si no tiene Org
//	TINES-xxx        → empleado publico
//	PASES-P1234567A  → persona fisica (pasaporte)
func clasificarPorSerialNumber(serialNum, org string) (domain.TipoCertificado, bool) {
	upper := strings.ToUpper(strings.TrimSpace(serialNum))
	if upper == "" {
		return "", false
	}

	switch {
	case strings.HasPrefix(upper, "VATES-") && org != "":
		return domain.TipoCertRepresentacion, true
	case strings.HasPrefix(upper, "VATES-"):
		return domain.TipoCertSello, true
	case strings.HasPrefix(upper, "TINES-"):
		return domain.TipoCertEmpleadoPublico, true
	}

	personaPrefijos := []string{"IDCES-", "NIFES-", "NIEES-", "PASES-", "PASNO-", "PASDE-", "PASUK-"}
	for _, p := range personaPrefijos {
		if strings.HasPrefix(upper, p) {
			return domain.TipoCertFisica, true
		}
	}
	return "", false
}

// PuedeDigitalmenteSign informa si el KeyUsage del certificado incluye firma digital.
func PuedeDigitalmenteSign(cert *x509.Certificate) bool {
	if cert.KeyUsage == 0 {
		return true // sin restriccion de uso → asumir que puede firmar
	}
	return cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 ||
		cert.KeyUsage&x509.KeyUsageContentCommitment != 0
}

// PuedeCifrar informa si el KeyUsage del certificado permite cifrado o acuerdo
// de clave. Se usa para decidir si una identidad puede actuar como destinatario
// de proteccion en el perfil de compatibilidad.
func PuedeCifrar(cert *x509.Certificate) bool {
	if cert == nil {
		return false
	}
	if cert.KeyUsage == 0 {
		return true // sin restriccion explicita → asumir uso general
	}
	return cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 ||
		cert.KeyUsage&x509.KeyUsageKeyAgreement != 0
}

// DiasHastaExpiracion devuelve los dias restantes hasta la caducidad.
// Negativo si ya caducó.
func DiasHastaExpiracion(cert *x509.Certificate) int {
	if cert.NotAfter.IsZero() {
		return 0
	}
	return int(time.Until(cert.NotAfter).Hours() / 24)
}
