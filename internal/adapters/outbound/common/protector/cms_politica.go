// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"strings"

	"github.com/deatil/go-cryptobin/ber"
	cryptobinpkcs7 "github.com/deatil/go-cryptobin/pkcs7"

	"grxfirma/internal/adapters/outbound/common/anidamientoasn1"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

var (
	oidCMSAES128CBC     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidCMSAES192CBC     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidCMSAES256CBC     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidCMSAES128GCM     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 6}
	oidCMSAES192GCM     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 26}
	oidCMSRSAPKCS1v15   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	cifradosCMSAbiertos = []asn1.ObjectIdentifier{
		oidCMSAES128GCM, oidCMSAES192GCM, oidCMSAES256GCM,
		// AES-CBC es lo que emite `openssl cms -encrypt` por defecto.
		oidCMSAES128CBC, oidCMSAES192CBC, oidCMSAES256CBC,
	}
)

type cmsEncryptedContentInfoAlgoritmos struct {
	ContentType                asn1.ObjectIdentifier
	ContentEncryptionAlgorithm pkix.AlgorithmIdentifier
	Resto                      asn1.RawValue `asn1:"optional"`
}

type cmsEncryptedDataAlgoritmos struct {
	Version              int
	EncryptedContentInfo cmsEncryptedContentInfoAlgoritmos
	Resto                asn1.RawValue `asn1:"optional"`
}

type cmsKeyTransAlgoritmos struct {
	Version                int
	RecipientIdentifier    asn1.RawValue
	KeyEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedKey           []byte
}

// comprobarAnidamientoCMS corta un CMS recibido con un anidamiento ASN.1
// mayor que anidamientoasn1.MaxProfundidad antes de darlo a un analizador
// recursivo (Ber2der de go-cryptobin, Parse/Decrypt de go-cryptobin y
// ber2der de digitorus/pkcs7). Un "30 80" repetido millones de veces agota la
// pila y eso es fatal en Go: ningún recover lo para (E2-05). Solo decide la
// profundidad; los demás defectos de forma los sigue decidiendo el analizador.
func comprobarAnidamientoCMS(data []byte) error {
	if err := anidamientoasn1.Comprobar(data, anidamientoasn1.MaxProfundidad); errors.Is(err, anidamientoasn1.ErrDemasiadoProfundo) {
		return fmt.Errorf("contenedor CMS no valido: %w", err)
	}
	return nil
}

// Aviso visible (clave de los catálogos) y prefijo del detalle de auditoría
// cuando se descifra un CMS con un cifrado heredado.
const (
	avisoCMSCifradoHeredado   = "Este archivo se cifró con un algoritmo antiguo que hoy no se considera seguro. Se ha descifrado para que no pierda el acceso, pero quien se lo envió debería protegerlo de nuevo con uno actual."
	detalleCMSCifradoHeredado = "cifrado_cms_heredado="
)

// Aviso visible (clave de los catálogos) y prefijo del detalle de auditoría
// cuando el sobre trae, además de destinatarios RSA, otros de un tipo que
// GrxFirma no usa (acuerdo de claves, clave compartida, contraseña u otro).
const (
	avisoCMSDestinatariosAjenos   = "Este archivo cifrado iba dirigido también a otros destinatarios de un tipo que GrxFirma no admite. No se han tenido en cuenta y el archivo se ha abierto con su certificado."
	detalleCMSDestinatariosAjenos = "destinatarios_cms_ignorados="
	maxDestinatariosCMS           = 256
)

// tiposDestinatarioAjeno son las etiquetas de RecipientInfo (RFC 5652, 6.2)
// que no son ktri.
var tiposDestinatarioAjeno = map[int]string{1: "kari", 2: "kekri", 3: "pwri", 4: "ori"}

// revisionCMS es lo que se comprueba de un CMS antes de descifrarlo.
type revisionCMS struct {
	// heredado es el OID del cifrado heredado, si lo es.
	heredado asn1.ObjectIdentifier
	// ajenos son los tipos de destinatario que se han quitado del sobre.
	ajenos []string
	// sinAjenos es el ContentInfo (DER) sin esos destinatarios; nil si no
	// había ninguno y se descifra el original.
	sinAjenos []byte
}

// validarAlgoritmosCMS es revisarCMS quedándose solo con el cifrado heredado.
func validarAlgoritmosCMS(data []byte) (heredado asn1.ObjectIdentifier, err error) {
	revision, err := revisarCMS(data)
	return revision.heredado, err
}

// revisarCMS comprueba, antes de pasar el contenedor a las bibliotecas, el
// cifrado del contenido y el transporte de clave (F-06).
// AES-GCM y AES-CBC se aceptan sin más. Cualquier otro cifrado que la
// biblioteca sepa descifrar (DES, 3DES, AES-ECB/OFB/CFB/CCM, RC2, RC5,
// SM4…) se acepta por compatibilidad con lo que GrxFirma abría antes y se
// devuelve su OID para avisar y registrarlo (lote 13b), salvo que la
// política de máquina prohibir_cifrado_cms_heredado lo impida. Un cifrado
// desconocido se rechaza siempre. El transporte de clave debe ser RSA
// (PKCS#1 v1.5 u OAEP) en cada destinatario ktri. Los destinatarios de otro
// tipo (kari, kekri, pwri, ori) se quitan del sobre, con aviso, si queda al
// menos uno ktri (lote 17a): ninguna de las dos bibliotecas los sabe leer y,
// con ellos dentro, el sobre entero no se abriría.
func revisarCMS(data []byte) (revisionCMS, error) {
	if err := comprobarAnidamientoCMS(data); err != nil {
		return revisionCMS{}, err
	}
	der, err := ber.Ber2der(data)
	if err != nil {
		return revisionCMS{}, fmt.Errorf("contenedor CMS no valido: %w", err)
	}
	var info cmsContentInfo
	if rest, err := asn1.Unmarshal(der, &info); err != nil || len(rest) != 0 {
		return revisionCMS{}, errors.New("contenedor CMS no valido")
	}
	var (
		cifrado  pkix.AlgorithmIdentifier
		revision revisionCMS
	)
	switch {
	case info.ContentType.Equal(oidCMSEncryptedData):
		var datos cmsEncryptedDataAlgoritmos
		if rest, err := asn1.Unmarshal(info.Content.Bytes, &datos); err != nil || len(rest) != 0 {
			return revisionCMS{}, errors.New("estructura CMS EncryptedData no valida")
		}
		cifrado = datos.EncryptedContentInfo.ContentEncryptionAlgorithm
	case info.ContentType.Equal(oidCMSEnvelopedData):
		sobre, err := algoritmosEnvelopedData(info.Content.Bytes)
		if err != nil {
			return revisionCMS{}, err
		}
		cifrado = sobre.cifrado
		if len(sobre.ajenos) > 0 {
			revision.ajenos = sobre.ajenos
			if revision.sinAjenos, err = contentInfoCMS(info.ContentType, sobre.sinAjenos); err != nil {
				return revisionCMS{}, err
			}
		}
	default:
		return revisionCMS{}, errors.New("tipo CMS no soportado")
	}
	if contieneOID(cifradosCMSAbiertos, cifrado.Algorithm) {
		return revision, nil
	}
	if !cifradoCMSConocido(cifrado) {
		return revisionCMS{}, fmt.Errorf("algoritmo de cifrado CMS no permitido: %s", cifrado.Algorithm)
	}
	if machinepolicy.OptIn(machinepolicy.ProhibirCifradoCMSHeredado) {
		return revisionCMS{}, fmt.Errorf("cifrado CMS heredado %s: lo prohibe la politica de maquina %s", cifrado.Algorithm, machinepolicy.ProhibirCifradoCMSHeredado)
	}
	revision.heredado = cifrado.Algorithm
	return revision, nil
}

// contentInfoCMS envuelve un EnvelopedData (DER) en su ContentInfo.
func contentInfoCMS(tipo asn1.ObjectIdentifier, contenido []byte) ([]byte, error) {
	oid, err := asn1.Marshal(tipo)
	if err != nil {
		return nil, err
	}
	explicito, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: contenido})
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: append(oid, explicito...)})
}

// cifradoCMSConocido indica si go-cryptobin, la biblioteca que descifra,
// sabe descifrar ese algoritmo: es exactamente lo que GrxFirma aceptaba
// antes de F-06 (digitorus/pkcs7, la de respaldo, admite un subconjunto).
func cifradoCMSConocido(alg pkix.AlgorithmIdentifier) bool {
	_, err := cryptobinpkcs7.GetCipher(alg)
	return err == nil
}

// conAvisoCifradoHeredado añade al resultado el aviso y el detalle de
// auditoría si el contenido venía con un cifrado heredado.
func conAvisoCifradoHeredado(out domain.UnprotectedPayload, heredado asn1.ObjectIdentifier) domain.UnprotectedPayload {
	if len(heredado) == 0 {
		return out
	}
	out.Avisos = append(out.Avisos, avisoCMSCifradoHeredado)
	out.Detalles = append(out.Detalles, detalleCMSCifradoHeredado+heredado.String())
	return out
}

// conAvisosRevisionCMS añade los avisos y detalles de auditoría de lo que se
// encontró al revisar el CMS: cifrado heredado y destinatarios ignorados.
func conAvisosRevisionCMS(out domain.UnprotectedPayload, revision revisionCMS) domain.UnprotectedPayload {
	out = conAvisoCifradoHeredado(out, revision.heredado)
	if len(revision.ajenos) > 0 {
		out.Avisos = append(out.Avisos, avisoCMSDestinatariosAjenos)
		out.Detalles = append(out.Detalles, detalleCMSDestinatariosAjenos+strings.Join(revision.ajenos, ","))
	}
	return out
}

// algoritmosEnvelopedData recorre EnvelopedData campo a campo: originatorInfo
// [0] y unprotectedAttrs [1] son opcionales y no se pueden describir con
// etiquetas en un struct de encoding/asn1 sin que capturen otro campo.
// sobreEnvelopedData es lo que se lee de EnvelopedData antes de descifrar.
type sobreEnvelopedData struct {
	cifrado pkix.AlgorithmIdentifier
	ajenos  []string
	// sinAjenos es el EnvelopedData (DER) solo con los destinatarios ktri;
	// nil si no había ajenos.
	sinAjenos []byte
}

func algoritmosEnvelopedData(content []byte) (sobreEnvelopedData, error) {
	var sobre asn1.RawValue
	if rest, err := asn1.Unmarshal(content, &sobre); err != nil || len(rest) != 0 ||
		sobre.Class != asn1.ClassUniversal || sobre.Tag != asn1.TagSequence {
		return sobreEnvelopedData{}, errors.New("estructura CMS EnvelopedData no valida")
	}
	resto := sobre.Bytes
	var version int
	resto, err := asn1.Unmarshal(resto, &version)
	if err != nil {
		return sobreEnvelopedData{}, errors.New("version CMS EnvelopedData no valida")
	}
	var campo asn1.RawValue
	desdeDestinatarios := resto
	if resto, err = asn1.Unmarshal(resto, &campo); err != nil {
		return sobreEnvelopedData{}, errors.New("estructura CMS EnvelopedData no valida")
	}
	if campo.Class == asn1.ClassContextSpecific && campo.Tag == 0 {
		desdeDestinatarios = resto
		if resto, err = asn1.Unmarshal(resto, &campo); err != nil {
			return sobreEnvelopedData{}, errors.New("estructura CMS EnvelopedData no valida")
		}
	}
	if campo.Class != asn1.ClassUniversal || campo.Tag != asn1.TagSet {
		return sobreEnvelopedData{}, errors.New("destinatarios CMS EnvelopedData no validos")
	}
	ktri, ajenos, err := validarDestinatariosCMS(campo.Bytes)
	if err != nil {
		return sobreEnvelopedData{}, err
	}
	var eci cmsEncryptedContentInfoAlgoritmos
	if _, err := asn1.Unmarshal(resto, &eci); err != nil {
		return sobreEnvelopedData{}, errors.New("contenido cifrado CMS EnvelopedData no valido")
	}
	out := sobreEnvelopedData{cifrado: eci.ContentEncryptionAlgorithm, ajenos: ajenos}
	if len(ajenos) > 0 {
		// La versión y originatorInfo, tal cual; el SET, solo con los ktri (un
		// subconjunto de un SET OF en DER sigue ordenado); lo que sigue, tal
		// cual. Ni los destinatarios ni su orden entran en el cifrado.
		set, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true, Bytes: bytes.Join(ktri, nil)})
		if err != nil {
			return sobreEnvelopedData{}, err
		}
		cuerpo := append([]byte(nil), sobre.Bytes[:len(sobre.Bytes)-len(desdeDestinatarios)]...)
		cuerpo = append(append(cuerpo, set...), resto...)
		if out.sinAjenos, err = asn1.Marshal(asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: cuerpo}); err != nil {
			return sobreEnvelopedData{}, err
		}
	}
	return out, nil
}

// validarDestinatariosCMS devuelve los destinatarios ktri (DER) y el tipo de
// los demás. Cada ktri tiene que usar RSA; si no hay ninguno ktri, el error
// es el de siempre.
func validarDestinatariosCMS(conjunto []byte) (ktri [][]byte, ajenos []string, err error) {
	if len(conjunto) == 0 {
		return nil, nil, errors.New("CMS EnvelopedData sin destinatarios")
	}
	for len(conjunto) > 0 {
		if len(ktri)+len(ajenos) >= maxDestinatariosCMS {
			return nil, nil, fmt.Errorf("CMS EnvelopedData con más de %d destinatarios", maxDestinatariosCMS)
		}
		var destinatario asn1.RawValue
		rest, err := asn1.Unmarshal(conjunto, &destinatario)
		if err != nil {
			return nil, nil, errors.New("destinatario CMS EnvelopedData no valido")
		}
		conjunto = rest
		if tipo, ok := tiposDestinatarioAjeno[destinatario.Tag]; ok && destinatario.Class == asn1.ClassContextSpecific && destinatario.IsCompound {
			ajenos = append(ajenos, tipo)
			continue
		}
		if destinatario.Class != asn1.ClassUniversal || destinatario.Tag != asn1.TagSequence {
			return nil, nil, errors.New("solo se admiten destinatarios CMS con transporte de clave RSA")
		}
		var info cmsKeyTransAlgoritmos
		if rest, err := asn1.Unmarshal(destinatario.FullBytes, &info); err != nil || len(rest) != 0 {
			return nil, nil, errors.New("destinatario CMS EnvelopedData no valido")
		}
		alg := info.KeyEncryptionAlgorithm.Algorithm
		if !alg.Equal(oidCMSRSAPKCS1v15) && !alg.Equal(oidCMSRSAESOAEP) {
			return nil, nil, fmt.Errorf("transporte de clave CMS no permitido: %s", alg)
		}
		ktri = append(ktri, destinatario.FullBytes)
	}
	if len(ktri) == 0 {
		return nil, nil, errors.New("solo se admiten destinatarios CMS con transporte de clave RSA")
	}
	return ktri, ajenos, nil
}

func contieneOID(lista []asn1.ObjectIdentifier, oid asn1.ObjectIdentifier) bool {
	for _, candidato := range lista {
		if candidato.Equal(oid) {
			return true
		}
	}
	return false
}
