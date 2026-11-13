// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package protector

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"

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

// politicaProhibirCifradoCMSHeredado es el valor de la política de máquina
// con el que un administrador impide descifrar CMS con un cifrado heredado.
const politicaProhibirCifradoCMSHeredado = "prohibir_cifrado_cms_heredado"

// Aviso visible (clave de los catálogos) y prefijo del detalle de auditoría
// cuando se descifra un CMS con un cifrado heredado.
const (
	avisoCMSCifradoHeredado   = "Este archivo se cifró con un algoritmo antiguo que hoy no se considera seguro. Se ha descifrado para que no pierda el acceso, pero quien se lo envió debería protegerlo de nuevo con uno actual."
	detalleCMSCifradoHeredado = "cifrado_cms_heredado="
)

// validarAlgoritmosCMS comprueba, antes de pasar el contenedor a las
// bibliotecas, el cifrado del contenido y el transporte de clave (F-06).
// AES-GCM y AES-CBC se aceptan sin más. Cualquier otro cifrado que la
// biblioteca sepa descifrar (DES, 3DES, AES-ECB/OFB/CFB/CCM, RC2, RC5,
// SM4…) se acepta por compatibilidad con lo que GrxFirma abría antes y se
// devuelve su OID para avisar y registrarlo (lote 13b), salvo que la
// política de máquina prohibir_cifrado_cms_heredado lo impida. Un cifrado
// desconocido se rechaza siempre. El transporte de clave debe ser RSA
// (PKCS#1 v1.5 u OAEP) y cada destinatario de tipo ktri.
func validarAlgoritmosCMS(data []byte) (heredado asn1.ObjectIdentifier, err error) {
	if err := comprobarAnidamientoCMS(data); err != nil {
		return nil, err
	}
	der, err := ber.Ber2der(data)
	if err != nil {
		return nil, fmt.Errorf("contenedor CMS no valido: %w", err)
	}
	var info cmsContentInfo
	if rest, err := asn1.Unmarshal(der, &info); err != nil || len(rest) != 0 {
		return nil, errors.New("contenedor CMS no valido")
	}
	var cifrado pkix.AlgorithmIdentifier
	switch {
	case info.ContentType.Equal(oidCMSEncryptedData):
		var datos cmsEncryptedDataAlgoritmos
		if rest, err := asn1.Unmarshal(info.Content.Bytes, &datos); err != nil || len(rest) != 0 {
			return nil, errors.New("estructura CMS EncryptedData no valida")
		}
		cifrado = datos.EncryptedContentInfo.ContentEncryptionAlgorithm
	case info.ContentType.Equal(oidCMSEnvelopedData):
		cifrado, err = algoritmosEnvelopedData(info.Content.Bytes)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("tipo CMS no soportado")
	}
	if contieneOID(cifradosCMSAbiertos, cifrado.Algorithm) {
		return nil, nil
	}
	if !cifradoCMSConocido(cifrado) {
		return nil, fmt.Errorf("algoritmo de cifrado CMS no permitido: %s", cifrado.Algorithm)
	}
	if machinepolicy.OptIn(politicaProhibirCifradoCMSHeredado) {
		return nil, fmt.Errorf("cifrado CMS heredado %s: lo prohibe la politica de maquina %s", cifrado.Algorithm, politicaProhibirCifradoCMSHeredado)
	}
	return cifrado.Algorithm, nil
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

// algoritmosEnvelopedData recorre EnvelopedData campo a campo: originatorInfo
// [0] y unprotectedAttrs [1] son opcionales y no se pueden describir con
// etiquetas en un struct de encoding/asn1 sin que capturen otro campo.
func algoritmosEnvelopedData(content []byte) (pkix.AlgorithmIdentifier, error) {
	var sobre asn1.RawValue
	if rest, err := asn1.Unmarshal(content, &sobre); err != nil || len(rest) != 0 ||
		sobre.Class != asn1.ClassUniversal || sobre.Tag != asn1.TagSequence {
		return pkix.AlgorithmIdentifier{}, errors.New("estructura CMS EnvelopedData no valida")
	}
	resto := sobre.Bytes
	var version int
	resto, err := asn1.Unmarshal(resto, &version)
	if err != nil {
		return pkix.AlgorithmIdentifier{}, errors.New("version CMS EnvelopedData no valida")
	}
	var campo asn1.RawValue
	if resto, err = asn1.Unmarshal(resto, &campo); err != nil {
		return pkix.AlgorithmIdentifier{}, errors.New("estructura CMS EnvelopedData no valida")
	}
	if campo.Class == asn1.ClassContextSpecific && campo.Tag == 0 {
		if resto, err = asn1.Unmarshal(resto, &campo); err != nil {
			return pkix.AlgorithmIdentifier{}, errors.New("estructura CMS EnvelopedData no valida")
		}
	}
	if campo.Class != asn1.ClassUniversal || campo.Tag != asn1.TagSet {
		return pkix.AlgorithmIdentifier{}, errors.New("destinatarios CMS EnvelopedData no validos")
	}
	if err := validarDestinatariosCMS(campo.Bytes); err != nil {
		return pkix.AlgorithmIdentifier{}, err
	}
	var eci cmsEncryptedContentInfoAlgoritmos
	if _, err := asn1.Unmarshal(resto, &eci); err != nil {
		return pkix.AlgorithmIdentifier{}, errors.New("contenido cifrado CMS EnvelopedData no valido")
	}
	return eci.ContentEncryptionAlgorithm, nil
}

func validarDestinatariosCMS(conjunto []byte) error {
	if len(conjunto) == 0 {
		return errors.New("CMS EnvelopedData sin destinatarios")
	}
	for len(conjunto) > 0 {
		var destinatario asn1.RawValue
		rest, err := asn1.Unmarshal(conjunto, &destinatario)
		if err != nil {
			return errors.New("destinatario CMS EnvelopedData no valido")
		}
		conjunto = rest
		if destinatario.Class != asn1.ClassUniversal || destinatario.Tag != asn1.TagSequence {
			return errors.New("solo se admiten destinatarios CMS con transporte de clave RSA")
		}
		var ktri cmsKeyTransAlgoritmos
		if rest, err := asn1.Unmarshal(destinatario.FullBytes, &ktri); err != nil || len(rest) != 0 {
			return errors.New("destinatario CMS EnvelopedData no valido")
		}
		alg := ktri.KeyEncryptionAlgorithm.Algorithm
		if !alg.Equal(oidCMSRSAPKCS1v15) && !alg.Equal(oidCMSRSAESOAEP) {
			return fmt.Errorf("transporte de clave CMS no permitido: %s", alg)
		}
	}
	return nil
}

func contieneOID(lista []asn1.ObjectIdentifier, oid asn1.ObjectIdentifier) bool {
	for _, candidato := range lista {
		if candidato.Equal(oid) {
			return true
		}
	}
	return false
}
