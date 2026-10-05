// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package csctest ofrece un servidor CSC API v2 simulado sobre httptest para
// las pruebas del cliente. Solo debe importarse desde ficheros _test.go.
//
// Implementa info, oauth2/authorize (con redirección inmediata, como si la
// persona hubiera aceptado), oauth2/token con PKCE S256, oauth2/revoke,
// credentials/list, credentials/info, credentials/sendOTP,
// credentials/authorize y signatures/signHash con claves RSA y ECDSA
// generadas al vuelo y una CA de pruebas.
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
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	// OAuthURL, si no está vacío, es el servidor OAuth que anuncia /info.
	OAuthURL string
	// InfoOverride, si no es nil, sustituye la respuesta de /info.
	InfoOverride func(w http.ResponseWriter, r *http.Request)

	codigos        map[string]codigoPendiente
	tokensServicio map[string]bool
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
		Credenciales:   map[string]*Credencial{},
		codigos:        map[string]codigoPendiente{},
		tokensServicio: map[string]bool{},
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
	escribirJSON(w, http.StatusOK, map[string]any{
		"specs":    "2.0.0.2",
		"name":     "CSC simulado",
		"region":   "ES",
		"authType": []string{"oauth2code"},
		"oauth2":   oauth,
		"methods":  []string{"credentials/list", "credentials/info", "credentials/authorize", "signatures/signHash"},
	})
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
	} else {
		s.tokensServicio[tok] = true
	}
	escribirJSON(w, http.StatusOK, map[string]any{"access_token": tok, "token_type": "Bearer", "expires_in": 3600})
}

func (s *Servidor) revoke(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	defer s.mu.Unlock()
	tok := r.PostForm.Get("token")
	if s.tokensServicio[tok] {
		delete(s.tokensServicio, tok)
		s.Revocados++
	}
	w.WriteHeader(http.StatusOK)
}

func portador(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// servicioAutorizado comprueba el token de servicio; el llamador tiene s.mu.
func (s *Servidor) servicioAutorizado(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost || !s.tokensServicio[portador(r)] {
		fallo(w, http.StatusUnauthorized, "invalid_token")
		return false
	}
	return true
}

func (s *Servidor) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.servicioAutorizado(w, r) {
		return
	}
	escribirJSON(w, http.StatusOK, map[string]any{"credentialIDs": []string{CredencialRSA, CredencialEC}})
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
	c, ok := s.Credenciales[p.CredentialID]
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
				base64.StdEncoding.EncodeToString(s.CA.Raw),
			},
		},
		"SCAL":      s.SCAL,
		"multisign": 1,
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
		if !s.tokensServicio[portador(r)] {
			fallo(w, http.StatusUnauthorized, "invalid_token")
			return
		}
		aut, ok = s.sads[p.SAD]
		delete(s.sads, p.SAD)
	} else {
		aut, ok = s.tokensCred[portador(r)]
		delete(s.tokensCred, portador(r))
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
			opciones = &rsa.PSSOptions{SaltLength: h.Size(), Hash: h}
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
