// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package domain

import (
	"errors"
	"net/url"
	"strings"
)

// ExchangeSessionState representa el estado de una sesion de intercambio remoto.
type ExchangeSessionState string

const (
	SessionActive    ExchangeSessionState = "active"
	SessionCompleted ExchangeSessionState = "completed"
	SessionCancelled ExchangeSessionState = "cancelled"
)

// ExchangeSession es un identificador abstracto de sesion de intercambio con un servidor remoto.
//
// Encapsula los datos necesarios para que el puerto ResultTransport pueda realizar operaciones
// de upload, retrieve, wait y cancel sin que el nucleo conozca el protocolo subyacente.
//
// Los campos UploadEndpoint y RetrieveEndpoint son URIs opacos asignados por el adaptador
// en la capa anticorrupcion al parsear la peticion entrante. El nucleo los trata como tokens
// opacos y no los interpreta.
type ExchangeSession struct {
	// RequestID identifica de forma unica la solicitud en el servidor remoto.
	RequestID string

	// SessionKey es la clave de sesion para autenticar las operaciones de intercambio.
	SessionKey string

	// UploadEndpoint es el URI opaco al que subir el resultado.
	UploadEndpoint string

	// RetrieveEndpoint es el URI opaco desde el que recuperar la solicitud.
	RetrieveEndpoint string

	// State es el estado actual de la sesion.
	State ExchangeSessionState
}

func (s ExchangeSession) Validate() error {
	if s.RequestID == "" {
		return errors.New("la sesion de intercambio debe tener un identificador de solicitud")
	}
	if s.UploadEndpoint == "" || s.RetrieveEndpoint == "" {
		return errors.New("la sesion de intercambio debe tener endpoints de subida y descarga")
	}
	return nil
}

// OrigenesEndpoints devuelve los orígenes (esquema://host[:puerto]) de la
// subida y la descarga, sin repetir, en ese orden. Son los que tiene que
// admitir la política de dominios de confianza antes de usar la sesión. Un
// endpoint sin esquema o sin host es un error.
func (s ExchangeSession) OrigenesEndpoints() ([]string, error) {
	out := make([]string, 0, 2)
	vistos := map[string]bool{}
	for _, endpoint := range []string{s.UploadEndpoint, s.RetrieveEndpoint} {
		u, err := url.Parse(strings.TrimSpace(endpoint))
		if err != nil {
			return nil, errors.New("endpoint de la sesion de intercambio no valido")
		}
		if u.Scheme == "" || u.Host == "" {
			return nil, errors.New("endpoint de la sesion de intercambio sin esquema u host")
		}
		origen := u.Scheme + "://" + u.Host
		clave := strings.ToLower(origen)
		if vistos[clave] {
			continue
		}
		vistos[clave] = true
		out = append(out, origen)
	}
	return out, nil
}

// HostsDistintos dice si la descarga y la subida van a hosts distintos. Es
// válido si los dos son de confianza, pero se anota: el resultado firmado no
// vuelve al mismo servidor que entregó el documento.
func (s ExchangeSession) HostsDistintos() bool {
	descarga, err1 := url.Parse(strings.TrimSpace(s.RetrieveEndpoint))
	subida, err2 := url.Parse(strings.TrimSpace(s.UploadEndpoint))
	if err1 != nil || err2 != nil {
		return true
	}
	return !strings.EqualFold(descarga.Hostname(), subida.Hostname())
}

func (s ExchangeSession) IsActive() bool {
	return s.State == SessionActive
}

func (s ExchangeSession) WithState(state ExchangeSessionState) ExchangeSession {
	s.State = state
	return s
}
