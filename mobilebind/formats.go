// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"errors"
	"mime"
	"path"
	"slices"
	"strings"

	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
)

// mobileFormat describe lo que el motor compartido genera para cada formato
// en Android. Los nombres son los del contrato y los de ParseSignatureFormat.
type mobileFormat struct {
	contract string
	actions  []string
	profiles []string
	rsaOnly  bool
}

// Mensaje cerrado: la UI lo traduce por coincidencia exacta.
const mobileFormatRequiresRSAMessage = "El formato elegido solo admite certificados con clave RSA."

var mobileFormats = map[string]mobileFormat{
	"cades":      {"CAdES", []string{"sign", "cosign", "countersign"}, []string{"baseline", "t", "lt", "lta"}, false},
	"pades":      {"PAdES", []string{"sign", "cosign"}, []string{"baseline", "t", "lt"}, false},
	"xades":      {"XAdES", []string{"sign", "cosign", "countersign"}, []string{"baseline", "t"}, true},
	"xmldsig":    {"XMLdSig", []string{"sign", "cosign"}, []string{"baseline"}, true},
	"odf":        {"ODF", []string{"sign", "cosign"}, []string{"baseline"}, true},
	"ooxml":      {"OOXML", []string{"sign", "cosign"}, []string{"baseline"}, true},
	"facturae":   {"FacturaE", []string{"sign"}, []string{"baseline"}, true},
	"asic-xades": {"ASiC-XAdES", []string{"sign"}, []string{"baseline"}, true},
	"verifactu":  {"VeriFactu", []string{"sign"}, []string{"baseline"}, true},
}

// mobileFormatOrder fija el orden estable del contrato y de las pruebas.
var mobileFormatOrder = []string{"cades", "pades", "xades", "xmldsig", "odf", "ooxml", "facturae", "asic-xades", "verifactu"}

var (
	odfExtensions   = []string{".odt", ".ods", ".odp", ".odg", ".odf"}
	ooxmlExtensions = []string{".docx", ".docm", ".dotx", ".dotm", ".xlsx", ".xlsm", ".xltx", ".xltm", ".pptx", ".pptm", ".ppsx", ".ppsm"}
)

// detectSignatureFormat aplica las reglas de «automático» del escritorio
// (extensión del fichero) y las completa con el tipo MIME que entrega SAF y
// con el contenido de las facturas, porque en Android el nombre no siempre
// conserva la extensión. Veri*Factu nunca se elige solo: como en escritorio,
// se pide de forma explícita.
func detectSignatureFormat(name, mimeType string, content []byte) string {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	extension := path.Ext(lowerName)
	mediaType, _, _ := mime.ParseMediaType(mimeType)
	mediaType = strings.ToLower(mediaType)
	switch {
	case mediaType == "application/pdf" || extension == ".pdf":
		return "pades"
	case slices.Contains(ooxmlExtensions, extension) || strings.HasPrefix(mediaType, "application/vnd.openxmlformats-officedocument."):
		return "ooxml"
	case slices.Contains(odfExtensions, extension) || strings.HasPrefix(mediaType, "application/vnd.oasis.opendocument."):
		return "odf"
	case extension == ".asics" || mediaType == "application/vnd.etsi.asic-s+zip":
		return "asic-xades"
	case extension == ".dsig" || extension == ".xmlsig":
		return "xmldsig"
	case isXMLDocument(extension, mediaType):
		if strings.HasSuffix(lowerName, ".facturae.xml") || strings.Contains(lowerName, "facturae") || isFacturaE(content) {
			return "facturae"
		}
		return "xades"
	default:
		return "cades"
	}
}

func isXMLDocument(extension, mediaType string) bool {
	return extension == ".xml" || extension == ".xsig" || strings.Contains(mediaType, "xml")
}

func isFacturaE(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	// Basta el primer elemento: se examina un prefijo para no copiar el XML.
	head := content[:min(len(content), 64<<10)]
	format, err := commonsigner.DetectarFormatoFactura(head)
	return err == nil && format == commonsigner.FormatoFacturaE
}

func validateFormatAction(format, action string) error {
	spec, ok := mobileFormats[format]
	if !ok {
		return newFacadeError("format no esta habilitado en mobile")
	}
	normalized := strings.ToLower(strings.TrimSpace(action))
	if normalized == "" {
		normalized = "sign"
	}
	if !slices.Contains(spec.actions, normalized) {
		if format == "pades" && normalized == "countersign" {
			return newFacadeError("PAdES no admite contrafirma; use CAdES o XAdES")
		}
		return newFacadeError("accion no soportada para este formato")
	}
	return nil
}

func formatRequiresRSA(format string) bool {
	return mobileFormats[format].rsaOnly
}

func contractProfilesByFormat() map[string][]string {
	out := make(map[string][]string, len(mobileFormats))
	for _, key := range mobileFormatOrder {
		out[mobileFormats[key].contract] = slices.Clone(mobileFormats[key].profiles)
	}
	return out
}

func contractActionsByFormat() map[string][]string {
	out := make(map[string][]string, len(mobileFormats))
	for _, key := range mobileFormatOrder {
		out[mobileFormats[key].contract] = slices.Clone(mobileFormats[key].actions)
	}
	return out
}

func contractKeyTypesByFormat() map[string][]string {
	out := make(map[string][]string, len(mobileFormats))
	for _, key := range mobileFormatOrder {
		if mobileFormats[key].rsaOnly {
			out[mobileFormats[key].contract] = []string{"RSA"}
		} else {
			out[mobileFormats[key].contract] = []string{"RSA", "ECDSA"}
		}
	}
	return out
}

// checkFormatPrerequisites adelanta, con mensajes cerrados, los rechazos que
// el motor daría tras pedir la aprobación: clave no RSA y registros que no
// son Veri*Factu.
func (f *Facade) checkFormatPrerequisites(format, certificateID string, content []byte) error {
	if formatRequiresRSA(format) && !f.sessionIdentityIsRSA(certificateID) {
		return newFacadeError(mobileFormatRequiresRSAMessage)
	}
	if format == "verifactu" && !commonsigner.EsRegistroVeriFactu(content) {
		return newFacadeError(verifactuKeyPrefix + "root")
	}
	return nil
}

// sessionIdentityIsRSA responde por la identidad indicada de la sesión. Sin
// esa identidad devuelve true para que el caso de uso dé su error habitual.
func (f *Facade) sessionIdentityIsRSA(certificateID string) bool {
	if f == nil || f.session == nil {
		return true
	}
	return f.session.isRSA(certificateID)
}

const verifactuKeyPrefix = "verifactu."

// signingError conserva solo las claves de localización cerradas del motor
// Veri*Factu; cualquier otro detalle queda en el mensaje genérico.
func signingError(err error) error {
	var localized interface{ LocalizationKey() string }
	if errors.As(err, &localized) {
		if key := localized.LocalizationKey(); strings.HasPrefix(key, verifactuKeyPrefix) && isClosedVeriFactuKey(key) {
			return newFacadeError(key)
		}
	}
	return safeOperationError("firma")
}

var closedVeriFactuKeys = []string{
	"limit", "xml", "root", "structure", "value", "profile", "signed", "key", "hash", "chain",
	"duplicate", "previous_missing", "signature", "certificate", "trust", "unsigned", "empty",
}

func isClosedVeriFactuKey(key string) bool {
	return slices.Contains(closedVeriFactuKeys, strings.TrimPrefix(key, verifactuKeyPrefix))
}
