// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls

// Información emergente para textos que no son fijos (estado que da el motor,
// mensajes de la barra de estado). El ToolTip adjunto pinta en AutoText y un
// texto con <img src="…"> haría que Qt descargara esa URL; este lo pinta tal
// cual. Mismo aspecto que el ToolTip del estilo: solo cambia el formato.
ToolTip {
    id: control
    contentItem: Text {
        text: control.text
        textFormat: Text.PlainText
        font: control.font
        wrapMode: Text.Wrap
        color: control.palette.toolTipText
    }
}
