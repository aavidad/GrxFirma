// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"grxfirma/internal/adapters/outbound/common/secmem"
)

const (
	// EsperaAutorizacionPorDefecto es el tiempo que se espera a que la
	// persona complete la autorización en el navegador.
	EsperaAutorizacionPorDefecto = 5 * time.Minute
	rutaCallback                 = "/callback"
	maxTokenBytes                = 16 * 1024
	// maxVidaToken acota expires_in: evita desbordar time.Duration y que un
	// servidor declare tokens prácticamente eternos.
	maxVidaToken    = 24 * time.Hour
	bytesAleatorios = 32
	// margenRenovacion adelanta la renovación del token de servicio para que
	// no caduque a mitad de una petición.
	margenRenovacion = 30 * time.Second
	rutaMetadatos    = "/.well-known/oauth-authorization-server"
)

// token es un token OAuth en memoria bloqueada (best effort) que se borra
// con destruir. Nunca se registra ni se serializa. refresco, si el servidor
// lo da, permite renovar el token de servicio sin volver al navegador.
type token struct {
	valor    *secmem.Blob
	refresco *secmem.Blob
	caduca   time.Time
}

func (t *token) puedeRenovar() bool {
	return t != nil && t.refresco != nil && t.refresco.Len() > 0
}

func (t *token) olvidarRefresco() {
	if t != nil && t.refresco != nil {
		t.refresco.Destroy()
		t.refresco = nil
	}
}

// puntosOAuth son los extremos del servidor de autorización ya validados.
// revocar puede ser nil si los metadatos no lo publican.
type puntosOAuth struct {
	autorizar *url.URL
	token     *url.URL
	revocar   *url.URL
}

// puntosDesdeBase deriva los extremos del URL base «oauth2» de /info, como
// define CSC: <base>/oauth2/authorize, /oauth2/token y /oauth2/revoke.
func puntosDesdeBase(base *url.URL) *puntosOAuth {
	unir := func(ruta string) *url.URL {
		u := *base
		u.Path = strings.TrimRight(u.Path, "/") + ruta
		return &u
	}
	return &puntosOAuth{
		autorizar: unir("/oauth2/authorize"),
		token:     unir("/oauth2/token"),
		revocar:   unir("/oauth2/revoke"),
	}
}

type metadatosOAuth struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RevocationEndpoint    string   `json:"revocation_endpoint"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
}

// descubrirOAuth lee los metadatos RFC 8414 del emisor anunciado en
// oauth2Issuer. El emisor y cada extremo deben ser https y estar en el host
// del servicio o en un par autorizado; el emisor de los metadatos debe ser
// exactamente el anunciado (RFC 8414, sección 3.3) y, si publica los métodos
// PKCE, debe admitir S256.
func (c *Cliente) descubrirOAuth(ctx context.Context, emisor string) (*puntosOAuth, error) {
	u, err := validarURLSegura(emisor)
	if err != nil {
		return nil, err
	}
	if !oauthPermitido(c.base, u, c.opc.ParesOAuth) {
		return nil, nuevoError(CodigoOAuthOtroHost, "", nil)
	}
	destino := *u
	destino.Path = rutaMetadatos + u.Path
	var m metadatosOAuth
	if err := obtenerJSON(ctx, c.http, destino.String(), &m); err != nil {
		return nil, err
	}
	declarado, err := validarURLSegura(m.Issuer)
	if err != nil || declarado.String() != u.String() {
		return nil, nuevoError(CodigoOAuthMetadatos, "issuer", nil)
	}
	if len(m.CodeChallengeMethods) > 0 && !contiene(m.CodeChallengeMethods, "S256") {
		return nil, nuevoError(CodigoOAuthMetadatos, "pkce", nil)
	}
	extremo := func(raw, nombre string, obligatorio bool) (*url.URL, error) {
		if strings.TrimSpace(raw) == "" && !obligatorio {
			return nil, nil
		}
		e, err := validarURLSegura(raw)
		if err != nil {
			return nil, nuevoError(CodigoOAuthMetadatos, nombre, nil)
		}
		if !oauthPermitido(c.base, e, c.opc.ParesOAuth) {
			return nil, nuevoError(CodigoOAuthOtroHost, nombre, nil)
		}
		return e, nil
	}
	p := &puntosOAuth{}
	if p.autorizar, err = extremo(m.AuthorizationEndpoint, "authorization_endpoint", true); err != nil {
		return nil, err
	}
	if p.token, err = extremo(m.TokenEndpoint, "token_endpoint", true); err != nil {
		return nil, err
	}
	if p.revocar, err = extremo(m.RevocationEndpoint, "revocation_endpoint", false); err != nil {
		return nil, err
	}
	return p, nil
}

func (t *token) bytes() []byte {
	if t == nil || t.valor == nil {
		return nil
	}
	return t.valor.Bytes()
}

func (t *token) vigente(ahora time.Time) bool {
	return t != nil && t.valor != nil && t.valor.Len() > 0 && (t.caduca.IsZero() || ahora.Before(t.caduca))
}

func (t *token) destruir() {
	if t != nil && t.valor != nil {
		t.valor.Destroy()
		t.valor = nil
	}
	t.olvidarRefresco()
}

type respuestaToken struct {
	AccessToken  json.RawMessage `json:"access_token"`
	RefreshToken json.RawMessage `json:"refresh_token"`
	TokenType    string          `json:"token_type"`
	ExpiresIn    int64           `json:"expires_in"`
}

// aleatorioURL devuelve n bytes aleatorios en base64url sin relleno.
func aleatorioURL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// retoPKCE calcula code_challenge = BASE64URL(SHA256(code_verifier)) (RFC 7636).
func retoPKCE(verificador string) string {
	suma := sha256.Sum256([]byte(verificador))
	return base64.RawURLEncoding.EncodeToString(suma[:])
}

type resultadoCallback struct {
	codigo string
	err    error
}

// autorizarOAuth completa el flujo Authorization Code con PKCE: abre el
// navegador, espera el código en un puerto efímero de 127.0.0.1 y lo canjea
// por un token. scope es "service" o "credential"; extra añade los
// parámetros propios de la autorización de credencial.
func (c *Cliente) autorizarOAuth(ctx context.Context, scope string, extra url.Values) (*token, error) {
	if c.oauth == nil {
		return nil, nuevoError(CodigoSinOAuth, "", nil)
	}
	verificador, err := aleatorioURL(bytesAleatorios)
	if err != nil {
		return nil, nuevoError(CodigoParametroInvalido, "", err)
	}
	estado, err := aleatorioURL(bytesAleatorios)
	if err != nil {
		return nil, nuevoError(CodigoParametroInvalido, "", err)
	}

	oyente, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nuevoError(CodigoRed, "", err)
	}
	direccion := oyente.Addr().String()
	redireccion := "http://" + direccion + rutaCallback

	consulta := url.Values{}
	for clave, valores := range extra {
		consulta[clave] = append([]string(nil), valores...)
	}
	consulta.Set("response_type", "code")
	consulta.Set("client_id", c.opc.ClientID)
	consulta.Set("redirect_uri", redireccion)
	consulta.Set("scope", scope)
	consulta.Set("code_challenge", retoPKCE(verificador))
	consulta.Set("code_challenge_method", "S256")
	consulta.Set("state", estado)
	autorizacion := *c.oauth.autorizar
	autorizacion.RawQuery = consulta.Encode()

	resultado := make(chan resultadoCallback, 1)
	var unaVez sync.Once
	entregar := func(r resultadoCallback) {
		unaVez.Do(func() { resultado <- r })
	}
	servidor := &http.Server{
		Handler:           c.manejadorCallback(direccion, estado, entregar),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       10 * time.Second,
		MaxHeaderBytes:    8 * 1024,
	}
	go func() { _ = servidor.Serve(oyente) }()
	defer func() {
		cierre, cancelar := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelar()
		_ = servidor.Shutdown(cierre)
	}()

	if err := c.opc.AbrirNavegador(ctx, autorizacion.String()); err != nil {
		return nil, nuevoError(CodigoNavegador, "", err)
	}

	espera := c.opc.EsperaAutorizacion
	if espera <= 0 {
		espera = EsperaAutorizacionPorDefecto
	}
	temporizador := time.NewTimer(espera)
	defer temporizador.Stop()

	var codigo string
	select {
	case <-ctx.Done():
		return nil, nuevoError(CodigoAutorizacionCaducada, "", ctx.Err())
	case <-temporizador.C:
		return nil, nuevoError(CodigoAutorizacionCaducada, "", nil)
	case r := <-resultado:
		if r.err != nil {
			return nil, r.err
		}
		codigo = r.codigo
	}

	return c.canjearCodigo(ctx, codigo, redireccion, verificador)
}

// manejadorCallback acepta una única respuesta en GET /callback con el state
// de esta petición. Un state distinto o ausente, otra ruta, otro método o un
// Host inesperado reciben un error y no cuentan: nunca se acepta un código
// que no se pidió ni se deja que un tercero aborte la espera.
func (c *Cliente) manejadorCallback(direccion, estado string, entregar func(resultadoCallback)) http.Handler {
	responder := func(w http.ResponseWriter, estadoHTTP int) {
		cabecera := w.Header()
		cabecera.Set("Content-Type", "text/plain; charset=utf-8")
		cabecera.Set("Cache-Control", "no-store")
		cabecera.Set("Referrer-Policy", "no-referrer")
		cabecera.Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(estadoHTTP)
		if estadoHTTP == http.StatusOK && c.opc.TextoCallback != "" {
			_, _ = w.Write([]byte(c.opc.TextoCallback))
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != rutaCallback || r.Method != http.MethodGet {
			responder(w, http.StatusNotFound)
			return
		}
		if r.Host != direccion {
			responder(w, http.StatusBadRequest)
			return
		}
		q := r.URL.Query()
		recibido := q.Get("state")
		if recibido == "" || subtle.ConstantTimeCompare([]byte(recibido), []byte(estado)) != 1 {
			// No se entrega nada: una petición ajena (otra web u otro
			// proceso local) no puede abortar el flujo. Se sigue esperando
			// el state correcto hasta el tiempo máximo.
			responder(w, http.StatusBadRequest)
			return
		}
		if fallo := q.Get("error"); fallo != "" {
			responder(w, http.StatusOK)
			entregar(resultadoCallback{err: nuevoError(CodigoAutorizacionDenegada, fallo, nil)})
			return
		}
		codigo := q.Get("code")
		if codigo == "" || len(codigo) > maxTokenBytes {
			responder(w, http.StatusBadRequest)
			entregar(resultadoCallback{err: nuevoError(CodigoRespuestaInvalida, "", nil)})
			return
		}
		responder(w, http.StatusOK)
		entregar(resultadoCallback{codigo: codigo})
	})
}

func (c *Cliente) canjearCodigo(ctx context.Context, codigo, redireccion, verificador string) (*token, error) {
	destino := *c.oauth.token
	campos := url.Values{}
	campos.Set("grant_type", "authorization_code")
	campos.Set("code", codigo)
	campos.Set("redirect_uri", redireccion)
	campos.Set("client_id", c.opc.ClientID)
	campos.Set("code_verifier", verificador)

	var respuesta respuestaToken
	// La closure lee el campo al salir: un defer con el argumento directo
	// lo evaluaría aquí, cuando todavía es nil.
	defer func() {
		secmem.Zeroize(respuesta.AccessToken)
		secmem.Zeroize(respuesta.RefreshToken)
	}()
	if err := postFormulario(ctx, c.http, destino.String(), campos, &respuesta); err != nil {
		return nil, err
	}
	return tokenDesdeRespuesta(&respuesta)
}

// renovarSesion canjea el refresh_token del token de servicio por uno nuevo
// (RFC 6749, sección 6). El llamador tiene c.mu. El cuerpo se compone en
// bytes para no copiar el refresh_token a un string. Si el servidor no da
// un refresh_token nuevo, se conserva el anterior. Si lo rechaza, se olvida:
// hay que volver a conectar.
func (c *Cliente) renovarSesion(ctx context.Context) error {
	actual := c.sesion
	if !actual.puedeRenovar() || c.oauth == nil {
		return nuevoError(CodigoSesionCaducada, "", nil)
	}
	cuerpo := make([]byte, 0, 3*actual.refresco.Len()+3*len(c.opc.ClientID)+64)
	cuerpo = append(cuerpo, "grant_type=refresh_token&refresh_token="...)
	cuerpo = anadirPorcentaje(cuerpo, actual.refresco.Bytes())
	cuerpo = append(cuerpo, "&client_id="...)
	cuerpo = anadirPorcentaje(cuerpo, []byte(c.opc.ClientID))
	defer secmem.Zeroize(cuerpo)
	var respuesta respuestaToken
	defer func() {
		secmem.Zeroize(respuesta.AccessToken)
		secmem.Zeroize(respuesta.RefreshToken)
	}()
	if err := enviar(ctx, c.http, c.oauth.token.String(), nil, tipoFormulario, cuerpo, &respuesta); err != nil {
		if CodigoDe(err) == CodigoServicio {
			actual.olvidarRefresco()
		}
		return nuevoError(CodigoSesionCaducada, "", err)
	}
	nuevo, err := tokenDesdeRespuesta(&respuesta)
	if err != nil {
		return nuevoError(CodigoSesionCaducada, "", err)
	}
	if !nuevo.puedeRenovar() {
		nuevo.refresco, actual.refresco = actual.refresco, nil
	}
	actual.destruir()
	c.sesion = nuevo
	return nil
}

// tokenDesdeRespuesta copia el token (y el refresh_token, si lo hay) a
// memoria protegida y borra siempre el texto decodificado de la respuesta.
func tokenDesdeRespuesta(r *respuestaToken) (*token, error) {
	defer func() {
		secmem.Zeroize(r.AccessToken)
		secmem.Zeroize(r.RefreshToken)
	}()
	if r.TokenType != "" && !strings.EqualFold(r.TokenType, "Bearer") {
		return nil, nuevoError(CodigoRespuestaInvalida, "token_type", nil)
	}
	valor, err := cadenaJSONSinCopia(r.AccessToken)
	if err != nil {
		return nil, err
	}
	t := &token{valor: secmem.New(valor)}
	secmem.Zeroize(valor)
	if len(bytes.TrimSpace(r.RefreshToken)) > 0 && string(bytes.TrimSpace(r.RefreshToken)) != "null" {
		refresco, err := cadenaJSONSinCopia(r.RefreshToken)
		if err != nil {
			t.destruir()
			return nil, nuevoError(CodigoRespuestaInvalida, "refresh_token", nil)
		}
		t.refresco = secmem.New(refresco)
		secmem.Zeroize(refresco)
	}
	if r.ExpiresIn > 0 {
		t.caduca = time.Now().Add(duracionAcotada(r.ExpiresIn))
	}
	return t, nil
}

// duracionAcotada convierte segundos a time.Duration sin desbordar.
func duracionAcotada(segundos int64) time.Duration {
	if segundos <= 0 {
		return 0
	}
	if segundos > int64(maxVidaToken/time.Second) {
		return maxVidaToken
	}
	return time.Duration(segundos) * time.Second
}

// cadenaJSONSinCopia extrae el contenido de una cadena JSON sin crear un
// string inmutable cuando no hay secuencias de escape (lo habitual en
// tokens). El llamador debe borrar el resultado.
func cadenaJSONSinCopia(raw json.RawMessage) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' || len(raw) > maxTokenBytes {
		return nil, nuevoError(CodigoRespuestaInvalida, "access_token", nil)
	}
	interior := raw[1 : len(raw)-1]
	if bytes.IndexByte(interior, '\\') >= 0 {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || s == "" {
			return nil, nuevoError(CodigoRespuestaInvalida, "access_token", err)
		}
		return []byte(s), nil
	}
	for _, b := range interior {
		if b < 0x20 || b > 0x7e {
			return nil, nuevoError(CodigoRespuestaInvalida, "access_token", nil)
		}
	}
	return append([]byte(nil), interior...), nil
}

// revocar pide al servidor OAuth que invalide el token y su refresh_token
// (RFC 7009). Es un esfuerzo razonable: si falla, el token caduca igualmente
// y ya se ha borrado de la memoria del proceso.
func (c *Cliente) revocar(t *token) {
	if c.oauth == nil || c.oauth.revocar == nil || t == nil {
		return
	}
	if t.vigente(time.Now()) {
		c.revocarValor(t.valor, "access_token")
	}
	if t.puedeRenovar() {
		c.revocarValor(t.refresco, "refresh_token")
	}
}

func (c *Cliente) revocarValor(valor *secmem.Blob, tipo string) {
	// El formulario se compone en un []byte para no copiar el token a un
	// string que no se podría borrar.
	cuerpo := make([]byte, 0, 3*valor.Len()+len(c.opc.ClientID)*3+64)
	cuerpo = append(cuerpo, "token="...)
	cuerpo = anadirPorcentaje(cuerpo, valor.Bytes())
	cuerpo = append(cuerpo, "&token_type_hint="...)
	cuerpo = append(cuerpo, tipo...)
	cuerpo = append(cuerpo, "&client_id="...)
	cuerpo = anadirPorcentaje(cuerpo, []byte(c.opc.ClientID))
	defer secmem.Zeroize(cuerpo)
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	_ = enviar(ctx, c.http, c.oauth.revocar.String(), nil, tipoFormulario, cuerpo, nil)
}
