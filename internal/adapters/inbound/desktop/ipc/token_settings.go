// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"unicode/utf8"

	"grxfirma/internal/ports"
)

type localTokenSettingsContextKey struct{}

func isLocalTokenSettingsAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "get_token_settings", "save_token_settings", "diagnose_token_settings":
		return true
	default:
		return false
	}
}

// Unlike legacy IPC operations, configuration of executable drivers requires
// the exact frontend PID. Recheck the kernel identity and revocation on every
// operation, including reads, rather than relying on same-user admission.
func (s *Servidor) authorizeLocalTokenSettings(ctx context.Context, conn net.Conn) (context.Context, bool) {
	state, _, _ := s.peerPID.snapshot()
	if state != vinculacionPublicada || ctx.Err() != nil {
		return ctx, false
	}
	pid, err := verificarPeer(conn)
	if err != nil || pid == 0 || s.peerPID.autorizar(ctx, pid) != nil {
		return ctx, false
	}
	return context.WithValue(ctx, localTokenSettingsContextKey{}, true), true
}

func tokenSettingsFrontendRequired(action string) respuesta {
	return ipcErrorResponse(strings.ToLower(strings.TrimSpace(action)),
		"token_settings_frontend_required", "admission",
		"Esta operación requiere la interfaz local autorizada de GrxFirma.", false)
}

// Administrative envelopes are exact too. Scope this to the three new
// actions so legacy signing clients retain their established parser behavior.
func validateTokenSettingsEnvelope(raw []byte) error {
	object, err := strictTokenSettingsObject(raw)
	if err != nil {
		return err
	}
	for name := range object {
		switch name {
		case "protocol", "requestId", "traceId", "action", "params":
		default:
			return ports.ErrTokenSettingsInvalid
		}
	}
	if _, exists := object["action"]; !exists {
		return ports.ErrTokenSettingsInvalid
	}
	if _, exists := object["params"]; !exists {
		return ports.ErrTokenSettingsInvalid
	}
	return nil
}

func (m *Manejador) handleLocalTokenSettings(ctx context.Context, action string, raw json.RawMessage) respuesta {
	if trusted, _ := ctx.Value(localTokenSettingsContextKey{}).(bool); !trusted {
		return tokenSettingsFrontendRequired(action)
	}
	var update ports.TokenSettingsUpdate
	if action == "save_token_settings" {
		var err error
		update, err = decodeTokenSettingsUpdate(raw)
		if err != nil {
			return tokenSettingsError(action, err)
		}
	} else if object, err := strictTokenSettingsObject(raw); err != nil || len(object) != 0 {
		return tokenSettingsError(action, ports.ErrTokenSettingsInvalid)
	}
	if ctx.Err() != nil {
		return tokenSettingsError(action, ctx.Err())
	}
	if m.TokenSettings == nil {
		if action == "save_token_settings" {
			return tokenSettingsError(action, ports.ErrTokenSettingsUnavailable)
		}
		return respuesta{OK: true, Action: action, Data: ports.TokenSettingsSnapshot{
			State: "unavailable", Modules: []ports.TokenModuleSetting{}, Checks: []ports.TokenSettingCheck{},
		}}
	}
	var result ports.TokenSettingsSnapshot
	var err error
	switch action {
	case "get_token_settings":
		result, err = m.TokenSettings.Load(ctx)
	case "save_token_settings":
		result, err = m.TokenSettings.Save(ctx, update)
	case "diagnose_token_settings":
		result, err = m.TokenSettings.Diagnose(ctx)
	}
	if err != nil {
		return tokenSettingsError(action, err)
	}
	return respuesta{OK: true, Action: action, Data: result}
}

// Backend errors may wrap filesystem/driver details. Send only fixed public
// codes/messages, never the underlying error or attacker-controlled JSON.
func tokenSettingsError(action string, err error) respuesta {
	code := "token_settings_failed"
	message := "No se pudo completar la operación de configuración de tarjetas."
	for _, entry := range []struct {
		err  error
		code string
	}{
		{ports.ErrTokenSettingsUnavailable, "token_settings_unavailable"},
		{ports.ErrTokenSettingsInvalid, "token_settings_invalid"},
		{ports.ErrTokenSettingsUnsafe, "token_settings_unsafe"},
		{ports.ErrTokenSettingsConflict, "token_settings_conflict"},
		{ports.ErrTokenSettingsConfirmation, "token_settings_confirmation"},
		{ports.ErrTokenSettingsWrite, "token_settings_write"},
	} {
		if errors.Is(err, entry.err) {
			code, message = entry.code, entry.err.Error()
			break
		}
	}
	return ipcErrorResponse(action, code, ipcPhaseOperation, message, false)
}

// encoding/json otherwise accepts duplicate and case-insensitive field names.
// Decode exact keys explicitly and disallow trailing values and null objects.
func strictTokenSettingsObject(raw []byte) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > 64*1024 || !utf8.Valid(raw) {
		return nil, ports.ErrTokenSettingsInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if first, err := dec.Token(); err != nil || first != json.Delim('{') {
		return nil, ports.ErrTokenSettingsInvalid
	}
	result := make(map[string]json.RawMessage)
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, ports.ErrTokenSettingsInvalid
		}
		name, ok := key.(string)
		if !ok {
			return nil, ports.ErrTokenSettingsInvalid
		}
		if _, exists := result[name]; exists {
			return nil, ports.ErrTokenSettingsInvalid
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, ports.ErrTokenSettingsInvalid
		}
		result[name] = value
	}
	if last, err := dec.Token(); err != nil || last != json.Delim('}') {
		return nil, ports.ErrTokenSettingsInvalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, ports.ErrTokenSettingsInvalid
	}
	return result, nil
}

func decodeTokenSettingsUpdate(raw []byte) (ports.TokenSettingsUpdate, error) {
	var update ports.TokenSettingsUpdate
	object, err := strictTokenSettingsObject(raw)
	if err != nil || len(object) != 5 {
		return update, ports.ErrTokenSettingsInvalid
	}
	for _, name := range []string{"enabled", "modules", "revision", "confirmed", "replaceInvalid"} {
		value, exists := object[name]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return update, ports.ErrTokenSettingsInvalid
		}
	}
	if json.Unmarshal(object["enabled"], &update.Enabled) != nil ||
		json.Unmarshal(object["confirmed"], &update.Confirmed) != nil ||
		json.Unmarshal(object["replaceInvalid"], &update.ReplaceInvalid) != nil ||
		json.Unmarshal(object["revision"], &update.Revision) != nil {
		return update, ports.ErrTokenSettingsInvalid
	}
	if update.Revision != "" {
		if len(update.Revision) != 64 {
			return update, ports.ErrTokenSettingsInvalid
		}
		if _, err := hex.DecodeString(update.Revision); err != nil {
			return update, ports.ErrTokenSettingsInvalid
		}
	}
	var modules []json.RawMessage
	if json.Unmarshal(object["modules"], &modules) != nil || len(modules) > 8 {
		return update, ports.ErrTokenSettingsInvalid
	}
	seen := make(map[string]bool)
	update.Modules = make([]ports.TokenModuleSetting, 0, len(modules))
	for _, rawModule := range modules {
		module, err := strictTokenSettingsObject(rawModule)
		var path string
		if err != nil || len(module) != 1 || json.Unmarshal(module["path"], &path) != nil ||
			len(path) == 0 || len(path) > 4096 || strings.ContainsRune(path, 0) || seen[path] {
			return update, ports.ErrTokenSettingsInvalid
		}
		seen[path] = true
		update.Modules = append(update.Modules, ports.TokenModuleSetting{Path: path})
	}
	if !update.Confirmed {
		return update, ports.ErrTokenSettingsConfirmation
	}
	return update, nil
}
