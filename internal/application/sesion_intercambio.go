// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"grxfirma/internal/domain"
	"grxfirma/internal/ports"
)

// ErrSinPoliticaConfianza indica que un caso de uso que habla con un servidor
// remoto se ha construido sin política de dominios de confianza. Falla
// cerrado: sin política no se descarga ni se sube nada.
var ErrSinPoliticaConfianza = errors.New("politica de dominios de confianza no configurada")

// ComprobarSesionIntercambio es la invariante común a todos los caminos que
// descargan una petición o suben un resultado (H-03): la sesión tiene que
// ser válida y la política de dominios de confianza tiene que admitir el
// origen de la descarga y el de la subida. Si son de hosts distintos, los dos
// tienen que ser de confianza, y se anota en el registro sin el identificador
// ni la clave de la sesión.
//
// Cada origen se evalúa una sola vez, porque la política puede preguntar a la
// persona. El formato del endpoint (https, http en loopback o el http de
// intranet del canal socket) lo siguen comprobando el adaptador y el
// transporte.
func ComprobarSesionIntercambio(ctx context.Context, politica ports.TrustPolicy, sesion domain.ExchangeSession) error {
	if politica == nil {
		return ErrSinPoliticaConfianza
	}
	if err := sesion.Validate(); err != nil {
		return err
	}
	origenes, err := sesion.OrigenesEndpoints()
	if err != nil {
		return err
	}
	for _, origen := range origenes {
		decision, err := politica.Evaluate(ctx, origen)
		if err != nil {
			return fmt.Errorf("no se pudo validar el origen %s: %w", origen, err)
		}
		if !decision.IsAllowed() {
			return fmt.Errorf("el origen %s no esta autorizado", origen)
		}
	}
	if sesion.HostsDistintos() {
		// Retrocompatibilidad: AutoFirma deja al portal elegir por separado
		// la descarga y la subida, y la descarga puede salir de serverUrl.
		// Los dos son de confianza, así que se admite y queda anotado.
		slog.Warn("sesion_intercambio_hosts_distintos", "origenes", len(origenes))
	}
	return nil
}
