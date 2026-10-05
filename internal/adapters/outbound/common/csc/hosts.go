// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package csc

import (
	"net"
	"net/url"
	"strings"
)

// ParOAuth autoriza que el servicio CSC Servicio delegue la autorización en
// el servidor OAuth OAuth, que está en otro host. Ambos son "host" o
// "host:puerto" (443 si no se indica).
type ParOAuth struct {
	Servicio string
	OAuth    string
}

// ParsearParesOAuth interpreta entradas "servicio=oauth". Cualquier entrada
// mal formada invalida la lista entera: una política que no se entiende no
// puede ampliar lo permitido.
func ParsearParesOAuth(entradas []string) ([]ParOAuth, error) {
	pares := make([]ParOAuth, 0, len(entradas))
	for _, e := range entradas {
		servicio, oauth, ok := strings.Cut(strings.TrimSpace(e), "=")
		if !ok {
			return nil, nuevoError(CodigoParametroInvalido, "oauth_pair", nil)
		}
		s, err := normalizarHost(servicio)
		if err != nil {
			return nil, err
		}
		o, err := normalizarHost(oauth)
		if err != nil {
			return nil, err
		}
		pares = append(pares, ParOAuth{Servicio: s, OAuth: o})
	}
	return pares, nil
}

// normalizarHost devuelve "host:puerto" en minúsculas a partir de
// "host[:puerto]", sin esquema, ruta ni credenciales.
func normalizarHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "/?#@ \t") {
		return "", nuevoError(CodigoParametroInvalido, "oauth_pair", nil)
	}
	u, err := url.Parse("https://" + raw)
	if err != nil || u.Hostname() == "" {
		return "", nuevoError(CodigoParametroInvalido, "oauth_pair", err)
	}
	return claveHost(u), nil
}

// claveHost identifica el destino de un URL https: host en minúsculas y
// puerto explícito.
func claveHost(u *url.URL) string {
	puerto := u.Port()
	if puerto == "" {
		puerto = "443"
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), puerto)
}

// oauthPermitido exige que el servidor OAuth anunciado por /info esté en el
// mismo host que el servicio, salvo que un par configurado lo autorice. Sin
// esta comprobación, un -csc-url falso con el client_id real de un
// prestador recibiría el token, el PIN y el OTP.
func oauthPermitido(servicio, oauth *url.URL, pares []ParOAuth) bool {
	s, o := claveHost(servicio), claveHost(oauth)
	if s == o {
		return true
	}
	for _, p := range pares {
		if p.Servicio == s && p.OAuth == o {
			return true
		}
	}
	return false
}
