// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package rest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"grxfirma/internal/adapters/outbound/common/localizador"
)

func TestCanonicalSecurityRoot_ResuelveAliasConDescendienteInexistente(t *testing.T) {
	realRoot := t.TempDir()
	aliasRoot := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(realRoot, aliasRoot); err != nil {
		t.Skipf("symlinks no disponibles: %v", err)
	}
	canonicalRealRoot, err := filepath.EvalSymlinks(realRoot)
	if err != nil {
		t.Fatalf("no se pudo resolver el directorio temporal real: %v", err)
	}

	got := canonicalSecurityRoot(filepath.Join(aliasRoot, "privado", "fichero"))
	want := filepath.Join(canonicalRealRoot, "privado", "fichero")
	if got != want {
		t.Fatalf("canonicalSecurityRoot() = %q, want %q", got, want)
	}
}

func TestAuthChallengeLimitaYLimpiaPendientes(t *testing.T) {
	adaptador := New(nil, nil, nil).WithCertificateAuth(strings.Repeat("a", sha256.Size*2), time.Minute)
	handler := adaptador.Routes()

	req := httptest.NewRequest(http.MethodGet, "/auth/challenge", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /auth/challenge = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}

	for i := 0; i < maxPendingRESTChallenges; i++ {
		req = httptest.NewRequest(http.MethodPost, "/auth/challenge", nil)
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("reto %d = %d, want %d; cuerpo=%s", i, rr.Code, http.StatusOK, rr.Body.String())
		}
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/challenge", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("reto sobre límite = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}

	adaptador.mu.Lock()
	for id, challenge := range adaptador.challenges {
		challenge.ExpiresAt = time.Now().Add(-time.Minute)
		adaptador.challenges[id] = challenge
	}
	adaptador.mu.Unlock()
	req = httptest.NewRequest(http.MethodPost, "/auth/challenge", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("reto tras limpiar expirados = %d, want %d", rr.Code, http.StatusOK)
	}
	adaptador.mu.Lock()
	pending := len(adaptador.challenges)
	adaptador.mu.Unlock()
	if pending != 1 {
		t.Fatalf("retos pendientes tras limpieza = %d, want 1", pending)
	}
}

func TestOpenAPIIncluyeAuthCertSoloPOSTCuandoEstaActiva(t *testing.T) {
	adaptador := New(nil, nil, nil).WithCertificateAuth(strings.Repeat("a", sha256.Size*2), time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("openapi = %d, want %d", rr.Code, http.StatusOK)
	}
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &document); err != nil {
		t.Fatalf("OpenAPI inválido: %v", err)
	}
	challenge, ok := document.Paths["/auth/challenge"]
	if !ok {
		t.Fatal("OpenAPI no incluye /auth/challenge con auth cert activa")
	}
	if _, ok := challenge["post"]; !ok {
		t.Error("OpenAPI no declara POST /auth/challenge")
	}
	if _, ok := challenge["get"]; ok {
		t.Error("OpenAPI declara GET /auth/challenge")
	}
	if _, ok := document.Paths["/auth/verify"]; !ok {
		t.Fatal("OpenAPI no incluye /auth/verify con auth cert activa")
	}
}

func TestAuthOKRevalidaGeneracionYAllowlist(t *testing.T) {
	fingerprint := strings.Repeat("a", sha256.Size*2)
	adaptador := New(nil, nil, nil).WithCertificateAuth(fingerprint, time.Minute)
	_, generation := adaptador.certificateAuthSnapshot()
	request := httptest.NewRequest(http.MethodGet, "/settings", nil)
	request.Header.Set("Authorization", "Bearer session-prueba")

	for _, tc := range []struct {
		name        string
		generation  uint64
		fingerprint string
		want        bool
	}{
		{
			name:        "generacion anterior",
			generation:  generation - 1,
			fingerprint: fingerprint,
		},
		{
			name:        "huella no autorizada",
			generation:  generation,
			fingerprint: strings.Repeat("b", sha256.Size*2),
		},
		{
			name:        "sesion vigente",
			generation:  generation,
			fingerprint: fingerprint,
			want:        true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adaptador.mu.Lock()
			adaptador.sessions["session-prueba"] = restSession{
				Token:       "session-prueba",
				Fingerprint: tc.fingerprint,
				ExpiresAt:   time.Now().Add(time.Minute),
				Generation:  tc.generation,
			}
			adaptador.mu.Unlock()
			if got := adaptador.authOK(request); got != tc.want {
				t.Fatalf("authOK() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAuthReconfiguracionConcurrenteNoTieneCarreras(t *testing.T) {
	fingerprintA := strings.Repeat("a", sha256.Size*2)
	fingerprintB := strings.Repeat("b", sha256.Size*2)
	adaptador := New(nil, nil, nil).
		WithBearerToken("bearer-prueba").
		WithCertificateAuth(fingerprintA, time.Minute)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if i%2 == 0 {
				adaptador.WithCertificateAuth(fingerprintA, time.Minute)
			} else {
				adaptador.WithCertificateAuth(fingerprintB, 2*time.Minute)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
			rr := httptest.NewRecorder()
			adaptador.Routes().ServeHTTP(rr, req)

			authRequest := httptest.NewRequest(http.MethodGet, "/settings", nil)
			authRequest.Header.Set("Authorization", "Bearer bearer-prueba")
			_ = adaptador.authOK(authRequest)
		}
	}()
	wg.Wait()
}

func TestRoutes_InstallPublicRoots_TLSLocalTrust(t *testing.T) {
	dir := t.TempDir()
	expectedRoot := filepath.Join(
		dir,
		"tls",
		"websocket-localhost-root.crt.pem",
	)

	adaptador := New(nil, nil, nil).WithConfigDir(dir).WithLocalizador(localizador.Para("es"))
	adaptador.tlsDeps = tlsRESTDependencies{
		managedTrustLifecycleSupported: func() bool {
			return true
		},
		ensureBrowserCompatibleCertificate: func(
			certDir string,
			prefix string,
		) (string, string, string, string, error) {
			return filepath.Join(certDir, prefix+".crt.pem"),
				filepath.Join(certDir, prefix+".key.pem"),
				filepath.Join(certDir, prefix+"-root.crt.pem"),
				"grxfirma-local-ca",
				nil
		},
		ensureManagedTrust: func(_ context.Context, certFile string) error {
			if certFile != expectedRoot {
				t.Fatalf("certFile = %q, want %q", certFile, expectedRoot)
			}
			return nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/trust/install-public-roots", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp publicRootsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido: %v", err)
	}
	if !resp.OK {
		t.Fatalf("se esperaba ok=true: %+v", resp)
	}
	if resp.LocalCertificateState != "generated" || resp.SystemTrustState != "installed" {
		t.Fatalf("estados inesperados: %+v", resp)
	}
	if strings.Contains(rr.Body.String(), expectedRoot) || strings.Contains(rr.Body.String(), dir) {
		t.Fatalf("la respuesta de instalación filtra rutas locales: %s", rr.Body.String())
	}
}

func TestRoutes_TLSClearStore_RetiraConfianzaAntesDeArtefactosPropios(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	managed := tlsRESTManagedArtifactPaths(tlsDir)
	for _, path := range managed {
		if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", path, err)
		}
	}
	unrelated := filepath.Join(tlsDir, "raiz-ajena.crt.pem")
	if err := os.WriteFile(unrelated, []byte("unrelated"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(unrelated) error = %v", err)
	}

	removeCalled := false
	adaptador := New(nil, nil, nil).WithConfigDir(dir)
	adaptador.tlsDeps = tlsRESTDependencies{
		managedTrustLifecycleSupported: func() bool {
			return true
		},
		removeManagedTrust: func(_ context.Context, rootFile string) error {
			removeCalled = true
			wantRoot := filepath.Join(
				tlsDir,
				defaultCertPrefix+"-root.crt.pem",
			)
			if rootFile != wantRoot {
				t.Fatalf("rootFile = %q, want %q", rootFile, wantRoot)
			}
			for _, path := range managed {
				if _, err := os.Lstat(path); err != nil {
					t.Fatalf(
						"la confianza se retiró después de borrar %q: %v",
						path,
						err,
					)
				}
			}
			return nil
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/tls/clear-store", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)

	var response tlsClearStoreResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}
	if rr.Code != http.StatusOK ||
		!response.OK ||
		response.Removed != len(managed) ||
		!removeCalled {
		t.Fatalf("respuesta de retirada inesperada: code=%d %#v", rr.Code, response)
	}
	for _, path := range managed {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("el artefacto propio %q no fue retirado: %v", path, err)
		}
	}
	if got, err := os.ReadFile(unrelated); err != nil ||
		string(got) != "unrelated" {
		t.Fatalf("se alteró el artefacto ajeno: %q, %v", got, err)
	}
}

func TestRoutes_TLSClearStore_FalloRetiradaConservaEvidencia(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	managed := tlsRESTManagedArtifactPaths(tlsDir)
	for _, path := range managed {
		if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", path, err)
		}
	}

	adaptador := New(nil, nil, nil).WithConfigDir(dir)
	adaptador.tlsDeps = tlsRESTDependencies{
		managedTrustLifecycleSupported: func() bool {
			return true
		},
		removeManagedTrust: func(context.Context, string) error {
			return errors.New("almacén ocupado")
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/tls/clear-store", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)

	var response tlsClearStoreResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}
	if response.OK ||
		response.Removed != 0 ||
		!strings.Contains(response.Message, "retirar la confianza") {
		t.Fatalf("el fallo de confianza no cerró la operación: %#v", response)
	}
	for _, path := range managed {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "owned" {
			t.Fatalf("el fallo no preservó %q: %q, %v", path, got, err)
		}
	}
}

func TestRoutes_TLSClearStore_RechazaDirectorioAliasSinTocarDestino(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	externalDir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.Symlink(externalDir, tlsDir); err != nil {
		t.Skipf("symlinks no disponibles: %v", err)
	}
	externalArtifact := filepath.Join(
		externalDir,
		defaultCertPrefix+".key.pem",
	)
	if err := os.WriteFile(externalArtifact, []byte("external"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(externalArtifact) error = %v", err)
	}

	removeCalled := false
	adaptador := New(nil, nil, nil).WithConfigDir(dir)
	adaptador.tlsDeps = tlsRESTDependencies{
		managedTrustLifecycleSupported: func() bool {
			return true
		},
		removeManagedTrust: func(context.Context, string) error {
			removeCalled = true
			return nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/tls/clear-store", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)

	var response tlsClearStoreResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}
	if response.OK || removeCalled {
		t.Fatalf("se aceptó un directorio TLS alias: %#v", response)
	}
	if got, err := os.ReadFile(externalArtifact); err != nil ||
		string(got) != "external" {
		t.Fatalf("se alteró el destino externo: %q, %v", got, err)
	}
}

func TestRoutes_TLSClearStore_RechazaArtefactoAliasSinRetirarConfianza(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tlsDir := filepath.Join(dir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		t.Fatalf("os.MkdirAll(tlsDir) error = %v", err)
	}
	external := filepath.Join(t.TempDir(), "external.key.pem")
	if err := os.WriteFile(external, []byte("external"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(external) error = %v", err)
	}
	managedAlias := filepath.Join(tlsDir, defaultCertPrefix+".key.pem")
	if err := os.Symlink(external, managedAlias); err != nil {
		t.Skipf("symlinks no disponibles: %v", err)
	}

	removeCalled := false
	adaptador := New(nil, nil, nil).WithConfigDir(dir)
	adaptador.tlsDeps = tlsRESTDependencies{
		managedTrustLifecycleSupported: func() bool {
			return true
		},
		removeManagedTrust: func(context.Context, string) error {
			removeCalled = true
			return nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/tls/clear-store", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)

	var response tlsClearStoreResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}
	if response.OK || removeCalled {
		t.Fatalf("se aceptó un artefacto TLS alias: %#v", response)
	}
	if got, err := os.ReadFile(external); err != nil ||
		string(got) != "external" {
		t.Fatalf("se alteró el destino externo: %q, %v", got, err)
	}
	if _, err := os.Lstat(managedAlias); err != nil {
		t.Fatalf("se alteró el alias rechazado: %v", err)
	}
}

func TestRoutes_InstallPublicRoots_RechazaRutaNoGestionada(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	trustCalled := false
	adaptador := New(nil, nil, nil).WithConfigDir(dir)
	adaptador.tlsDeps = tlsRESTDependencies{
		managedTrustLifecycleSupported: func() bool {
			return true
		},
		ensureBrowserCompatibleCertificate: func(
			certDir string,
			prefix string,
		) (string, string, string, string, error) {
			return filepath.Join(certDir, prefix+".crt.pem"),
				filepath.Join(certDir, prefix+".key.pem"),
				filepath.Join(t.TempDir(), prefix+"-root.crt.pem"),
				"grxfirma-local-ca",
				nil
		},
		ensureManagedTrust: func(context.Context, string) error {
			trustCalled = true
			return nil
		},
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/trust/install-public-roots",
		nil,
	)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)

	var response publicRootsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}
	if response.OK || trustCalled {
		t.Fatalf("se aceptó una raíz fuera del almacén gestionado: %#v", response)
	}
	if strings.Contains(rr.Body.String(), dir) {
		t.Fatalf("el error filtró una ruta local: %s", rr.Body.String())
	}
}

func TestRoutes_ServiceStatus_SinServicioDevuelveNoConfigurado(t *testing.T) {
	adaptador := New(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/service/status", nil)
	rr := httptest.NewRecorder()
	adaptador.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("codigo inesperado: %d cuerpo=%s", rr.Code, rr.Body.String())
	}
	var resp serviceStatusResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido: %v", err)
	}
	if !resp.OK || resp.Status.Method != "no-configurado" {
		t.Fatalf("status inesperado: %+v", resp)
	}
}

func TestValidateCertificateChainAt_UsaLaCadenaVerificada(t *testing.T) {
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	rootDER, root, rootKey := createValidationCertificate(t, nil, nil, pkix.Name{CommonName: "Root T087"}, now, true, x509.KeyUsageCertSign|x509.KeyUsageCRLSign)
	leafDER, _, _ := createValidationCertificate(t, root, rootKey, pkix.Name{CommonName: "Leaf T087"}, now, false, x509.KeyUsageDigitalSignature)

	roots := x509.NewCertPool()
	roots.AddCert(root)
	got, err := validateCertificateChainAt("cert-t087", [][]byte{leafDER}, roots, now)
	if err != nil {
		t.Fatalf("validateCertificateChainAt() error = %v", err)
	}
	if !got.Valid || !got.Trusted || !got.TimeValid || !got.DigitalSignatureUsage {
		t.Fatalf("resultado válido inesperado: %+v", got)
	}
	if got.ChainDepth != 2 {
		t.Fatalf("ChainDepth = %d, quería la cadena verificada hoja+raíz", got.ChainDepth)
	}
	if got.PresentedChainDepth != 1 {
		t.Fatalf("PresentedChainDepth = %d, quería solo la hoja presentada", got.PresentedChainDepth)
	}
	if got.FingerprintSHA256 == "" || got.Subject == "" || got.Issuer == "" {
		t.Fatalf("metadatos incompletos: %+v", got)
	}
	if len(rootDER) == 0 {
		t.Fatal("fixture raíz vacío")
	}
}

func TestValidateCertificateChainAt_NoConfiaEnRaizPresentada(t *testing.T) {
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	rootDER, root, rootKey := createValidationCertificate(t, nil, nil, pkix.Name{CommonName: "Root ajena T087"}, now, true, x509.KeyUsageCertSign|x509.KeyUsageCRLSign)
	leafDER, _, _ := createValidationCertificate(t, root, rootKey, pkix.Name{CommonName: "Leaf ajena T087"}, now, false, x509.KeyUsageDigitalSignature)

	got, err := validateCertificateChainAt("cert-untrusted", [][]byte{leafDER, rootDER}, x509.NewCertPool(), now)
	if err != nil {
		t.Fatalf("validateCertificateChainAt() error = %v", err)
	}
	if got.Valid || got.Trusted {
		t.Fatalf("una raíz aportada por el solicitante no debe convertirse en ancla: %+v", got)
	}
	if got.ChainDepth != 0 || got.PresentedChainDepth != 2 {
		t.Fatalf("profundidades inesperadas: %+v", got)
	}
	if !containsString(got.Issues, "untrusted") {
		t.Fatalf("falta incidencia untrusted: %+v", got.Issues)
	}
}

func TestValidateCertificateChainAt_RechazaEntradaMalformada(t *testing.T) {
	if _, err := validateCertificateChainAt("cert", nil, x509.NewCertPool(), time.Now()); err == nil {
		t.Fatal("cadena vacía: esperaba error")
	}
	if _, err := validateCertificateChainAt("cert", [][]byte{[]byte("no-der")}, x509.NewCertPool(), time.Now()); err == nil {
		t.Fatal("DER inválido: esperaba error")
	}
	tooLong := make([][]byte, 17)
	if _, err := validateCertificateChainAt("cert", tooLong, x509.NewCertPool(), time.Now()); err == nil {
		t.Fatal("cadena demasiado larga: esperaba error")
	}
}

func createValidationCertificate(
	t *testing.T,
	parent *x509.Certificate,
	parentKey ed25519.PrivateKey,
	subject pkix.Name,
	now time.Time,
	isCA bool,
	keyUsage x509.KeyUsage,
) ([]byte, *x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey() error = %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               subject,
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              keyUsage,
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if parent == nil {
		parent = template
		parentKey = privateKey
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, publicKey, parentKey)
	if err != nil {
		t.Fatalf("x509.CreateCertificate() error = %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate() error = %v", err)
	}
	return der, certificate, privateKey
}

func containsString(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}
