// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

package main

import (
	"context"
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

// destinoGuardado clasifica la extensión de un fichero que la web pide guardar.
type destinoGuardado int

const (
	destinoHabitual destinoGuardado = iota
	destinoConfirmar
	destinoBloqueado
)

// extensionesGuardadoHabituales son las firmas y los datos que la web guarda
// normalmente con save y signandsave. Se guardan sin preguntar más.
var extensionesGuardadoHabituales = map[string]bool{
	".pdf": true, ".xml": true, ".csig": true, ".xsig": true, ".p7s": true, ".sig": true,
	".txt": true, ".bin": true, ".json": true, ".zip": true, ".asice": true, ".asics": true,
}

// extensionesEjecutables son tipos que Windows (o el escritorio en Linux)
// ejecuta, monta o interpreta al abrirlos. Una web no debe poder dejar uno en
// disco a través de GrxFirma aunque el usuario pulse "Guardar" sin fijarse.
var extensionesEjecutables = map[string]bool{
	".exe": true, ".com": true, ".bat": true, ".cmd": true, ".scr": true, ".pif": true,
	".msi": true, ".msp": true, ".msc": true, ".mst": true, ".lnk": true, ".url": true,
	".ps1": true, ".psm1": true, ".psd1": true, ".ps2": true, ".msh": true,
	".vb": true, ".vbs": true, ".vbe": true, ".js": true, ".jse": true, ".wsf": true,
	".wsh": true, ".wsc": true, ".ws": true, ".sct": true, ".hta": true, ".cpl": true,
	".dll": true, ".ocx": true, ".sys": true, ".jar": true, ".reg": true, ".scf": true,
	".inf": true, ".ins": true, ".isp": true, ".job": true, ".shb": true, ".shs": true,
	".appref-ms": true, ".application": true, ".xbap": true, ".gadget": true,
	".settingcontent-ms": true, ".library-ms": true, ".search-ms": true,
	".searchconnector-ms": true, ".chm": true, ".hlp": true,
	".iso": true, ".img": true, ".vhd": true, ".vhdx": true,
	".msix": true, ".msixbundle": true, ".appx": true, ".appxbundle": true, ".appinstaller": true,
	".xll": true, ".xlam": true, ".ppam": true, ".docm": true, ".dotm": true, ".xlsm": true,
	".xltm": true, ".xlsb": true, ".pptm": true, ".potm": true, ".ppsm": true, ".sldm": true,
	".ade": true, ".adp": true, ".mde": true, ".accde": true, ".mdb": true, ".accdb": true,
	".py": true, ".pyw": true, ".pl": true, ".rb": true, ".sh": true, ".bash": true,
	".zsh": true, ".ksh": true, ".csh": true, ".run": true, ".command": true, ".app": true,
	".desktop": true,
}

func extensionGuardado(path string) string {
	return strings.ToLower(filepath.Ext(strings.TrimRight(path, ". ")))
}

func clasificarDestinoGuardado(path string) destinoGuardado {
	ext := extensionGuardado(path)
	switch {
	case extensionesEjecutables[ext]:
		return destinoBloqueado
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

// autorizarDestinoGuardado devuelve "" si se puede escribir en path o la
// respuesta legacy que corresponde. Los tipos peligrosos se bloquean; los que
// no son firmas ni datos habituales piden confirmación explícita y, sin forma
// de pedirla, no se guardan.
func (h *legacyWebSocketHandler) autorizarDestinoGuardado(ctx context.Context, path string) string {
	switch clasificarDestinoGuardado(path) {
	case destinoHabitual:
		return ""
	case destinoBloqueado:
		destinoEjecutable(path)
		return "SAF_05: Tipo de fichero no permitido por seguridad"
	}
	if h == nil || h.approval == nil {
		return "SAF_05: Tipo de fichero no permitido por seguridad"
	}
	ext := extensionGuardado(path)
	if ext == "" {
		ext = "?"
	}
	mensaje := tl(claveConfirmarGuardado,
		application.NeutralizarTextoAprobacion(ext, application.MaxRunasAprobacionCampo),
		application.NeutralizarTextoAprobacion(path, application.MaxRunasAprobacionCampo))
	aprobador := h.approval
	if conTitulo, ok := aprobador.(aprobacionConTitulo); ok {
		if otro := conTitulo.ConTitulo(tl(claveTituloGuardado)); otro != nil {
			aprobador = otro
		}
	}
	aprobado, err := aprobador.Request(ctx, mensaje)
	if err != nil || !aprobado {
		return "CANCEL"
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
