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
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/ports"
)

const tokenSettingsValidUpdate = `{"enabled":false,"modules":[],"revision":"","confirmed":true,"replaceInvalid":false}`

func TestTokenSettingsStrictEnvelope(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"get_token_settings", "save_token_settings", "diagnose_token_settings"} {
		t.Run(action, func(t *testing.T) {
			valid := `{"action":"` + action + `","params":{}}`
			var request peticion
			if err := json.Unmarshal([]byte(valid), &request); err != nil {
				t.Fatal(err)
			}
			for name, raw := range map[string]string{
				"duplicate_params":  strings.Replace(valid, `"params":{}`, `"params":{},"params":{}`, 1),
				"escaped_duplicate": strings.Replace(valid, `"params":{}`, `"params":{},"par\u0061ms":{}`, 1),
				"duplicate_action":  strings.Replace(valid, `"action":`, `"action":"ping","action":`, 1),
				"case_params":       strings.Replace(valid, `"params"`, `"Params"`, 1),
				"case_action":       strings.Replace(valid, `"action"`, `"Action"`, 1),
				"case_correlation":  strings.Replace(valid, `"params":{}`, `"params":{},"RequestId":"qa-request"`, 1),
				"unknown":           strings.Replace(valid, `"params":{}`, `"params":{},"pin":"QA-SECRET"`, 1),
				"missing_params":    `{"action":"` + action + `"}`,
			} {
				t.Run(name, func(t *testing.T) {
					if err := json.Unmarshal([]byte(raw), &request); err == nil {
						t.Fatal("accepted ambiguous administrative envelope")
					}
				})
			}
		})
	}
	var legacy peticion
	if err := json.Unmarshal([]byte(`{"Action":"ping","params":{},"legacyField":true}`), &legacy); err != nil || legacy.Action != "ping" {
		t.Fatalf("legacy behavior changed: %v", err)
	}
}

type tokenSettingsFake struct {
	loads     atomic.Int32
	saves     atomic.Int32
	diagnoses atomic.Int32
	err       error
	result    ports.TokenSettingsSnapshot
}

func (f *tokenSettingsFake) Load(context.Context) (ports.TokenSettingsSnapshot, error) {
	f.loads.Add(1)
	return f.result, f.err
}
func (f *tokenSettingsFake) Save(context.Context, ports.TokenSettingsUpdate) (ports.TokenSettingsSnapshot, error) {
	f.saves.Add(1)
	return f.result, f.err
}
func (f *tokenSettingsFake) Diagnose(context.Context) (ports.TokenSettingsSnapshot, error) {
	f.diagnoses.Add(1)
	return f.result, f.err
}
func (f *tokenSettingsFake) calls() int32 {
	return f.loads.Load() + f.saves.Load() + f.diagnoses.Load()
}

func TestTokenSettingsStrictUpdate(t *testing.T) {
	t.Parallel()
	if update, err := decodeTokenSettingsUpdate([]byte(tokenSettingsValidUpdate)); err != nil || !update.Confirmed || update.Enabled {
		t.Fatalf("valid disabled request rejected: %+v %v", update, err)
	}
	for _, field := range []string{"enabled", "modules", "revision", "confirmed", "replaceInvalid"} {
		for _, mode := range []string{"missing", "null"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				var value map[string]any
				_ = json.Unmarshal([]byte(tokenSettingsValidUpdate), &value)
				if mode == "missing" {
					delete(value, field)
				} else {
					value[field] = nil
				}
				raw, _ := json.Marshal(value)
				if _, err := decodeTokenSettingsUpdate(raw); !errors.Is(err, ports.ErrTokenSettingsInvalid) {
					t.Fatalf("accepted missing/null %s: %v", field, err)
				}
			})
		}
	}
	cases := map[string]string{
		"empty": "", "null": "null", "array": "[]", "trailing": tokenSettingsValidUpdate + "{}",
		"duplicate":         strings.Replace(tokenSettingsValidUpdate, `"enabled":false`, `"enabled":true,"enabled":false`, 1),
		"escaped_duplicate": strings.Replace(tokenSettingsValidUpdate, `"enabled":false`, `"enabled":true,"enabl\u0065d":false`, 1),
		"case_variant":      strings.Replace(tokenSettingsValidUpdate, `"enabled"`, `"Enabled"`, 1),
		"unknown":           strings.Replace(tokenSettingsValidUpdate, `"enabled":false`, `"enabled":false,"pin":"QA-DO-NOT-REFLECT"`, 1),
		"boolean_string":    strings.Replace(tokenSettingsValidUpdate, `"enabled":false`, `"enabled":"false"`, 1),
		"bad_revision":      strings.Replace(tokenSettingsValidUpdate, `"revision":""`, `"revision":"`+strings.Repeat("z", 64)+`"`, 1),
		"short_revision":    strings.Replace(tokenSettingsValidUpdate, `"revision":""`, `"revision":"a"`, 1),
		"large":             strings.Repeat(" ", 64*1024) + tokenSettingsValidUpdate,
		"invalid_utf8":      strings.Replace(tokenSettingsValidUpdate, `"revision":""`, "\"revision\":\"\xff\"", 1),
	}
	for name, modules := range map[string]string{
		"object": `{}`, "null_entry": `[null]`, "missing_path": `[{}]`, "path_null": `[{"path":null}]`,
		"path_case": `[{"Path":"/qa/module.so"}]`, "path_empty": `[{"path":""}]`,
		"path_nul":           `[{"path":"/qa/\u0000module.so"}]`,
		"path_large":         `[{"path":"/` + strings.Repeat("x", 4096) + `"}]`,
		"path_duplicate_key": `[{"path":"/qa/a.so","path":"/qa/b.so"}]`,
		"path_extra_key":     `[{"path":"/qa/a.so","helper":"/qa/evil"}]`,
		"duplicate_module":   `[{"path":"/qa/a.so"},{"path":"/qa/a.so"}]`,
		"nine":               `[` + strings.Repeat(`{"path":"/qa/a.so"},`, 8) + `{"path":"/qa/z.so"}]`,
	} {
		cases["modules_"+name] = strings.Replace(tokenSettingsValidUpdate, `"modules":[]`, `"modules":`+modules, 1)
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeTokenSettingsUpdate([]byte(raw)); !errors.Is(err, ports.ErrTokenSettingsInvalid) {
				t.Fatalf("invalid request accepted: %v", err)
			}
		})
	}
	unconfirmed := strings.Replace(tokenSettingsValidUpdate, `"confirmed":true`, `"confirmed":false`, 1)
	if _, err := decodeTokenSettingsUpdate([]byte(unconfirmed)); !errors.Is(err, ports.ErrTokenSettingsConfirmation) {
		t.Fatalf("unconfirmed: %v", err)
	}
	validModules := strings.Replace(tokenSettingsValidUpdate, `"modules":[]`, `"modules":[{"path":"/qa/prueba.so"}]`, 1)
	validModules = strings.Replace(validModules, `"revision":""`, `"revision":"`+strings.Repeat("a", 64)+`"`, 1)
	if update, err := decodeTokenSettingsUpdate([]byte(validModules)); err != nil || len(update.Modules) != 1 {
		t.Fatalf("valid modules: %+v %v", update, err)
	}
}

func TestTokenSettingsHandlerTrustAndValidation(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"get_token_settings", "save_token_settings", "diagnose_token_settings"} {
		t.Run(action, func(t *testing.T) {
			fake := &tokenSettingsFake{}
			m := &Manejador{TokenSettings: fake}
			raw := json.RawMessage(`{}`)
			if action == "save_token_settings" {
				raw = json.RawMessage(tokenSettingsValidUpdate)
			}
			resp := m.despachar(context.Background(), peticion{Action: action, Params: raw})
			if resp.OK || resp.ErrorCode != "token_settings_frontend_required" || fake.calls() != 0 {
				t.Fatalf("direct dispatch bypassed frontend: %+v", resp)
			}
			ctx := context.WithValue(context.Background(), localTokenSettingsContextKey{}, true)
			resp = m.despachar(ctx, peticion{Action: action, Params: json.RawMessage(`{"pin":"QA-SECRET"}`)})
			if resp.OK || resp.ErrorCode != "token_settings_invalid" || fake.calls() != 0 {
				t.Fatalf("invalid params reached service: %+v", resp)
			}
			if resp = m.despachar(ctx, peticion{Action: action, Params: raw}); !resp.OK || fake.calls() != 1 {
				t.Fatalf("trusted valid request failed: %+v", resp)
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if resp = m.despachar(cancelled, peticion{Action: action, Params: raw}); resp.OK || fake.calls() != 1 {
				t.Fatalf("cancelled request reached service: %+v", resp)
			}
		})
	}
}

func TestTokenSettingsSanitizedErrorsAndSavedWarning(t *testing.T) {
	t.Parallel()
	for code, err := range map[string]error{
		"token_settings_unavailable":  ports.ErrTokenSettingsUnavailable,
		"token_settings_invalid":      ports.ErrTokenSettingsInvalid,
		"token_settings_unsafe":       ports.ErrTokenSettingsUnsafe,
		"token_settings_conflict":     ports.ErrTokenSettingsConflict,
		"token_settings_confirmation": ports.ErrTokenSettingsConfirmation,
		"token_settings_write":        ports.ErrTokenSettingsWrite,
		"token_settings_failed":       errors.New("internal error"),
	} {
		t.Run(code, func(t *testing.T) {
			resp := tokenSettingsError("save_token_settings", fmt.Errorf("QA-SECRET-PATH: %w", err))
			wire, _ := json.Marshal(resp)
			if resp.OK || resp.ErrorCode != code || resp.Retryable || strings.Contains(string(wire), "QA-SECRET") {
				t.Fatalf("unsafe error response: %s", wire)
			}
		})
	}
	ctx := context.WithValue(context.Background(), localTokenSettingsContextKey{}, true)
	fake := &tokenSettingsFake{result: ports.TokenSettingsSnapshot{State: "configured", RestartRequired: true, Warning: "durability_unconfirmed"}}
	m := &Manejador{TokenSettings: fake}
	resp := m.despachar(ctx, peticion{Action: "save_token_settings", Params: json.RawMessage(tokenSettingsValidUpdate)})
	if !resp.OK || resp.Data.(ports.TokenSettingsSnapshot).Warning != "durability_unconfirmed" {
		t.Fatalf("saved change reported as failed: %+v", resp)
	}
	m.TokenSettings = nil
	resp = m.despachar(ctx, peticion{Action: "get_token_settings", Params: json.RawMessage(`{}`)})
	if !resp.OK || resp.Data.(ports.TokenSettingsSnapshot).Available {
		t.Fatalf("missing provider: %+v", resp)
	}
}

func TestTokenSettingsRejectsUnknownKernelPID(t *testing.T) {
	t.Parallel()
	fake := &tokenSettingsFake{}
	srv := New(&Manejador{TokenSettings: fake})
	if err := srv.peerPID.preparar(); err != nil {
		t.Fatal(err)
	}
	if err := srv.peerPID.publicar(1234); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() { defer close(done); srv.servirConexion(context.Background(), server) }()
	defer func() { _ = client.Close(); <-done }()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	enc, dec := json.NewEncoder(client), json.NewDecoder(client)
	for _, action := range []string{"get_token_settings", "save_token_settings", "diagnose_token_settings"} {
		params := json.RawMessage(`{}`)
		if action == "save_token_settings" {
			params = json.RawMessage(tokenSettingsValidUpdate)
		}
		if err := enc.Encode(peticion{Action: action, Params: params, RequestID: "qa-request", TraceID: "qa-trace"}); err != nil {
			t.Fatal(err)
		}
		var resp respuesta
		if err := dec.Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if resp.OK || resp.ErrorCode != "token_settings_frontend_required" || resp.RequestID != "qa-request" || resp.TraceID != "qa-trace" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	}
	if fake.calls() != 0 {
		t.Fatal("unknown kernel peer accessed settings")
	}
}

func TestTokenSettingsHelloIsDiscoverableNotHardwareSupport(t *testing.T) {
	t.Parallel()
	hello := desktopIPCHello()
	for _, action := range []string{"get_token_settings", "save_token_settings", "diagnose_token_settings"} {
		found := false
		for _, candidate := range hello.Actions {
			if action == candidate {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing action %s", action)
		}
	}
	for _, capability := range hello.Capabilities {
		if strings.Contains(capability, "pkcs11") || strings.Contains(capability, "hardware-signing") {
			t.Fatalf("administrative API must not imply hardware support: %s", capability)
		}
	}
}
