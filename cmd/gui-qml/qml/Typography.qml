// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

pragma Singleton
import QtQuick
import QtQuick.Window

QtObject {
    // Qt aplica el DPI de la pantalla a las coordenadas lógicas. Esta escala
    // adicional respeta una fuente de escritorio mayor que la base de 13 px.
    readonly property FontMetrics systemMetrics: FontMetrics {}
    readonly property real systemPixelSize: systemMetrics.font.pixelSize > 0
        ? systemMetrics.font.pixelSize : systemMetrics.font.pointSize * 96 / 72
    readonly property real systemScale: Math.max(1, systemPixelSize / 13)
    property int textScalePercent: 100
    readonly property var percentages: [100, 125, 150, 175, 200]
    readonly property real userScale: percentages.indexOf(textScalePercent) >= 0 ? textScalePercent / 100 : 1
    readonly property real scale: systemScale * userScale
    readonly property int control: Math.round(systemPixelSize * userScale)
    readonly property int size10: Math.round(10 * scale)
    readonly property int size11: Math.round(11 * scale)
    readonly property int size12: Math.round(12 * scale)
    readonly property int size13: Math.round(13 * scale)
    readonly property int size14: Math.round(14 * scale)
    readonly property int size15: Math.round(15 * scale)
    readonly property int size16: Math.round(16 * scale)
    readonly property int size17: Math.round(17 * scale)
    readonly property int size18: Math.round(18 * scale)
    readonly property int size20: Math.round(20 * scale)
    readonly property int size22: Math.round(22 * scale)
    readonly property int size24: Math.round(24 * scale)
    readonly property int size28: Math.round(28 * scale)
    readonly property int size32: Math.round(32 * scale)
    // Las alturas mínimas acompañan al texto; el contenido puede crecer más.
    function minimum(base) { return Math.ceil(base * scale) }
    // Un tamaño natural puede superar al panel visible aunque un antecesor
    // haya crecido por su contenido. El límite lo da el área de desplazamiento.
    function availableWidth(item) {
        const itemX = item.x
        let available = Infinity
        for (let ancestor = item.parent; ancestor; ancestor = ancestor.parent) {
            const ancestorX = ancestor.x
            if (!ancestor.clip) continue
            const left = Math.max(0, item.mapToItem(ancestor, 0, 0).x)
            available = Math.min(available, ancestor.width - 2 * left)
        }
        return Math.max(0, available)
    }
}
