// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"grxfirma/internal/adapters/outbound/common/secmem"
)

const (
	// maxRespuestaBytes acota cualquier respuesta del servicio. Una
	// credencial con su cadena ocupa pocos KiB; 1 MiB deja margen sin
	// permitir agotar memoria.
	maxRespuestaBytes = 1 << 20
	// tiempoPeticion es el máximo de cada petición HTTP completa.
	tiempoPeticion = 30 * time.Second
	// tiempoNegociacionTLS acota el establecimiento de la conexión segura.
	tiempoNegociacionTLS = 10 * time.Second
	rutaAPI              = "/csc/v2"
)

var errRedireccion = errors.New("csc: redirection refused")

// nuevoClienteHTTP deriva un cliente endurecido del recibido (para heredar el
// proxy de la organización o, en pruebas, las raíces del servidor simulado):
// TLS 1.2 como mínimo con verificación obligatoria, tiempos máximos y
// ninguna redirección.
func nuevoClienteHTTP(base *http.Client) *http.Client {
	var transporte http.RoundTripper = http.DefaultTransport
	if base != nil && base.Transport != nil {
		transporte = base.Transport
	}
	if t, ok := transporte.(*http.Transport); ok {
		c := t.Clone()
		if c.TLSClientConfig == nil {
			c.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		if c.TLSClientConfig.MinVersion < tls.VersionTLS12 {
			c.TLSClientConfig.MinVersion = tls.VersionTLS12
		}
		// La verificación del certificado del servidor no es negociable.
		c.TLSClientConfig.InsecureSkipVerify = false
		c.TLSHandshakeTimeout = tiempoNegociacionTLS
		c.ResponseHeaderTimeout = tiempoPeticion
		transporte = c
	}
	return &http.Client{
		Transport: transporte,
		Timeout:   tiempoPeticion,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedireccion
		},
	}
}

// validarURLSegura exige https, un host y ningún componente que pueda
// ocultar el destino real (credenciales embebidas, fragmento o consulta).
func validarURLSegura(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nuevoError(CodigoURLInvalida, "", nil)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, nuevoError(CodigoURLInvalida, "", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, nuevoError(CodigoSoloHTTPS, u.Scheme, nil)
	}
	if u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Opaque != "" {
		return nil, nuevoError(CodigoURLInvalida, "", nil)
	}
	u.Scheme = "https"
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u, nil
}

// baseAPI normaliza el URL del servicio para que termine en /csc/v2.
func baseAPI(raw string) (*url.URL, error) {
	u, err := validarURLSegura(raw)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(u.Path, rutaAPI) {
		u.Path += rutaAPI
	}
	return u, nil
}

func unirRuta(base *url.URL, ruta string) string {
	copia := *base
	copia.Path = strings.TrimRight(copia.Path, "/") + "/" + strings.TrimLeft(ruta, "/")
	return copia.String()
}

type respuestaErrorServicio struct {
	Error string `json:"error"`
}

// postJSON envía una petición JSON y decodifica la respuesta. token, si no
// es nil, viaja como portador; el cuerpo se borra al terminar porque puede
// contener PIN, OTP o SAD.
func postJSON(ctx context.Context, cliente *http.Client, destino string, token []byte, peticion, respuesta any) error {
	cuerpo, err := json.Marshal(peticion)
	if err != nil {
		return nuevoError(CodigoParametroInvalido, "", err)
	}
	defer secmem.Zeroize(cuerpo)
	return enviar(ctx, cliente, destino, token, "application/json", cuerpo, respuesta)
}

// postFormulario envía un formulario application/x-www-form-urlencoded, que
// es lo que exigen los puntos OAuth 2.0 (RFC 6749, sección 4.1.3).
func postFormulario(ctx context.Context, cliente *http.Client, destino string, campos url.Values, respuesta any) error {
	cuerpo := []byte(campos.Encode())
	defer secmem.Zeroize(cuerpo)
	return enviar(ctx, cliente, destino, nil, "application/x-www-form-urlencoded", cuerpo, respuesta)
}

func enviar(ctx context.Context, cliente *http.Client, destino string, token []byte, tipo string, cuerpo []byte, respuesta any) error {
	if _, err := validarURLSegura(destino); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destino, bytes.NewReader(cuerpo))
	if err != nil {
		return nuevoError(CodigoURLInvalida, "", err)
	}
	req.Header.Set("Content-Type", tipo)
	req.Header.Set("Accept", "application/json")
	if len(token) > 0 {
		req.Header.Set("Authorization", "Bearer "+string(token))
	}
	resp, err := cliente.Do(req)
	if err != nil {
		if errors.Is(err, errRedireccion) {
			return nuevoError(CodigoRedireccion, "", nil)
		}
		// El error de red no incluye la petición: solo el destino, que no es
		// secreto. Se envuelve para diagnóstico local.
		return nuevoError(CodigoRed, "", err)
	}
	defer resp.Body.Close()
	datos, err := io.ReadAll(io.LimitReader(resp.Body, maxRespuestaBytes+1))
	defer secmem.Zeroize(datos)
	if err != nil {
		return nuevoError(CodigoRed, "", err)
	}
	if len(datos) > maxRespuestaBytes {
		return nuevoError(CodigoRespuestaGrande, "", nil)
	}
	if resp.StatusCode != http.StatusOK {
		var fallo respuestaErrorServicio
		detalle := strconv.Itoa(resp.StatusCode)
		if json.Unmarshal(datos, &fallo) == nil && fallo.Error != "" {
			detalle += "/" + fallo.Error
		}
		return nuevoError(CodigoServicio, detalle, nil)
	}
	if respuesta == nil {
		return nil
	}
	if err := json.Unmarshal(datos, respuesta); err != nil {
		return nuevoError(CodigoRespuestaInvalida, "", err)
	}
	return nil
}
