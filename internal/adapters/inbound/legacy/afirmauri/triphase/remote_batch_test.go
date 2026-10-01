// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"grxfirma/internal/adapters/inbound/legacy/afirmauri"
	"grxfirma/internal/domain"
	"grxfirma/internal/security/cryptopolicy"
	"grxfirma/internal/security/machinepolicy"
)

type mockLegacyBatchKey struct {
	chain [][]byte
}

func TestHashLegacyPreData_RechazaSHA1SinOptIn(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	if _, err := hashLegacyPreData([]byte("PRE"), crypto.SHA1); !errors.Is(err, cryptopolicy.ErrSHA1Disabled) {
		t.Fatalf("hashLegacyPreData() error = %v", err)
	}
}

func TestSignLegacyBatchPreData_RechazaSHA1AntesDelAdaptador(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirSHA1Legacy, false)
	key := &mockLegacyBatchPreKey{}
	if _, err := signLegacyBatchPreData([]byte("PRE"), key, "SHA1withRSA", crypto.SHA1, nil); !errors.Is(err, cryptopolicy.ErrSHA1Disabled) {
		t.Fatalf("signLegacyBatchPreData() error = %v", err)
	}
	if len(key.opts) != 0 {
		t.Fatal("el adaptador recibió el PRE pese a estar SHA-1 deshabilitado")
	}
}

func TestFingerprintLegacyBatchBody_UsaSHA256(t *testing.T) {
	const body = "respuesta remota"
	sum := sha256.Sum256([]byte(body))
	want := fmt.Sprintf("len=%d sha12=%x", len(body), sum[:6])
	if got := fingerprintLegacyBatchBody(body); got != want {
		t.Fatalf("fingerprintLegacyBatchBody() = %q, want %q", got, want)
	}
}

func (m *mockLegacyBatchKey) KeyID() string { return "mock-batch-key" }

func (m *mockLegacyBatchKey) SignDigest(digest []byte, _ crypto.Hash) ([]byte, error) {
	return append([]byte("pk1:"), digest...), nil
}

func (m *mockLegacyBatchKey) CertificateChainDER() [][]byte {
	out := make([][]byte, 0, len(m.chain))
	for _, der := range m.chain {
		out = append(out, append([]byte(nil), der...))
	}
	return out
}

type mockLegacyBatchPreKey struct {
	mockLegacyBatchKey
	opts []map[string]string
}

func (m *mockLegacyBatchPreKey) SignPreData(preData []byte, algorithm string, options map[string]string) (string, error) {
	copia := make(map[string]string, len(options))
	for k, v := range options {
		copia[k] = v
	}
	m.opts = append(m.opts, copia)
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("pre:%s:%s", algorithm, string(preData)))), nil
}

func TestExecuteLegacyRemote_ReplicaFlujoBatchV1(t *testing.T) {
	t.Parallel()

	rawBatch := []byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc-1","datareference":"token-remoto"}]}`)
	preHashB64 := base64.StdEncoding.EncodeToString([]byte("digest-batch"))
	postResult := []byte(`{"signs":[{"id":"doc-1","result":"DONE_AND_SAVED"}]}`)

	var (
		waitVisto   bool
		preVisto    bool
		postVisto   bool
		uploadVisto bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pre":
			preVisto = true
			if r.Method != http.MethodPost {
				t.Fatalf("prefirma método inesperado: %s", r.Method)
			}
			if got := r.URL.Query().Get("json"); got == "" {
				t.Fatal("prefirma sin parámetro json")
			}
			if got := r.URL.Query().Get("certs"); got == "" {
				t.Fatal("prefirma sin parámetro certs")
			}
			_ = json.NewEncoder(w).Encode(legacyBatchPreResponse{
				TD: &legacyTriphaseDataResponse{
					Format: "CAdES",
					SignInfo: []legacyTriphaseSignInfo{
						{ID: "doc-1", Params: map[string]string{"PRE": preHashB64}},
					},
				},
				Results: []legacyBatchSingleResult{
					{ID: "doc-1", Result: "DONE_AND_SAVED"},
				},
			})
		case "/post":
			postVisto = true
			if got := r.URL.Query().Get("json"); got == "" {
				t.Fatal("postfirma sin parámetro json")
			}
			if got := r.URL.Query().Get("certs"); got == "" {
				t.Fatal("postfirma sin parámetro certs")
			}
			tdB64 := r.URL.Query().Get("tridata")
			if tdB64 == "" {
				t.Fatal("postfirma sin tridata")
			}
			tdJSON, err := base64.URLEncoding.DecodeString(tdB64)
			if err != nil {
				t.Fatalf("tridata no es base64 urlsafe válido: %v", err)
			}
			if want := base64.URLEncoding.EncodeToString(tdJSON); tdB64 != want {
				t.Fatalf("tridata no mantiene codificación urlsafe de V1: got=%q want=%q", tdB64, want)
			}
			if !strings.Contains(string(tdJSON), "PK1") {
				t.Fatalf("tridata firmada no contiene PK1: %s", string(tdJSON))
			}
			_, _ = w.Write(postResult)
		case "/store":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload): %v", err)
			}
			if r.Form.Get("dat") == "#WAIT" {
				waitVisto = true
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
				return
			}
			uploadVisto = true
			if got := r.Form.Get("op"); got != "put" {
				t.Fatalf("op upload = %q", got)
			}
			if got := r.Form.Get("id"); got != "req-batch-v1" {
				t.Fatalf("id upload = %q", got)
			}
			dat := r.Form.Get("dat")
			if dat == "" {
				t.Fatal("upload sin dat")
			}
			decoded, err := base64.StdEncoding.DecodeString(dat)
			if err != nil {
				t.Fatalf("dat no es base64 estándar: %v", err)
			}
			if string(decoded) != string(postResult) {
				t.Fatalf("resultado subido inesperado: %s", string(decoded))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("SAVE_OK"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	batch := NewBatch(executor)
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{
		chain: [][]byte{[]byte("cert-der-hoja"), []byte("cert-der-ca")},
	})

	err := batch.ExecuteLegacyRemote(ctx, afirmauri.RemoteBatchCommand{
		Session: domain.ExchangeSession{
			RequestID:        "req-batch-v1",
			RetrieveEndpoint: srv.URL + "/store",
			UploadEndpoint:   srv.URL + "/store",
			State:            domain.SessionActive,
		},
		Payload:          rawBatch,
		IsJSONBatch:      true,
		PreSignEndpoint:  srv.URL + "/pre",
		PostSignEndpoint: srv.URL + "/post",
	})
	if err != nil {
		t.Fatalf("ExecuteLegacyRemote: %v", err)
	}
	if !waitVisto || !preVisto || !postVisto || !uploadVisto {
		t.Fatalf("flujo batch remoto incompleto: wait=%v pre=%v post=%v upload=%v", waitVisto, preVisto, postVisto, uploadVisto)
	}
}

func TestExecuteLegacyRemoteResult_WebSocketDirectoNoExigeSesionDeIntercambio(t *testing.T) {
	t.Parallel()

	rawBatch := []byte(`<?xml version="1.0" encoding="UTF-8"?><signbatch stoponerror="true" algorithm="SHA512withRSA"><singlesign Id="doc-1"><datasource>token-remoto</datasource><format>XAdES</format><suboperation>sign</suboperation></singlesign></signbatch>`)
	postResult := []byte(`<?xml version="1.0" encoding="UTF-8"?><signs><signresult id="doc-1" result="DONE_AND_SAVED"/></signs>`)

	var (
		preVisto  bool
		postVisto bool
		waitVisto bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pre":
			preVisto = true
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><xml><firmas><firma Id="doc-1"><param n="PRE">` + base64.StdEncoding.EncodeToString([]byte("digest-batch")) + `</param></firma></firmas></xml>`))
		case "/post":
			postVisto = true
			_, _ = w.Write(postResult)
		case "/store":
			waitVisto = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	batch := NewBatch(executor)
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{
		chain: [][]byte{[]byte("cert-der-hoja")},
	})

	result, err := batch.ExecuteLegacyRemoteResult(ctx, afirmauri.RemoteBatchCommand{
		Payload:          rawBatch,
		IsJSONBatch:      false,
		PreSignEndpoint:  srv.URL + "/pre",
		PostSignEndpoint: srv.URL + "/post",
		NeedCert:         true,
	})
	if err != nil {
		t.Fatalf("ExecuteLegacyRemoteResult: %v", err)
	}
	if !preVisto || !postVisto {
		t.Fatalf("flujo directo incompleto: pre=%v post=%v", preVisto, postVisto)
	}
	if waitVisto {
		t.Fatal("el flujo websocket directo no debe llamar a wait/upload")
	}
	parts := strings.Split(result, "|")
	if len(parts) != 2 {
		t.Fatalf("resultado=%q, want batch|cert", result)
	}
	decodedBatch, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("batch no es base64 estándar: %v", err)
	}
	if string(decodedBatch) != string(postResult) {
		t.Fatalf("batch devuelto inesperado: %s", string(decodedBatch))
	}
	decodedCert, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("cert no es base64 estándar: %v", err)
	}
	if string(decodedCert) != "cert-der-hoja" {
		t.Fatalf("cert devuelto inesperado: %q", string(decodedCert))
	}
}

func TestSignLegacyBatchTriphase_NormalizaIDComoV1(t *testing.T) {
	t.Parallel()

	td, err := signLegacyBatchTriphase(&legacyTriphaseDataResponse{
		Format: "CAdES",
		SignInfo: []legacyTriphaseSignInfo{
			{
				ID:     "",
				SignID: "sign-1",
				Params: map[string]string{
					"PRE": base64.StdEncoding.EncodeToString([]byte("digest-batch")),
				},
			},
			{
				ID:     "",
				SignID: "",
				Params: map[string]string{
					"ID":  "param-id-2",
					"PRE": base64.StdEncoding.EncodeToString([]byte("digest-batch-2")),
				},
			},
		},
	}, &mockLegacyBatchKey{}, "SHA256withRSA", nil, nil)
	if err != nil {
		t.Fatalf("signLegacyBatchTriphase: %v", err)
	}
	if len(td.SignInfo) != 2 {
		t.Fatalf("signinfo=%d, want 2", len(td.SignInfo))
	}
	if got := td.SignInfo[0].ID; got != "sign-1" {
		t.Fatalf("ID[0]=%q, want sign-1", got)
	}
	if got := td.SignInfo[0].SignID; got != "sign-1" {
		t.Fatalf("SignID[0]=%q, want sign-1", got)
	}
	if got := td.SignInfo[1].ID; got != "param-id-2" {
		t.Fatalf("ID[1]=%q, want param-id-2", got)
	}
	if got := td.SignInfo[1].SignID; got != "" {
		t.Fatalf("SignID[1]=%q, want empty", got)
	}
}

func TestLegacyBatchCertVariants_PriorizaHojaSola(t *testing.T) {
	t.Parallel()

	variants := legacyBatchCertVariants([][]byte{
		[]byte("cert-hoja"),
		[]byte("cert-ca"),
	})
	if len(variants) < 2 {
		t.Fatalf("variants=%d, want >=2", len(variants))
	}

	hojaURL := base64.URLEncoding.EncodeToString([]byte("cert-hoja"))
	hojaSTD := base64.StdEncoding.EncodeToString([]byte("cert-hoja"))
	cadenaURL := hojaURL + ";" + base64.URLEncoding.EncodeToString([]byte("cert-ca"))

	if got := variants[0]; got != hojaURL {
		t.Fatalf("variant[0]=%q, want hoja urlsafe %q", got, hojaURL)
	}
	if hojaSTD != hojaURL && len(variants) > 1 {
		if got := variants[1]; got != hojaSTD {
			t.Fatalf("variant[1]=%q, want hoja std %q", got, hojaSTD)
		}
	}
	encontroCadena := false
	for _, variant := range variants[1:] {
		if variant == cadenaURL {
			encontroCadena = true
			break
		}
	}
	if !encontroCadena {
		t.Fatalf("no se encontró la variante de cadena urlsafe %q en %v", cadenaURL, variants)
	}
}

func TestShouldFallbackLegacyBatchFormStatus_Admite414(t *testing.T) {
	t.Parallel()

	if !shouldFallbackLegacyBatchFormStatus(http.StatusRequestURITooLong) {
		t.Fatal("414 debe forzar fallback a form-body")
	}
	if !shouldFallbackLegacyBatchFormStatus(http.StatusBadRequest) {
		t.Fatal("400 genérico (p. ej. página Tomcat sin detalle) debe forzar fallback a form-body")
	}
	if shouldFallbackLegacyBatchFormStatus(http.StatusInternalServerError) {
		t.Fatal("500 no debe activar fallback silencioso")
	}
	if shouldFallbackLegacyBatchFormStatus(http.StatusForbidden) {
		t.Fatal("403 no debe activar fallback")
	}
}

// Reproduce el presignador de JCyL: rechaza con un 400 genérico la query en
// POST y el relleno Base64 escapado, y solo acepta el cuerpo en crudo.
func TestLegacyBatchPostURL_ServidorEstiloJCyLAceptaCuerpoCrudo(t *testing.T) {
	t.Parallel()

	const valor = "PD94bWw-_=="
	peticiones := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones++
		raw, _ := io.ReadAll(r.Body)
		if r.URL.RawQuery != "" || string(raw) != "xml="+valor {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("<html><h1>HTTP Status 400 – Bad Request</h1></html>"))
			return
		}
		_, _ = w.Write([]byte("<xml>OK</xml>"))
	}))
	defer srv.Close()

	executor := New(srv.Client())
	variante := legacyBatchParamEncodingVariants()[0]
	u, err := buildLegacyBatchURL(srv.URL+"/BatchPresigner", [][2]string{{"xml", valor}}, variante.Encode)
	if err != nil {
		t.Fatal(err)
	}
	body, err := executor.legacyBatchPostURL(context.Background(), u)
	if err != nil {
		t.Fatalf("legacyBatchPostURL: %v", err)
	}
	if got := string(body); got != "<xml>OK</xml>" {
		t.Fatalf("body=%q", got)
	}
	if peticiones != 2 {
		t.Fatalf("peticiones=%d, want 2 (query y cuerpo crudo)", peticiones)
	}
}

func TestLegacyBatchPostPrepared_ReintentaFormBodyAnteErrorDeProtocoloEnQuery(t *testing.T) {
	t.Parallel()

	var (
		queryVisto bool
		formVisto  bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.RawQuery != "":
			queryVisto = true
			w.Header().Set("Connection", "close")
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("el servidor de prueba no soporta hijack")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_, _ = io.WriteString(conn, "garbage")
			_ = conn.Close()
		default:
			formVisto = true
			if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/x-www-form-urlencoded") {
				t.Fatalf("content-type=%q, want form-urlencoded", got)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if got := r.Form.Get("json"); got != "{\"ok\":true}" {
				t.Fatalf("json=%q, want %q", got, "{\"ok\":true}")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK-FORM"))
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	body, err := executor.legacyBatchPostPrepared(
		context.Background(),
		srv.URL+"/presign?json=%7B%22ok%22%3Atrue%7D",
		srv.URL+"/presign",
		"json=%7B%22ok%22%3Atrue%7D",
	)
	if err != nil {
		t.Fatalf("legacyBatchPostPrepared: error inesperado: %v", err)
	}
	if !queryVisto {
		t.Fatal("no se ejecutó el intento por query-string")
	}
	if !formVisto {
		t.Fatal("no se ejecutó el fallback por form-body")
	}
	if got := strings.TrimSpace(string(body)); got != "OK-FORM" {
		t.Fatalf("body=%q, want %q", got, "OK-FORM")
	}
}

func TestLegacyBatchPostPrepared_ReintentaAnteConexionCerrada(t *testing.T) {
	t.Parallel()

	intentos := 0
	executor := New(&http.Client{})
	executor.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			intentos++
			if intentos == 1 {
				return nil, fmt.Errorf("write tcp 127.0.0.1:12345->127.0.0.1:443: use of closed network connection")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("OK-RETRY")),
				Request:    req,
			}, nil
		}),
	}
	executor.fallbackHTTPClient = nil

	body, err := executor.legacyBatchPostPrepared(
		context.Background(),
		"https://example.test/postsign?json=%7B%22ok%22%3Atrue%7D",
		"https://example.test/postsign",
		"json=%7B%22ok%22%3Atrue%7D",
	)
	if err != nil {
		t.Fatalf("legacyBatchPostPrepared: error inesperado: %v", err)
	}
	if intentos != 2 {
		t.Fatalf("intentos=%d, want 2", intentos)
	}
	if got := strings.TrimSpace(string(body)); got != "OK-RETRY" {
		t.Fatalf("body=%q, want %q", got, "OK-RETRY")
	}
}

func TestSignLegacyBatchTriphase_MezclaExtraParamsGlobalesYPorFirma(t *testing.T) {
	t.Parallel()

	key := &mockLegacyBatchPreKey{}
	td, err := signLegacyBatchTriphase(&legacyTriphaseDataResponse{
		Format: "CAdES",
		SignInfo: []legacyTriphaseSignInfo{
			{
				ID: "doc-1",
				Params: map[string]string{
					"PRE": base64.StdEncoding.EncodeToString([]byte("digest-batch")),
				},
			},
		},
	}, key, "SHA256withRSA",
		map[string]string{"mode": "implicit", "profile": "T"},
		map[string]map[string]string{"doc-1": {"profile": "B", "policy": "demo"}},
	)
	if err != nil {
		t.Fatalf("signLegacyBatchTriphase: %v", err)
	}
	if len(td.SignInfo) != 1 {
		t.Fatalf("signinfo=%d, want 1", len(td.SignInfo))
	}
	if got := td.SignInfo[0].Params["PK1"]; got == "" {
		t.Fatal("PK1 vacío")
	}
	if len(key.opts) != 1 {
		t.Fatalf("opts registradas=%d, want 1", len(key.opts))
	}
	if got := key.opts[0]["mode"]; got != "implicit" {
		t.Fatalf("mode=%q, want implicit", got)
	}
	if got := key.opts[0]["profile"]; got != "B" {
		t.Fatalf("profile=%q, want B", got)
	}
	if got := key.opts[0]["policy"]; got != "demo" {
		t.Fatalf("policy=%q, want demo", got)
	}
}

func TestMergeLegacyPresignResults_LimpiaCamposLegacyComoV1(t *testing.T) {
	t.Parallel()

	rawBatch := []byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc-1","datareference":"token-remoto","format":"XAdES","suboperation":"cosign","extraparams":"abc=1"}]}`)
	updated, err := mergeLegacyPresignResults(rawBatch, []legacyBatchSingleResult{
		{ID: "doc-1", Result: "DONE_AND_SAVED", Description: "ok"},
	})
	if err != nil {
		t.Fatalf("mergeLegacyPresignResults: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(updated, &root); err != nil {
		t.Fatalf("json resultado inválido: %v", err)
	}
	signs, ok := root["singlesigns"].([]any)
	if !ok || len(signs) != 1 {
		t.Fatalf("singlesigns inesperado: %#v", root["singlesigns"])
	}
	sign, ok := signs[0].(map[string]any)
	if !ok {
		t.Fatalf("firma inesperada: %#v", signs[0])
	}
	for _, field := range []string{"datareference", "format", "suboperation", "extraparams"} {
		if _, exists := sign[field]; exists {
			t.Fatalf("el campo %q debía eliminarse en el merge legacy: %#v", field, sign)
		}
	}
	if got := sign["result"]; got != "DONE_AND_SAVED" {
		t.Fatalf("result inesperado: %#v", got)
	}
	if got := sign["description"]; got != "ok" {
		t.Fatalf("description inesperada: %#v", got)
	}
}

func TestExecuteLegacyRemote_ConClaveSesionCifraRespuestaLegacy(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)

	rawBatch := []byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc-1","datareference":"token-remoto"}]}`)
	preHashB64 := base64.StdEncoding.EncodeToString([]byte("digest-batch"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pre":
			_ = json.NewEncoder(w).Encode(legacyBatchPreResponse{
				TD: &legacyTriphaseDataResponse{
					Format: "CAdES",
					SignInfo: []legacyTriphaseSignInfo{
						{ID: "doc-1", Params: map[string]string{"PRE": preHashB64}},
					},
				},
			})
		case "/post":
			_, _ = w.Write([]byte(`{"signs":[{"id":"doc-1","result":"DONE_AND_SAVED"}]}`))
		case "/store":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload): %v", err)
			}
			if r.Form.Get("dat") == "#WAIT" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
				return
			}
			dat := r.Form.Get("dat")
			if !strings.Contains(dat, ".") {
				t.Fatalf("se esperaba formato legacy padding.payload, obtenido %q", dat)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("SAVE_OK"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	batch := NewBatch(executor)
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{
		chain: [][]byte{[]byte("cert-der-hoja")},
	})

	err := batch.ExecuteLegacyRemote(ctx, afirmauri.RemoteBatchCommand{
		Session: domain.ExchangeSession{
			RequestID:        "req-batch-key",
			SessionKey:       "abcd",
			RetrieveEndpoint: srv.URL + "/store",
			UploadEndpoint:   srv.URL + "/store",
			State:            domain.SessionActive,
		},
		Payload:          rawBatch,
		IsJSONBatch:      true,
		PreSignEndpoint:  srv.URL + "/pre",
		PostSignEndpoint: srv.URL + "/post",
		NeedCert:         true,
	})
	if err != nil {
		t.Fatalf("ExecuteLegacyRemote con clave de sesión: %v", err)
	}
}

func TestExecuteLegacyRemote_UploadFallbackLegacyCuandoJavaStyleDevuelveNoOK(t *testing.T) {
	t.Parallel()

	rawBatch := []byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc-1","datareference":"token-remoto"}]}`)
	preHashB64 := base64.StdEncoding.EncodeToString([]byte("digest-batch"))

	var (
		intentosUpload int
		vioLegacyAlias bool
		storeURL       string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pre":
			_ = json.NewEncoder(w).Encode(legacyBatchPreResponse{
				TD: &legacyTriphaseDataResponse{
					Format: "CAdES",
					SignInfo: []legacyTriphaseSignInfo{
						{ID: "doc-1", Params: map[string]string{"PRE": preHashB64}},
					},
				},
			})
		case "/post":
			_, _ = w.Write([]byte(`{"signs":[{"id":"doc-1","result":"DONE_AND_SAVED"}]}`))
		case "/store":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload): %v", err)
			}
			if r.Form.Get("dat") == "#WAIT" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
				return
			}
			intentosUpload++
			if intentosUpload < 3 {
				if got := r.Form.Get("op"); got != "put" {
					t.Fatalf("op upload = %q", got)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("AUTHENTICATION_ERROR"))
				return
			}
			if got := r.Form.Get("locale"); got != "es" {
				t.Fatalf("locale legacy = %q", got)
			}
			if got := r.Form.Get("customToken"); got != "abc123" {
				t.Fatalf("customToken legacy = %q", got)
			}
			if got := r.Form.Get("storageservlet"); got != storeURL {
				t.Fatalf("storageservlet legacy = %q", got)
			}
			vioLegacyAlias = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("SAVE_OK"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	storeURL = srv.URL + "/store"

	executor := New(srv.Client())
	batch := NewBatch(executor)
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{
		chain: [][]byte{[]byte("cert-der-hoja")},
	})

	err := batch.ExecuteLegacyRemote(ctx, afirmauri.RemoteBatchCommand{
		Session: domain.ExchangeSession{
			RequestID:        "req-batch-fallback",
			RetrieveEndpoint: storeURL,
			UploadEndpoint:   storeURL,
			State:            domain.SessionActive,
		},
		Payload:          rawBatch,
		IsJSONBatch:      true,
		PreSignEndpoint:  srv.URL + "/pre",
		PostSignEndpoint: srv.URL + "/post",
		LegacyParams: url.Values{
			"locale":         []string{"es"},
			"customToken":    []string{"abc123"},
			"storageservlet": []string{storeURL},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteLegacyRemote con fallback legacy: %v", err)
	}
	if intentosUpload != 3 {
		t.Fatalf("intentos upload = %d, want 3", intentosUpload)
	}
	if !vioLegacyAlias {
		t.Fatal("no se ejecuto el fallback legacy con parametros originales")
	}
}

func TestExecuteLegacyRemote_PriorizaUploadJavaPorQuerySinKey(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)

	rawBatch := []byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc-1","datareference":"token-remoto"}]}`)
	preHashB64 := base64.StdEncoding.EncodeToString([]byte("digest-batch"))

	var (
		uploadVisto bool
		queryURL    *url.URL
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pre":
			_ = json.NewEncoder(w).Encode(legacyBatchPreResponse{
				TD: &legacyTriphaseDataResponse{
					Format: "CAdES",
					SignInfo: []legacyTriphaseSignInfo{
						{ID: "doc-1", Params: map[string]string{"PRE": preHashB64}},
					},
				},
			})
		case "/post":
			_, _ = w.Write([]byte(`{"signs":[{"id":"doc-1","result":"DONE_AND_SAVED"}]}`))
		case "/store":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload): %v", err)
			}
			if r.Form.Get("dat") == "#WAIT" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
				return
			}
			uploadVisto = true
			queryURL = r.URL
			if got := r.URL.Query().Get("op"); got != "put" {
				t.Fatalf("op upload query = %q", got)
			}
			if got := r.URL.Query().Get("id"); got != "req-batch-query" {
				t.Fatalf("id upload query = %q", got)
			}
			if got := r.URL.Query().Get("key"); got != "" {
				t.Fatalf("key upload query = %q, want empty", got)
			}
			if got := r.Header.Get("Content-Type"); got != "" {
				t.Fatalf("content-type upload query = %q, want empty", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("SAVE_OK"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	batch := NewBatch(executor)
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{
		chain: [][]byte{[]byte("cert-der-hoja")},
	})

	err := batch.ExecuteLegacyRemote(ctx, afirmauri.RemoteBatchCommand{
		Session: domain.ExchangeSession{
			RequestID:        "req-batch-query",
			SessionKey:       "abcd",
			RetrieveEndpoint: srv.URL + "/store",
			UploadEndpoint:   srv.URL + "/store",
			State:            domain.SessionActive,
		},
		Payload:          rawBatch,
		IsJSONBatch:      true,
		PreSignEndpoint:  srv.URL + "/pre",
		PostSignEndpoint: srv.URL + "/post",
	})
	if err != nil {
		t.Fatalf("ExecuteLegacyRemote upload query: %v", err)
	}
	if !uploadVisto {
		t.Fatal("no se ejecutó el upload por query")
	}
	if queryURL == nil || queryURL.RawQuery == "" {
		t.Fatal("el upload no se realizó por query string")
	}
}

func TestExecuteLegacyRemote_MantieneWaitPeriodicoMientrasProcesa(t *testing.T) {
	rawBatch := []byte(`{"format":"CAdES","algorithm":"SHA256withRSA","singlesigns":[{"id":"doc-1","datareference":"token-remoto"}]}`)
	preHashB64 := base64.StdEncoding.EncodeToString([]byte("digest-batch"))

	originalInterval := legacyBatchWaitInterval
	legacyBatchWaitInterval = 5 * time.Millisecond
	defer func() { legacyBatchWaitInterval = originalInterval }()

	var waitCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pre":
			time.Sleep(25 * time.Millisecond)
			_ = json.NewEncoder(w).Encode(legacyBatchPreResponse{
				TD: &legacyTriphaseDataResponse{
					Format: "CAdES",
					SignInfo: []legacyTriphaseSignInfo{
						{ID: "doc-1", Params: map[string]string{"PRE": preHashB64}},
					},
				},
			})
		case "/post":
			_, _ = w.Write([]byte(`{"signs":[{"id":"doc-1","result":"DONE_AND_SAVED"}]}`))
		case "/store":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm(upload): %v", err)
			}
			if r.Form.Get("dat") == "#WAIT" {
				waitCount++
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("SAVE_OK"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	batch := NewBatch(executor)
	ctx := ContextWithSigningKey(context.Background(), &mockLegacyBatchKey{
		chain: [][]byte{[]byte("cert-der-hoja")},
	})

	err := batch.ExecuteLegacyRemote(ctx, afirmauri.RemoteBatchCommand{
		Session: domain.ExchangeSession{
			RequestID:        "req-batch-wait-loop",
			RetrieveEndpoint: srv.URL + "/store",
			UploadEndpoint:   srv.URL + "/store",
			State:            domain.SessionActive,
		},
		Payload:          rawBatch,
		IsJSONBatch:      true,
		PreSignEndpoint:  srv.URL + "/pre",
		PostSignEndpoint: srv.URL + "/post",
	})
	if err != nil {
		t.Fatalf("ExecuteLegacyRemote con wait periodico: %v", err)
	}
	if waitCount < 2 {
		t.Fatalf("waitCount = %d, want >= 2", waitCount)
	}
}
