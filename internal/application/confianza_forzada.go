// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// rondasConfianzaForzable limita las repeticiones con las que se comprueba si
// aceptar los obstáculos bastaría: aceptar una CA puede dejar a la vista un
// certificado de componente o una clave corta que antes no se llegó a mirar.
const rondasConfianzaForzable = 3

// ResultadoConfianzaForzada resume, para la CLI y el escritorio, lo que la
// verificación hizo con la confianza que el usuario puede forzar.
type ResultadoConfianzaForzada struct {
	// Aplicada: obstáculos que se dieron por buenos porque el usuario los
	// tenía aceptados.
	Aplicada []domain.ObstaculoConfianza
	// Forzable: obstáculos que el usuario podría aceptar en su equipo y con
	// los que la firma saldría válida. Vacío si aceptarlos no bastaría.
	Forzable []domain.ObstaculoConfianza
	// Prohibida: la política de máquina no permite usar lo aceptado.
	Prohibida bool
}

// cargarConfianzaForzada lee lo aceptado. Si no se puede leer, no se aplica
// nada y se trata como prohibido (falla cerrado).
func (uc *VerifySignatureUseCase) cargarConfianzaForzada(ctx context.Context) domain.ConfianzaForzada {
	confianza, err := uc.confianzaForzada.CargarConfianzaForzada(ctx)
	if err != nil {
		slog.Warn("no se pudo leer la confianza aceptada por el usuario; no se aplica", "error", err)
		return domain.ConfianzaForzada{Prohibida: true}
	}
	return confianza
}

// completarConfianzaForzada asegura el aviso de lo aplicado y, si la firma no
// es válida solo por obstáculos forzables, lo dice para que el usuario pueda
// aceptarlos. La comprobación repite la verificación suponiendo aceptados los
// obstáculos; las consultas de revocación se reutilizan.
func (uc *VerifySignatureUseCase) completarConfianzaForzada(ctx context.Context, cmd VerifyCommand, anchors domain.CertificateChain, registro *domain.RegistroConfianzaForzada, verification *domain.VerificationResult) ResultadoConfianzaForzada {
	if registro == nil {
		return ResultadoConfianzaForzada{}
	}
	confianza := registro.Confianza()
	out := ResultadoConfianzaForzada{Prohibida: confianza.Prohibida}
	if verification.Valid {
		out.Aplicada = registro.Aplicadas()
		for _, obstaculo := range out.Aplicada {
			*verification = verification.MarcarConfianzaForzada(obstaculo)
		}
		return out
	}
	// Lo aceptado no bastó: no se deja el aviso de «se da por válida».
	*verification = quitarMarcasConfianzaForzada(*verification)
	obstaculos := registro.Obstaculos()
	if len(obstaculos) == 0 {
		return out
	}
	if confianza.Prohibida {
		verification.Warnings = anadirAvisoUnico(verification.Warnings, domain.ClaveConfianzaForzadaProhibida)
		return out
	}
	hipotetica := confianza
	hipotetica.Aceptaciones = append([]domain.AceptacionConfianza(nil), confianza.Aceptaciones...)
	for ronda := 0; ronda < rondasConfianzaForzable; ronda++ {
		for _, obstaculo := range obstaculos {
			hipotetica.Aceptaciones = append(hipotetica.Aceptaciones, domain.AceptacionConfianza{Tipo: obstaculo.Tipo, Huella: obstaculo.Huella})
		}
		prueba := registro.Derivar(hipotetica)
		repetida, _, err := uc.verificar(domain.ConRegistroConfianzaForzada(ctx, prueba), cmd, anchors)
		if err != nil {
			return out
		}
		if repetida.Valid {
			out.Forzable = obstaculos
			verification.Warnings = anadirAvisoUnico(verification.Warnings, domain.ClavePistaConfianzaForzable)
			return out
		}
		nuevos := prueba.Obstaculos()
		if len(nuevos) == 0 {
			return out
		}
		obstaculos = append(obstaculos, nuevos...)
	}
	return out
}

func quitarMarcasConfianzaForzada(v domain.VerificationResult) domain.VerificationResult {
	aplicadas := map[string]bool{}
	for _, tipo := range domain.TiposConfianzaForzada() {
		aplicadas[tipo.ClaveAplicada()] = true
	}
	avisos := v.Warnings[:0:0]
	for _, aviso := range v.Warnings {
		if !aplicadas[aviso] {
			avisos = append(avisos, aviso)
		}
	}
	v.Warnings = avisos
	evidencias := v.Evidence[:0:0]
	for _, evidencia := range v.Evidence {
		if evidencia.Type != domain.EvidenciaConfianzaForzada {
			evidencias = append(evidencias, evidencia)
		}
	}
	v.Evidence = evidencias
	if aplicadas[v.Trust.Reason] {
		v.Trust.Reason = ""
	}
	return v
}

func anadirAvisoUnico(lista []string, valor string) []string {
	for _, existente := range lista {
		if existente == valor {
			return lista
		}
	}
	return append(lista, valor)
}

// codigosConfianzaForzada son los códigos de auditoría de una firma dada por
// válida gracias a lo aceptado: tipo y comienzo de la huella.
func codigosConfianzaForzada(valida bool, aplicadas []domain.ObstaculoConfianza) []string {
	if !valida {
		return nil
	}
	var out []string
	for _, obstaculo := range aplicadas {
		codigo := "confianza_forzada=" + string(obstaculo.Tipo)
		if len(obstaculo.Huella) >= 16 {
			codigo += ":" + obstaculo.Huella[:16]
		}
		out = append(out, codigo)
	}
	return out
}

// GestionConfianzaForzadaUseCase lista, acepta y quita lo que el usuario
// acepta en su equipo. Solo lo componen la CLI y el escritorio.
type GestionConfianzaForzadaUseCase struct {
	almacen ports.AlmacenConfianzaForzada
	auditor *AuditUseCase
	ahora   func() time.Time
}

// NuevoGestionConfianzaForzadaUseCase construye el caso de uso.
func NuevoGestionConfianzaForzadaUseCase(almacen ports.AlmacenConfianzaForzada, auditor *AuditUseCase) *GestionConfianzaForzadaUseCase {
	return &GestionConfianzaForzadaUseCase{almacen: almacen, auditor: auditor, ahora: time.Now}
}

// ErrConfianzaForzadaNoConfigurada indica que el canal no admite gestionar
// la confianza forzada.
var ErrConfianzaForzadaNoConfigurada = errors.New("verify.forced_trust.error.unavailable")

// Listar devuelve lo aceptado y si la política lo prohíbe.
func (uc *GestionConfianzaForzadaUseCase) Listar(ctx context.Context) (domain.ConfianzaForzada, error) {
	if uc == nil || uc.almacen == nil {
		return domain.ConfianzaForzada{}, ErrConfianzaForzadaNoConfigurada
	}
	return uc.almacen.CargarConfianzaForzada(ctx)
}

// Aceptar guarda la aceptación explícita del usuario tras ver el aviso.
func (uc *GestionConfianzaForzadaUseCase) Aceptar(ctx context.Context, obstaculo domain.ObstaculoConfianza) (domain.AceptacionConfianza, error) {
	if uc == nil || uc.almacen == nil {
		return domain.AceptacionConfianza{}, ErrConfianzaForzadaNoConfigurada
	}
	aceptacion, err := domain.AceptacionConfianza{
		Tipo:       obstaculo.Tipo,
		Huella:     obstaculo.Huella,
		Sujeto:     obstaculo.Sujeto,
		AceptadaEn: uc.ahora(),
	}.Normalizar()
	if err != nil {
		return domain.AceptacionConfianza{}, err
	}
	if err := uc.almacen.AceptarConfianzaForzada(ctx, aceptacion); err != nil {
		uc.auditar(ctx, "confianza_forzada_aceptar", aceptacion, false, err)
		return domain.AceptacionConfianza{}, err
	}
	uc.auditar(ctx, "confianza_forzada_aceptar", aceptacion, true, nil)
	return aceptacion, nil
}

// Quitar borra una aceptación. Devuelve false si no existía.
func (uc *GestionConfianzaForzadaUseCase) Quitar(ctx context.Context, tipo domain.TipoConfianzaForzada, huella string) (bool, error) {
	if uc == nil || uc.almacen == nil {
		return false, ErrConfianzaForzadaNoConfigurada
	}
	if !tipo.Valido() {
		return false, domain.ErrConfianzaTipoDesconocido
	}
	if tipo.PorCertificado() {
		huella = domain.NormalizarHuellaConfianza(huella)
		if huella == "" {
			return false, domain.ErrConfianzaHuellaInvalida
		}
	} else {
		huella = ""
	}
	quitada, err := uc.almacen.QuitarConfianzaForzada(ctx, tipo, huella)
	uc.auditar(ctx, "confianza_forzada_quitar", domain.AceptacionConfianza{Tipo: tipo, Huella: huella}, err == nil, err)
	return quitada, err
}

func (uc *GestionConfianzaForzadaUseCase) auditar(ctx context.Context, operacion string, aceptacion domain.AceptacionConfianza, ok bool, err error) {
	if uc.auditor == nil {
		return
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType:          operacion,
		CertificateFingerprint: aceptacion.Huella,
		Success:                ok,
		ErrorSummary:           sanitizarError(err),
		Compatibilidad:         []string{"confianza_forzada=" + string(aceptacion.Tipo)},
	})
}
