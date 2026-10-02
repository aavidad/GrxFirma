// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode"

	"grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/application"
	"grxfirma/internal/domain"
)

type cambiosV2Response struct {
	Estado          string   `json:"estado"`
	Detalle         []string `json:"detalle"`
	BytesNoFirmados int      `json:"bytesNoFirmados,omitempty"`
}

type firmaV2Response struct {
	Orden                           int                     `json:"orden"`
	ByteRange                       [4]int                  `json:"byteRange"`
	RevisionHuellaSHA256            string                  `json:"revisionHuellaSHA256"`
	ContenidoFirmadoHuellaSHA256    string                  `json:"contenidoFirmadoHuellaSHA256"`
	RevisionLongitud                int                     `json:"revisionLongitud"`
	CubreDocumentoCompletoHastaAqui bool                    `json:"cubreDocumentoCompletoHastaAqui"`
	Integridad                      aspectoDictamenResponse `json:"integridad"`
	CertificadoHuellaSHA256         string                  `json:"certificadoHuellaSHA256,omitempty"`
	Serie                           string                  `json:"serie,omitempty"`
	Asunto                          string                  `json:"asunto,omitempty"`
	Emisor                          string                  `json:"emisor,omitempty"`
	Cadena                          aspectoDictamenResponse `json:"cadena"`
	Certificado                     aspectoDictamenResponse `json:"certificado"`
	Revocacion                      aspectoDictamenResponse `json:"revocacion"`
	SelloTiempo                     aspectoDictamenResponse `json:"selloTiempo"`
	TipoFirma                       string                  `json:"tipoFirma"`
	NivelDocMDP                     *int                    `json:"nivelDocMDP"`
	CambiosDesdeAnterior            cambiosV2Response       `json:"cambiosDesdeAnterior"`
}

type dictamenV2Response struct {
	Contrato             string                      `json:"contrato"`
	Estado               string                      `json:"estado"`
	Motivo               string                      `json:"motivo"`
	Formato              string                      `json:"formato"`
	ComprobadoEn         string                      `json:"comprobadoEn"`
	Integridad           aspectoDictamenResponse     `json:"integridad"`
	Cadena               aspectoDictamenResponse     `json:"cadena"`
	Certificado          aspectoDictamenResponse     `json:"certificado"`
	Revocacion           aspectoDictamenResponse     `json:"revocacion"`
	SelloTiempo          aspectoDictamenResponse     `json:"selloTiempo"`
	VinculoOriginal      aspectoDictamenResponse     `json:"vinculoOriginal"`
	HuellaFirmadoSHA256  string                      `json:"huellaFirmadoSHA256"`
	HuellaOriginalSHA256 string                      `json:"huellaOriginalSHA256,omitempty"`
	Extensiones          extensionesDictamenResponse `json:"extensiones"`
	Firmas               []firmaV2Response           `json:"firmas"`
	CambiosPosteriores   cambiosV2Response           `json:"cambiosPosteriores"`
}

type verifyV2Response struct {
	OK       bool               `json:"ok"`
	Valid    bool               `json:"valid"`
	Reason   string             `json:"reason"`
	Details  []string           `json:"details"`
	Signers  []string           `json:"signers"`
	Dictamen dictamenV2Response `json:"dictamen"`
}

func convertirCambiosV2(change signer.CambiosPDFV2) cambiosV2Response {
	details := append([]string{}, change.Detalle...)
	return cambiosV2Response{Estado: change.Estado, Detalle: details, BytesNoFirmados: change.BytesNoFirmados}
}

func (a *Adaptador) evaluarDictamenV2(ctx context.Context, pdfBytes, original []byte) verifyV2Response {
	now := time.Now().UTC()
	dict := dictamenV2Response{
		Contrato: contratoDictamenV2, Estado: "indeterminada", Motivo: "pdf_no_comprobado", Formato: "PAdES", ComprobadoEn: now.Format(time.RFC3339),
		Integridad:      aspectoDictamenResponse{Estado: domain.IntegridadParcial, Motivo: "pdf_no_comprobado"},
		Cadena:          aspectoDictamenResponse{Estado: domain.CadenaNoComprobada},
		Certificado:     aspectoDictamenResponse{Estado: domain.CertificadoNoComprobado},
		Revocacion:      aspectoDictamenResponse{Estado: domain.RevocacionNoComprobada},
		SelloTiempo:     aspectoDictamenResponse{Estado: domain.SelloNoComprobado},
		VinculoOriginal: aspectoDictamenResponse{Estado: domain.VinculoNoAportado},
		Extensiones:     extensionesDictamenResponse{RevocacionRemota: domain.ExtensionDesactivada, SelloTiempoRemoto: domain.ExtensionDesactivada},
		Firmas:          []firmaV2Response{}, CambiosPosteriores: cambiosV2Response{Estado: "no_comprobados", Detalle: []string{"pdf_no_comprobado"}},
	}
	dict.HuellaFirmadoSHA256 = digestV2(pdfBytes)
	if original != nil {
		dict.HuellaOriginalSHA256 = digestV2(original)
		dict.VinculoOriginal = aspectoDictamenResponse{Estado: domain.VinculoNoAcreditado, Motivo: "original_no_acreditado"}
	}
	result := verifyV2Response{OK: true, Valid: false, Reason: dict.Motivo, Details: []string{}, Signers: []string{}, Dictamen: dict}
	inspection, err := signer.InspeccionarPAdESV2(pdfBytes, a.MaxV2Firmas, a.MaxV2Revisiones, a.MaxV2PDFBytes)
	if err != nil {
		if strings.Contains(err.Error(), "fuera_de_limite") {
			result.Dictamen.Motivo = "limite_excedido"
		}
		result.Reason = result.Dictamen.Motivo
		return result
	}
	result.Dictamen.CambiosPosteriores = convertirCambiosV2(inspection.CambiosPosteriores)
	composed := domain.DictamenVerificacion{
		Integridad:      domain.AspectoDictamen{Estado: domain.IntegridadValida},
		VinculoOriginal: domain.AspectoDictamen{Estado: domain.VinculoNoAportado},
	}
	if original != nil {
		if len(inspection.Longitudes) > 0 && len(original) == inspection.Longitudes[0] && bytes.Equal(original, pdfBytes[:len(original)]) &&
			len(inspection.Firmas) > 0 && inspection.Firmas[0].CambiosDesdeAnterior.Estado == "permitidos" {
			composed.VinculoOriginal = domain.AspectoDictamen{Estado: domain.VinculoAcreditado, Fuente: "pades_revision"}
		} else {
			composed.VinculoOriginal = domain.AspectoDictamen{Estado: domain.VinculoNoAcreditado, Motivo: "original_no_es_revision_previa"}
		}
	}
	incomplete := false
	for i, sig := range inspection.Firmas {
		entry := firmaV2Response{
			Orden: i + 1, ByteRange: sig.ByteRange, RevisionHuellaSHA256: sig.RevisionHuellaSHA256,
			ContenidoFirmadoHuellaSHA256: sig.ContenidoHuellaSHA256, RevisionLongitud: sig.RevisionLongitud,
			CubreDocumentoCompletoHastaAqui: sig.CubreRevisionCompleta,
			Integridad:                      aspectoDictamenResponse{Estado: domain.IntegridadParcial, Motivo: "firma_no_comprobada"},
			Cadena:                          aspectoDictamenResponse{Estado: domain.CadenaNoComprobada}, Certificado: aspectoDictamenResponse{Estado: domain.CertificadoNoComprobado},
			Revocacion: aspectoDictamenResponse{Estado: domain.RevocacionNoComprobada}, SelloTiempo: aspectoDictamenResponse{Estado: domain.SelloNoComprobado},
			TipoFirma: sig.TipoFirma, NivelDocMDP: sig.NivelDocMDP, CambiosDesdeAnterior: convertirCambiosV2(sig.CambiosDesdeAnterior),
		}
		result.Dictamen.Firmas = append(result.Dictamen.Firmas, entry)
		if ctx.Err() != nil {
			incomplete = true
			continue
		}
		cmsDoc, err := domain.NewDocument("firma.csig", sig.CMSDER, "application/pkcs7-signature")
		if err != nil {
			incomplete = true
			continue
		}
		contentDoc, err := domain.NewDocument("contenido.bin", sig.ContenidoFirmado, "application/octet-stream")
		if err != nil {
			incomplete = true
			continue
		}
		verification, err := a.Verificar.Execute(ctx, application.VerifyCommand{SignedDocument: cmsDoc, OriginalDocument: &contentDoc})
		if err != nil || verification.Dictamen == nil || len(verification.Dictamen.Firmantes) != 1 {
			incomplete = true
			continue
		}
		v1 := verification.Dictamen
		f := v1.Firmantes[0]
		composed.Firmantes = append(composed.Firmantes, f)
		if v1.Integridad.Estado == domain.IntegridadNoValida {
			composed.Integridad = v1.Integridad
		} else if v1.Integridad.Estado != domain.IntegridadValida && composed.Integridad.Estado == domain.IntegridadValida {
			composed.Integridad = v1.Integridad
		}
		entry.Integridad = aspectoDictamen(v1.Integridad)
		entry.CertificadoHuellaSHA256, entry.Serie, entry.Asunto, entry.Emisor = f.CertificadoHuellaSHA256, textoCertificadoV2(f.Serie, 128), textoCertificadoV2(f.Asunto, 1024), textoCertificadoV2(f.Emisor, 1024)
		entry.Cadena, entry.Certificado, entry.Revocacion, entry.SelloTiempo = aspectoDictamen(f.Cadena), aspectoDictamen(f.Certificado), aspectoDictamen(f.Revocacion), aspectoDictamen(f.SelloTiempo)
		result.Dictamen.Firmas[i] = entry
		result.Signers = append(result.Signers, f.CertificadoHuellaSHA256)
	}
	if incomplete {
		result.Dictamen.Motivo = "firmas_no_comprobadas"
		result.Dictamen.Integridad = aspectoDictamenResponse{Estado: domain.IntegridadParcial, Motivo: "firma_no_comprobada"}
		result.Dictamen.VinculoOriginal = aspectoDictamen(composed.VinculoOriginal)
		result.Reason = result.Dictamen.Motivo
		return result
	}
	composed = composed.Componer()
	result.Dictamen.Estado, result.Dictamen.Motivo = string(composed.Estado), string(composed.Motivo)
	result.Dictamen.Integridad = aspectoDictamen(composed.Integridad)
	result.Dictamen.Cadena = aspectoDictamen(composed.Cadena)
	result.Dictamen.Certificado = aspectoDictamen(composed.Certificado)
	result.Dictamen.Revocacion = aspectoDictamen(composed.Revocacion)
	result.Dictamen.SelloTiempo = aspectoDictamen(composed.SelloTiempo)
	result.Dictamen.VinculoOriginal = aspectoDictamen(composed.VinculoOriginal)
	result.Dictamen.Extensiones = extensionesDictamenResponse{RevocacionRemota: composed.Extensiones.RevocacionRemota, SelloTiempoRemoto: composed.Extensiones.SelloTiempoRemoto}
	if result.Dictamen.Estado == "valida" && composed.SelloTiempo.Estado == domain.SelloNoComprobado {
		result.Dictamen.Estado, result.Dictamen.Motivo = "indeterminada", string(domain.MotivoDictamenSelloTiempoNoAcreditado)
	}
	if result.Dictamen.Estado != "no_valida" {
		for _, sig := range result.Dictamen.Firmas {
			applyChangeV2(&result.Dictamen, sig.CambiosDesdeAnterior)
			if !sig.CubreDocumentoCompletoHastaAqui {
				result.Dictamen.Estado, result.Dictamen.Motivo = "no_valida", "byterange_no_cubre_revision_completa"
			}
		}
		applyChangeV2(&result.Dictamen, result.Dictamen.CambiosPosteriores)
	}
	result.Valid = result.Dictamen.Estado == "valida"
	result.Reason = result.Dictamen.Motivo
	return result
}

func applyChangeV2(dict *dictamenV2Response, change cambiosV2Response) {
	switch change.Estado {
	case "no_permitidos":
		dict.Estado, dict.Motivo = "no_valida", "cambios_no_permitidos"
	case "no_comprobados":
		if dict.Estado == "valida" {
			dict.Estado, dict.Motivo = "indeterminada", "cambios_no_comprobados"
		}
	}
}

func digestV2(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func textoCertificadoV2(value string, maxRunes int) string {
	var out strings.Builder
	count := 0
	for _, r := range value {
		if unicode.IsControl(r) {
			continue
		}
		if count >= maxRunes {
			break
		}
		out.WriteRune(r)
		count++
	}
	return out.String()
}
