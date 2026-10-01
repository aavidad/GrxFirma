// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/desktop/tokenruntime"
	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/machinepolicy"
	"grxfirma/internal/testsupport/tsatest"
	"grxfirma/presentation/desktop/certpicker"
	"software.sslmate.com/src/go-pkcs12"
)

func runE2EAislado(ctx context.Context, args []string, stderr *bytes.Buffer) int {
	return runE2EAisladoConAprobador(ctx, args, stderr, aprobacionE2E{})
}

func runE2EAisladoConAprobador(
	ctx context.Context,
	args []string,
	stderr *bytes.Buffer,
	aprobador ports.UserApproval,
	selectores ...certpicker.CertSelector,
) int {
	var selector certpicker.CertSelector = certpicker.NewHeadless()
	if len(selectores) > 0 && selectores[0] != nil {
		selector = selectores[0]
	}
	return runConDependencias(
		ctx,
		args,
		stderr,
		func(ctx context.Context, stderr io.Writer, rawURI string) int {
			return handleProtocolRequestWithGraceConDependencias(
				ctx,
				stderr,
				rawURI,
				nil,
				func(configDir, p12Dir, p12Password string) (*runtimeAfirmaURI, error) {
					return construirRuntimeAfirmaURIConDependencias(
						configDir,
						p12Dir,
						p12Password,
						cargarDirectorioP12,
						selector,
					)
				},
				aprobador,
			)
		},
	)
}

type selectorCertificadoE2EFunc func(context.Context, []domain.CertificateRef) (certpicker.ResultadoSeleccion, error)

func (f selectorCertificadoE2EFunc) Select(ctx context.Context, refs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
	return f(ctx, refs)
}

type aprobacionE2E struct{}

func (aprobacionE2E) Request(context.Context, string) (bool, error) {
	return true, nil
}

type rechazoE2E struct{}

func (rechazoE2E) Request(context.Context, string) (bool, error) {
	return false, nil
}

func TestRun_ProcesaURIRegistrableYSubeResultado(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	var (
		retrieveVisto bool
		presignVisto  bool
		postsignVisto bool
		uploadVisto   bool
		uploadCuerpo  string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/RetrieveService" && r.URL.Query().Get("op") == "get":
			retrieveVisto = true
			if got := r.URL.Query().Get("id"); got != "req-e2e" {
				t.Errorf("id retrieve = %q, want %q", got, "req-e2e")
			}
			if got := r.URL.Query().Get("v"); got != "1_0" {
				t.Errorf("v retrieve = %q, want %q", got, "1_0")
			}
			resp := map[string]string{
				"dat":         base64.StdEncoding.EncodeToString([]byte("documento-original")),
				"op":          "sign",
				"format":      "CAdES",
				"algorithm":   "SHA256withRSA",
				"extraParams": "mode=implicit",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/RetrieveService":
			_ = r.ParseForm()
			switch r.Form.Get("op") {
			case "get":
				retrieveVisto = true
				resp := map[string]string{
					"dat":         base64.StdEncoding.EncodeToString([]byte("documento-original")),
					"op":          "sign",
					"format":      "CAdES",
					"algorithm":   "SHA256withRSA",
					"extraParams": "mode=implicit",
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return
			case "sign":
				presignVisto = true
				if got := r.Form.Get("format"); got != "CAdES" {
					t.Errorf("format presign = %q, want %q", got, "CAdES")
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{
					"sign":        base64.StdEncoding.EncodeToString([]byte("hash-a-firmar")),
					"extraParams": "",
				})
				return
			case "aspsign":
				postsignVisto = true
				if r.Form.Get("dat") == "" {
					t.Error("postsign sin firma local")
				}
				_, _ = w.Write([]byte("resultado-firmado"))
				return
			default:
				http.Error(w, "op no soportada", http.StatusBadRequest)
				return
			}
		case r.Method == http.MethodPost && r.URL.Path == "/StorageService" && r.URL.Query().Get("op") == "put":
			uploadVisto = true
			uploadCuerpo = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origin := srv.URL
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(origin) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}

	p12Path := filepath.Join(p12Dir, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12(t, "Cert Handler E2E", "valor-prueba"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}

	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")

	retrieve := url.QueryEscape(srv.URL + "/RetrieveService")
	storage := url.QueryEscape(srv.URL + "/StorageService")
	rawURI := "afirma://sign?id=req-e2e&rtservlet=" + retrieve + "&stservlet=" + storage + "&signFormat=CAdES"

	var stderr bytes.Buffer
	if code := runE2EAislado(context.Background(), []string{rawURI}, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}

	if !retrieveVisto {
		t.Fatal("no se recibio llamada a RetrieveService")
	}
	if !presignVisto {
		t.Fatal("no se recibio llamada de presign")
	}
	if !postsignVisto {
		t.Fatal("no se recibio llamada de postsign")
	}
	if !uploadVisto {
		t.Fatal("no se recibio llamada a StorageService")
	}
	if !strings.Contains(uploadCuerpo, "dat=") {
		t.Fatalf("upload inesperado: %q", uploadCuerpo)
	}
	dat, err := url.ParseQuery(uploadCuerpo)
	if err != nil {
		t.Fatalf("ParseQuery(upload): %v", err)
	}
	resultadoB64 := dat.Get("dat")
	if resultadoB64 == "" {
		t.Fatalf("upload sin dat: %q", uploadCuerpo)
	}
	resultado, err := base64.StdEncoding.DecodeString(resultadoB64)
	if err != nil {
		t.Fatalf("DecodeString(resultado): %v", err)
	}
	if string(resultado) != "resultado-firmado" {
		t.Fatalf("resultado subido = %q, want %q", string(resultado), "resultado-firmado")
	}
}

func TestRun_ProcesaURIRegistrableConXAdESTYtsaURLPorPeticion(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	var tsaVisto bool
	tsaResponder := tsatest.NewResponder(t)
	tsaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tsaVisto = true
		if r.Method != http.MethodPost {
			t.Fatalf("metodo TSA inesperado: %s", r.Method)
		}
		tsaResponder.ServeHTTP(w, r)
	}))
	defer tsaSrv.Close()

	var (
		presignVisto  bool
		postsignVisto bool
		uploadVisto   bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/RetrieveService" && r.URL.Query().Get("op") == "get":
			resp := map[string]string{
				"dat":         base64.StdEncoding.EncodeToString([]byte("documento-original")),
				"op":          "sign",
				"format":      "XAdES",
				"algorithm":   "SHA256withRSA",
				"extraParams": "profile=baseline\ntsaURL=" + tsaSrv.URL,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/RetrieveService":
			_ = r.ParseForm()
			switch r.Form.Get("op") {
			case "get":
				resp := map[string]string{
					"dat":         base64.StdEncoding.EncodeToString([]byte("documento-original")),
					"op":          "sign",
					"format":      "XAdES",
					"algorithm":   "SHA256withRSA",
					"extraParams": "profile=baseline\ntsaURL=" + tsaSrv.URL,
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return
			case "sign":
				presignVisto = true
				if got := r.Form.Get("format"); got != "XAdES" {
					t.Errorf("format presign = %q, want %q", got, "XAdES")
				}
				// En trifásico el cliente no llama a la TSA: la tsaURL viaja
				// en extraParams al servidor de prefirma para que él la use.
				if ep := r.Form.Get("extraParams"); ep != "" {
					if decoded, err := base64.StdEncoding.DecodeString(ep); err == nil {
						if strings.Contains(string(decoded), tsaSrv.URL) {
							tsaVisto = true
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{
					"sign":        base64.StdEncoding.EncodeToString([]byte("hash-a-firmar")),
					"extraParams": "",
				})
				return
			case "aspsign":
				postsignVisto = true
				if r.Form.Get("dat") == "" {
					t.Error("postsign sin firma local")
				}
				_, _ = w.Write([]byte("resultado-firmado"))
				return
			default:
				http.Error(w, "op no soportada", http.StatusBadRequest)
				return
			}
		case r.Method == http.MethodPost && r.URL.Path == "/StorageService" && r.URL.Query().Get("op") == "put":
			uploadVisto = true
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origin := srv.URL
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(origin) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}

	p12Path := filepath.Join(p12Dir, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12RSA(t, "Cert Handler TSA E2E", "valor-prueba"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}

	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")
	t.Setenv("GRXFIRMA_TSA_URL", "")

	retrieve := url.QueryEscape(srv.URL + "/RetrieveService")
	storage := url.QueryEscape(srv.URL + "/StorageService")
	rawURI := "afirma://sign?id=req-e2e-tsa&rtservlet=" + retrieve + "&stservlet=" + storage + "&signFormat=XAdES"

	var stderr bytes.Buffer
	if code := runE2EAislado(context.Background(), []string{rawURI}, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if !presignVisto || !postsignVisto || !uploadVisto {
		t.Fatalf("flujo remoto incompleto: presign=%v postsign=%v upload=%v", presignVisto, postsignVisto, uploadVisto)
	}
	if !tsaVisto {
		t.Fatal("tsaURL no se propagó en extraParams al servidor de prefirma")
	}
}

func TestRun_ProcesaURIRegistrableConPAdESTYtsaURLPorPeticion(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	var tsaVisto bool
	tsaResponder := tsatest.NewResponder(t)
	tsaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tsaVisto = true
		if r.Method != http.MethodPost {
			t.Fatalf("metodo TSA inesperado: %s", r.Method)
		}
		tsaResponder.ServeHTTP(w, r)
	}))
	defer tsaSrv.Close()

	var (
		presignVisto  bool
		postsignVisto bool
		uploadVisto   bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/RetrieveService" && r.URL.Query().Get("op") == "get":
			resp := map[string]string{
				"dat":         base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n%origen\n")),
				"op":          "sign",
				"format":      "PAdES",
				"algorithm":   "SHA256withRSA",
				"extraParams": "profile=baseline\ntsaURL=" + tsaSrv.URL,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/RetrieveService":
			_ = r.ParseForm()
			switch r.Form.Get("op") {
			case "get":
				resp := map[string]string{
					"dat":         base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n%origen\n")),
					"op":          "sign",
					"format":      "PAdES",
					"algorithm":   "SHA256withRSA",
					"extraParams": "profile=baseline\ntsaURL=" + tsaSrv.URL,
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return
			case "sign":
				presignVisto = true
				if got := r.Form.Get("format"); got != "PAdES" {
					t.Errorf("format presign = %q, want %q", got, "PAdES")
				}
				// En trifásico el cliente no llama a la TSA: la tsaURL viaja
				// en extraParams al servidor de prefirma para que él la use.
				if ep := r.Form.Get("extraParams"); ep != "" {
					if decoded, err := base64.StdEncoding.DecodeString(ep); err == nil {
						if strings.Contains(string(decoded), tsaSrv.URL) {
							tsaVisto = true
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{
					"sign":        base64.StdEncoding.EncodeToString([]byte("hash-a-firmar")),
					"extraParams": "",
				})
				return
			case "aspsign":
				postsignVisto = true
				if r.Form.Get("dat") == "" {
					t.Error("postsign sin firma local")
				}
				_, _ = w.Write([]byte("resultado-firmado"))
				return
			default:
				http.Error(w, "op no soportada", http.StatusBadRequest)
				return
			}
		case r.Method == http.MethodPost && r.URL.Path == "/StorageService" && r.URL.Query().Get("op") == "put":
			uploadVisto = true
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origin := srv.URL
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(origin) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}

	p12Path := filepath.Join(p12Dir, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12RSA(t, "Cert Handler TSA PDF E2E", "valor-prueba"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}

	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")
	t.Setenv("GRXFIRMA_TSA_URL", "")

	retrieve := url.QueryEscape(srv.URL + "/RetrieveService")
	storage := url.QueryEscape(srv.URL + "/StorageService")
	rawURI := "afirma://sign?id=req-e2e-pades-tsa&rtservlet=" + retrieve + "&stservlet=" + storage + "&signFormat=PAdES"

	var stderr bytes.Buffer
	if code := runE2EAislado(context.Background(), []string{rawURI}, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if !presignVisto || !postsignVisto || !uploadVisto {
		t.Fatalf("flujo remoto incompleto: presign=%v postsign=%v upload=%v", presignVisto, postsignVisto, uploadVisto)
	}
	if !tsaVisto {
		t.Fatal("tsaURL no se propagó en extraParams al servidor de prefirma")
	}
}

func TestRun_ProcesaURIBatchRemotoJSONCompatV1(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	rawBatch := []byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc-1","datareference":"token-remoto"}]}`)
	preHashB64 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))

	var (
		waitVisto   bool
		preVisto    bool
		postVisto   bool
		uploadVisto bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pre":
			preVisto = true
			if got := r.URL.Query().Get("json"); got == "" {
				t.Fatal("prefirma batch sin json")
			}
			if got := r.URL.Query().Get("certs"); got == "" {
				t.Fatal("prefirma batch sin certs")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"td": map[string]any{
					"format": "CAdES",
					"signinfo": []map[string]any{
						{"id": "doc-1", "params": map[string]string{"PRE": preHashB64}},
					},
				},
			})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/post":
			postVisto = true
			if got := r.URL.Query().Get("tridata"); got == "" {
				t.Fatal("postfirma batch sin tridata")
			}
			_, _ = w.Write([]byte(`{"signs":[{"id":"doc-1","result":"DONE_AND_SAVED"}]}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/StorageService":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload batch): %v", err)
			}
			if r.Form.Get("dat") == "#WAIT" {
				waitVisto = true
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
				return
			}
			uploadVisto = true
			if got := r.Form.Get("op"); got != "put" {
				t.Fatalf("op upload batch = %q", got)
			}
			if got := r.Form.Get("id"); got != "req-batch-e2e" {
				t.Fatalf("id upload batch = %q", got)
			}
			dat := r.Form.Get("dat")
			if dat == "" {
				t.Fatal("upload batch sin dat")
			}
			decoded, err := base64.StdEncoding.DecodeString(dat)
			if err != nil {
				t.Fatalf("DecodeString(upload batch): %v", err)
			}
			if string(decoded) != `{"signs":[{"id":"doc-1","result":"DONE_AND_SAVED"}]}` {
				t.Fatalf("resultado batch inesperado: %s", string(decoded))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("SAVE_OK"))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origin := srv.URL
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(origin) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}

	p12Path := filepath.Join(p12Dir, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12RSA(t, "Cert Handler Batch E2E", "valor-prueba"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}

	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")

	rawURI := "afirma://batch?id=req-batch-e2e&stservlet=" + url.QueryEscape(srv.URL+"/StorageService") +
		"&jsonbatch=true&batchpresignerurl=" + url.QueryEscape(srv.URL+"/pre") +
		"&batchpostsignerurl=" + url.QueryEscape(srv.URL+"/post") +
		"&dat=" + base64.StdEncoding.EncodeToString(rawBatch)

	var stderr bytes.Buffer
	if code := runE2EAislado(context.Background(), []string{rawURI}, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}
	if !waitVisto || !preVisto || !postVisto || !uploadVisto {
		t.Fatalf("flujo batch remoto incompleto: wait=%v pre=%v post=%v upload=%v", waitVisto, preVisto, postVisto, uploadVisto)
	}
}

func TestRun_BatchLocalStopOnErrorNoIniciaTrabajosPosteriores(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	var (
		retrieveCalls atomic.Int32
		presignCalls  atomic.Int32
		sha1Calls     atomic.Int32
		postsignCalls atomic.Int32
		uploadCalls   atomic.Int32
	)
	preHashB64 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x33}, 32))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		switch r.Form.Get("op") {
		case "get":
			call := retrieveCalls.Add(1)
			algorithm := "SHA256withRSA"
			if call == 2 {
				algorithm = "SHA1withRSA"
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"dat":         base64.StdEncoding.EncodeToString([]byte("documento-sintetico")),
				"op":          "sign",
				"format":      "CAdES",
				"algorithm":   algorithm,
				"extraParams": "",
			})
		case "sign":
			presignCalls.Add(1)
			if r.Form.Get("algorithm") == "SHA1withRSA" {
				sha1Calls.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"sign":        preHashB64,
				"extraParams": "",
			})
		case "aspsign":
			postsignCalls.Add(1)
			_, _ = w.Write([]byte("resultado-final-sintetico"))
		case "put":
			uploadCalls.Add(1)
			decoded, err := base64.StdEncoding.DecodeString(r.Form.Get("dat"))
			if err != nil || string(decoded) != "resultado-final-sintetico" {
				http.Error(w, "bad upload", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("SAVE_OK"))
		default:
			http.Error(w, "bad operation", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(srv.URL) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}
	p12Path := filepath.Join(p12Dir, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12RSA(t, "Cert Handler Batch StopOnError", "valor-prueba"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}
	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")

	item := func(id string) map[string]string {
		return map[string]string{
			"id":            id,
			"datareference": base64.StdEncoding.EncodeToString([]byte("documento-" + id)),
			"format":        "CAdES",
			"suboperation":  "sign",
		}
	}
	rawBatch, err := json.Marshal(map[string]any{
		"stoponerror": true,
		"format":      "CAdES",
		"algorithm":   "SHA256withRSA",
		"singlesigns": []map[string]string{
			item("primero"),
			item("error"),
			item("omitido"),
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(batch): %v", err)
	}
	endpoint := url.QueryEscape(srv.URL + "/StorageService")
	rawURI := "afirma://batch?id=req-stoponerror&rtservlet=" + endpoint +
		"&stservlet=" + endpoint +
		"&jsonbatch=true&dat=" + url.QueryEscape(base64.StdEncoding.EncodeToString(rawBatch))

	var stderr bytes.Buffer
	if code := runE2EAislado(context.Background(), []string{rawURI}, &stderr); code != 1 {
		t.Fatalf("run() code = %d, want 1; stderr=%s", code, stderr.String())
	}
	if got := retrieveCalls.Load(); got != 2 {
		t.Fatalf("retrieve calls = %d, want 2", got)
	}
	if got := presignCalls.Load(); got != 2 {
		t.Fatalf("presign calls = %d, want 2", got)
	}
	if got := sha1Calls.Load(); got != 1 {
		t.Fatalf("SHA1 presign calls = %d, want 1", got)
	}
	if got := postsignCalls.Load(); got != 1 {
		t.Fatalf("postsign calls = %d, want 1", got)
	}
	if got := uploadCalls.Load(); got != 1 {
		t.Fatalf("upload calls = %d, want 1", got)
	}
}

func TestRun_BatchCanceladoAntesDeRedNoContactaServicios(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	var requestCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCalls.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(srv.URL) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}
	p12Path := filepath.Join(p12Dir, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12RSA(t, "Cert Handler Batch Cancel", "valor-prueba"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}
	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")

	rawBatch := []byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"cancelado","datareference":"token-remoto"}],"stoponerror":true}`)
	rawURI := "afirma://batch?id=req-cancel&stservlet=" +
		url.QueryEscape(srv.URL+"/StorageService") +
		"&jsonbatch=true&batchpresignerurl=" + url.QueryEscape(srv.URL+"/pre") +
		"&batchpostsignerurl=" + url.QueryEscape(srv.URL+"/post") +
		"&dat=" + url.QueryEscape(base64.StdEncoding.EncodeToString(rawBatch))

	var stderr bytes.Buffer
	if code := runE2EAisladoConAprobador(
		context.Background(),
		[]string{rawURI},
		&stderr,
		rechazoE2E{},
	); code != 1 {
		t.Fatalf("run() code = %d, want 1; stderr=%s", code, stderr.String())
	}
	if got := requestCalls.Load(); got != 0 {
		t.Fatalf("requests after cancellation = %d, want 0", got)
	}
}

func TestRun_ProcesaURISelectCertYSubeCertificado(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	var (
		uploadVisto bool
		uploadForm  url.Values
	)

	p12Path := filepath.Join(p12Dir, "prueba.p12")
	p12Fixture, certFixture := generarP12ConCert(t, "Cert Handler SelectCert", "valor-prueba")
	if err := os.WriteFile(p12Path, p12Fixture, 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/StorageService":
			uploadVisto = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload selectcert): %v", err)
			}
			if got := r.Form.Get("op"); got != "put" {
				t.Fatalf("op upload selectcert = %q, want %q", got, "put")
			}
			if got := r.Form.Get("id"); got != "req-selectcert-e2e" {
				t.Fatalf("id upload selectcert = %q, want %q", got, "req-selectcert-e2e")
			}
			uploadForm = r.Form
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origin := srv.URL
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(origin) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}

	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")

	storage := url.QueryEscape(srv.URL + "/StorageService")
	rawURI := "afirma://selectcert?id=req-selectcert-e2e&stservlet=" + storage

	var stderr bytes.Buffer
	if code := runE2EAislado(context.Background(), []string{rawURI}, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}

	if !uploadVisto {
		t.Fatal("no se recibió subida de certificado al StorageService")
	}
	dat := uploadForm.Get("dat")
	if dat == "" {
		t.Fatalf("upload selectcert sin dat: %v", uploadForm)
	}
	certSubido, err := base64.URLEncoding.DecodeString(dat)
	if err != nil {
		t.Fatalf("DecodeString(dat selectcert): %v", err)
	}
	if len(certSubido) == 0 {
		t.Fatal("certificado subido vacío")
	}
	if _, err := x509.ParseCertificate(certSubido); err != nil {
		t.Fatalf("certificado subido no es DER X.509 válido: %v", err)
	}
	if !bytes.Equal(certSubido, certFixture.Raw) {
		t.Fatalf(
			"selectcert subió un certificado ajeno al fixture: got sha256=%x, want sha256=%x",
			sha256.Sum256(certSubido),
			sha256.Sum256(certFixture.Raw),
		)
	}
}

func TestRun_ProcesaURISelectCertConFallbackLegacy(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	var (
		intentos     int
		datJavaStyle string
		certLegacy   string
		tokenLegacy  string
	)

	p12Path := filepath.Join(p12Dir, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12(t, "Cert Handler SelectCert Legacy", "valor-prueba"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/StorageService":
			intentos++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload selectcert fallback): %v", err)
			}
			if got := r.Form.Get("id"); got != "req-selectcert-fallback" {
				t.Fatalf("id upload selectcert fallback = %q, want %q", got, "req-selectcert-fallback")
			}
			if intentos == 1 {
				datJavaStyle = r.Form.Get("dat")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("AUTHENTICATION_ERROR"))
				return
			}
			certLegacy = r.Form.Get("cert")
			tokenLegacy = r.Form.Get("customToken")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("SAVE_OK"))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origin := srv.URL
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(origin) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}

	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")

	storage := url.QueryEscape(srv.URL + "/StorageService")
	rawURI := "afirma://selectcert?id=req-selectcert-fallback&stservlet=" + storage + "&customToken=abc123"

	var stderr bytes.Buffer
	if code := runE2EAislado(context.Background(), []string{rawURI}, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}

	if intentos != 2 {
		t.Fatalf("intentos upload selectcert = %d, want 2", intentos)
	}
	if datJavaStyle == "" {
		t.Fatal("primer intento java-style sin dat")
	}
	if certLegacy == "" {
		t.Fatal("fallback legacy sin cert")
	}
	if tokenLegacy != "abc123" {
		t.Fatalf("customToken legacy = %q, want %q", tokenLegacy, "abc123")
	}
	certSubido, err := base64.StdEncoding.DecodeString(certLegacy)
	if err != nil {
		t.Fatalf("DecodeString(cert legacy): %v", err)
	}
	if _, err := x509.ParseCertificate(certSubido); err != nil {
		t.Fatalf("certificado legacy no es DER X.509 válido: %v", err)
	}
}

// Con clave de sesión el certificado solo viaja cifrado y la clave nunca
// llega al servidor intermedio (igual que en AutoFirma Java). Antes, si el
// servidor rechazaba la subida cifrada, se reintentaba con el certificado en
// claro y la propia clave, anulando el cifrado.
func TestRun_ProcesaURISelectCertConSessionKeyNoReintentaEnClaro(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)

	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	p12Path := filepath.Join(p12Dir, "prueba.p12")
	if err := os.WriteFile(p12Path, generarP12(t, "Cert Handler SelectCert SessionKey", "valor-prueba"), 0o600); err != nil {
		t.Fatalf("WriteFile(p12): %v", err)
	}

	var intentos int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/StorageService" {
			http.NotFound(w, r)
			return
		}
		intentos++
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm(upload selectcert sessionkey): %v", err)
		}
		if r.Form.Has("key") || r.Form.Has("cert") {
			t.Errorf("subida con clave o certificado en claro: claves=%v", r.Form)
		}
		if dat := r.Form.Get("dat"); dat == "" {
			t.Error("subida sin dat")
		} else if der, err := base64.StdEncoding.DecodeString(dat); err == nil {
			if _, err := x509.ParseCertificate(der); err == nil {
				t.Error("el certificado se subió en claro pese a existir clave de sesión")
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("AUTHENTICATION_ERROR"))
	}))
	defer srv.Close()

	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(srv.URL) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}

	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")

	storage := url.QueryEscape(srv.URL + "/StorageService")
	rawURI := "afirma://selectcert?id=req-selectcert-key&stservlet=" + storage + "&key=abcd&customToken=abc123"

	var stderr bytes.Buffer
	if code := runE2EAislado(context.Background(), []string{rawURI}, &stderr); code == 0 {
		t.Fatal("run() debía fallar: el servidor rechazó la única subida cifrada")
	}
	if intentos != 1 {
		t.Fatalf("intentos upload selectcert sessionkey = %d, want 1", intentos)
	}
}

func TestRun_ProcesaURISelectCertReutilizaPreferenciaPersistente(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "grxfirma")
	p12Dir := filepath.Join(configDir, "pkcs12")
	if err := os.MkdirAll(p12Dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(pkcs12): %v", err)
	}

	primeroP12, primeroCert := generarP12ConCert(t, "AAA Cert Primero", "valor-prueba")
	segundoP12, segundoCert := generarP12ConCert(t, "ZZZ Cert Preferido", "valor-prueba")
	if err := os.WriteFile(filepath.Join(p12Dir, "a-primero.p12"), primeroP12, 0o600); err != nil {
		t.Fatalf("WriteFile(a-primero.p12): %v", err)
	}
	if err := os.WriteFile(filepath.Join(p12Dir, "z-preferido.p12"), segundoP12, 0o600); err != nil {
		t.Fatalf("WriteFile(z-preferido.p12): %v", err)
	}

	var certSubido []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/StorageService":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload selectcert preferencia): %v", err)
			}
			dat := r.Form.Get("dat")
			if dat == "" {
				t.Fatal("upload selectcert preferencia sin dat")
			}
			var err error
			certSubido, err = base64.URLEncoding.DecodeString(dat)
			if err != nil {
				t.Fatalf("DecodeString(dat preferencia): %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	origin := srv.URL
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	trustJSON := "{\n  " + strconvQuote(origin) + ": \"allowed\"\n}\n"
	if err := os.WriteFile(filepath.Join(configDir, "trusted-domains.json"), []byte(trustJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(trusted-domains): %v", err)
	}

	preferencia := map[string]string{
		origin: idCertificado(segundoCert.Raw),
	}
	preferenciaJSON, err := json.MarshalIndent(preferencia, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(preferencia): %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "preferred-certificates.json"), preferenciaJSON, 0o600); err != nil {
		t.Fatalf("WriteFile(preferred-certificates.json): %v", err)
	}

	setTestUserHome(t, home)
	t.Setenv("GRXFIRMA_PKCS12_DIR", p12Dir)
	t.Setenv("GRXFIRMA_PKCS12_PASSWORD", "valor-prueba")

	storage := url.QueryEscape(srv.URL + "/StorageService")
	rawURI := "afirma://selectcert?id=req-selectcert-pref&stservlet=" + storage

	var selectorCalls atomic.Int32
	selector := selectorCertificadoE2EFunc(func(_ context.Context, refs []domain.CertificateRef) (certpicker.ResultadoSeleccion, error) {
		selectorCalls.Add(1)
		for _, ref := range refs {
			if ref.ID == idCertificado(primeroCert.Raw) {
				return certpicker.ResultadoSeleccion{Certificado: ref, Recuerdo: certpicker.NoRecordar}, nil
			}
		}
		return certpicker.ResultadoSeleccion{}, errors.New("el catálogo QA no contiene la identidad elegida explícitamente")
	})
	var stderr bytes.Buffer
	if code := runE2EAisladoConAprobador(context.Background(), []string{rawURI}, &stderr, aprobacionE2E{}, selector); code != 0 {
		t.Fatalf("run() code = %d, stderr = %s", code, stderr.String())
	}

	if len(certSubido) == 0 {
		t.Fatal("no se subió certificado en flujo selectcert con preferencia")
	}
	subido, err := x509.ParseCertificate(certSubido)
	if err != nil {
		t.Fatalf("ParseCertificate(subido): %v", err)
	}
	// El contrato normal reutiliza la preferencia persistente sin selector.
	// Preview (y la UI nativa explícita) exige elección para toda identidad:
	// el arnés elige el primero y debe prevalecer sobre el segundo guardado.
	esperado := segundoCert
	var llamadasEsperadas int32
	if tokenruntime.EnabledInBuild() || nativeProtocolUIEnabled() {
		esperado = primeroCert
		llamadasEsperadas = 1
	}
	if got := selectorCalls.Load(); got != llamadasEsperadas {
		t.Fatalf("llamadas al selector = %d, want %d", got, llamadasEsperadas)
	}
	if got, want := subido.Subject.CommonName, esperado.Subject.CommonName; got != want {
		t.Fatalf("common name certificado preferido = %q, want %q", got, want)
	}
	if !bytes.Equal(certSubido, esperado.Raw) {
		t.Fatal("se subió una identidad distinta de la exigida por el contrato de selección")
	}
}

func generarP12(t *testing.T, commonName, password string) []byte {
	t.Helper()

	priv, cert := generarMaterialCertificado(t, commonName)
	data, err := pkcs12.Modern.Encode(priv, cert, nil, password)
	if err != nil {
		t.Fatalf("pkcs12.Encode: %v", err)
	}
	return data
}

func generarP12ConCert(t *testing.T, commonName, password string) ([]byte, *x509.Certificate) {
	t.Helper()

	priv, cert := generarMaterialCertificado(t, commonName)
	data, err := pkcs12.Modern.Encode(priv, cert, nil, password)
	if err != nil {
		t.Fatalf("pkcs12.Encode: %v", err)
	}
	return data, cert
}

func generarP12RSA(t *testing.T, commonName, password string) []byte {
	t.Helper()

	priv, cert := generarMaterialCertificadoRSA(t, commonName)
	data, err := pkcs12.Modern.Encode(priv, cert, nil, password)
	if err != nil {
		t.Fatalf("pkcs12.Encode: %v", err)
	}
	return data
}

func generarMaterialCertificado(t *testing.T, commonName string) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(47),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return priv, cert
}

func idCertificado(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func generarMaterialCertificadoRSA(t *testing.T, commonName string) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey RSA: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(48),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate RSA: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate RSA: %v", err)
	}
	return priv, cert
}

func strconvQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
