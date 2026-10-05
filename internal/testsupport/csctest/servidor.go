// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package csctest ofrece un servidor CSC API v2 simulado sobre httptest para
// las pruebas del cliente. Solo debe importarse desde ficheros _test.go.
//
// Implementa info, oauth2/authorize (con redirección inmediata, como si la
// persona hubiera aceptado), oauth2/token con PKCE S256 y refresh_token,
// oauth2/revoke, los metadatos RFC 8414 de oauth2Issuer, credentials/list
// con paginación, credentials/info, credentials/sendOTP,
// credentials/authorize (con numSignatures y multisign) y
// signatures/signHash con claves RSA y ECDSA generadas al vuelo y una CA de
// pruebas.
package csctest

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// ClientID es el identificador OAuth que acepta el simulador.
	ClientID = "grxfirma-pruebas"
	// CredencialRSA y CredencialEC son los identificadores de credencial.
	CredencialRSA = "cred-rsa"
	CredencialEC  = "cred-ec"
	// PIN y OTP esperados en modo explicit.
	PIN = "1234"
	OTP = "567890"
)

// Credencial simulada.
type Credencial struct {
	ID          string
	Clave       crypto.Signer
	Certificado *x509.Certificate
}

type codigoPendiente struct {
	reto, redireccion, scope, credencial string
	hashes                               []string
}

type autorizacion struct {
	credencial string
	hashes     []string
	usado      bool
}

// Servidor es el servicio CSC simulado.
type Servidor struct {
	*httptest.Server
	CA           *x509.Certificate
	Credenciales map[string]*Credencial

	mu sync.Mutex
	// Modo de autorización de las credenciales: implicit, explicit u oauth2code.
	Modo string
	// SCAL anunciado ("1" o "2").
	SCAL string
	// AuthV21 anuncia la autorización con el objeto "auth" de CSC 2.1.
	AuthV21 bool
	// StateFalso hace que el servidor OAuth devuelva un state distinto.
	StateFalso bool
	// FirmaFalsa hace que signHash devuelva una firma que no verifica.
	FirmaFalsa bool
	// ECDSACruda devuelve las firmas ECDSA como r||s en lugar de DER.
	ECDSACruda bool
	// CadenaAjena anuncia como cadena una CA que no emitió el certificado.
	CadenaAjena *x509.Certificate
	// SalPSS, si no es cero, fuerza esa longitud de sal en las firmas PSS.
	SalPSS int
	// RevocadosCred cuenta los tokens de credencial revocados.
	RevocadosCred int
	// OAuthURL, si no está vacío, es el servidor OAuth que anuncia /info.
	OAuthURL string
	// InfoOverride, si no es nil, sustituye la respuesta de /info.
	InfoOverride func(w http.ResponseWriter, r *http.Request)
	// Multisign es el máximo de firmas por autorización que se anuncia y se
	// exige en credentials/authorize (1 por defecto).
	Multisign int
	// MultisignJSON, si no está vacío, se anuncia tal cual como multisign.
	MultisignJSON string
	// CredencialesExtra añade identificadores al listado (usan la clave RSA).
	CredencialesExtra int
	// TamPagina, si no es cero, pagina credentials/list con ese tamaño.
	TamPagina int
	// PaginaCiclica devuelve siempre el mismo nextPageToken.
	PaginaCiclica bool
	// DarRefresh incluye refresh_token al emitir el token de servicio.
	DarRefresh bool
	// RefreshSinRotar no emite un refresh_token nuevo al renovar.
	RefreshSinRotar bool
	// RefreshRechazado hace que la renovación falle con invalid_grant.
	RefreshRechazado bool
	// VidaToken son los segundos de expires_in del token de servicio (3600
	// por defecto). El servidor rechaza con 401 los tokens caducados.
	VidaToken int
	// Emisor, si no está vacío, se anuncia como oauth2Issuer (ruta bajo el
	// propio servidor) en lugar de oauth2, y publica sus metadatos RFC 8414
	// con extremos en /as/.
	Emisor string
	// MetadatosOverride permite alterar los metadatos antes de enviarlos.
	MetadatosOverride func(m map[string]any)
	// Renovaciones cuenta los refresh_token canjeados.
	Renovaciones int
	// RevocadosRefresh cuenta los refresh_token revocados.
	RevocadosRefresh int
	// UltimoNumSignatures es el numSignatures de la última autorización.
	UltimoNumSignatures int
	// PeticionesListado cuenta las páginas pedidas.
	PeticionesListado int

	codigos        map[string]codigoPendiente
	tokensServicio map[string]time.Time
	refrescos      map[string]bool
	tokensCred     map[string]autorizacion
	sads           map[string]autorizacion
	// ResumenesFirmados recoge todo lo que llegó a signHash.
	ResumenesFirmados [][]byte
	Revocados         int
	OTPEnviados       int
	Autorizaciones    int
	UltimoSignAlgo    string
}

// Nuevo arranca el servidor TLS simulado y lo cierra al terminar la prueba.
func Nuevo(t testing.TB) *Servidor {
	t.Helper()
	s := &Servidor{
		Modo:           "implicit",
		SCAL:           "1",
		Multisign:      1,
		Credenciales:   map[string]*Credencial{},
		codigos:        map[string]codigoPendiente{},
		tokensServicio: map[string]time.Time{},
		refrescos:      map[string]bool{},
		tokensCred:     map[string]autorizacion{},
		sads:           map[string]autorizacion{},
	}
	caClave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caPlantilla := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "CA de pruebas CSC"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caPlantilla, caPlantilla, &caClave.PublicKey, caClave)
	if err != nil {
		t.Fatal(err)
	}
	s.CA, _ = x509.ParseCertificate(caDER)

	rsaClave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecClave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range []struct {
		id    string
		clave crypto.Signer
		cn    string
	}{{CredencialRSA, rsaClave, "Firmante remoto RSA"}, {CredencialEC, ecClave, "Firmante remoto EC"}} {
		plantilla := &x509.Certificate{
			SerialNumber: big.NewInt(int64(10 + i)),
			Subject:      pkix.Name{CommonName: c.cn},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(12 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		}
		der, err := x509.CreateCertificate(rand.Reader, plantilla, s.CA, c.clave.Public(), caClave)
		if err != nil {
			t.Fatal(err)
		}
		cert, _ := x509.ParseCertificate(der)
		s.Credenciales[c.id] = &Credencial{ID: c.id, Clave: c.clave, Certificado: cert}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/csc/v2/info", s.info)
	mux.HandleFunc("/oauth2/authorize", s.authorize)
	mux.HandleFunc("/oauth2/token", s.token)
	mux.HandleFunc("/oauth2/revoke", s.revoke)
	mux.HandleFunc("/as/authorize", s.authorize)
	mux.HandleFunc("/as/token", s.token)
	mux.HandleFunc("/as/revoke", s.revoke)
	mux.HandleFunc("/.well-known/oauth-authorization-server/", s.metadatos)
	mux.HandleFunc("/csc/v2/credentials/list", s.list)
	mux.HandleFunc("/csc/v2/credentials/info", s.credInfo)
	mux.HandleFunc("/csc/v2/credentials/sendOTP", s.sendOTP)
	mux.HandleFunc("/csc/v2/credentials/authorize", s.credAuthorize)
	mux.HandleFunc("/csc/v2/signatures/signHash", s.signHash)
	s.Server = httptest.NewTLSServer(mux)
	t.Cleanup(s.Close)
	return s
}

// Configurar cambia la configuración del simulador de forma segura.
func (s *Servidor) Configurar(f func(*Servidor)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// Leer consulta el estado del simulador de forma segura.
func (s *Servidor) Leer(f func(*Servidor)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// Navegador simula a la persona que acepta en el navegador: sigue la
// autorización del servidor hasta el callback local de 127.0.0.1.
func (s *Servidor) Navegador() func(context.Context, string) error {
	return func(ctx context.Context, destino string) error {
		cliente := s.Client()
		sinRedirecciones := *cliente
		sinRedirecciones.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, nil)
		if err != nil {
			return err
		}
		resp, err := sinRedirecciones.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusFound {
			return errors.New("csctest: authorize sin redirección")
		}
		callback := resp.Header.Get("Location")
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, callback, nil)
		if err != nil {
			return err
		}
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		return resp.Body.Close()
	}
}

func escribirJSON(w http.ResponseWriter, estado int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(estado)
	_ = json.NewEncoder(w).Encode(v)
}

func fallo(w http.ResponseWriter, estado int, codigo string) {
	escribirJSON(w, estado, map[string]string{"error": codigo, "error_description": "simulated"})
}

func aleatorio() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Servidor) info(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	override := s.InfoOverride
	oauth := s.URL
	if s.OAuthURL != "" {
		oauth = s.OAuthURL
	}
	s.mu.Unlock()
	if override != nil {
		override(w, r)
		return
	}
	if r.Method != http.MethodPost {
		fallo(w, http.StatusMethodNotAllowed, "invalid_request")
		return
	}
	respuesta := map[string]any{
		"specs":    "2.0.0.2",
		"name":     "CSC simulado",
		"region":   "ES",
		"authType": []string{"oauth2code"},
		"oauth2":   oauth,
		"methods":  []string{"credentials/list", "credentials/info", "credentials/authorize", "signatures/signHash"},
	}
	s.mu.Lock()
	if s.Emisor != "" {
		delete(respuesta, "oauth2")
		respuesta["oauth2Issuer"] = s.URL + s.Emisor
	}
	s.mu.Unlock()
	escribirJSON(w, http.StatusOK, respuesta)
}

// metadatos publica los metadatos RFC 8414 del emisor anunciado.
func (s *Servidor) metadatos(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	emisor, alterar := s.Emisor, s.MetadatosOverride
	s.mu.Unlock()
	if r.Method != http.MethodGet || emisor == "" ||
		r.URL.Path != "/.well-known/oauth-authorization-server"+emisor {
		fallo(w, http.StatusNotFound, "not_found")
		return
	}
	m := map[string]any{
		"issuer":                           s.URL + emisor,
		"authorization_endpoint":           s.URL + "/as/authorize",
		"token_endpoint":                   s.URL + "/as/token",
		"revocation_endpoint":              s.URL + "/as/revoke",
		"code_challenge_methods_supported": []string{"S256"},
		"response_types_supported":         []string{"code"},
	}
	if alterar != nil {
		alterar(m)
	}
	escribirJSON(w, http.StatusOK, m)
}

func (s *Servidor) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redireccion := q.Get("redirect_uri")
	if q.Get("client_id") != ClientID || q.Get("response_type") != "code" ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" ||
		!strings.HasPrefix(redireccion, "http://127.0.0.1:") {
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	scope := q.Get("scope")
	pendiente := codigoPendiente{reto: q.Get("code_challenge"), redireccion: redireccion, scope: scope}
	if scope == "credential" {
		pendiente.credencial = q.Get("credentialID")
		for _, h := range strings.Split(q.Get("hashes"), ",") {
			crudo, err := base64.URLEncoding.DecodeString(h)
			if err != nil {
				fallo(w, http.StatusBadRequest, "invalid_request")
				return
			}
			pendiente.hashes = append(pendiente.hashes, base64.StdEncoding.EncodeToString(crudo))
		}
	} else if scope != "service" {
		fallo(w, http.StatusBadRequest, "invalid_scope")
		return
	}
	codigo := aleatorio()
	s.mu.Lock()
	s.codigos[codigo] = pendiente
	estado := q.Get("state")
	if s.StateFalso {
		estado = aleatorio()
	}
	s.mu.Unlock()
	destino, _ := url.Parse(redireccion)
	consulta := url.Values{"code": {codigo}, "state": {estado}}
	destino.RawQuery = consulta.Encode()
	// Simulador de servidor OAuth: redirige solo a http://127.0.0.1:<puerto>, comprobado arriba.
	http.Redirect(w, r, destino.String(), http.StatusFound) // nosemgrep: go.lang.security.injection.open-redirect.open-redirect
}

func (s *Servidor) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.Method != http.MethodPost {
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.PostForm.Get("grant_type") == "refresh_token" {
		s.renovar(w, r)
		return
	}
	codigo := r.PostForm.Get("code")
	pendiente, ok := s.codigos[codigo]
	delete(s.codigos, codigo) // un solo uso
	suma := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || r.PostForm.Get("grant_type") != "authorization_code" ||
		r.PostForm.Get("client_id") != ClientID || r.PostForm.Get("client_secret") != "" ||
		r.PostForm.Get("redirect_uri") != pendiente.redireccion ||
		base64.RawURLEncoding.EncodeToString(suma[:]) != pendiente.reto {
		fallo(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	tok := aleatorio()
	if pendiente.scope == "credential" {
		s.tokensCred[tok] = autorizacion{credencial: pendiente.credencial, hashes: pendiente.hashes}
		escribirJSON(w, http.StatusOK, map[string]any{"access_token": tok, "token_type": "Bearer", "expires_in": 3600})
		return
	}
	escribirJSON(w, http.StatusOK, s.emitirServicioLocked(true))
}

// emitirServicioLocked crea un token de servicio y, si procede, su
// refresh_token. El llamador tiene s.mu.
func (s *Servidor) emitirServicioLocked(conRefresco bool) map[string]any {
	vida := s.VidaToken
	if vida <= 0 {
		vida = 3600
	}
	tok := aleatorio()
	s.tokensServicio[tok] = time.Now().Add(time.Duration(vida) * time.Second)
	respuesta := map[string]any{"access_token": tok, "token_type": "Bearer", "expires_in": vida}
	if s.DarRefresh && conRefresco {
		ref := aleatorio()
		s.refrescos[ref] = true
		respuesta["refresh_token"] = ref
	}
	return respuesta
}

// renovar canjea un refresh_token (cliente público, sin secreto). El
// llamador tiene s.mu.
func (s *Servidor) renovar(w http.ResponseWriter, r *http.Request) {
	ref := r.PostForm.Get("refresh_token")
	if s.RefreshRechazado || !s.refrescos[ref] || r.PostForm.Get("client_id") != ClientID ||
		r.PostForm.Get("client_secret") != "" {
		fallo(w, http.StatusBadRequest, "invalid_grant")
		return
	}
	rotar := !s.RefreshSinRotar
	if rotar {
		delete(s.refrescos, ref)
	}
	s.Renovaciones++
	escribirJSON(w, http.StatusOK, s.emitirServicioLocked(rotar))
}

// CaducarTokensServicio invalida en el servidor los tokens de servicio
// emitidos, como si hubieran caducado antes de lo anunciado.
func (s *Servidor) CaducarTokensServicio() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tok := range s.tokensServicio {
		s.tokensServicio[tok] = time.Now().Add(-time.Second)
	}
}

func (s *Servidor) revoke(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	defer s.mu.Unlock()
	tok := r.PostForm.Get("token")
	if _, ok := s.tokensServicio[tok]; ok {
		delete(s.tokensServicio, tok)
		s.Revocados++
	}
	if s.refrescos[tok] {
		delete(s.refrescos, tok)
		s.RevocadosRefresh++
	}
	if _, ok := s.tokensCred[tok]; ok {
		delete(s.tokensCred, tok)
		s.RevocadosCred++
	}
	w.WriteHeader(http.StatusOK)
}

func portador(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// servicioAutorizado comprueba el token de servicio; el llamador tiene s.mu.
func (s *Servidor) servicioAutorizado(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost || !s.tokenServicioVigente(portador(r)) {
		fallo(w, http.StatusUnauthorized, "invalid_token")
		return false
	}
	return true
}

// tokenServicioVigente: el llamador tiene s.mu.
func (s *Servidor) tokenServicioVigente(tok string) bool {
	caduca, ok := s.tokensServicio[tok]
	return ok && time.Now().Before(caduca)
}

func (s *Servidor) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.servicioAutorizado(w, r) {
		return
	}
	s.PeticionesListado++
	var p struct {
		MaxResults int    `json:"maxResults"`
		PageToken  string `json:"pageToken"`
	}
	_ = json.NewDecoder(r.Body).Decode(&p)
	ids := []string{CredencialRSA, CredencialEC}
	for i := 0; i < s.CredencialesExtra; i++ {
		ids = append(ids, fmt.Sprintf("cred-extra-%d", i))
	}
	tam := s.TamPagina
	if tam <= 0 {
		escribirJSON(w, http.StatusOK, map[string]any{"credentialIDs": ids})
		return
	}
	if p.MaxResults > 0 && p.MaxResults < tam {
		tam = p.MaxResults
	}
	inicio := 0
	if p.PageToken != "" {
		n, err := strconv.Atoi(strings.TrimPrefix(p.PageToken, "p"))
		if err != nil || n < 0 || n > len(ids) {
			fallo(w, http.StatusBadRequest, "invalid_request")
			return
		}
		inicio = n
	}
	fin := min(inicio+tam, len(ids))
	respuesta := map[string]any{"credentialIDs": ids[inicio:fin]}
	switch {
	case s.PaginaCiclica:
		respuesta["nextPageToken"] = "p0"
	case fin < len(ids):
		respuesta["nextPageToken"] = "p" + strconv.Itoa(fin)
	}
	escribirJSON(w, http.StatusOK, respuesta)
}

func (s *Servidor) credInfo(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.servicioAutorizado(w, r) {
		return
	}
	var p struct {
		CredentialID string `json:"credentialID"`
	}
	_ = json.NewDecoder(r.Body).Decode(&p)
	c, ok := s.credencialLocked(p.CredentialID)
	if !ok {
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	algos := []string{"1.2.840.113549.1.1.1", "1.2.840.113549.1.1.11"}
	if _, ec := c.Clave.(*ecdsa.PrivateKey); ec {
		algos = []string{"1.2.840.10045.2.1"}
	}
	respuesta := map[string]any{
		"key": map[string]any{"status": "enabled", "algo": algos},
		"cert": map[string]any{
			"status": "valid",
			"certificates": []string{
				base64.StdEncoding.EncodeToString(c.Certificado.Raw),
				base64.StdEncoding.EncodeToString(s.cadena().Raw),
			},
		},
		"SCAL":      s.SCAL,
		"multisign": s.Multisign,
	}
	if s.MultisignJSON != "" {
		respuesta["multisign"] = json.RawMessage(s.MultisignJSON)
	}
	explicito := s.Modo == "explicit"
	if s.AuthV21 {
		objetos := []map[string]string{}
		if explicito {
			objetos = append(objetos, map[string]string{"type": "Password", "id": "PIN"}, map[string]string{"type": "Password", "id": "OTP"})
		}
		respuesta["auth"] = map[string]any{"mode": s.Modo, "objects": objetos}
	} else {
		respuesta["authMode"] = s.Modo
		if explicito {
			respuesta["PIN"] = map[string]string{"presence": "true", "format": "N"}
			respuesta["OTP"] = map[string]string{"presence": "true", "type": "online"}
		}
	}
	escribirJSON(w, http.StatusOK, respuesta)
}

// credencialLocked resuelve también los identificadores extra del listado,
// que usan la credencial RSA. El llamador tiene s.mu.
func (s *Servidor) credencialLocked(id string) (*Credencial, bool) {
	if c, ok := s.Credenciales[id]; ok {
		return c, true
	}
	if strings.HasPrefix(id, "cred-extra-") {
		c, ok := s.Credenciales[CredencialRSA]
		return c, ok
	}
	return nil, false
}

// cadena devuelve la CA que se anuncia; el llamador tiene s.mu.
func (s *Servidor) cadena() *x509.Certificate {
	if s.CadenaAjena != nil {
		return s.CadenaAjena
	}
	return s.CA
}

func (s *Servidor) sendOTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.servicioAutorizado(w, r) {
		return
	}
	s.OTPEnviados++
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

func (s *Servidor) credAuthorize(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.servicioAutorizado(w, r) {
		return
	}
	var p struct {
		CredentialID     string   `json:"credentialID"`
		NumSignatures    int      `json:"numSignatures"`
		Hashes           []string `json:"hashes"`
		HashAlgorithmOID string   `json:"hashAlgorithmOID"`
		PIN              string   `json:"PIN"`
		OTP              string   `json:"OTP"`
		AuthData         []struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		} `json:"authData"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || s.Credenciales[p.CredentialID] == nil ||
		p.NumSignatures != len(p.Hashes) || p.NumSignatures < 1 || p.HashAlgorithmOID == "" {
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if p.NumSignatures > max(s.Multisign, 1) {
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	s.UltimoNumSignatures = p.NumSignatures
	if s.Modo == "oauth2code" {
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if s.Modo == "explicit" {
		pin, otp := p.PIN, p.OTP
		for _, d := range p.AuthData {
			switch d.ID {
			case "PIN":
				pin = d.Value
			case "OTP":
				otp = d.Value
			}
		}
		if pin != PIN || otp != OTP {
			fallo(w, http.StatusBadRequest, "invalid_pin")
			return
		}
	}
	s.Autorizaciones++
	sad := aleatorio()
	s.sads[sad] = autorizacion{credencial: p.CredentialID, hashes: p.Hashes}
	escribirJSON(w, http.StatusOK, map[string]any{"SAD": sad, "expiresIn": 300})
}

func (s *Servidor) signHash(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var p struct {
		CredentialID     string   `json:"credentialID"`
		SAD              string   `json:"SAD"`
		Hashes           []string `json:"hashes"`
		HashAlgorithmOID string   `json:"hashAlgorithmOID"`
		SignAlgo         string   `json:"signAlgo"`
	}
	if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&p) != nil {
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var aut autorizacion
	var ok bool
	if p.SAD != "" {
		if !s.tokenServicioVigente(portador(r)) {
			fallo(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		aut, ok = s.sads[p.SAD]
		delete(s.sads, p.SAD)
	} else {
		aut, ok = s.tokensCred[portador(r)]
		if ok && aut.usado {
			ok = false
		}
		if ok {
			aut.usado = true
			s.tokensCred[portador(r)] = aut
		}
	}
	if !ok || aut.credencial != p.CredentialID || strings.Join(aut.hashes, ",") != strings.Join(p.Hashes, ",") {
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var h crypto.Hash
	switch p.HashAlgorithmOID {
	case "2.16.840.1.101.3.4.2.1":
		h = crypto.SHA256
	case "2.16.840.1.101.3.4.2.2":
		h = crypto.SHA384
	case "2.16.840.1.101.3.4.2.3":
		h = crypto.SHA512
	default:
		fallo(w, http.StatusBadRequest, "invalid_request")
		return
	}
	s.UltimoSignAlgo = p.SignAlgo
	c := s.Credenciales[p.CredentialID]
	firmas := make([]string, 0, len(p.Hashes))
	for _, b64 := range p.Hashes {
		resumen, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(resumen) != h.Size() {
			fallo(w, http.StatusBadRequest, "invalid_request")
			return
		}
		s.ResumenesFirmados = append(s.ResumenesFirmados, resumen)
		var opciones crypto.SignerOpts = h
		if p.SignAlgo == "1.2.840.113549.1.1.10" {
			sal := h.Size()
			if s.SalPSS != 0 {
				sal = s.SalPSS
			}
			opciones = &rsa.PSSOptions{SaltLength: sal, Hash: h}
		}
		firma, err := c.Clave.Sign(rand.Reader, resumen, opciones)
		if err != nil {
			fallo(w, http.StatusInternalServerError, "server_error")
			return
		}
		if k, ec := c.Clave.(*ecdsa.PrivateKey); ec && s.ECDSACruda {
			firma = ecdsaCruda(k, firma)
		}
		if s.FirmaFalsa {
			firma[len(firma)/2] ^= 0xff
		}
		firmas = append(firmas, base64.StdEncoding.EncodeToString(firma))
	}
	escribirJSON(w, http.StatusOK, map[string]any{"signatures": firmas})
}

func ecdsaCruda(k *ecdsa.PrivateKey, der []byte) []byte {
	var rs struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(der, &rs); err != nil {
		return der
	}
	tam := (k.Curve.Params().BitSize + 7) / 8
	out := make([]byte, 2*tam)
	rs.R.FillBytes(out[:tam])
	rs.S.FillBytes(out[tam:])
	return out
}
