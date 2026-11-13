// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package signer

import "strings"

// Cambios posteriores a una firma PDF para el verificador PAdES clásico.
//
// Una firma solo cubre su revisión. Lo que se añade después se clasifica con
// el análisis v2 (InspeccionarPAdESV2), que sigue la cadena xref/Prev igual
// que un visor: solo cuenta lo que el lector resolvería de verdad, no lo que
// parece haber en el texto del fichero.
//
// Invariante: con cobertura parcial, una firma solo puede quedar válida si
// cada revisión posterior sale «ninguno» o «permitidos». Si el análisis falla
// o deja algo sin comprobar, la firma queda indeterminada; si encuentra
// cambios no permitidos, inválida.

const (
	cambiosPDFNinguno       = "ninguno"
	cambiosPDFPermitidos    = "permitidos"
	cambiosPDFNoComprobados = "no_comprobados"
	cambiosPDFNoPermitidos  = "no_permitidos"
)

type revisionesPAdES struct {
	inspeccion InspeccionPDFV2
	err        error
	colaInocua bool
}

func inspeccionarRevisionesPAdES(data []byte) *revisionesPAdES {
	inspeccion, err := InspeccionarPAdESV2(data, maxPAdESV2Signatures, maxPAdESV2Revisions)
	r := &revisionesPAdES{inspeccion: inspeccion, err: err}
	if err == nil && inspeccion.BytesDespuesUltimaRevision > 0 {
		r.colaInocua = colaPDFInocua(data[len(data)-inspeccion.BytesDespuesUltimaRevision:])
	}
	return r
}

// maxColaPDFInocua queda por debajo de la ventana en la que los lectores
// buscan %%EOF: más bytes podrían llevarlos a reconstruir el fichero.
const maxColaPDFInocua = 512

// colaPDFInocua admite tras el último %%EOF solo espacios y comentarios
// cortos sin palabras clave de estructura (lo que añaden algunos programas
// al copiar o subir el fichero). Nada de eso cambia lo que se muestra.
func colaPDFInocua(cola []byte) bool {
	if len(cola) > maxColaPDFInocua {
		return false
	}
	texto := string(cola)
	for _, clave := range []string{"obj", "xref", "trailer", "stream", "%%EOF"} {
		if strings.Contains(texto, clave) {
			return false
		}
	}
	for _, linea := range strings.FieldsFunc(texto, func(r rune) bool { return r == '\n' || r == '\r' }) {
		linea = strings.Trim(linea, "\x00\t\f ")
		if linea != "" && !strings.HasPrefix(linea, "%") {
			return false
		}
	}
	return true
}

// sinColaInocua retira el aviso de bytes finales cuando son inocuos; si no
// queda otro motivo, la actualización se trata como lo que era antes.
func sinColaInocua(cambio CambiosPDFV2) CambiosPDFV2 {
	restantes := make([]string, 0, len(cambio.Detalle))
	for _, detalle := range cambio.Detalle {
		if detalle != "bytes_posteriores_sin_firma" {
			restantes = append(restantes, detalle)
		}
	}
	if len(restantes) == len(cambio.Detalle) {
		return cambio
	}
	cambio.Detalle = restantes
	switch {
	case len(restantes) == 0:
		cambio.Estado = cambiosPDFNinguno
	case soloDetalles(restantes, "dss_anadido"):
		cambio.Estado = cambiosPDFPermitidos
	}
	return cambio
}

func soloDetalles(detalles []string, admitidos ...string) bool {
	for _, detalle := range detalles {
		ok := false
		for _, admitido := range admitidos {
			ok = ok || detalle == admitido
		}
		if !ok {
			return false
		}
	}
	return true
}

// cambiosTrasFirma devuelve el peor estado de todo lo añadido después de la
// firma con ese ByteRange y los motivos que lo explican.
func (r *revisionesPAdES) cambiosTrasFirma(byteRange [4]int) (string, []string) {
	if r == nil || r.err != nil {
		motivo := "revisiones_pdf_no_comprobadas"
		if r != nil {
			motivo += ": " + r.err.Error()
		}
		return cambiosPDFNoComprobados, []string{motivo}
	}
	firmas := r.inspeccion.Firmas
	actual := -1
	for i := range firmas {
		if firmas[i].ByteRange == byteRange {
			actual = i
			break
		}
	}
	if actual < 0 {
		return cambiosPDFNoComprobados, []string{"firma_no_localizada_en_revisiones"}
	}
	total := CambiosPDFV2{Estado: cambiosPDFNinguno}
	if !firmas[actual].CubreRevisionCompleta {
		total = combinarCambios(total, CambiosPDFV2{Estado: cambiosPDFNoComprobados, Detalle: []string{"byterange_no_cubre_revision_completa"}})
	}
	for _, siguiente := range firmas[actual+1:] {
		total = combinarCambios(total, cambioEstructuralPAdES(siguiente.CambiosDesdeAnterior))
	}
	posteriores := r.inspeccion.CambiosPosteriores
	if r.colaInocua {
		posteriores = sinColaInocua(posteriores)
	}
	total = combinarCambios(total, cambioEstructuralPAdES(posteriores))
	switch total.Estado {
	case cambiosPDFNoPermitidos, cambiosPDFNoComprobados:
		return total.Estado, total.Detalle
	default:
		return total.Estado, nil
	}
}

// cambioEstructuralPAdES acepta una actualización cuya estructura ya
// comprobó entera el análisis v2 y que solo deja pendiente la confianza de un
// sello de documento o la apariencia de una firma nueva: no redefine nada de
// lo firmado. La firma nueva se verifica aparte, como cualquier otra del PDF.
func cambioEstructuralPAdES(cambio CambiosPDFV2) CambiosPDFV2 {
	if !cambioSoloPendienteDeEvaluar(cambio) {
		return cambio
	}
	return CambiosPDFV2{Estado: cambiosPDFPermitidos, Detalle: cambio.Detalle, BytesNoFirmados: cambio.BytesNoFirmados}
}
