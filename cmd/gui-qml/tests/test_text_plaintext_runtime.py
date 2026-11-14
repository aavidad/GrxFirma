# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""LD-01 en ejecución: los bloques reales de la interfaz no descargan nada.

Se copia de main.qml el Text que pinta «Firmantes:» tras firmar y de
FacturaePanel.qml el Label de los avisos de la factura, se cargan con el `qml`
de Qt (plataforma offscreen) con un firmante y un aviso que llevan
`<img src="http://127.0.0.1:<puerto>/…">`, y un servidor local cuenta las
conexiones. Con `textFormat: Text.PlainText` no debe llegar ninguna. Como
comprobación de la propia prueba, el mismo bloque sin esa línea sí hace la
petición (así se ve que la prueba habría fallado antes del arreglo).

Se salta si no hay `qml6`/`qml` con QtQuick.Controls en el equipo.
"""

from __future__ import annotations

import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import tempfile
import threading
import unittest

from test_text_plaintext_contract import cierre

QML = Path(__file__).resolve().parents[1] / "qml"
FORMATO_PLANO = re.compile(r"^\s*textFormat:\s*Text\.PlainText\s*\n", re.M)


def bloque_con(fichero: Path, marcador: str, elemento: str) -> str:
    texto = fichero.read_text(encoding="utf-8")
    posicion = texto.index(marcador)
    inicio = texto.rindex(elemento + " {", 0, posicion)
    apertura = inicio + len(elemento) + 2
    return texto[inicio:cierre(texto, apertura)]


def herramienta_qml() -> str | None:
    for nombre in ("qml6", "qml"):
        ruta = shutil.which(nombre)
        if ruta:
            return ruta
    return None


class Sumidero:
    """Servidor TCP local que solo cuenta las conexiones que recibe."""

    def __init__(self) -> None:
        self.socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.socket.bind(("127.0.0.1", 0))
        self.socket.listen(8)
        self.socket.settimeout(0.2)
        self.puerto = self.socket.getsockname()[1]
        self.conexiones = 0
        self.parar = threading.Event()
        self.hilo = threading.Thread(target=self._atender, daemon=True)
        self.hilo.start()

    def _atender(self) -> None:
        while not self.parar.is_set():
            try:
                cliente, _ = self.socket.accept()
            except OSError:
                continue
            self.conexiones += 1
            cliente.close()

    def cerrar(self) -> None:
        self.parar.set()
        self.hilo.join(timeout=2)
        self.socket.close()


@unittest.skipUnless(herramienta_qml(), "no hay qml6/qml en este equipo")
class TextPlainTextRuntime(unittest.TestCase):
    def setUp(self) -> None:
        self.directorio = tempfile.TemporaryDirectory()
        self.addCleanup(self.directorio.cleanup)
        sonda = Path(self.directorio.name) / "sonda.qml"
        sonda.write_text(
            "import QtQuick\nimport QtQuick.Controls\nimport QtQuick.Layouts\n"
            "Item { Component.onCompleted: Qt.quit() }\n",
            encoding="utf-8",
        )
        if self.ejecutar(sonda) is None:
            self.skipTest("qml no carga QtQuick.Controls y QtQuick.Layouts")

    def ejecutar(self, fichero: Path) -> int | None:
        entorno = dict(os.environ, QT_QPA_PLATFORM="offscreen")
        entorno.pop("http_proxy", None)
        entorno.pop("HTTP_PROXY", None)
        try:
            resultado = subprocess.run(
                [herramienta_qml(), str(fichero)],
                env=entorno,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                timeout=20,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return None
        salida = resultado.stdout.decode("utf-8", "replace")
        if "is not installed" in salida or "is not a type" in salida:
            return None
        return resultado.returncode

    def conexiones_al_cargar(self, plantilla: str, bloque: str) -> int:
        sumidero = Sumidero()
        try:
            url = f"http://127.0.0.1:{sumidero.puerto}/x.png"
            documento = plantilla.replace("@BLOQUE@", bloque).replace("@URL@", url)
            fichero = Path(self.directorio.name) / "arnes.qml"
            fichero.write_text(documento, encoding="utf-8")
            self.ejecutar(fichero)
            return sumidero.conexiones
        finally:
            sumidero.cerrar()

    def comprobar(self, plantilla: str, bloque: str) -> None:
        self.assertRegex(bloque, FORMATO_PLANO, "el bloque no declara texto plano")
        self.assertEqual(self.conexiones_al_cargar(plantilla, bloque), 0)
        sin_formato = FORMATO_PLANO.sub("", bloque, count=1)
        self.assertGreater(
            self.conexiones_al_cargar(plantilla, sin_formato),
            0,
            "sin textFormat la prueba debería ver la descarga; el arnés no sirve",
        )

    def test_firmantes_tras_firmar(self):
        bloque = bloque_con(
            QML / "main.qml",
            'text: tr("Firmantes: ") + verificationSignerShortText(',
            "Text",
        )
        signers = (QML / "VerificationSigners.js").resolve().as_uri()
        plantilla = (
            "import QtQuick\n"
            f'import "{QML.resolve().as_uri()}"\n'
            f'import "{signers}" as Signers\n'
            "Item {\n"
            "    id: window\n"
            "    width: 400; height: 120\n"
            "    property var currentOutputVerificationDetails: ({})\n"
            "    function tr(s) { return s }\n"
            "    function verificationSignerShortText(details) {\n"
            "        return Signers.readableName('CN=\\\\<img src=\\\\\"@URL@\\\\\"\\\\>Ana García,C=ES')\n"
            "    }\n"
            "    @BLOQUE@\n"
            "    Timer { interval: 1500; running: true; onTriggered: Qt.quit() }\n"
            "}\n"
        )
        self.comprobar(plantilla, bloque)

    def test_avisos_de_la_factura(self):
        bloque = bloque_con(
            QML / "FacturaePanel.qml",
            'modelData.field + ": " + modelData.message',
            "Label",
        )
        plantilla = (
            "import QtQuick\nimport QtQuick.Controls\nimport QtQuick.Layouts\n"
            f'import "{QML.resolve().as_uri()}"\n'
            "Item {\n"
            "    id: panel\n"
            "    width: 400; height: 120\n"
            "    property var theme: ({ textColor: \"black\" })\n"
            "    function tr(s) { return s }\n"
            "    ColumnLayout {\n"
            "        Repeater {\n"
            "            model: [{ level: \"warning\", field: \"SchemaVersion\","
            " message: '<img src=\"@URL@\"> no la admite FACe' }]\n"
            "            delegate: @BLOQUE@\n"
            "        }\n"
            "    }\n"
            "    Timer { interval: 1500; running: true; onTriggered: Qt.quit() }\n"
            "}\n"
        )
        self.comprobar(plantilla, bloque)


if __name__ == "__main__":
    unittest.main()
