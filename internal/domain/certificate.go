// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"errors"
	"time"
)

// TipoCertificado clasifica el proposito juridico de un certificado electronico.
// Se usa en la interfaz para filtrar certificados segun el contexto de firma.
type TipoCertificado string

const (
	// TipoCertFisica es un certificado de persona fisica (DNI/NIE electronico, FNMT ciudadano).
	TipoCertFisica TipoCertificado = "fisica"

	// TipoCertRepresentacion es un certificado de representante de persona juridica.
	// Permite firmar en nombre de una empresa u organizacion.
	TipoCertRepresentacion TipoCertificado = "representacion"

	// TipoCertSello es un certificado de sello electronico de empresa (firma automatizada).
	TipoCertSello TipoCertificado = "sello"

	// TipoCertEmpleadoPublico es un certificado de empleado publico de la Administracion.
	TipoCertEmpleadoPublico TipoCertificado = "empleado_publico"

	// TipoCertDesconocido se usa cuando no se puede determinar el tipo.
	TipoCertDesconocido TipoCertificado = "desconocido"
)

// CertificateRef es una referencia opaca a un certificado disponible en el sistema.
// No contiene material criptografico; solo metadatos para identificar y mostrar al usuario.
type CertificateRef struct {
	// ID es un identificador opaco asignado por el adaptador que gestiona el almacen.
	ID string

	// Subject es el nombre distinguido del titular del certificado.
	Subject string

	// Issuer es el nombre distinguido del emisor.
	Issuer string

	// NotAfter es la fecha de expiracion del certificado.
	NotAfter time.Time

	// Fingerprint es el hash SHA-256 del certificado en hexadecimal, para identificacion segura.
	Fingerprint string

	// HasSigningKey indica que el catálogo comprobó durante su enumeración,
	// sin abrir ni usar la clave, que la identidad tiene una clave privada
	// asociada. No implica que el certificado esté vigente ni que una tarjeta
	// vaya a autorizar la operación cuando se solicite la firma.
	HasSigningKey bool `json:"-"`

	// HasLocalDecryptionKey confirma que la identidad se importó con una clave
	// RSA local compatible con Desproteger. El catálogo puede informar esto sin
	// abrir ni usar la clave durante la exportación del certificado público.
	HasLocalDecryptionKey bool `json:"-"`

	// SigningKeyNeedsUnlock means the token requires login and enumeration
	// could not prove a matching private key. It permits offering an explicit
	// local unlock flow, but never asserts that a signing key exists.
	SigningKeyNeedsUnlock bool `json:"-"`

	// Tipo clasifica el proposito juridico del certificado.
	// Puede estar vacio si el adaptador no lo determina.
	Tipo TipoCertificado

	// Organizacion es el nombre de la empresa u organismo emisor del certificado,
	// si lo hay (campo O= del Subject). Util para representacion y sello.
	Organizacion string

	// NIF es el numero de identificacion fiscal extraido del Subject (serialNumber ETSI).
	// Formato estandar ETSI EN 319 412-1: "IDCES-12345678A", "VATES-A12345678", etc.
	NIF string

	// DER es el certificado X.509 codificado, para los filtros de selección
	// que piden las webs (uso de clave, políticas, emisor LDAP...). Nunca se
	// serializa: es material público, pero no forma parte del contrato JSON.
	DER []byte `json:"-"`

	// ChainDER conserva los emisores públicos que ya venían en una credencial
	// importada. No contiene claves ni concede confianza a esos emisores.
	ChainDER [][]byte `json:"-"`
}

func (c CertificateRef) IsExpired(now time.Time) bool {
	return now.After(c.NotAfter)
}

func (c CertificateRef) Validate() error {
	if c.ID == "" {
		return errors.New("el identificador del certificado no puede estar vacio")
	}
	if c.Fingerprint == "" {
		return errors.New("la huella del certificado no puede estar vacia")
	}
	return nil
}

// CertificateChain contiene las anclas de confianza configuradas para una
// verificacion. Certificates conserva las referencias de presentacion y
// DERCertificates transporta el material X.509 necesario para comprobar una
// cadena criptograficamente. UseSystemRoots solicita además el almacén raíz
// nativo de la plataforma.
type CertificateChain struct {
	Certificates    []CertificateRef
	DERCertificates [][]byte
	UseSystemRoots  bool
}

func (c CertificateChain) IsEmpty() bool {
	return len(c.Certificates) == 0 && len(c.DERCertificates) == 0 && !c.UseSystemRoots
}

// CertificateSelection es el resultado de que el usuario haya seleccionado un certificado.
type CertificateSelection struct {
	Certificate CertificateRef
	Confirmed   bool
}
