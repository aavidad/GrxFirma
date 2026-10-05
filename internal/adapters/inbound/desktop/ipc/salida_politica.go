// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package ipc

import (
	"context"
	"errors"
	"strings"

	"grxfirma/internal/adapters/outbound/desktop/filesystem"
)

// La política de sobrescritura se decide siempre aquí, en el motor, tanto si
// el cliente manda la ruta de salida como si la calcula el motor:
//
//   - overwriteConfirmed=true: la persona confirmó el reemplazo de esa ruta
//     exacta en un diálogo de guardar del sistema. Es la única forma de
//     reemplazar un fichero que no pasa por la preferencia.
//   - overwrite con valor reconocido: la política que el cliente copia de la
//     preferencia de la persona («rename», «fail» o «force»).
//   - overwrite vacío: la preferencia guardada (signOverwrite).
//   - cualquier otro caso: renombrar, que nunca destruye datos.
//
// La escritura final se publica en exclusiva (enlace duro u O_EXCL), de modo
// que un fichero que aparezca entre la comprobación y la escritura tampoco se
// reemplaza salvo con «force» o con la confirmación explícita.

// politicaSobrescrituraDesdeTexto traduce el valor del protocolo. El segundo
// resultado es false si el valor está vacío o no se reconoce.
func politicaSobrescrituraDesdeTexto(valor string) (filesystem.PoliticaSobreescritura, bool) {
	switch strings.ToLower(strings.TrimSpace(valor)) {
	case "force", "overwrite", "true":
		return filesystem.PoliticaForzar, true
	case "fail", "error":
		return filesystem.PoliticaFallar, true
	case "rename":
		return filesystem.PoliticaRenombrar, true
	default:
		return filesystem.PoliticaRenombrar, false
	}
}

// politicaSalidaIPC resuelve la política efectiva de una petición.
func (m *Manejador) politicaSalidaIPC(ctx context.Context, solicitada string, confirmada bool) filesystem.PoliticaSobreescritura {
	if confirmada {
		return filesystem.PoliticaForzar
	}
	if politica, ok := politicaSobrescrituraDesdeTexto(solicitada); ok {
		return politica
	}
	if strings.TrimSpace(solicitada) != "" {
		return filesystem.PoliticaRenombrar
	}
	if doc, err := m.cargarPreferenciasFirma(ctx); err == nil && doc.Firma.Overwrite != nil {
		if politica, ok := politicaSobrescrituraDesdeTexto(*doc.Firma.Overwrite); ok {
			return politica
		}
	}
	return filesystem.PoliticaRenombrar
}

// guardarSalidaIPC escribe un resultado en una carpeta que ya existe (validada
// con validarRutaEscritura) aplicando la política. Devuelve la ruta real.
func (m *Manejador) guardarSalidaIPC(ctx context.Context, ruta, solicitada string, confirmada bool, datos []byte) (string, error) {
	politica := m.politicaSalidaIPC(ctx, solicitada, confirmada)
	return filesystem.NuevoEscritorResultado(politica).EscribirEnDirectorioExistente(ruta, datos)
}

// guardarSalidaEstrictaIPC es como guardarSalidaIPC pero crea las carpetas que
// falten y rechaza enlaces en ellas, como hacían ya proteger, huellas y
// exportar certificado.
func (m *Manejador) guardarSalidaEstrictaIPC(ctx context.Context, ruta, solicitada string, confirmada bool, datos []byte) (string, error) {
	politica := m.politicaSalidaIPC(ctx, solicitada, confirmada)
	return filesystem.NuevoEscritorResultado(politica).Escribir(ruta, datos)
}

// mensajeErrorSalidaIPC devuelve un mensaje claro cuando la política impidió
// reemplazar un fichero existente, o el genérico indicado en otro caso.
func (m *Manejador) mensajeErrorSalidaIPC(err error, ruta, generico string) string {
	if errors.Is(err, filesystem.ErrSalidaExiste) {
		return m.t("error.salida_existe", ruta)
	}
	return generico
}
