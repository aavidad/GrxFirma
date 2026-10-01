// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package mobile

// EstadoPantalla describe el estado simple de la presentacion mobile.
type EstadoPantalla string

const (
	EstadoReposo     EstadoPantalla = "reposo"
	EstadoProcesando EstadoPantalla = "procesando"
	EstadoExito      EstadoPantalla = "exito"
	EstadoError      EstadoPantalla = "error"
)

// ViewModelOperacion representa el estado visible de una operacion mobile.
type ViewModelOperacion struct {
	Titulo   string
	Detalle  string
	Estado   EstadoPantalla
	AccionID string
}

// NuevaOperacionFirma crea el estado inicial para una operacion de firma mobile.
func NuevaOperacionFirma(nombreDocumento string) ViewModelOperacion {
	return ViewModelOperacion{
		Titulo:   "Firma de documento",
		Detalle:  nombreDocumento,
		Estado:   EstadoReposo,
		AccionID: "firmar_documento",
	}
}

// MarcarProcesando cambia el estado a ejecucion.
func MarcarProcesando(vm ViewModelOperacion, detalle string) ViewModelOperacion {
	vm.Estado = EstadoProcesando
	vm.Detalle = detalle
	return vm
}

// MarcarExito cambia el estado a exito.
func MarcarExito(vm ViewModelOperacion, detalle string) ViewModelOperacion {
	vm.Estado = EstadoExito
	vm.Detalle = detalle
	return vm
}

// MarcarError cambia el estado a error.
func MarcarError(vm ViewModelOperacion, detalle string) ViewModelOperacion {
	vm.Estado = EstadoError
	vm.Detalle = detalle
	return vm
}
