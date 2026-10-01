// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package pkcs11worker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func validRequest(operation string) Request {
	// Ruta sintética: el backend de estos tests es un doble, nunca dlopen.
	// Windows exige volumen además de raíz para filepath.IsAbs; /opt/... no
	// satisface ese contrato y ocultaba las pruebas posteriores de PIN/servidor.
	modulePath := "/opt/qa/module.so"
	if runtime.GOOS == "windows" {
		modulePath = `C:\qa\module.dll`
	}
	r := Request{Version: ProtocolVersion, ID: strings.Repeat("a", 32), Operation: operation, ModulePath: modulePath}
	if operation != "list" {
		r.CertificateID = "qa-identity"
		r.Fingerprint = strings.Repeat("b", 64)
	}
	if operation == "sign" {
		r.Hash = "sha256"
		r.Digest = make([]byte, 32)
	}
	return r
}

func TestRequestValidation(t *testing.T) {
	for _, op := range []string{"list", "describe", "sign"} {
		if err := validRequest(op).Validate(); err != nil {
			t.Fatal(err)
		}
	}
	tests := map[string]func(*Request){
		"version":         func(r *Request) { r.Version = 0 },
		"transaction":     func(r *Request) { r.ID = "unbound" },
		"bad hex":         func(r *Request) { r.ID = strings.Repeat("z", 32) },
		"relative module": func(r *Request) { r.ModulePath = "module.so" },
		"nul module":      func(r *Request) { r.ModulePath += "\x00" },
		"module size":     func(r *Request) { r.ModulePath += strings.Repeat("x", 4096) },
		"operation":       func(r *Request) { r.Operation = "export_private_key" },
		"identity":        func(r *Request) { r.CertificateID = "" },
		"fingerprint":     func(r *Request) { r.Fingerprint = "unbound" },
		"digest":          func(r *Request) { r.Digest = r.Digest[:31] },
		"sha1":            func(r *Request) { r.Hash = "sha1"; r.Digest = make([]byte, 20) },
		"sha384 size":     func(r *Request) { r.Hash = "sha384" },
		"list extra":      func(r *Request) { r.Operation = "list" },
		"describe extra":  func(r *Request) { r.Operation = "describe" },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			r := validRequest("sign")
			change(&r)
			if r.Validate() == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
	for hash, size := range map[string]int{"sha384": 48, "sha512": 64} {
		r := validRequest("sign")
		r.Hash = hash
		r.Digest = make([]byte, size)
		if r.Validate() != nil {
			t.Fatal(hash)
		}
	}
}

func TestRequestModulePathUsesNativeAbsolutePath(t *testing.T) {
	r := validRequest("list")
	if !filepath.IsAbs(r.ModulePath) {
		t.Fatalf("fixture no absoluta en %s: %q", runtime.GOOS, r.ModulePath)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("fixture válida rechazada: %v", err)
	}
	invalid := []string{"module.so", "./module.so", "../module.so"}
	if runtime.GOOS == "windows" {
		// No convertir rutas POSIX o relativas a la unidad en absolutas para
		// acomodar fixtures: la política productiva permanece cerrada.
		invalid = append(invalid, "/opt/qa/module.so", `\qa\module.dll`, `C:qa\module.dll`)
	}
	for _, path := range invalid {
		r.ModulePath = path
		if !errors.Is(r.Validate(), ErrProtocol) {
			t.Fatalf("ruta no absoluta aceptada: %q", path)
		}
	}
}

func TestFrameBoundsBeforePayloadRead(t *testing.T) {
	var header [5]byte
	header[0] = FrameRequest
	binary.BigEndian.PutUint32(header[1:], ^uint32(0))
	input := bytes.NewReader(append(header[:], []byte("never read")...))
	if _, _, err := ReadFrame(input, MaxRequestBytes); !errors.Is(err, ErrProtocol) {
		t.Fatal(err)
	}
	if input.Len() != len("never read") {
		t.Fatal("oversized frame consumed payload")
	}
	for _, data := range [][]byte{nil, {1}, {1, 0, 0, 0, 3, 2}} {
		if _, _, err := ReadFrame(bytes.NewReader(data), 10); err == nil {
			t.Fatal("truncated frame accepted")
		}
	}
	var out bytes.Buffer
	if WriteFrame(&out, FrameRequest, make([]byte, 11), 10) == nil || out.Len() != 0 {
		t.Fatal("oversized output written")
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > 2 {
		p = p[:2]
	}
	return w.Buffer.Write(p)
}

type stalledWriter struct{}

func (stalledWriter) Write([]byte) (int, error) { return 0, nil }

func TestFrameShortWritesAndBinaryPIN(t *testing.T) {
	var writer shortWriter
	pin := []byte{'1', 0, 255, '2'}
	if err := WritePINReply(&writer, pin, false); err != nil {
		t.Fatal(err)
	}
	kind, data, err := ReadFrame(&writer, MaxPINBytes+1)
	if err != nil || kind != FramePINReply || !bytes.Equal(data, append([]byte{1}, pin...)) {
		t.Fatalf("binary PIN changed: kind=%d err=%v", kind, err)
	}
	if !bytes.Equal(pin, []byte{'1', 0, 255, '2'}) {
		t.Fatal("borrowed caller buffer changed")
	}
	if err := WriteFrame(stalledWriter{}, FrameRequest, nil, 10); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	if WritePINReply(io.Discard, []byte("secret"), true) == nil {
		t.Fatal("cancellation carried secret")
	}
	if WritePINReply(io.Discard, make([]byte, MaxPINBytes+1), false) == nil {
		t.Fatal("large PIN accepted")
	}
}

func TestStrictJSONRejectsUnexpectedOrAdditionalData(t *testing.T) {
	for _, payload := range []string{`{"version":1,"pin":"never in JSON"}`, `{} {}`, `{} trailing`, `null {}`} {
		var request Request
		if decodeJSON([]byte(payload), &request) == nil {
			t.Fatal("unexpected JSON accepted")
		}
	}
}
