// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"

	"grxfirma/internal/ports"
)

// ResolvePlatformProfileUseCase resuelve las capacidades observables de la plataforma actual.
type ResolvePlatformProfileUseCase struct {
	proveedor ports.CapabilityProfileProvider
	auditor   *AuditUseCase
	eventos   ports.EventPublisher
}

// NuevoResolvePlatformProfileUseCase construye el caso de uso ResolvePlatformProfile.
func NuevoResolvePlatformProfileUseCase(
	proveedor ports.CapabilityProfileProvider,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *ResolvePlatformProfileUseCase {
	return &ResolvePlatformProfileUseCase{
		proveedor: proveedor,
		auditor:   auditor,
		eventos:   eventos,
	}
}

// Ejecutar obtiene el perfil de capacidades observable de la plataforma actual.
func (uc *ResolvePlatformProfileUseCase) Ejecutar(ctx context.Context, _ ResolvePlatformProfileCommand) (ResolvePlatformProfileResult, error) {
	if err := ctx.Err(); err != nil {
		return ResolvePlatformProfileResult{}, err
	}
	if uc == nil || uc.proveedor == nil {
		return ResolvePlatformProfileResult{}, errors.New("proveedor de capacidades de plataforma no configurado")
	}

	_ = uc.publicarEvento(ctx, "perfil_plataforma_resolucion_iniciada", "")

	profile, err := uc.proveedor.Profile(ctx)
	if err != nil {
		uc.auditar(ctx, profile, false, err)
		return ResolvePlatformProfileResult{}, fmt.Errorf("no se pudo resolver el perfil de plataforma: %w", err)
	}

	_ = uc.publicarEvento(ctx, "perfil_plataforma_resolucion_completada", resumirPerfil(profile))
	uc.auditar(ctx, profile, true, nil)
	return ResolvePlatformProfileResult{Profile: profile}, nil
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *ResolvePlatformProfileUseCase) Execute(ctx context.Context, cmd ResolvePlatformProfileCommand) (ResolvePlatformProfileResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *ResolvePlatformProfileUseCase) auditar(ctx context.Context, profile ports.CapabilityProfile, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	summary := ""
	if err != nil {
		summary = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "perfil_plataforma",
		DocumentName:  resumirPerfil(profile),
		Success:       success,
		ErrorSummary:  summary,
	})
}

func (uc *ResolvePlatformProfileUseCase) publicarEvento(ctx context.Context, tipo, detalle string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    tipo,
		Payload: []byte(detalle),
	})
}

func resumirPerfil(profile ports.CapabilityProfile) string {
	valores := []bool{
		profile.HasSecureStorage,
		profile.HasBiometricPrompt,
		profile.HasSmartCardAccess,
		profile.HasDocumentPicker,
		profile.HasLocalServer,
		profile.HasNativeMessaging,
		profile.HasLegacyAfirmaURI,
		profile.HasMobileDeepLink,
		profile.HasLocalTLSTrust,
		profile.HasPDFPreview,
		profile.HasTemporaryStorage,
	}
	total := 0
	for _, enabled := range valores {
		if enabled {
			total++
		}
	}
	return fmt.Sprintf("capacidades=%d", total)
}
