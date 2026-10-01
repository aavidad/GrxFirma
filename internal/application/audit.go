// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"grxfirma/internal/ports"
)

// AuditUseCase es el unico responsable de la politica de sanitizacion de evidencias.
// EvidenceLogger solo recibe registros ya saneados por este caso de uso.
// Ningun otro caso de uso escribe directamente en EvidenceLogger.
type AuditUseCase struct {
	reloj  ports.Clock
	logger ports.EvidenceLogger
}

// NuevoAuditUseCase construye el caso de uso de auditoria.
func NuevoAuditUseCase(reloj ports.Clock, logger ports.EvidenceLogger) *AuditUseCase {
	return &AuditUseCase{reloj: reloj, logger: logger}
}

// Registrar sanitiza el comando de auditoria y lo persiste via EvidenceLogger.
func (uc *AuditUseCase) Registrar(ctx context.Context, cmd AuditCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if uc == nil || uc.logger == nil {
		return nil
	}

	ahora := time.Now().UTC()
	if uc.reloj != nil {
		ahora = uc.reloj.Now().UTC()
	}

	registro := AuditRecord{
		Timestamp:     ahora,
		OperationType: cmd.OperationType,
		Origin:        sanitizarOrigen(cmd.Origin),
		DocumentName:  sanitizarNombre(cmd.DocumentName),
		DocumentHash:  calcularHashDocumento(cmd.DocumentData, cmd.DocumentHash),
		Format:        sanitizarFormato(cmd.Format),
		Success:       cmd.Success,
		Result:        resultadoAudit(cmd.Success),
		ErrorSummary:  sanitizarResumenError(cmd.ErrorSummary),
	}

	if cmd.CertificateFingerprint != "" {
		registro.CertificateFingerprint = sanitizarHuella(cmd.CertificateFingerprint)
	} else if cmd.CertificateID != "" {
		registro.CertificateFingerprint = fmt.Sprintf("id:%s", cmd.CertificateID)
	}

	evidencia := ports.Evidence{
		Type:      "operacion." + registro.OperationType,
		Timestamp: registro.Timestamp,
		Payload:   serializarRegistro(registro),
	}

	return uc.logger.Log(ctx, evidencia)
}

// sanitizarOrigen limita la longitud del origen y elimina caracteres no imprimibles.
func sanitizarOrigen(origen string) string {
	return truncarYLimpiar(origen, 200)
}

// sanitizarNombre limita el nombre del documento para el log.
func sanitizarNombre(nombre string) string {
	return truncarYLimpiar(nombre, 255)
}

// sanitizarResumenError asegura que el resumen de error no supera un tamano razonable.
// No debe contener rutas de sistema, stacktraces ni datos de payload.
func sanitizarResumenError(resumen string) string {
	return truncarYLimpiar(resumen, 500)
}

func sanitizarFormato(formato string) string {
	return truncarYLimpiar(formato, 32)
}

func sanitizarHuella(huella string) string {
	return truncarYLimpiar(strings.ToLower(huella), 128)
}

func resultadoAudit(success bool) string {
	if success {
		return "ok"
	}
	return "error"
}

func calcularHashDocumento(data []byte, hash string) string {
	if hash != "" {
		return truncarYLimpiar(strings.ToLower(hash), 128)
	}
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func truncarYLimpiar(valor string, maxLen int) string {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(valor, ""))
	clean = strings.TrimSpace(clean)
	if len(clean) > maxLen {
		return clean[:maxLen]
	}
	return clean
}

// serializarRegistro convierte el registro en JSON para EvidenceLogger.
func serializarRegistro(r AuditRecord) []byte {
	data, err := json.Marshal(r)
	if err != nil {
		fallback := fmt.Sprintf(`{"operacion":"%s","resultado":"error_serializacion"}`, truncarYLimpiar(r.OperationType, 64))
		return []byte(fallback)
	}
	return data
}
