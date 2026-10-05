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
	bytesAleatorios              = 32
)

// token es un token OAuth en memoria bloqueada (best effort) que se borra
// con destruir. Nunca se registra ni se serializa.
type token struct {
	valor  *secmem.Blob
	caduca time.Time
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
}

type respuestaToken struct {
	AccessToken json.RawMessage `json:"access_token"`
	TokenType   string          `json:"token_type"`
	ExpiresIn   int64           `json:"expires_in"`
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
	autorizacion := *c.oauth
	autorizacion.Path = strings.TrimRight(autorizacion.Path, "/") + "/oauth2/authorize"
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

// manejadorCallback acepta una única respuesta en GET /callback. Un state
// distinto, ausente o un Host inesperado terminan el flujo: se prefiere
// fallar cerrado antes que aceptar un código que no se pidió.
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
			responder(w, http.StatusBadRequest)
			entregar(resultadoCallback{err: nuevoError(CodigoStateInvalido, "", nil)})
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
	destino := *c.oauth
	destino.Path = strings.TrimRight(destino.Path, "/") + "/oauth2/token"
	campos := url.Values{}
	campos.Set("grant_type", "authorization_code")
	campos.Set("code", codigo)
	campos.Set("redirect_uri", redireccion)
	campos.Set("client_id", c.opc.ClientID)
	campos.Set("code_verifier", verificador)

	var respuesta respuestaToken
	defer secmem.Zeroize(respuesta.AccessToken)
	if err := postFormulario(ctx, c.http, destino.String(), campos, &respuesta); err != nil {
		return nil, err
	}
	if respuesta.TokenType != "" && !strings.EqualFold(respuesta.TokenType, "Bearer") {
		return nil, nuevoError(CodigoRespuestaInvalida, "token_type", nil)
	}
	valor, err := cadenaJSONSinCopia(respuesta.AccessToken)
	if err != nil {
		return nil, err
	}
	t := &token{valor: secmem.New(valor)}
	secmem.Zeroize(valor)
	if respuesta.ExpiresIn > 0 {
		t.caduca = time.Now().Add(time.Duration(respuesta.ExpiresIn) * time.Second)
	}
	return t, nil
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

// revocar pide al servidor OAuth que invalide el token (RFC 7009). Es un
// esfuerzo razonable: si falla, el token caduca igualmente y ya se ha
// borrado de la memoria del proceso.
func (c *Cliente) revocar(t *token) {
	if c.oauth == nil || !t.vigente(time.Now()) {
		return
	}
	destino := *c.oauth
	destino.Path = strings.TrimRight(destino.Path, "/") + "/oauth2/revoke"
	campos := url.Values{}
	campos.Set("token", string(t.bytes()))
	campos.Set("token_type_hint", "access_token")
	campos.Set("client_id", c.opc.ClientID)
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	_ = postFormulario(ctx, c.http, destino.String(), campos, nil)
}
