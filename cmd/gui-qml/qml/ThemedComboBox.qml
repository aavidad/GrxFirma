// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "ThemeContrast.js" as Contrast

// Desplegable (sobre todo editable) con el borde de ThemedTextField: 3:1 con el
// fondo en todos los temas, foco de 2 px y error con errorColor.
ComboBox {
    font.pixelSize: Typography.control
    id: control
    // El ancho natural incluye la opción más larga, el indicador y los márgenes.
    implicitContentWidthPolicy: Typography.scale > 1 ? ComboBox.WidestText : ComboBox.ContentItemImplicitWidth
    FontMetrics { id: optionMetrics; font: control.font }
    readonly property real widestOptionWidth: {
        const measuredFont = optionMetrics.font
        const currentModel = control.model
        const role = control.textRole
        let widest = optionMetrics.advanceWidth(control.displayText)
        for (let i = 0; i < control.count; ++i)
            widest = Math.max(widest, optionMetrics.advanceWidth(control.textAt(i)))
        return widest
    }
    implicitWidth: Math.max(background ? background.implicitWidth + leftInset + rightInset : 0,
        (Typography.scale > 1 ? Math.ceil(widestOptionWidth) : implicitContentWidth) + leftPadding + rightPadding)
    Layout.minimumWidth: Typography.scale > 1
        ? Math.min(implicitWidth, parent ? parent.width : implicitWidth, Typography.availableWidth(control)) : 0
    // Cuando una opción supera incluso el ancho del panel, se presenta completa
    // en varias líneas. El editor nativo se conserva para los campos editables.
    Text {
        id: wrappedLabel
        textFormat: Text.PlainText
        text: control.displayText
        font: control.font
        color: control.enabled ? control.palette.text : control.palette.disabled.text
        wrapMode: Text.WordWrap
        verticalAlignment: Text.AlignVCenter
        horizontalAlignment: control.mirrored ? Text.AlignRight : Text.AlignLeft
        visible: !control.editable && Typography.scale > 1
    }
    Binding {
        target: control
        property: "contentItem"
        value: wrappedLabel
        when: !control.editable && Typography.scale > 1
        restoreMode: Binding.RestoreBindingOrValue
    }
    delegate: ItemDelegate {
        required property int index
        width: control.popup ? control.popup.availableWidth : control.width
        text: control.textAt(index)
        font: control.font
        highlighted: control.highlightedIndex === index
        contentItem: Text {
            textFormat: Text.PlainText
            text: parent.text
            font: control.font
            color: control.palette.text
            wrapMode: Text.WordWrap
            verticalAlignment: Text.AlignVCenter
        }
    }
    property bool hasError: false
    property color errorColor: "#b42318"
    readonly property color fieldColor: control.palette.base
    readonly property color borderColor: Contrast.accentOn(control.fieldColor, control.palette.mid, 3.0)
    readonly property color focusBorderColor: Contrast.accentOn(control.fieldColor, control.palette.highlight, 3.0)
    readonly property color errorBorderColor: Contrast.accentOn(control.fieldColor, control.errorColor, 3.0)
    background: Rectangle {
        implicitWidth: 120
        implicitHeight: 36
        radius: 4
        color: control.fieldColor
        opacity: control.enabled ? 1 : 0.6
        border.color: control.hasError ? control.errorBorderColor
                                       : (control.activeFocus ? control.focusBorderColor : control.borderColor)
        border.width: control.hasError || control.activeFocus ? 2 : 1
    }
}
