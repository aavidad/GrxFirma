// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"grxfirma/internal/adapters/inbound/common/cli"
	"grxfirma/internal/adapters/outbound/common/eni"
	"grxfirma/internal/adapters/outbound/common/securefile"
	commonsigner "grxfirma/internal/adapters/outbound/common/signer"
	"grxfirma/internal/ports"
)

const maxFacturaIPCTamano = 10 * 1024 * 1024

type invoiceValidationParams struct {
	InputPath string `json:"inputPath"`
}

type invoiceIssueIPC struct {
	Level   string `json:"level"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type invoiceValidationResult struct {
	Format   string            `json:"format"`
	Valid    bool              `json:"valid"`
	Errors   int               `json:"errors"`
	Warnings int               `json:"warnings"`
	Issues   []invoiceIssueIPC `json:"issues"`
	Report   string            `json:"report"`
}

func (m *Manejador) handleValidateInvoice(ctx context.Context, raw json.RawMessage) respuesta {
	action := "validate_invoice"
	var p invoiceValidationParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return respuesta{Action: action, Error: "parámetros no válidos"}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if err := validarRutaLectura(p.InputPath); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	data, err := securefile.ReadFileLimit(p.InputPath, maxFacturaIPCTamano)
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	format, incidents, err := commonsigner.RevisarFactura(data)
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	result := invoiceValidationResult{Format: string(format), Valid: true, Issues: make([]invoiceIssueIPC, 0, len(incidents))}
	var report strings.Builder
	fmt.Fprintf(&report, "%s: %s\n", m.t("Formato"), format)
	for _, incident := range incidents {
		result.Issues = append(result.Issues, invoiceIssueIPC{Level: string(incident.Nivel), Field: incident.Campo, Message: incident.Mensaje})
		fmt.Fprintln(&report, incident.String())
		if incident.Nivel == commonsigner.IncidenciaError {
			result.Errors++
			result.Valid = false
		} else {
			result.Warnings++
		}
	}
	if len(incidents) == 0 {
		fmt.Fprintln(&report, m.t("La factura no presenta incidencias."))
	}
	result.Report = report.String()
	return respuesta{OK: true, Action: action, Data: result}
}

type eniDocumentParams struct {
	InputPath    string            `json:"inputPath"`
	OriginalPath string            `json:"originalPath"`
	OutputPath   string            `json:"outputPath"`
	Options      map[string]string `json:"options"`
}

func (m *Manejador) handleGenerateENIDocument(ctx context.Context, raw json.RawMessage) respuesta {
	action := "generate_eni_document"
	var p eniDocumentParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return respuesta{Action: action, Error: "parámetros no válidos"}
	}
	if err := validarRutaLectura(p.InputPath); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if err := validarRutaEscritura(p.OutputPath); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if filepath.Clean(p.InputPath) == filepath.Clean(p.OutputPath) || (p.OriginalPath != "" && filepath.Clean(p.OriginalPath) == filepath.Clean(p.OutputPath)) {
		return respuesta{Action: action, Error: "la salida debe ser distinta de los ficheros de entrada"}
	}
	signature, err := leerFicheroSeguroIPC(p.InputPath)
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	var original []byte
	if p.OriginalPath != "" {
		if err := validarRutaLectura(p.OriginalPath); err != nil {
			return respuesta{Action: action, Error: err.Error()}
		}
		original, err = leerFicheroSeguroIPC(p.OriginalPath)
		if err != nil {
			return respuesta{Action: action, Error: err.Error()}
		}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	doc, err := cli.DocumentoENIDesdeFirma(signature, original, p.Options)
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	output, err := eni.Generar(doc, time.Now())
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if err := securefile.WriteFileAtomic(p.OutputPath, output, 0o600); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	return respuesta{OK: true, Action: action, Data: map[string]any{"outputPath": p.OutputPath}}
}

type eniFileParams struct {
	DirectoryPath string            `json:"directoryPath"`
	OutputPath    string            `json:"outputPath"`
	CertificateID string            `json:"certificateId"`
	Options       map[string]string `json:"options"`
}

func (m *Manejador) handleGenerateENIFile(ctx context.Context, raw json.RawMessage) respuesta {
	action := "generate_eni_file"
	var p eniFileParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return respuesta{Action: action, Error: "parámetros no válidos"}
	}
	if err := validarDirectorioLectura(p.DirectoryPath); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if err := validarRutaEscritura(p.OutputPath); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	inputDir, err := filepath.EvalSymlinks(p.DirectoryPath)
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	outputDir, err := filepath.EvalSymlinks(filepath.Dir(p.OutputPath))
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if filepath.Clean(inputDir) == filepath.Clean(outputDir) {
		return respuesta{Action: action, Error: "guarde el expediente fuera de la carpeta de documentos"}
	}
	entries, err := os.ReadDir(p.DirectoryPath)
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if len(entries) == 0 || len(entries) > maxIPCBatchDocuments {
		return respuesta{Action: action, Error: "la carpeta debe contener entre 1 y 128 documentos ENI"}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	docs := make([]eni.DocumentoExpediente, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return respuesta{Action: action, Error: err.Error()}
		}
		if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(entry.Name()), ".xml") {
			return respuesta{Action: action, Error: "la carpeta solo puede contener documentos ENI XML regulares"}
		}
		path := filepath.Join(p.DirectoryPath, entry.Name())
		if filepath.Clean(path) == filepath.Clean(p.OutputPath) {
			return respuesta{Action: action, Error: "la salida debe estar fuera de la carpeta de entrada"}
		}
		if err := validarRutaLectura(path); err != nil {
			return respuesta{Action: action, Error: err.Error()}
		}
		data, err := leerFicheroSeguroIPC(path)
		if err != nil {
			return respuesta{Action: action, Error: err.Error()}
		}
		total += int64(len(data))
		if total > maxIPCBatchTotalBytes {
			return respuesta{Action: action, Error: "la carpeta supera el tamaño máximo permitido"}
		}
		docs = append(docs, eni.DocumentoExpediente{XML: data})
	}
	opt := func(k string) string { return strings.TrimSpace(p.Options["exp."+k]) }
	meta := eni.MetadatosExpediente{Identificador: opt("identificador"), Clasificacion: opt("clasificacion"), Estado: strings.ToUpper(opt("estado"))}
	if meta.Estado == "" {
		meta.Estado = "E01"
	}
	for _, organ := range strings.Split(opt("organo"), ",") {
		if organ = strings.ToUpper(strings.TrimSpace(organ)); organ != "" {
			meta.Organos = append(meta.Organos, organ)
		}
	}
	meta.Interesados = strings.Split(opt("interesado"), ",")
	if date := opt("fechaApertura"); date != "" {
		meta.FechaApertura, err = time.Parse(time.RFC3339, date)
		if err != nil {
			return respuesta{Action: action, Error: "la fecha de apertura debe tener formato ISO 8601"}
		}
	}
	if err := meta.Validar(); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if m.Claves == nil {
		return respuesta{Action: action, Error: "no hay proveedor de claves disponible"}
	}
	ref, ok, err := m.resolverCertRefPorID(ctx, strings.TrimSpace(p.CertificateID))
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if !ok {
		return respuesta{Action: action, Error: "seleccione un certificado válido"}
	}
	key, err := m.Claves.KeyFor(ctx, ref)
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	defer ports.CloseSigningKey(key)
	sign := func(node []byte, id string) ([]byte, error) {
		return commonsigner.FirmarNodoXAdES(node, id, key, p.Options)
	}
	output, err := eni.GenerarExpediente(meta, docs, sign, commonsigner.CanonicalizarExclusivo, time.Now())
	if err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	if len(output) == 0 {
		return respuesta{Action: action, Error: errors.New("el expediente está vacío").Error()}
	}
	if err := securefile.WriteFileAtomic(p.OutputPath, output, 0o600); err != nil {
		return respuesta{Action: action, Error: err.Error()}
	}
	return respuesta{OK: true, Action: action, Data: map[string]any{"outputPath": p.OutputPath, "documents": len(docs)}}
}
