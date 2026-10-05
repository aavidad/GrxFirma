// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// maxCredenciales limita cuántos identificadores se aceptan del listado.
	maxCredenciales = 100
	maxClientID     = 256
)

// TipoSecreto distingue el dato que se pide a la persona.
type TipoSecreto string

const (
	SecretoPIN TipoSecreto = "pin"
	SecretoOTP TipoSecreto = "otp"
)

// Opciones configura el cliente. No existe secreto de cliente: GrxFirma es un
// cliente público (RFC 8252) y se protege con PKCE.
type Opciones struct {
	// URLServicio es el URL base del servicio CSC (https). Si no termina en
	// /csc/v2 se añade.
	URLServicio string
	// ClientID es el identificador OAuth registrado por el prestador.
	ClientID string
	// HTTP es el cliente base (proxy de la organización, raíces de prueba).
	// El paquete lo endurece: no se usa tal cual.
	HTTP *http.Client
	// AbrirNavegador abre el URL de autorización en el navegador del sistema.
	AbrirNavegador func(ctx context.Context, destino string) error
	// PedirSecreto obtiene un PIN u OTP. El llamador del paquete lee por
	// stdin sin eco; el paquete borra el valor tras usarlo.
	PedirSecreto func(ctx context.Context, tipo TipoSecreto) ([]byte, error)
	// TextoCallback es el texto ya traducido que ve la persona en el
	// navegador al volver de la autorización.
	TextoCallback string
	// EsperaAutorizacion limita la espera del navegador.
	EsperaAutorizacion time.Duration
	// Idioma se envía como "lang" al descubrir el servicio (opcional).
	Idioma string
	// ParesOAuth autoriza servidores OAuth en un host distinto del servicio.
	// Debe venir de la configuración o de la política, nunca del servicio.
	ParesOAuth []ParOAuth
}

// InfoServicio es la parte de la respuesta de /info que usa el cliente.
type InfoServicio struct {
	Specs    string   `json:"specs"`
	Name     string   `json:"name"`
	Region   string   `json:"region"`
	AuthType []string `json:"authType"`
	OAuth2   string   `json:"oauth2"`
	Methods  []string `json:"methods"`
}

// Cliente habla con un servicio CSC. No es seguro copiarlo; sí usarlo desde
// varias goroutines.
type Cliente struct {
	opc   Opciones
	http  *http.Client
	base  *url.URL
	oauth *url.URL

	mu      sync.Mutex
	info    *InfoServicio
	sesion  *token
	cerrado bool
}

// Nuevo valida las opciones sin contactar todavía con el servicio.
func Nuevo(opc Opciones) (*Cliente, error) {
	base, err := baseAPI(opc.URLServicio)
	if err != nil {
		return nil, err
	}
	if !clientIDValido(opc.ClientID) {
		return nil, nuevoError(CodigoParametroInvalido, "client_id", nil)
	}
	if opc.AbrirNavegador == nil {
		return nil, nuevoError(CodigoParametroInvalido, "browser", nil)
	}
	cliente, err := nuevoClienteHTTP(opc.HTTP)
	if err != nil {
		return nil, err
	}
	return &Cliente{opc: opc, http: cliente, base: base}, nil
}

func clientIDValido(id string) bool {
	if id == "" || len(id) > maxClientID {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] <= 0x20 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

// Info descubre el servicio (POST /csc/v2/info). No requiere autorización.
func (c *Cliente) Info(ctx context.Context) (*InfoServicio, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.infoLocked(ctx)
}

func (c *Cliente) infoLocked(ctx context.Context) (*InfoServicio, error) {
	if c.cerrado {
		return nil, nuevoError(CodigoSesionCerrada, "", nil)
	}
	if c.info != nil {
		copia := *c.info
		return &copia, nil
	}
	peticion := map[string]string{}
	if c.opc.Idioma != "" {
		peticion["lang"] = c.opc.Idioma
	}
	var info InfoServicio
	if err := postJSON(ctx, c.http, unirRuta(c.base, "info"), nil, peticion, &info); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(strings.TrimSpace(info.Specs), "2.") {
		return nil, nuevoError(CodigoRespuestaInvalida, "specs", nil)
	}
	if !contiene(info.AuthType, "oauth2code") || strings.TrimSpace(info.OAuth2) == "" {
		return nil, nuevoError(CodigoSinOAuth, "", nil)
	}
	oauth, err := validarURLSegura(info.OAuth2)
	if err != nil {
		return nil, err
	}
	if !oauthPermitido(c.base, oauth, c.opc.ParesOAuth) {
		return nil, nuevoError(CodigoOAuthOtroHost, "", nil)
	}
	c.oauth = oauth
	c.info = &info
	copia := info
	return &copia, nil
}

// Autorizar obtiene el token de servicio (scope "service") mediante el
// navegador del sistema. Si ya hay uno vigente no vuelve a pedirlo.
func (c *Cliente) Autorizar(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.infoLocked(ctx); err != nil {
		return err
	}
	if c.sesion.vigente(time.Now()) {
		return nil
	}
	t, err := c.autorizarOAuth(ctx, "service", nil)
	if err != nil {
		return err
	}
	c.sesion.destruir()
	c.sesion = t
	return nil
}

// tokenSesion devuelve el token de servicio vigente; el llamador tiene c.mu.
func (c *Cliente) tokenSesion() ([]byte, error) {
	if c.cerrado {
		return nil, nuevoError(CodigoSesionCerrada, "", nil)
	}
	if !c.sesion.vigente(time.Now()) {
		return nil, nuevoError(CodigoAutorizacionCaducada, "", nil)
	}
	return c.sesion.bytes(), nil
}

type peticionListado struct {
	MaxResults int `json:"maxResults"`
}

type respuestaListado struct {
	CredentialIDs []string `json:"credentialIDs"`
}

// ListarCredenciales devuelve los identificadores de credencial de la
// persona autorizada (credentials/list).
func (c *Cliente) ListarCredenciales(ctx context.Context) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, err := c.tokenSesion()
	if err != nil {
		return nil, err
	}
	var respuesta respuestaListado
	if err := postJSON(ctx, c.http, unirRuta(c.base, "credentials/list"), t, peticionListado{MaxResults: maxCredenciales}, &respuesta); err != nil {
		return nil, err
	}
	if len(respuesta.CredentialIDs) > maxCredenciales {
		return nil, nuevoError(CodigoDemasiadasCredenciales, "", nil)
	}
	ids := make([]string, 0, len(respuesta.CredentialIDs))
	for _, id := range respuesta.CredentialIDs {
		if !credencialIDValido(id) {
			return nil, nuevoError(CodigoRespuestaInvalida, "credentialID", nil)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// Close revoca el token de servicio, lo borra de la memoria e impide nuevas
// operaciones. Es idempotente.
func (c *Cliente) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cerrado {
		return nil
	}
	c.cerrado = true
	c.revocar(c.sesion)
	c.sesion.destruir()
	c.sesion = nil
	return nil
}

func contiene(lista []string, buscado string) bool {
	for _, v := range lista {
		if strings.EqualFold(strings.TrimSpace(v), buscado) {
			return true
		}
	}
	return false
}
