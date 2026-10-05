// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobilebind

import (
	"sync"

	"grxfirma/internal/adapters/outbound/common/informeverificacion"
	"grxfirma/internal/adapters/outbound/common/localizador"
	"grxfirma/internal/domain"
)

// catalogoMotor resuelve los textos que el motor móvil devuelve a la app.
// Se resuelven en castellano porque la app los traduce con su propio
// catálogo usando esa frase como clave; el texto vive en el catálogo de
// datos, no en el código.
var catalogoMotor = sync.OnceValue(func() *localizador.Localizador { return localizador.Para("es") })

func textoMotor(clave string) string { return catalogoMotor().T(clave) }

// maxReportHTMLBytes acota el informe que cruza JNI.
const maxReportHTMLBytes = 4 << 20

// verificationReportHTML reutiliza el informe imprimible de escritorio
// (plantilla html/template con CSP sin scripts). Sin anclas del sistema la
// confianza no está evaluada: el informe no puede declarar la firma válida,
// así que muestra «firma íntegra · validez del certificado no acreditada».
func (f *Facade) verificationReportHTML(name string, content []byte, result domain.VerificationResult) ([]byte, error) {
	if !f.systemTrustAnchors {
		result.Valid = false
		result.Trust.Status = domain.VerificationStatusUnknown
		result.Trust.Reason = textoMotor("movil.informe.confianza_no_evaluada")
	}
	html, err := informeverificacion.HTML(informeverificacion.Datos{
		NombreDocumento: sanitizeOutputText(name, 200),
		Contenido:       content,
		Resultado:       result,
		Fecha:           f.now(),
		VersionApp:      engineVersion,
	})
	if err != nil {
		return nil, err
	}
	if len(html) > maxReportHTMLBytes {
		return nil, newFacadeError("informe de verificacion demasiado grande")
	}
	return html, nil
}
