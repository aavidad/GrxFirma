// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"grxfirma/internal/ports"
)

// NotifyUserUseCase emite una notificacion por el canal disponible.
type NotifyUserUseCase struct {
	desktop ports.DesktopNotification
	mobile  ports.MobilePushNotification
	auditor *AuditUseCase
	eventos ports.EventPublisher
}

// NuevoNotifyUserUseCase construye el caso de uso NotifyUser.
func NuevoNotifyUserUseCase(
	desktop ports.DesktopNotification,
	mobile ports.MobilePushNotification,
	auditor *AuditUseCase,
	eventos ports.EventPublisher,
) *NotifyUserUseCase {
	return &NotifyUserUseCase{
		desktop: desktop,
		mobile:  mobile,
		auditor: auditor,
		eventos: eventos,
	}
}

// Ejecutar emite la notificacion por desktop o mobile, con degradacion controlada.
func (uc *NotifyUserUseCase) Ejecutar(ctx context.Context, cmd NotifyUserCommand) (NotifyUserResult, error) {
	if err := ctx.Err(); err != nil {
		return NotifyUserResult{}, err
	}
	title := strings.TrimSpace(cmd.Title)
	body := strings.TrimSpace(cmd.Body)
	if title == "" {
		return NotifyUserResult{}, errors.New("el titulo de la notificacion no puede estar vacio")
	}
	if body == "" {
		return NotifyUserResult{}, errors.New("el cuerpo de la notificacion no puede estar vacio")
	}
	if uc == nil || (uc.desktop == nil && uc.mobile == nil) {
		return NotifyUserResult{}, errors.New("no hay canales de notificacion configurados")
	}

	_ = uc.publicarEvento(ctx, "notificacion_usuario_iniciada", title)

	if uc.desktop != nil {
		if err := uc.desktop.Notify(ctx, title, body); err == nil {
			_ = uc.publicarEvento(ctx, "notificacion_usuario_completada", "desktop")
			uc.auditar(ctx, cmd, true, nil)
			return NotifyUserResult{Channel: "desktop"}, nil
		}
	}

	if uc.mobile != nil {
		if err := uc.mobile.Push(ctx, title, body); err == nil {
			_ = uc.publicarEvento(ctx, "notificacion_usuario_completada", "mobile")
			uc.auditar(ctx, cmd, true, nil)
			return NotifyUserResult{Channel: "mobile"}, nil
		}
	}

	err := fmt.Errorf("no se pudo emitir la notificacion por ningun canal")
	uc.auditar(ctx, cmd, false, err)
	return NotifyUserResult{}, err
}

// Execute expone el caso de uso con el nombre neutro esperado por los adaptadores.
func (uc *NotifyUserUseCase) Execute(ctx context.Context, cmd NotifyUserCommand) (NotifyUserResult, error) {
	return uc.Ejecutar(ctx, cmd)
}

func (uc *NotifyUserUseCase) auditar(ctx context.Context, cmd NotifyUserCommand, success bool, err error) {
	if uc == nil || uc.auditor == nil {
		return
	}
	summary := ""
	if err != nil {
		summary = sanitizarError(err)
	}
	_ = uc.auditor.Registrar(ctx, AuditCommand{
		OperationType: "notificacion_usuario",
		DocumentName:  cmd.Title,
		Success:       success,
		ErrorSummary:  summary,
	})
}

func (uc *NotifyUserUseCase) publicarEvento(ctx context.Context, tipo, detalle string) error {
	if uc == nil || uc.eventos == nil {
		return nil
	}
	return uc.eventos.Publish(ctx, ports.Event{
		Type:    tipo,
		Payload: []byte(detalle),
	})
}
