// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"

	"grxfirma/internal/application"
	"grxfirma/internal/ports"
	"grxfirma/internal/security/avisos"
)

// claveConfirmarGuardado es la clave del catálogo para confirmar un tipo de
// fichero poco habitual. Es variable porque tl trata su primer argumento como
// formato y la clave no lleva marcadores.
var claveConfirmarGuardado = "approval.save.extension"

// claveTituloGuardado es el título del diálogo que confirma ese guardado (A-09):
// no es una firma y no debe salir como «Confirmación de firma».
const claveTituloGuardado = "approval.save.title"

// claveConfirmarGuardadoConRiesgo es la confirmación de los tipos que pueden
// llevar macros o código (lote 13c). Variable por el mismo motivo que la
// anterior.
var claveConfirmarGuardadoConRiesgo = "approval.save.extension.risky"

// destinoGuardado clasifica la extensión de un fichero que la web pide guardar.
type destinoGuardado int

const (
	destinoHabitual destinoGuardado = iota
	destinoConfirmar
	destinoConRiesgo
	destinoBloqueado
)

// extensionesGuardadoHabituales son las firmas y los datos que la web guarda
// normalmente con save y signandsave. Se guardan sin preguntar más.
var extensionesGuardadoHabituales = map[string]bool{
	".pdf": true, ".xml": true, ".csig": true, ".xsig": true, ".p7s": true, ".sig": true,
	".txt": true, ".bin": true, ".json": true, ".zip": true, ".asice": true, ".asics": true,
}

// extensionesEjecutables son tipos que Windows (o el escritorio en Linux)
// ejecuta o interpreta al abrirlos. Una web no debe poder dejar uno en disco
// a través de GrxFirma aunque el usuario pulse "Guardar" sin fijarse. Es la
// lista que ya se bloqueaba antes del lote 4; no se amplía para no romper
// guardados que las sedes ya hacían (lote 13c).
var extensionesEjecutables = map[string]bool{
	".exe": true, ".com": true, ".bat": true, ".cmd": true, ".scr": true, ".pif": true,
	".msi": true, ".msp": true, ".msc": true, ".lnk": true, ".url": true, ".ps1": true,
	".psm1": true, ".vbs": true, ".vbe": true, ".js": true, ".jse": true, ".wsf": true,
	".wsh": true, ".hta": true, ".cpl": true, ".dll": true, ".jar": true, ".reg": true,
	".scf": true, ".inf": true, ".appref-ms": true, ".application": true,
	".settingcontent-ms": true, ".gadget": true, ".sh": true, ".desktop": true,
}

// extensionesConRiesgo son los tipos que el lote 4 (A-09) añadió al bloqueo:
// se ejecutan, se montan o llevan macros, pero antes se guardaban y algunas
// sedes los usan (ofimática con macros, por ejemplo). Con interfaz se pide
// confirmación con un aviso más claro; sin ella se guardan con aviso en el
// registro. En Windows llevan, como todo lo que se guarda desde la web, la
// marca Zone.Identifier.
var extensionesConRiesgo = map[string]bool{
	".mst": true, ".psd1": true, ".ps2": true, ".msh": true, ".vb": true,
	".wsc": true, ".ws": true, ".sct": true, ".ocx": true, ".sys": true,
	".ins": true, ".isp": true, ".job": true, ".shb": true, ".shs": true,
	".xbap": true, ".library-ms": true, ".search-ms": true, ".searchconnector-ms": true,
	".chm": true, ".hlp": true, ".iso": true, ".img": true, ".vhd": true, ".vhdx": true,
	".msix": true, ".msixbundle": true, ".appx": true, ".appxbundle": true, ".appinstaller": true,
	".xll": true, ".xlam": true, ".ppam": true, ".docm": true, ".dotm": true, ".xlsm": true,
	".xltm": true, ".xlsb": true, ".pptm": true, ".potm": true, ".ppsm": true, ".sldm": true,
	".ade": true, ".adp": true, ".mde": true, ".accde": true, ".mdb": true, ".accdb": true,
	".py": true, ".pyw": true, ".pl": true, ".rb": true, ".bash": true,
	".zsh": true, ".ksh": true, ".csh": true, ".run": true, ".command": true, ".app": true,
}

func extensionGuardado(path string) string {
	return strings.ToLower(filepath.Ext(strings.TrimRight(path, ". ")))
}

func clasificarDestinoGuardado(path string) destinoGuardado {
	ext := extensionGuardado(path)
	switch {
	case extensionesEjecutables[ext]:
		return destinoBloqueado
	case extensionesConRiesgo[ext]:
		return destinoConRiesgo
	case extensionesGuardadoHabituales[ext]:
		return destinoHabitual
	default:
		return destinoConfirmar
	}
}

// destinoEjecutable rechaza guardar datos de la web con una extensión
// ejecutable y lo explica al usuario.
func destinoEjecutable(path string) bool {
	if clasificarDestinoGuardado(path) != destinoBloqueado {
		return false
	}
	avisos.Registrar(
		"Guardado bloqueado por seguridad",
		"la web intentó guardar un fichero de tipo "+extensionGuardado(path)+", que el sistema podría ejecutar; GrxFirma nunca guarda ficheros ejecutables. Guarda la firma con su extensión habitual (.csig, .xsig, .pdf...)",
	)
	return true
}

// aprobacionSinInterfaz la cumplen los aprobadores que no pueden preguntar al
// usuario (compilación headless).
type aprobacionSinInterfaz interface {
	SinInterfaz() bool
}

func sinInterfazDeConfirmacion(aprobador ports.UserApproval) bool {
	if aprobador == nil {
		return true
	}
	sin, ok := aprobador.(aprobacionSinInterfaz)
	return ok && sin.SinInterfaz()
}

// autorizarDestinoGuardado devuelve "" si se puede escribir en path o la
// respuesta legacy que corresponde. Los ejecutables que ya se bloqueaban
// antes del lote 4 se bloquean. Los demás tipos que no son firmas ni datos
// habituales piden confirmación explícita (con un aviso más claro si pueden
// llevar macros o código); sin interfaz para pedirla se guardan, como antes
// del lote 4, con un aviso en el registro y, en Windows, con la marca de
// procedencia (lote 13c).
func (h *legacyWebSocketHandler) autorizarDestinoGuardado(ctx context.Context, path string) string {
	clase := clasificarDestinoGuardado(path)
	switch clase {
	case destinoHabitual:
		return ""
	case destinoBloqueado:
		destinoEjecutable(path)
		return "SAF_05: Tipo de fichero no permitido por seguridad"
	}
	ext := extensionGuardado(path)
	if ext == "" {
		ext = "?"
	}
	conRiesgo := clase == destinoConRiesgo
	var aprobador ports.UserApproval
	if h != nil {
		aprobador = h.approval
	}
	if sinInterfazDeConfirmacion(aprobador) {
		slog.Warn("guardado_web_sin_confirmacion", "extension", ext, "con_riesgo", conRiesgo)
		return ""
	}
	clave := claveConfirmarGuardado
	if conRiesgo {
		clave = claveConfirmarGuardadoConRiesgo
	}
	mensaje := tl(clave,
		application.NeutralizarTextoAprobacion(ext, application.MaxRunasAprobacionCampo),
		application.NeutralizarTextoAprobacion(path, application.MaxRunasAprobacionCampo))
	if conTitulo, ok := aprobador.(aprobacionConTitulo); ok {
		if otro := conTitulo.ConTitulo(tl(claveTituloGuardado)); otro != nil {
			aprobador = otro
		}
	}
	aprobado, err := aprobador.Request(ctx, mensaje)
	if err != nil || !aprobado {
		return "CANCEL"
	}
	if conRiesgo {
		slog.Warn("guardado_web_tipo_con_riesgo", "extension", ext)
	}
	return ""
}

// aprobacionConTitulo la cumplen los aprobadores con ventana propia: devuelven
// una copia que muestra otro título, para confirmaciones que no son de firma.
type aprobacionConTitulo interface {
	ConTitulo(titulo string) ports.UserApproval
}

// contenidoZoneIdentifier compone el flujo Zone.Identifier de Windows con la
// zona de Internet (3). El origen solo se añade si es una URL http(s) limpia,
// en ASCII imprimible, para que no pueda inyectar líneas en el fichero.
func contenidoZoneIdentifier(origen string) string {
	contenido := "[ZoneTransfer]\r\nZoneId=3\r\n"
	if referrer := origenParaZona(origen); referrer != "" {
		contenido += "ReferrerUrl=" + referrer + "\r\n"
	}
	return contenido
}

func origenParaZona(origen string) string {
	for _, r := range origen {
		if r <= 0x20 || r >= 0x7f {
			return ""
		}
	}
	u, err := url.Parse(origen)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
