// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"grxfirma/internal/adapters/outbound/common/revocationclient"
)

// modoTSAOpciones dice qué destinos admite la tsaURL que llega en las opciones
// del trabajo de firma, según quién elige esas opciones (E1-12 y lote 13a).
type modoTSAOpciones int

const (
	// tsaOpcionesWeb: las opciones las elige una página web (afirma://,
	// WebSocket, Native Messaging). Sin loopback, redes privadas ni enlace
	// local, salvo las TSA de la lista de la política de máquina. Es el valor
	// por defecto.
	tsaOpcionesWeb modoTSAOpciones = iota
	// tsaOpcionesUsuario: las elige el propio usuario del equipo (CLI,
	// interfaz de escritorio y sus ajustes). Cualquier destino.
	tsaOpcionesUsuario
	// tsaOpcionesREST: servidor REST local. Por compatibilidad se admite
	// una TSA interna con aviso en el registro; si la política define la
	// lista de TSA permitidas, solo valen las de la lista.
	tsaOpcionesREST
)

// PermitirTSADeOpcionesEnRedLocal deja que la tsaURL de las opciones apunte a
// cualquier destino, también la red interna. Solo para superficies en las que
// las opciones las fija el propio usuario del equipo (la CLI, la interfaz de
// escritorio y sus ajustes), nunca para las que atienden a páginas web.
func (m *MotorFirmaGo) PermitirTSADeOpcionesEnRedLocal() *MotorFirmaGo {
	if m == nil {
		return nil
	}
	m.tsaOpciones = tsaOpcionesUsuario
	return m
}

// AvisarTSADeOpcionesEnRedLocal es el modo del servidor REST local: una
// tsaURL en la red interna se admite (los organismos con TSA interna la usan
// así) y queda un aviso en el registro con el host. Si la política define la
// lista de TSA permitidas (ConTSAPermitidas), solo se admiten las de la lista.
func (m *MotorFirmaGo) AvisarTSADeOpcionesEnRedLocal() *MotorFirmaGo {
	if m == nil {
		return nil
	}
	m.tsaOpciones = tsaOpcionesREST
	return m
}

// ConTSAPermitidas fija la lista de TSA permitidas de la política de máquina
// (clave tsa_permitidas). nil deja la lista sin definir; una lista definida,
// aunque esté vacía, cuenta como definida. Se compara el origen: esquema, host
// y puerto. En REST restringe: solo valen las de la lista. En los caminos web
// solo exceptúa: una TSA interna de la lista se admite y las públicas siguen
// valiendo. En la CLI y la interfaz de escritorio no se aplica.
func (m *MotorFirmaGo) ConTSAPermitidas(lista []string) *MotorFirmaGo {
	if m == nil {
		return nil
	}
	if lista == nil {
		m.tsaPermitidas = nil
		return m
	}
	m.tsaPermitidas = make(map[string]struct{}, len(lista))
	for _, entrada := range lista {
		if origen, ok := origenTSA(entrada); ok {
			m.tsaPermitidas[origen] = struct{}{}
		} else {
			slog.Warn("tsa_permitidas_entrada_no_valida")
		}
	}
	return m
}

// clienteHTTPTSAOpciones devuelve el cliente HTTP con el que se consulta una
// tsaURL recibida en las opciones, según el modo del motor.
func (m *MotorFirmaGo) clienteHTTPTSAOpciones(tsaURL string, base *http.Client) *http.Client {
	modo := tsaOpcionesWeb
	var permitidas map[string]struct{}
	if m != nil {
		modo = m.tsaOpciones
		permitidas = m.tsaPermitidas
	}
	if modo == tsaOpcionesUsuario {
		return base
	}
	origen, valido := origenTSA(tsaURL)
	_, listada := permitidas[origen]
	listada = listada && valido
	switch modo {
	case tsaOpcionesREST:
		if permitidas != nil {
			if !listada {
				slog.Warn("tsa_opciones_no_permitida", "host", hostTSA(tsaURL))
				return clienteTSARechazada()
			}
			return base
		}
		return avisarTSAEnRedLocal(base)
	default:
		if listada {
			return base
		}
		// La URL la elige una página web: mismos destinos permitidos que
		// OCSP, CRL y AIA (sin loopback, redes privadas ni enlace local,
		// comprobado al resolver y al conectar).
		return revocationclient.ProtegerClienteHTTP(base)
	}
}

// origenTSA normaliza una URL a esquema://host:puerto en minúsculas.
func origenTSA(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	esquema := strings.ToLower(u.Scheme)
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if (esquema != "http" && esquema != "https") || host == "" {
		return "", false
	}
	puerto := u.Port()
	if puerto == "" {
		puerto = "443"
		if esquema == "http" {
			puerto = "80"
		}
	}
	return esquema + "://" + net.JoinHostPort(host, puerto), true
}

// hostTSA devuelve solo el host de la URL, para el registro: sin credenciales,
// ruta ni consulta.
func hostTSA(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func clienteTSARechazada() *http.Client {
	return &http.Client{Transport: transporteError{err: errors.New("TSA no permitida por la política de máquina (tsa_permitidas)")}}
}

type transporteError struct{ err error }

func (t transporteError) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

// avisarTSAEnRedLocal envuelve el transporte para anotar en el registro las
// peticiones a una TSA en la red interna, sin bloquearlas. Solo se anota el
// host.
func avisarTSAEnRedLocal(base *http.Client) *http.Client {
	cliente := &http.Client{}
	if base != nil {
		*cliente = *base
	}
	transporte := cliente.Transport
	if transporte == nil {
		transporte = http.DefaultTransport
	}
	cliente.Transport = transporteAvisoRedLocal{base: transporte}
	return cliente
}

type transporteAvisoRedLocal struct{ base http.RoundTripper }

func (t transporteAvisoRedLocal) RoundTrip(req *http.Request) (*http.Response, error) {
	if req != nil && req.URL != nil && destinoEnRedLocal(req) {
		slog.WarnContext(req.Context(), "tsa_opciones_red_local", "host", req.URL.Hostname())
	}
	return t.base.RoundTrip(req)
}

func destinoEnRedLocal(req *http.Request) bool {
	host := strings.TrimSuffix(req.URL.Hostname(), ".")
	if ip := net.ParseIP(host); ip != nil {
		return !revocationclient.EsDireccionPublica(ip)
	}
	direcciones, err := net.DefaultResolver.LookupIPAddr(req.Context(), host)
	if err != nil {
		return false
	}
	for _, d := range direcciones {
		if !revocationclient.EsDireccionPublica(d.IP) {
			return true
		}
	}
	return false
}
