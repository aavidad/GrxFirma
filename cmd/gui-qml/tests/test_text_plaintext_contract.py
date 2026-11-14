# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""LD-01 e ID-05: ningún Text ni Label interpreta como marcado un texto que
no sea fijo.

Qt pinta en `Text.AutoText` (el valor por defecto de Text y Label) como texto
enriquecido todo lo que parezca HTML. Un nombre de firmante, un certificado
remoto o un aviso de factura con `<img src="https://…">` haría que la
aplicación descargara esa URL, y con `<font>` o `<b>` cambiaría el aspecto de
la tarjeta. Por eso todo Text o Label cuyo `text:` no sea un literal o una
traducción fija declara `textFormat: Text.PlainText`.

Los ToolTip adjuntos tampoco admiten `textFormat`: solo pueden llevar textos
fijos, el resumen del certificado (que escapa cada dato) o, para datos de
fuera, el componente PlainToolTip.
"""

from __future__ import annotations

from pathlib import Path
import re
import unittest

QML = Path(__file__).resolve().parents[1] / "qml"

# Elementos que pintan su `text` con Qt::mightBeRichText si no se dice otra cosa.
ELEMENTO = re.compile(r"(?<![\w.])(Text|Label)\s*\{")

# Excepciones justificadas: (fichero, marcador dentro del bloque) -> formato.
EXCEPCIONES = {
    # Ayuda contextual: textos del catálogo, escapados con escapeHtml antes de
    # añadir la negrita del nombre de la opción.
    ("HelpButton.qml", "control.styledEntry("): "Text.StyledText",
}

LITERAL = r'"(?:[^"\\\n]|\\.)*"|\'(?:[^\'\\\n]|\\.)*\''
TRADUCCION_FIJA = re.compile(r"\b(?:qsTr|tr|window\.tr)\s*\(\s*(?:" + LITERAL + r")\s*\)")
LITERAL_RE = re.compile(LITERAL)


def quitar_comentarios(texto: str) -> str:
    """Quita comentarios // y /* */ sin tocar las cadenas."""
    salida = []
    i, n = 0, len(texto)
    while i < n:
        c = texto[i]
        if c in "\"'":
            j = i + 1
            while j < n and texto[j] != c and texto[j] != "\n":
                j += 2 if texto[j] == "\\" else 1
            salida.append(texto[i:j + 1])
            i = j + 1
        elif texto.startswith("//", i):
            j = texto.find("\n", i)
            i = n if j < 0 else j
        elif texto.startswith("/*", i):
            j = texto.find("*/", i + 2)
            fin = n if j < 0 else j + 2
            salida.append("\n" * texto.count("\n", i, fin))
            i = fin
        else:
            salida.append(c)
            i += 1
    return "".join(salida)


def cierre(texto: str, inicio: int) -> int:
    """Posición justo después de la llave que cierra la abierta en inicio-1."""
    nivel, i, n = 1, inicio, len(texto)
    while nivel and i < n:
        c = texto[i]
        if c in "\"'":
            j = i + 1
            while j < n and texto[j] != c and texto[j] != "\n":
                j += 2 if texto[j] == "\\" else 1
            i = j + 1
            continue
        nivel += {"{": 1, "}": -1}.get(c, 0)
        i += 1
    return i


def propiedades_propias(bloque: str) -> str:
    """El bloque sin los elementos hijos ni los cuerpos de funciones."""
    salida, i, n = [], 0, len(bloque)
    while i < n:
        c = bloque[i]
        if c in "\"'":
            j = i + 1
            while j < n and bloque[j] != c and bloque[j] != "\n":
                j += 2 if bloque[j] == "\\" else 1
            salida.append(bloque[i:j + 1])
            i = j + 1
        elif c == "{":
            fin = cierre(bloque, i + 1)
            salida.append("{…}")
            i = fin
        else:
            salida.append(c)
            i += 1
    return "".join(salida)


def bloques(texto: str):
    texto = quitar_comentarios(texto)
    for m in ELEMENTO.finditer(texto):
        fin = cierre(texto, m.end())
        bloque = texto[m.end():fin - 1]
        yield texto.count("\n", 0, m.start()) + 1, bloque, propiedades_propias(bloque)


def expresion(propio: str, nombre: str) -> str | None:
    m = re.search(r"(?:^|[;{\n])\s*" + re.escape(nombre) + r"\s*:\s*", propio)
    if not m:
        return None
    resto = propio[m.end():]
    lineas = []
    todas = resto.split("\n")
    for posicion, linea in enumerate(todas):
        lineas.append(linea)
        # La expresión sigue en la línea siguiente si esta acaba en operador o
        # si la siguiente empieza por uno.
        siguiente = todas[posicion + 1] if posicion + 1 < len(todas) else ""
        if not (
            re.search(r"(\+|\?|:|\|\||&&|\(|,|\.)\s*$", linea)
            or re.match(r"\s*(\+|\?|:|\|\||&&|\)|\.)", siguiente)
        ):
            break
    valor = "\n".join(lineas)
    # En las líneas con varias propiedades, la expresión acaba en el «;».
    sin_literales = LITERAL_RE.sub(lambda x: "x" * len(x.group(0)), valor)
    corte = sin_literales.find(";")
    if corte >= 0:
        valor = valor[:corte]
    return valor.strip()


def es_fija(valor: str) -> bool:
    """Solo literales y traducciones de literales, unidos con «+» o elegidos
    con una condición cualquiera."""
    if "{…}" in valor:
        return False
    marcado = TRADUCCION_FIJA.sub(" F ", valor)
    marcado = LITERAL_RE.sub(" F ", marcado)
    # Una condición elige entre valores fijos: lo que va antes de «?» no se pinta.
    partes = re.split(r"\?|:", marcado)
    valores = partes[1:] if len(partes) > 1 else partes
    for parte in valores:
        if not re.fullmatch(r"[\sF+()]*", parte) or "F" not in parte:
            return False
    return True


class TextPlainTextContract(unittest.TestCase):
    def test_text_y_label_con_datos_declaran_texto_plano(self):
        revisados = 0
        for fichero in sorted(QML.rglob("*.qml")):
            texto = fichero.read_text(encoding="utf-8")
            for linea, bloque, propio in bloques(texto):
                valor = expresion(propio, "text")
                if valor is None or es_fija(valor):
                    continue
                revisados += 1
                formatos = re.findall(r"(?:^|[;\n{])\s*textFormat\s*:\s*([\w.]+)", propio)
                esperado = "Text.PlainText"
                for (nombre, marcador), formato in EXCEPCIONES.items():
                    if fichero.name == nombre and marcador in bloque:
                        esperado = formato
                with self.subTest(fichero=fichero.name, linea=linea, texto=valor[:80]):
                    self.assertEqual(formatos, [esperado], "falta textFormat: Text.PlainText")
        self.assertGreater(revisados, 100, "el analizador no encuentra los Text de main.qml")

    def test_tooltips_adjuntos_solo_con_textos_fijos(self):
        permitidos = {
            # Resumen del certificado: escapa cada dato (test_resumen_del_certificado_escapa).
            "window.compactCertificateTooltip(modelData)",
            # Textos propios de la interfaz.
            "effectiveName",
            "navButtonRoot.text",
        }
        for fichero in sorted(QML.rglob("*.qml")):
            texto = quitar_comentarios(fichero.read_text(encoding="utf-8"))
            for m in re.finditer(r"ToolTip\.text\s*:\s*", texto):
                linea = texto.count("\n", 0, m.start()) + 1
                valor = expresion(texto[m.start():], "ToolTip.text")
                with self.subTest(fichero=fichero.name, linea=linea, texto=valor[:80]):
                    self.assertTrue(
                        es_fija(valor) or valor in permitidos,
                        "ToolTip.text con datos: usa PlainToolTip",
                    )

    def test_plain_tooltip_pinta_texto_plano(self):
        componente = (QML / "PlainToolTip.qml").read_text(encoding="utf-8")
        self.assertRegex(componente, r"(?m)^ToolTip\s*\{")
        self.assertIn("textFormat: Text.PlainText", componente)

    def test_resumen_del_certificado_escapa(self):
        main = quitar_comentarios((QML / "main.qml").read_text(encoding="utf-8"))
        inicio = main.index("function compactCertificateTooltip(cert)")
        cuerpo = main[inicio:cierre(main, main.index("{", inicio) + 1)]
        devuelto = cuerpo[cuerpo.index("return tr(\"<b>Titular:</b> \")"):]
        for dato in ("subject", "issuer", "serial", "validTo", "status", "fingerprint"):
            self.assertIn("escapeMarkup(" + dato + ")", devuelto, dato)
        funcion = main[main.index("function escapeMarkup(value)"):]
        funcion = funcion[:cierre(funcion, funcion.index("{") + 1)]
        for entidad in ("&amp;", "&lt;", "&gt;", "&quot;"):
            self.assertIn(entidad, funcion)

    def test_titulo_del_dialogo_en_texto_plano(self):
        dialogo = (QML / "ThemedDialog.qml").read_text(encoding="utf-8")
        cabecera = dialogo[dialogo.index("id: titleLabel"):]
        cabecera = cabecera[:cabecera.index("}")]
        self.assertIn("textFormat: Text.PlainText", cabecera)

    def test_ningun_dialogo_sin_tema_con_titulo_variable(self):
        for fichero in sorted(QML.rglob("*.qml")):
            texto = quitar_comentarios(fichero.read_text(encoding="utf-8"))
            for m in re.finditer(r"(?<![\w.])(Dialog|Popup|Drawer)\s*\{", texto):
                propio = propiedades_propias(texto[m.end():cierre(texto, m.end()) - 1])
                valor = expresion(propio, "title")
                if valor is None:
                    continue
                linea = texto.count("\n", 0, m.start()) + 1
                with self.subTest(fichero=fichero.name, linea=linea):
                    self.assertTrue(es_fija(valor), "título variable en un Dialog sin tema")

    def test_el_analizador_detecta_los_casos_del_informe(self):
        ejemplo = """
        Item {
            Text { text: tr("Firmantes: ") + verificationSignerShortText(d); color: "white" }
            Label {
                text: modelData.field + ": " +
                      modelData.message
                wrapMode: Text.Wrap
            }
            Text { text: tr("Fijo"); font.bold: true }
            Label { text: ok ? tr("Sí") : tr("No") }
            Text { textFormat: Text.PlainText; text: nombre }
        }
        """
        resultado = [
            (expresion(p, "text"), re.findall(r"textFormat\s*:\s*([\w.]+)", p))
            for _, _, p in bloques(ejemplo)
        ]
        self.assertFalse(es_fija(resultado[0][0]))
        self.assertIn("modelData.message", resultado[1][0])
        self.assertFalse(es_fija(resultado[1][0]))
        self.assertTrue(es_fija(resultado[2][0]))
        self.assertTrue(es_fija(resultado[3][0]))
        self.assertEqual(resultado[4][1], ["Text.PlainText"])


if __name__ == "__main__":
    unittest.main()
