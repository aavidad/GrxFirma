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
	// maxCredencialesPagina es lo que se pide en cada página del listado y
	// lo máximo que se acepta en una.
	maxCredencialesPagina = 100
	// maxCredenciales limita cuántos identificadores se aceptan en total
	// sumando todas las páginas.
	maxCredenciales = 1000
	// maxPaginasListado corta un servidor que no deja de paginar.
	maxPaginasListado = 20
	maxPageToken      = 1024
	maxClientID       = 256
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
	// EnvioOTPManual hace que la firma no pida al servicio el OTP en línea:
	// lo pide quien llama con [Cliente.EnviarOTP] antes de firmar. Lo usa la
	// interfaz gráfica, que necesita mostrar el código antes de enviar la
	// firma; la CLI deja que lo pida la propia autorización.
	EnvioOTPManual bool
}

// InfoServicio es la parte de la respuesta de /info que usa el cliente.
type InfoServicio struct {
	Specs    string   `json:"specs"`
	Name     string   `json:"name"`
	Region   string   `json:"region"`
	AuthType []string `json:"authType"`
	OAuth2   string   `json:"oauth2"`
	// OAuth2Issuer (CSC 2.1) es el emisor cuyos metadatos RFC 8414 dicen
	// dónde están los extremos de autorización. Si viene, tiene prioridad
	// sobre OAuth2.
	OAuth2Issuer string   `json:"oauth2Issuer"`
	Methods      []string `json:"methods"`
}

// Cliente habla con un servicio CSC. No es seguro copiarlo; sí usarlo desde
// varias goroutines.
type Cliente struct {
	opc   Opciones
	http  *http.Client
	base  *url.URL
	oauth *puntosOAuth

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
	emisor := strings.TrimSpace(info.OAuth2Issuer)
	if !contiene(info.AuthType, "oauth2code") || (strings.TrimSpace(info.OAuth2) == "" && emisor == "") {
		return nil, nuevoError(CodigoSinOAuth, "", nil)
	}
	var puntos *puntosOAuth
	if emisor != "" {
		p, err := c.descubrirOAuth(ctx, emisor)
		if err != nil {
			return nil, err
		}
		puntos = p
	} else {
		oauth, err := validarURLSegura(info.OAuth2)
		if err != nil {
			return nil, err
		}
		if !oauthPermitido(c.base, oauth, c.opc.ParesOAuth) {
			return nil, nuevoError(CodigoOAuthOtroHost, "", nil)
		}
		puntos = puntosDesdeBase(oauth)
	}
	c.oauth = puntos
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
	if c.sesion.vigente(time.Now().Add(margenRenovacion)) {
		return nil
	}
	if c.sesion.puedeRenovar() && c.renovarSesion(ctx) == nil {
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
// Si está a punto de caducar y el servidor dio un refresh_token, lo renueva.
// Si ha caducado y no se puede renovar, pide volver a conectar.
func (c *Cliente) tokenSesion(ctx context.Context) ([]byte, error) {
	if c.cerrado {
		return nil, nuevoError(CodigoSesionCerrada, "", nil)
	}
	ahora := time.Now()
	if c.sesion.vigente(ahora.Add(margenRenovacion)) {
		return c.sesion.bytes(), nil
	}
	if c.sesion.puedeRenovar() && c.renovarSesion(ctx) == nil {
		return c.sesion.bytes(), nil
	}
	if c.sesion.vigente(ahora) {
		return c.sesion.bytes(), nil
	}
	if c.sesion == nil {
		return nil, nuevoError(CodigoAutorizacionCaducada, "", nil)
	}
	return nil, nuevoError(CodigoSesionCaducada, "", nil)
}

// sesionRechazada trata un 401 del servicio con el token de servicio: el
// token ya no vale aunque su caducidad local no haya llegado. Se marca como
// caducado para que la siguiente operación lo renueve o pida conectar. El
// llamador tiene c.mu.
func (c *Cliente) sesionRechazada(err error) error {
	if !es401(err) || c.sesion == nil {
		return err
	}
	c.sesion.caduca = time.Now().Add(-time.Second)
	return nuevoError(CodigoSesionCaducada, "", err)
}

// postServicio envía una petición idempotente con el token de servicio. Si
// el servicio responde 401 y hay refresh_token, renueva y repite una vez.
// El llamador tiene c.mu.
func (c *Cliente) postServicio(ctx context.Context, ruta string, peticion, respuesta any) error {
	for intento := 0; ; intento++ {
		t, err := c.tokenSesion(ctx)
		if err != nil {
			return err
		}
		err = postJSON(ctx, c.http, unirRuta(c.base, ruta), t, peticion, respuesta)
		if err == nil || !es401(err) {
			return err
		}
		err = c.sesionRechazada(err)
		if intento > 0 || !c.sesion.puedeRenovar() {
			return err
		}
	}
}

type peticionListado struct {
	MaxResults int    `json:"maxResults"`
	PageToken  string `json:"pageToken,omitempty"`
}

type respuestaListado struct {
	CredentialIDs []string `json:"credentialIDs"`
	NextPageToken string   `json:"nextPageToken"`
}

// ListarCredenciales devuelve los identificadores de credencial de la
// persona autorizada (credentials/list), recorriendo todas las páginas
// (pageToken/nextPageToken) hasta [maxCredenciales] identificadores y
// [maxPaginasListado] páginas. Un token de página repetido o mal formado
// invalida el listado, para que un servidor defectuoso no lo haga infinito.
func (c *Cliente) ListarCredenciales(ctx context.Context) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, maxCredencialesPagina)
	vistos := make(map[string]bool, maxCredencialesPagina)
	tokensVistos := map[string]bool{}
	pagina := ""
	for n := 0; ; n++ {
		if n >= maxPaginasListado {
			return nil, nuevoError(CodigoDemasiadasCredenciales, "", nil)
		}
		var respuesta respuestaListado
		peticion := peticionListado{MaxResults: maxCredencialesPagina, PageToken: pagina}
		if err := c.postServicio(ctx, "credentials/list", peticion, &respuesta); err != nil {
			return nil, err
		}
		if len(respuesta.CredentialIDs) > maxCredencialesPagina {
			return nil, nuevoError(CodigoDemasiadasCredenciales, "", nil)
		}
		for _, id := range respuesta.CredentialIDs {
			if !credencialIDValido(id) {
				return nil, nuevoError(CodigoRespuestaInvalida, "credentialID", nil)
			}
			if vistos[id] {
				continue
			}
			if len(ids) >= maxCredenciales {
				return nil, nuevoError(CodigoDemasiadasCredenciales, "", nil)
			}
			vistos[id] = true
			ids = append(ids, id)
		}
		siguiente := respuesta.NextPageToken
		if siguiente == "" {
			return ids, nil
		}
		if !pageTokenValido(siguiente) || tokensVistos[siguiente] {
			return nil, nuevoError(CodigoRespuestaInvalida, "nextPageToken", nil)
		}
		tokensVistos[siguiente] = true
		pagina = siguiente
	}
}

func pageTokenValido(t string) bool {
	if len(t) > maxPageToken {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] < 0x21 || t[i] > 0x7e {
			return false
		}
	}
	return true
}

// ServidorOAuth devuelve el extremo de autorización que se abrirá en el
// navegador, sin consulta, para mostrar su host antes de conectar. Está
// vacío hasta que se ha descubierto el servicio.
func (c *Cliente) ServidorOAuth() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.oauth == nil {
		return ""
	}
	return c.oauth.autorizar.String()
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
