// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package isolatedtokenstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/pkcs11worker"
	"grxfirma/internal/testsupport/pkcs11sandbox"
)

// The production client launches its executable without arguments and with a
// clean environment. This synthetic peer never loads C or accesses hardware.
func TestMain(m *testing.M) {
	if len(os.Args) == 1 {
		kind, payload, err := pkcs11worker.ReadFrame(os.Stdin, pkcs11worker.MaxRequestBytes)
		if err != nil || kind != pkcs11worker.FrameRequest {
			os.Exit(2)
		}
		var request pkcs11worker.Request
		if json.Unmarshal(payload, &request) != nil {
			os.Exit(2)
		}
		challenge, err := json.Marshal(pkcs11worker.PINChallenge{Version: request.Version, ID: request.ID})
		if err != nil || pkcs11worker.WriteFrame(os.Stdout, pkcs11worker.FramePINChallenge, challenge, pkcs11worker.MaxResponseBytes) != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProductionClientRejectsPINInCatalogAndDescribe(t *testing.T) {
	pkcs11sandbox.Require(t)
	// Some build-cache paths are group-writable under umask 002; copy the
	// current binary to an explicitly private fixture, never relax policy.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(dir, "synthetic-helper")
	sourcePath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := os.OpenFile(executable, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	module := filepath.Join(dir, "synthetic-module")
	if err := os.WriteFile(module, []byte("not a native module; never loaded"), 0600); err != nil {
		t.Fatal(err)
	}
	var pinCalls atomic.Int32
	store := New(pkcs11worker.Client{Executable: executable, ModulePath: module, Timeout: 5 * time.Second,
		PINSource: func(context.Context, pkcs11worker.PINMode) ([]byte, error) {
			pinCalls.Add(1)
			return nil, errors.New("unexpected PIN callback")
		}})
	if _, err := store.List(context.Background()); !errors.Is(err, pkcs11worker.ErrProtocol) {
		t.Fatalf("list error=%v", err)
	}
	f := newFixture(t, "EC", nil)
	if _, err := store.KeyFor(context.Background(), f.ref); !errors.Is(err, pkcs11worker.ErrProtocol) {
		t.Fatalf("describe error=%v", err)
	}
	if pinCalls.Load() != 0 {
		t.Fatal("read-only operation invoked PIN source")
	}
}
