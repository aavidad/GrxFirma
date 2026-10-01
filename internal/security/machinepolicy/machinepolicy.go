// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Package machinepolicy lee la política de máquina que solo puede fijar un
// administrador.
//
// En Windows la política vive en HKLM\SOFTWARE\Policies\GrxFirma, la
// ubicación que despliega una GPO y que un usuario sin privilegios no puede
// escribir. No se usan rutas como C:\etc ni %ProgramData%: cualquier usuario
// autenticado puede crear carpetas en ellas y plantar una política que afecte
// a todos los usuarios del equipo.
//
// En el resto de sistemas no hay almacén nativo y Native devuelve false: la
// política sigue leyéndose de /etc/grxfirma, propiedad de root.
package machinepolicy

// RegistryKey es la subclave de HKLM donde se publica la política en Windows.
const RegistryKey = `SOFTWARE\Policies\GrxFirma`

// Nombres de valor de la política de máquina.
const (
	AllowedDomains           = "allowed_domains"
	WebsocketHabilitado      = "websocket_habilitado"
	WebsocketPermitido       = "websocket_permitido"
	RestHabilitado           = "rest_habilitado"
	TofuHabilitado           = "tofu_habilitado"
	DirectorioP12            = "directorio_p12"
	DominiosDeConfianza      = "dominios_de_confianza"
	NivelLog                 = "nivel_log"
	TimeoutOperacionSegundos = "timeout_operacion_segundos"
	MaxTamanoDocumentoBytes  = "max_tamano_documento_bytes"
	PermitirOrigenVacio      = "permitir_origen_vacio"
	PermitirDESLegacy        = "permitir_des_legacy"
	PermitirRutasDirectas    = "permitir_rutas_directas_protocolo"
	AprobacionAutomaticaHost = "aprobacion_automatica_nativehost"
	PermitirSHA1Legacy       = "permitir_sha1_legacy"
	PermitirCMSAESECBLegacy  = "permitir_cms_aes_ecb_legacy"
)

// Native indica si la plataforma dispone de un almacén de política de
// máquina propio (registro de Windows).
func Native() bool { return native() }

// Strings devuelve un valor multicadena. present es false si no está fijado.
func Strings(name string) (values []string, present bool, err error) {
	return readStrings(name)
}

// String devuelve un valor de texto. present es false si no está fijado.
func String(name string) (value string, present bool, err error) {
	return readString(name)
}

// Int64 devuelve un valor entero. present es false si no está fijado.
func Int64(name string) (value int64, present bool, err error) {
	return readInt64(name)
}

// Bool interpreta un valor entero distinto de cero como verdadero.
func Bool(name string) (value bool, present bool, err error) {
	n, present, err := readInt64(name)
	return n != 0, present, err
}
