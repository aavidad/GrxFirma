// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"errors"
	"strings"

	"grxfirma/internal/domain"
)

// BatchItemInput representa la entrada primitiva para construir un trabajo de lote
// sin exponer tipos de dominio a los adaptadores.
type BatchItemInput struct {
	Nombre    string
	Contenido []byte
	TipoMIME  string
	Formato   string
	Accion    string
	// Opciones contiene los overrides propios del documento. Cuando se usa
	// NewProcessBatchCommandWithDefaults, prevalecen por clave sobre la plantilla.
	Opciones map[string]string
}

// ParseSignatureFormat convierte una etiqueta textual al formato de firma de dominio.
func ParseSignatureFormat(raw string) (domain.SignatureFormat, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", "auto", "cades", "cadestri", "cades-bes", "cms", "pkcs7", "pkcs#7", "cms/pkcs#7", "cms/pkcs7":
		return domain.FormatCAdES, nil
	// Las variantes "tri" de @firma (CAdEStri, XAdEStri, PAdEStri, FacturaEtri)
	// piden la misma firma calculada en servidor trifásico; localmente se
	// genera el formato equivalente, como ya se hacía con CAdEStri.
	case "xades", "xades-bes", "xadestri",
		"xades detached", "xades enveloping", "xades enveloped", "xades externally detached":
		return domain.FormatXAdES, nil
	case "pades", "pades-basic", "padestri", "adobe pdf", "adobe pdf triphase", "pdf":
		return domain.FormatPAdES, nil
	case "xmldsig", "xmlsig", "xml-dsig",
		"xmldsig detached", "xmldsig enveloping", "xmldsig enveloped", "xmldsig externally detached":
		return domain.SignatureFormat("XMLdSig"), nil
	case "odf", "odf (open document format)":
		return domain.SignatureFormat("ODF"), nil
	case "ooxml", "ooxml (office open xml)":
		return domain.SignatureFormat("OOXML"), nil
	case "facturae", "facturaetri", "factura-e":
		return domain.SignatureFormat("FacturaE"), nil
	case "none", "nonetri", "pkcs1", "pkcs#1":
		return domain.SignatureFormat("PKCS1"), nil
	case "cades-asic-s", "cades-asic-s-tri", "asic-cades", "asiccades":
		return domain.SignatureFormat("ASiC-CAdES"), nil
	case "asic-xades", "asicxades", "xades-asic", "xadesasics", "xades-asic-s", "xades-asic-s-tri":
		return domain.SignatureFormat("ASiC-XAdES"), nil
	default:
		if normalized == "" {
			return "", errors.New("formato de firma no soportado")
		}
		return domain.SignatureFormat(strings.TrimSpace(raw)), nil
	}
}

// ParseSignatureAction convierte una etiqueta textual a la acción de firma de dominio.
func ParseSignatureAction(raw string) (domain.SignatureAction, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "sign":
		return domain.ActionSign, nil
	case "cosign":
		return domain.ActionCoSign, nil
	case "countersign":
		return domain.ActionCounterSign, nil
	default:
		return "", errors.New("accion de firma no soportada")
	}
}

// ParseProtectionProfile convierte una etiqueta textual al perfil de proteccion de dominio.
func ParseProtectionProfile(raw string) (domain.ProtectionProfile, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "alto", "strong", "pq", "mlkem", "mlkem768", "alto-mlkem768-aes256gcm":
		return domain.ProtectionProfileStrong, nil
	case "compat", "compat-rsa", "rsa", "rsa-oaep", "compat-rsa-oaep-aes256gcm":
		return domain.ProtectionProfileCompat, nil
	default:
		return "", errors.New("perfil de proteccion no soportado")
	}
}

// NewSignCommand crea un SignCommand a partir de tipos primitivos.
func NewSignCommand(nombre string, contenido []byte, tipoMIME string, formato string, accion string, certificateID string, options map[string]string) (SignCommand, error) {
	documento, err := domain.NewDocument(nombre, contenido, tipoMIME)
	if err != nil {
		return SignCommand{}, err
	}
	signFormat, err := ParseSignatureFormat(formato)
	if err != nil {
		return SignCommand{}, err
	}
	signAction, err := ParseSignatureAction(accion)
	if err != nil {
		return SignCommand{}, err
	}
	return SignCommand{
		Document:      documento,
		Format:        signFormat,
		Action:        signAction,
		CertificateID: certificateID,
		Options:       options,
	}, nil
}

// NewProcessBatchCommand crea un ProcessBatchCommand a partir de entradas primitivas.
// Se conserva como contrato compatible para los adaptadores que no necesitan una
// plantilla global de opciones.
func NewProcessBatchCommand(items []BatchItemInput) (ProcessBatchCommand, error) {
	return NewProcessBatchCommandWithDefaults(items, nil)
}

// NewProcessBatchCommandWithDefaults crea un ProcessBatchCommand y aplica las
// opciones por defecto del lote a cada trabajo. Las opciones declaradas por un
// documento prevalecen por clave sobre la plantilla global, lo que permite
// sobrescribir, entre otros parámetros, la página o rango de un sello visible.
//
// Tanto la plantilla como los overrides se copian para que ningún adaptador ni
// trabajo pueda modificar accidentalmente las opciones de los demás.
func NewProcessBatchCommandWithDefaults(items []BatchItemInput, defaultOptions map[string]string) (ProcessBatchCommand, error) {
	if len(items) == 0 {
		return ProcessBatchCommand{}, errors.New("el lote no puede estar vacio")
	}
	jobs := make([]domain.SignatureJob, 0, len(items))
	for _, item := range items {
		documento, err := domain.NewDocument(item.Nombre, item.Contenido, item.TipoMIME)
		if err != nil {
			return ProcessBatchCommand{}, err
		}
		formato, err := ParseSignatureFormat(item.Formato)
		if err != nil {
			return ProcessBatchCommand{}, err
		}
		accion, err := ParseSignatureAction(item.Accion)
		if err != nil {
			return ProcessBatchCommand{}, err
		}
		jobs = append(jobs, domain.SignatureJob{
			Document: documento,
			Format:   formato,
			Action:   accion,
			Options:  mergeBatchOptions(defaultOptions, item.Opciones),
		})
	}
	return ProcessBatchCommand{Jobs: jobs}, nil
}

func mergeBatchOptions(defaults, overrides map[string]string) map[string]string {
	if len(defaults) == 0 && len(overrides) == 0 {
		return nil
	}
	merged := make(map[string]string, len(defaults)+len(overrides))
	for key, value := range defaults {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

// NewProtectCommand crea un ProtectCommand a partir de tipos primitivos.
func NewProtectCommand(nombre string, contenido []byte, tipoMIME string, profile string, recipientIDs []string, options map[string]string) (ProtectCommand, error) {
	documento, err := domain.NewDocument(nombre, contenido, tipoMIME)
	if err != nil {
		return ProtectCommand{}, err
	}
	protectionProfile, err := ParseProtectionProfile(profile)
	if err != nil {
		return ProtectCommand{}, err
	}
	return ProtectCommand{
		Document:     documento,
		Profile:      protectionProfile,
		RecipientIDs: append([]string(nil), recipientIDs...),
		Options:      options,
	}, nil
}

// NewProtectAndSignCommand crea un ProtectAndSignCommand a partir de tipos primitivos.
func NewProtectAndSignCommand(nombre string, contenido []byte, tipoMIME string, profile string, recipientIDs []string, certificateID string, options map[string]string) (ProtectAndSignCommand, error) {
	documento, err := domain.NewDocument(nombre, contenido, tipoMIME)
	if err != nil {
		return ProtectAndSignCommand{}, err
	}
	protectionProfile, err := ParseProtectionProfile(profile)
	if err != nil {
		return ProtectAndSignCommand{}, err
	}
	return ProtectAndSignCommand{
		Document:      documento,
		Profile:       protectionProfile,
		RecipientIDs:  append([]string(nil), recipientIDs...),
		CertificateID: certificateID,
		Options:       options,
	}, nil
}
