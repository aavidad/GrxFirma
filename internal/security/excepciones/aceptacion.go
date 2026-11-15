// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// SPDX-License-Identifier: EUPL-1.2
package excepciones

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"grxfirma/internal/adapters/outbound/common/auditlog"
	"grxfirma/internal/adapters/outbound/common/securefile"
	"grxfirma/internal/ports"
)

const AcceptanceVersion = "2026-11-15.1"
const AcceptanceField = "excepciones_inseguras_aceptaciones"
const acceptanceOrigin = "aceptado desde el aviso"

var ErrAcceptanceRequired = errors.New("excepciones.aceptacion_requerida")
var ErrAcceptanceVersion = errors.New("excepciones.version_aceptacion_invalida")
var ErrNonLocal = errors.New("excepciones.canal_no_local")

type localAuthorityKey struct{}
type localAuthority uint8

const (
	desktopAuthority localAuthority = iota + 1
	androidAuthority
	protocolDialogAuthority
	cliAuthority
)

// Estas capacidades se crean exclusivamente en los adaptadores locales de
// confianza. Ningún nombre de canal, parámetro web o JSON se convierte en ellas.
func ForLocalDesktop(ctx context.Context) context.Context {
	return context.WithValue(ctx, localAuthorityKey{}, desktopAuthority)
}
func ForLocalAndroid(ctx context.Context) context.Context {
	return context.WithValue(ctx, localAuthorityKey{}, androidAuthority)
}
func ForLocalProtocolDialog(ctx context.Context) context.Context {
	return context.WithValue(ctx, localAuthorityKey{}, protocolDialogAuthority)
}
func ForLocalCLI(ctx context.Context) context.Context {
	return context.WithValue(ctx, localAuthorityKey{}, cliAuthority)
}
func IsLocal(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	a, _ := ctx.Value(localAuthorityKey{}).(localAuthority)
	return a >= desktopAuthority && a <= cliAuthority
}
func localChannel(ctx context.Context) string {
	a, _ := ctx.Value(localAuthorityKey{}).(localAuthority)
	switch a {
	case desktopAuthority:
		return "desktop"
	case androidAuthority:
		return "android"
	case protocolDialogAuthority:
		return "protocol_dialog"
	case cliAuthority:
		return "cli"
	}
	return ""
}

type ActivationRequest struct {
	ID                string `json:"id"`
	Accepted          bool   `json:"accepted"`
	AcceptanceVersion string `json:"acceptanceVersion"`
}
type Acceptance struct {
	ID          string    `json:"id"`
	AcceptedAt  time.Time `json:"accepted_at"`
	TextVersion string    `json:"text_version"`
	Origin      string    `json:"origin"`
	Channel     string    `json:"channel"`
}

// ActivateLocal no recibe ni confía en un canal declarado por el solicitante.
// La versión identifica exactamente el texto ofrecido por esta compilación.
func ActivateLocal(ctx context.Context, dir string, r ActivationRequest) (Acceptance, error) {
	if !IsLocal(ctx) {
		return Acceptance{}, ErrNonLocal
	}
	if err := ctx.Err(); err != nil {
		return Acceptance{}, err
	}
	if !r.Accepted {
		return Acceptance{}, ErrAcceptanceRequired
	}
	if r.AcceptanceVersion != AcceptanceVersion {
		return Acceptance{}, ErrAcceptanceVersion
	}
	e, ok := Find(r.ID)
	if !ok {
		return Acceptance{}, ErrAcceptanceRequired
	}
	writeMu.Lock()
	defer writeMu.Unlock()
	effective := state(e, dir, nil)
	block := blockForState(e, effective)
	if !block.Activable {
		return Acceptance{}, block
	}
	receipt := Acceptance{ID: r.ID, AcceptedAt: time.Now().UTC(), TextVersion: AcceptanceVersion, Origin: acceptanceOrigin, Channel: localChannel(ctx)}
	if err := writeUserSettings(ctx, dir, r.ID, true, &receipt); err != nil {
		return Acceptance{}, err
	}
	return receipt, nil
}

// La auditoría registra la aceptación antes de publicar el ajuste. Si falla,
// no se activa. Un fallo posterior de persistencia puede dejar una aceptación
// registrada sin ajuste activo; el registro no afirma que se haya completado.
func writeUserSettings(ctx context.Context, dir, id string, enabled bool, receipt *Acceptance) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.json")
	raw, err := securefile.ReadFileLimit(path, 4<<20)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	doc := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &doc); err != nil {
			return err
		}
		if doc == nil {
			return ErrAcceptanceRequired
		}
	}
	values := map[string]bool{}
	if data, ok := doc[UserField]; ok {
		if err = json.Unmarshal(data, &values); err != nil {
			return err
		}
		if values == nil {
			values = map[string]bool{}
		}
	}
	values[id] = enabled
	doc[UserField], err = json.Marshal(values)
	if err != nil {
		return err
	}
	if receipt != nil {
		receipts := map[string]Acceptance{}
		if data, ok := doc[AcceptanceField]; ok {
			if err = json.Unmarshal(data, &receipts); err != nil {
				return err
			}
			if receipts == nil {
				receipts = map[string]Acceptance{}
			}
		}
		receipts[id] = *receipt
		doc[AcceptanceField], err = json.Marshal(receipts)
		if err != nil {
			return err
		}
	}
	raw, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > 4<<20 {
		return ErrAcceptanceRequired
	}
	audit, err := json.Marshal(struct {
		Type       string      `json:"type"`
		ID         string      `json:"id"`
		Enabled    bool        `json:"enabled"`
		Timestamp  time.Time   `json:"timestamp"`
		Acceptance *Acceptance `json:"acceptance,omitempty"`
	}{"unsafe_exception.preference", id, enabled, time.Now().UTC(), receipt})
	if err != nil {
		return err
	}
	logger := auditlog.New(filepath.Join(dir, "unsafe-exceptions-audit.jsonl"), 1<<20)
	if err = logger.Log(ctx, ports.Evidence{Payload: audit}); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return securefile.WriteFileAtomic(path, append(raw, '\n'), 0600)
}

func readAcceptance(dir, id string) *Acceptance {
	raw, err := securefile.ReadFileLimit(filepath.Join(dir, "settings.json"), 4<<20)
	if err != nil {
		return nil
	}
	var doc struct {
		Receipts map[string]Acceptance `json:"excepciones_inseguras_aceptaciones"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	r, ok := doc.Receipts[id]
	if !ok {
		return nil
	}
	return &r
}

// ReservedSettings impide que un guardado genérico eluda la aceptación.
func ReservedSettings(key string) bool { return key == UserField || key == AcceptanceField }
