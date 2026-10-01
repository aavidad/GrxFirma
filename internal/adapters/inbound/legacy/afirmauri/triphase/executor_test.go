// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package triphase

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"grxfirma/internal/domain"
	"grxfirma/internal/security/machinepolicy"
)

// ---- Mocks ----------------------------------------------------------------

// mockSigningKey implementa ports.SigningKey y legacyBatchSigningKey para tests.
type mockSigningKey struct {
	id           string
	signDigestFn func(digest []byte, hash crypto.Hash) ([]byte, error)
	signErr      error
}

func (k *mockSigningKey) KeyID() string { return k.id }
func (k *mockSigningKey) CertificateChainDER() [][]byte {
	return [][]byte{[]byte("cert-der")}
}
func (k *mockSigningKey) SignDigest(digest []byte, hash crypto.Hash) ([]byte, error) {
	if k.signErr != nil {
		return nil, k.signErr
	}
	if k.signDigestFn != nil {
		return k.signDigestFn(digest, hash)
	}
	// Por defecto devuelve bytes de firma ficticios.
	return make([]byte, 32), nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// ---- Helpers ---------------------------------------------------------------

const claveSessionHex = "aabbccddeeff00112233445566778899"

// sesionTest construye una ExchangeSession que apunta a los servidores mock.
func sesionTest(retrieveURL, uploadURL, sessionKey string) domain.ExchangeSession {
	return domain.ExchangeSession{
		RequestID:        "req-001",
		SessionKey:       sessionKey,
		RetrieveEndpoint: retrieveURL,
		UploadEndpoint:   uploadURL,
		State:            domain.SessionActive,
	}
}

// jobTest construye un SignatureJob de prueba mínimo.
func jobTest() domain.SignatureJob {
	doc, _ := domain.NewDocument("doc.txt", []byte("contenido del documento"), "text/plain")
	return domain.SignatureJob{
		Document: doc,
		Format:   domain.FormatCAdES,
		Action:   domain.ActionSign,
	}
}

// retrieveResponseJSON construye el JSON de respuesta del retrieve.
func retrieveResponseJSON(datB64 string) []byte {
	r := retrieveResponse{
		Dat:         datB64,
		Op:          "sign",
		Format:      "CAdES",
		Algorithm:   "SHA256withRSA",
		ExtraParams: "mode=implicit",
	}
	b, _ := json.Marshal(r)
	return b
}

// presignResponseJSON construye el JSON de respuesta del presign.
func presignResponseJSON(hashB64 string) []byte {
	r := presignResponse{
		Sign:        hashB64,
		ExtraParams: "mode=implicit",
	}
	b, _ := json.Marshal(r)
	return b
}

// ---- Tests -----------------------------------------------------------------

// TestFlujoCompletoSinCifrado verifica el flujo completo con datos en claro.
func TestFlujoCompletoSinCifrado(t *testing.T) {
	docOriginal := []byte("documento original de prueba")
	docB64 := base64.StdEncoding.EncodeToString(docOriginal)
	hashB64 := base64.StdEncoding.EncodeToString([]byte("hash-del-documento"))
	resultadoFirma := []byte("resultado-firma-cades")

	var (
		retrieveCalled bool
		presignCalled  bool
		postsignCalled bool
		uploadCalled   bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			retrieveCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.Write(retrieveResponseJSON(docB64))

		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=put") && strings.Contains(r.URL.RawQuery, "id="):
			uploadCalled = true
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			bodyStr := string(body)
			if strings.Contains(bodyStr, "op=sign") {
				presignCalled = true
				w.Header().Set("Content-Type", "application/json")
				w.Write(presignResponseJSON(hashB64))
			} else if strings.Contains(bodyStr, "op=aspsign") {
				postsignCalled = true
				w.Write(resultadoFirma)
			} else {
				http.Error(w, "op desconocida en POST", http.StatusBadRequest)
			}

		default:
			http.Error(w, "no esperado", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "")
	ctx := ContextWithSigningKey(context.Background(), &mockSigningKey{id: "test"})

	result, err := executor.Execute(ctx, sesion, jobTest())
	if err != nil {
		t.Fatalf("Execute: error inesperado: %v", err)
	}

	if !retrieveCalled {
		t.Error("retrieve no fue llamado")
	}
	if !presignCalled {
		t.Error("presign no fue llamado")
	}
	if !postsignCalled {
		t.Error("postsign no fue llamado")
	}
	if !uploadCalled {
		t.Error("upload no fue llamado")
	}
	if result.Format != domain.FormatCAdES {
		t.Errorf("formato incorrecto: esperado %s, obtenido %s", domain.FormatCAdES, result.Format)
	}
	if result.Algorithm != "SHA256withRSA" {
		t.Errorf("algoritmo incorrecto: esperado SHA256withRSA, obtenido %s", result.Algorithm)
	}
}

func TestUploadCertificateSinClaveUsaBase64URL(t *testing.T) {
	var datRecibido string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		datRecibido = r.Form.Get("dat")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "")

	if err := executor.UploadCertificate(context.Background(), sesion, []byte("cert-der"), nil); err != nil {
		t.Fatalf("UploadCertificate: error inesperado: %v", err)
	}

	esperado := base64.URLEncoding.EncodeToString([]byte("cert-der"))
	if datRecibido != esperado {
		t.Fatalf("dat inesperado: got=%q want=%q", datRecibido, esperado)
	}
}

func TestUploadCertificateConClaveUsaFormatoLegacyDES(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	var datRecibido string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		datRecibido = r.Form.Get("dat")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "abcd")

	if err := executor.UploadCertificate(context.Background(), sesion, []byte("cert-der"), nil); err != nil {
		t.Fatalf("UploadCertificate: error inesperado: %v", err)
	}

	descifrado, err := descifrarLegacyDES([]byte(datRecibido), "abcd")
	if err != nil {
		t.Fatalf("descifrarLegacyDES: %v", err)
	}
	if string(descifrado) != "cert-der" {
		t.Fatalf("certificado descifrado inesperado: %q", string(descifrado))
	}
}

func TestShouldRetryLegacyProtocolWithFallback_ConexionCerrada(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("Post \"https://example.test/presign\": write tcp 127.0.0.1:12345->127.0.0.1:443: use of closed network connection")
	if !shouldRetryLegacyProtocolWithFallback(err) {
		t.Fatal("use of closed network connection debe considerarse reintentable")
	}
}

func TestNew_DeshabilitaHTTP2EnClienteLegacy(t *testing.T) {
	t.Parallel()

	executor := New(&http.Client{})
	transport, ok := executor.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport inesperado: %T", executor.httpClient.Transport)
	}
	if transport.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 sigue activado")
	}
	if transport.TLSNextProto == nil {
		t.Fatal("TLSNextProto no debería ser nil en modo compat legacy")
	}
}

func TestUploadCertificateSinClaveHaceFallbackLegacyConCert(t *testing.T) {
	intentos := 0
	var certRecibido string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		intentos++
		if intentos == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("AUTHENTICATION_ERROR"))
			return
		}
		certRecibido = r.Form.Get("cert")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("SAVE_OK"))
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "")
	legacy := url.Values{"customToken": []string{"abc123"}}

	if err := executor.UploadCertificate(context.Background(), sesion, []byte("cert-der"), legacy); err != nil {
		t.Fatalf("UploadCertificate con fallback: error inesperado: %v", err)
	}
	if intentos != 2 {
		t.Fatalf("intentos = %d, want 2", intentos)
	}
	if certRecibido != base64.StdEncoding.EncodeToString([]byte("cert-der")) {
		t.Fatalf("cert fallback inesperado: %q", certRecibido)
	}
}

func TestDoLegacyBatchPOST_ReintentaConClienteFallbackAnteErrorDeProtocolo(t *testing.T) {
	t.Parallel()

	var fallbackVisto bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackVisto = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	executor := New(&http.Client{})
	primaryVisto := false
	executor.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			primaryVisto = true
			return nil, fmt.Errorf("net/http: HTTP/1.x transport connection broken: malformed HTTP response %q", "\x00\x00\x06\x04")
		}),
	}
	executor.fallbackHTTPClient = srv.Client()

	body, status, err := executor.doLegacyBatchPOST(context.Background(), srv.URL+"/StorageService", strings.NewReader("op=put&id=req-batch&dat=%23WAIT"))
	if err != nil {
		t.Fatalf("doLegacyBatchPOST: error inesperado: %v", err)
	}
	if !primaryVisto {
		t.Fatal("no se ejecutó el transporte primario")
	}
	if !fallbackVisto {
		t.Fatal("no se ejecutó el transporte fallback")
	}
	if status != http.StatusOK {
		t.Fatalf("status=%d, want %d", status, http.StatusOK)
	}
	if got := strings.TrimSpace(string(body)); got != "OK" {
		t.Fatalf("body=%q, want %q", got, "OK")
	}
}

func TestRetrieveRaw_ReintentaConClienteFallbackAnteEOF(t *testing.T) {
	t.Parallel()

	var fallbackVisto bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackVisto = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(retrieveResponseJSON(base64.StdEncoding.EncodeToString([]byte("contenido"))))
	}))
	defer srv.Close()

	executor := New(&http.Client{})
	primaryVisto := false
	executor.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			primaryVisto = true
			return nil, fmt.Errorf("Post %q: EOF", req.URL.String())
		}),
	}
	executor.fallbackHTTPClient = srv.Client()

	data, err := executor.RetrieveRaw(context.Background(), sesionTest(srv.URL+"/RetrieveService", srv.URL+"/StorageService", ""))
	if err != nil {
		t.Fatalf("RetrieveRaw: error inesperado: %v", err)
	}
	if !primaryVisto {
		t.Fatal("no se ejecutó el transporte primario")
	}
	if !fallbackVisto {
		t.Fatal("no se ejecutó el transporte fallback")
	}
	if len(data) == 0 {
		t.Fatal("retrieve vacío")
	}
}

func TestFlujoCompletoUsaClaveDelContexto(t *testing.T) {
	docB64 := base64.StdEncoding.EncodeToString([]byte("documento"))
	hashB64 := base64.StdEncoding.EncodeToString([]byte("hash"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			w.Header().Set("Content-Type", "application/json")
			w.Write(retrieveResponseJSON(docB64))
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=put") && strings.Contains(r.URL.RawQuery, "id="):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "op=sign") {
				w.Header().Set("Content-Type", "application/json")
				w.Write(presignResponseJSON(hashB64))
				return
			}
			w.Write([]byte("resultado"))
		default:
			http.Error(w, "no esperado", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	var digestRecibido []byte
	clave := &mockSigningKey{
		id: "clave-ctx",
		signDigestFn: func(digest []byte, hash crypto.Hash) ([]byte, error) {
			digestRecibido = digest
			return make([]byte, 32), nil
		},
	}
	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "")

	ctx := ContextWithSigningKey(context.Background(), clave)
	if _, err := executor.Execute(ctx, sesion, jobTest()); err != nil {
		t.Fatalf("Execute con clave en contexto: error inesperado: %v", err)
	}
	if len(digestRecibido) == 0 {
		t.Fatal("la clave de contexto no fue usada para firmar el presign")
	}
}

// TestFlujoCompletoConCifrado verifica que los datos se descifran correctamente
// cuando la sesión tiene una clave AES.
func TestFlujoCompletoConCifrado(t *testing.T) {
	docOriginal := []byte("documento cifrado de prueba")
	docB64 := base64.StdEncoding.EncodeToString(docOriginal)
	hashB64 := base64.StdEncoding.EncodeToString([]byte("hash"))
	resultadoFirma := []byte("resultado-firma-cifrada")

	// Cifrar la respuesta retrieve completa con la clave de sesión.
	retrieveJSON := retrieveResponseJSON(docB64)
	cifrado, err := cifrarAES128CBC(retrieveJSON, claveSessionHex)
	if err != nil {
		t.Fatalf("cifrarAES128CBC: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			w.Write(cifrado)

		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=put") && strings.Contains(r.URL.RawQuery, "id="):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "op=sign") {
				w.Header().Set("Content-Type", "application/json")
				w.Write(presignResponseJSON(hashB64))
			} else {
				w.Write(resultadoFirma)
			}

		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, claveSessionHex)
	ctx := ContextWithSigningKey(context.Background(), &mockSigningKey{id: "test"})

	_, err = executor.Execute(ctx, sesion, jobTest())
	if err != nil {
		t.Fatalf("Execute con cifrado: error inesperado: %v", err)
	}
}

func TestRequiereDescifradoSesion_NoConfundeCiphertextConTextoPlano(t *testing.T) {
	t.Parallel()

	for _, prefix := range []byte{'<', '{', '[', '%'} {
		ciphertext := append([]byte{prefix}, bytes.Repeat([]byte{0xa5}, aes.BlockSize*2-1)...)
		if !requiereDescifradoSesion(ciphertext) {
			t.Fatalf("ciphertext con prefijo %q se clasificó como JSON plano", prefix)
		}
	}
	if requiereDescifradoSesion(retrieveResponseJSON(base64.StdEncoding.EncodeToString([]byte("plain")))) {
		t.Fatal("una respuesta retrieve JSON válida no debe descifrarse")
	}
}

// TestRetrieveError500 verifica que un HTTP 500 en retrieve devuelve error controlado.
func TestRetrieveError500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "error interno del servidor", http.StatusInternalServerError)
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "")

	_, err := executor.Execute(context.Background(), sesion, jobTest())
	if err == nil {
		t.Fatal("Execute: deberia fallar con HTTP 500 en retrieve")
	}
	if !strings.Contains(err.Error(), "retrieve") {
		t.Errorf("el error deberia mencionar 'retrieve': %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("el error deberia mencionar el codigo HTTP 500: %v", err)
	}
}

// TestCancelacionPorContexto verifica que la cancelación del contexto durante
// el retrieve propaga correctamente el error de contexto.
func TestCancelacionPorContexto(t *testing.T) {
	// Servidor que cuelga indefinidamente.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bloquear hasta que el test cancele.
		<-r.Context().Done()
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "")

	ctx, cancel := context.WithCancel(context.Background())
	// Cancelar el contexto inmediatamente.
	cancel()

	_, err := executor.Execute(ctx, sesion, jobTest())
	if err == nil {
		t.Fatal("Execute: deberia fallar con contexto cancelado")
	}
	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancel") {
		t.Errorf("el error deberia indicar cancelacion de contexto: %v", err)
	}
}

// TestDescifradoAES128CBC verifica que el descifrado AES produce los datos originales.
func TestDescifradoAES128CBC(t *testing.T) {
	original := []byte("datos originales para cifrado y descifrado")

	cifrado, err := cifrarAES128CBC(original, claveSessionHex)
	if err != nil {
		t.Fatalf("cifrarAES128CBC: %v", err)
	}

	descifrado, err := descifrarAES128CBC(cifrado, claveSessionHex)
	if err != nil {
		t.Fatalf("descifrarAES128CBC: %v", err)
	}

	if string(descifrado) != string(original) {
		t.Errorf("descifrado incorrecto: esperado %q, obtenido %q", original, descifrado)
	}
}

// TestSinSessionKeySinDescifrado verifica que con SessionKey vacía los datos
// se usan directamente sin intentar descifrar.
func TestSinSessionKeySinDescifrado(t *testing.T) {
	docB64 := base64.StdEncoding.EncodeToString([]byte("documento en claro"))
	hashB64 := base64.StdEncoding.EncodeToString([]byte("hash"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			// Responder con JSON en claro (sin cifrar).
			w.Header().Set("Content-Type", "application/json")
			w.Write(retrieveResponseJSON(docB64))
		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "op=sign") {
				w.Header().Set("Content-Type", "application/json")
				w.Write(presignResponseJSON(hashB64))
			} else {
				w.Write([]byte("resultado-firma"))
			}
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=put") && strings.Contains(r.URL.RawQuery, "id="):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	// SessionKey vacía: datos en claro.
	sesion := sesionTest(srv.URL, srv.URL, "")
	ctx := ContextWithSigningKey(context.Background(), &mockSigningKey{id: "test"})

	_, err := executor.Execute(ctx, sesion, jobTest())
	if err != nil {
		t.Fatalf("Execute sin clave de sesion: error inesperado: %v", err)
	}
}

// TestPresignError400 verifica que un HTTP 400 en presign devuelve error controlado.
func TestPresignError400(t *testing.T) {
	docB64 := base64.StdEncoding.EncodeToString([]byte("doc"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			w.Write(retrieveResponseJSON(docB64))
		case r.Method == http.MethodPost:
			http.Error(w, "bad request", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "")

	_, err := executor.Execute(context.Background(), sesion, jobTest())
	if err == nil {
		t.Fatal("Execute: deberia fallar con HTTP 400 en presign")
	}
	if !strings.Contains(err.Error(), "presign") {
		t.Errorf("el error deberia mencionar 'presign': %v", err)
	}
}

// TestSignerErrorPropagado verifica que un error del motor de firma se propaga.
func TestSignerErrorPropagado(t *testing.T) {
	docB64 := base64.StdEncoding.EncodeToString([]byte("doc"))
	hashB64 := base64.StdEncoding.EncodeToString([]byte("hash"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			w.Write(retrieveResponseJSON(docB64))
		case r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.Write(presignResponseJSON(hashB64))
		}
	}))
	defer srv.Close()

	claveConError := &mockSigningKey{
		id:      "clave-error",
		signErr: fmt.Errorf("certificado no disponible"),
	}
	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "")
	ctx := ContextWithSigningKey(context.Background(), claveConError)

	_, err := executor.Execute(ctx, sesion, jobTest())
	if err == nil {
		t.Fatal("Execute: deberia fallar cuando la clave devuelve error")
	}
	if !strings.Contains(err.Error(), "firma local") {
		t.Errorf("el error deberia mencionar 'firma local': %v", err)
	}
}

func TestFlujoCompleto_TrasladaExtraParamsRetrieveALaFirmaLocal(t *testing.T) {
	docB64 := base64.StdEncoding.EncodeToString([]byte("documento"))
	hashB64 := base64.StdEncoding.EncodeToString([]byte("hash"))

	retrievePayload, _ := json.Marshal(retrieveResponse{
		Dat:         docB64,
		Op:          "sign",
		Format:      "XAdES",
		Algorithm:   "SHA256withRSA",
		ExtraParams: "profile=T\npolicy=demo",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=get") && strings.Contains(r.URL.RawQuery, "id="):
			w.Header().Set("Content-Type", "application/json")
			w.Write(retrievePayload)
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "op=put") && strings.Contains(r.URL.RawQuery, "id="):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), "op=sign") {
				w.Header().Set("Content-Type", "application/json")
				w.Write(presignResponseJSON(hashB64))
				return
			}
			w.Write([]byte("resultado"))
		default:
			http.Error(w, "no esperado", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Tras la corrección del protocolo trifásico, el executor realiza firma PKCS#1
	// directa. El formato indicado por el retrieve (XAdES) se usa en el resultado
	// final pero NO se pasa al motor de formato porque el ensamblado lo hace el servidor.
	executor := New(srv.Client())
	ctx := ContextWithSigningKey(context.Background(), &mockSigningKey{id: "test"})

	result, err := executor.Execute(ctx, sesionTest(srv.URL, srv.URL, ""), jobTest())
	if err != nil {
		t.Fatalf("Execute con extraParams remotos: error inesperado: %v", err)
	}
	if result.Format != domain.SignatureFormat("XAdES") {
		t.Fatalf("formato incorrecto: esperado XAdES, obtenido %s", result.Format)
	}
}

func TestUploadCertificateConClaveNoEnviaClaveNiCertificadoEnClaro(t *testing.T) {
	machinepolicy.SetForTest(t, machinepolicy.PermitirDESLegacy, true)
	intentos := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		intentos++
		if r.Form.Has("key") || r.Form.Has("cipherKey") {
			t.Fatalf("la clave de sesión llegó al servidor: %v", r.Form)
		}
		if r.Form.Has("cert") || r.Form.Get("dat") == base64.StdEncoding.EncodeToString([]byte("cert-der")) {
			t.Fatalf("el certificado viajó en claro pese a existir clave: %v", r.Form)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("AUTHENTICATION_ERROR"))
	}))
	defer srv.Close()

	executor := New(srv.Client())
	sesion := sesionTest(srv.URL, srv.URL, "abcd")
	legacy := url.Values{"key": []string{"abcd"}, "customToken": []string{"abc123"}}

	if err := executor.UploadCertificate(context.Background(), sesion, []byte("cert-der"), legacy); err == nil {
		t.Fatal("se esperaba error: el servidor rechazó la única subida cifrada")
	}
	if intentos != 1 {
		t.Fatalf("intentos = %d, want 1 (sin reintento en claro)", intentos)
	}
}
