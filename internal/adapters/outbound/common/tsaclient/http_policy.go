// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package tsaclient

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

const timestampRequestTimeout = 15 * time.Second

// boundedHTTPClient conserva transport/proxy y políticas más estrictas sin
// modificar el cliente compartido. La URL inicial puede ser interna y estar
// administrada; una redirección no puede cambiar origen ni convertir POST en GET.
func boundedHTTPClient(configured *http.Client) *http.Client {
	client := &http.Client{}
	if configured != nil {
		*client = *configured
	}
	if client.Timeout <= 0 || client.Timeout > timestampRequestTimeout {
		client.Timeout = timestampRequestTimeout
	}
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || len(via) >= 5 {
			return errors.New("demasiadas redirecciones TSA")
		}
		origin := via[0].URL
		if req.Method != http.MethodPost || req.URL.Scheme != origin.Scheme ||
			!strings.EqualFold(req.URL.Host, origin.Host) || req.URL.User != nil {
			return errors.New("la redirección TSA debe conservar origen y método POST")
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
	return client
}
