// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

//go:build linux

package ipc

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/tokenruntime"
	"grxfirma/internal/ports"
)

func TestTokenSettingsKernelFrontendBinding(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"unbound", "wrong_pid", "bound_then_revoked"} {
		t.Run(mode, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "af2-token-ipc-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(dir)
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, "ipc.sock"), Net: "unix"})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			fake := &tokenSettingsFake{}
			srv := New(&Manejador{TokenSettings: fake})
			if mode != "unbound" {
				if err := srv.peerPID.preparar(); err != nil {
					t.Fatal(err)
				}
				pid := uint32(os.Getpid())
				if mode == "wrong_pid" {
					pid++
				}
				if err := srv.peerPID.publicar(pid); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				server, err := listener.AcceptUnix()
				if err != nil {
					done <- err
					return
				}
				srv.servirConexion(context.Background(), server)
				done <- nil
			}()
			client, err := net.DialUnix("unix", nil, listener.Addr().(*net.UnixAddr))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = client.Close()
				if err := <-done; err != nil {
					t.Error(err)
				}
			}()
			_ = client.SetDeadline(time.Now().Add(5 * time.Second))
			enc, dec := json.NewEncoder(client), json.NewDecoder(client)
			request := func(action string) respuesta {
				t.Helper()
				params := json.RawMessage(`{}`)
				if action == "save_token_settings" {
					params = json.RawMessage(tokenSettingsValidUpdate)
				}
				if err := enc.Encode(peticion{Action: action, Params: params}); err != nil {
					t.Fatal(err)
				}
				var resp respuesta
				if err := dec.Decode(&resp); err != nil {
					t.Fatal(err)
				}
				return resp
			}
			for _, action := range []string{"get_token_settings", "save_token_settings", "diagnose_token_settings"} {
				resp := request(action)
				if mode == "bound_then_revoked" {
					if !resp.OK {
						t.Fatalf("bound kernel PID rejected: %+v", resp)
					}
				} else if resp.OK || resp.ErrorCode != "token_settings_frontend_required" {
					t.Fatalf("untrusted PID accepted: %+v", resp)
				}
			}
			if mode == "bound_then_revoked" {
				if fake.calls() != 3 {
					t.Fatalf("calls: %d", fake.calls())
				}
				srv.peerPID.denegar()
				for _, action := range []string{"get_token_settings", "save_token_settings", "diagnose_token_settings"} {
					if resp := request(action); resp.OK || resp.ErrorCode != "token_settings_frontend_required" {
						t.Fatalf("revocation bypassed: %+v", resp)
					}
				}
				if fake.calls() != 3 {
					t.Fatal("revoked connection reached service")
				}
			} else if fake.calls() != 0 {
				t.Fatal("untrusted connection reached service")
			}
			// Existing non-administrative actions retain legacy compatibility.
			if resp := request("ping"); !resp.OK {
				t.Fatalf("legacy ping broken: %+v", resp)
			}
		})
	}
}

// Real Unix peer + real settings manager, with an isolated empty directory.
// No certificate store, module, helper, network or signing provider is started.
func TestTokenSettingsRealManagerOverIPC(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "af2-token-roundtrip-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	configDir := filepath.Join(dir, "config")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, "ipc.sock"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	srv := New(&Manejador{}).WithLocalTokenSettings(tokenruntime.NewManager(configDir))
	if err := srv.peerPID.preparar(); err != nil {
		t.Fatal(err)
	}
	if err := srv.peerPID.publicar(uint32(os.Getpid())); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			done <- err
			return
		}
		srv.servirConexion(context.Background(), conn)
		done <- nil
	}()
	client, err := net.DialUnix("unix", nil, listener.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = client.Close()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	enc, dec := json.NewEncoder(client), json.NewDecoder(client)
	request := func(action string, params any) (respuesta, ports.TokenSettingsSnapshot) {
		t.Helper()
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		if err := enc.Encode(peticion{Action: action, Params: raw}); err != nil {
			t.Fatal(err)
		}
		var resp respuesta
		if err := dec.Decode(&resp); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(resp.Data)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot ports.TokenSettingsSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			t.Fatal(err)
		}
		return resp, snapshot
	}
	resp, initial := request("get_token_settings", map[string]any{})
	if !resp.OK {
		t.Fatalf("load: %+v", resp)
	}
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("load created directory: %v", err)
	}
	update := ports.TokenSettingsUpdate{Enabled: false, Modules: []ports.TokenModuleSetting{{Path: filepath.Join(dir, "absent-qa-driver.so")}}, Revision: initial.Revision, Confirmed: true}
	if !tokenruntime.EnabledInBuild() {
		if initial.Available || initial.Editable || initial.State != "unavailable" {
			t.Fatalf("production enabled: %+v", initial)
		}
		if resp, _ = request("save_token_settings", update); resp.OK || resp.ErrorCode != "token_settings_unavailable" {
			t.Fatalf("production save: %+v", resp)
		}
		if resp, snapshot := request("diagnose_token_settings", map[string]any{}); !resp.OK || snapshot.Available {
			t.Fatalf("production diagnosis: %+v", resp)
		}
		if _, err := os.Stat(configDir); !os.IsNotExist(err) {
			t.Fatalf("production performed config IO: %v", err)
		}
		return
	}
	if !initial.Available || !initial.Editable || initial.State != "missing" || len(initial.Revision) != 64 {
		t.Fatalf("preview initial: %+v", initial)
	}
	resp, saved := request("save_token_settings", update)
	if !resp.OK || saved.State != "disabled" || !saved.RestartRequired || saved.Revision == initial.Revision || len(saved.Modules) != 1 {
		t.Fatalf("preview save: %+v %+v", resp, saved)
	}
	info, err := os.Stat(filepath.Join(configDir, "tokens.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("saved permissions: %v %v", info, err)
	}
	if resp, _ = request("save_token_settings", update); resp.OK || resp.ErrorCode != "token_settings_conflict" {
		t.Fatalf("stale revision accepted: %+v", resp)
	}
	resp, diagnosis := request("diagnose_token_settings", map[string]any{})
	if !resp.OK || diagnosis.Revision != saved.Revision || !diagnosis.RestartRequired {
		t.Fatalf("diagnosis: %+v %+v", resp, diagnosis)
	}
	if len(diagnosis.Checks) != 6 {
		t.Fatalf("diagnostic checks: %+v", diagnosis.Checks)
	}
	for _, check := range diagnosis.Checks {
		if check.ID == "hardware" && check.Status != "not_checked" {
			t.Fatal("hardware overclaim")
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "absent-qa-driver.so")); !os.IsNotExist(err) {
		t.Fatal("created or used driver")
	}
	reopened, err := tokenruntime.NewManager(configDir).Load(context.Background())
	if err != nil || reopened.Revision != saved.Revision || reopened.RestartRequired {
		t.Fatalf("reopened settings: %+v %v", reopened, err)
	}
}
