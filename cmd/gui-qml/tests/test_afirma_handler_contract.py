#!/usr/bin/env python3
# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

"""«Programa para firmar en las webs» en Qt (lote 15): tres opciones, la
selección muestra lo que pasa de verdad (effective), la política de la
organización deja el ajuste en solo lectura y dice por qué, y sin AutoFirma
las opciones que lo necesitan no se pueden elegir y se explica el motivo."""

import json
from pathlib import Path
import re
import unittest

GUI = Path(__file__).resolve().parents[1]
ROOT = GUI.parents[1]
QML = (GUI / "qml/main.qml").read_text(encoding="utf-8")
IPC = (GUI / "ipcbridge.cpp").read_text(encoding="utf-8")
LOCALES = ROOT / "internal/adapters/outbound/common/localizador/locales"


def card():
    start = QML.index('id: afirmaHandlerCard')
    return QML[start:QML.index('id: browserRepairCol', start)]


def function(name):
    start = QML.index(f"function {name}(")
    return QML[start:QML.index("\n                }\n", start)]


class AfirmaHandlerQtContract(unittest.TestCase):
    def test_three_options_follow_the_effective_handler(self):
        body = card()
        for handler, radio in (("grxfirma", "afirmaGrxFirmaRadio"),
                               ("autofirma", "afirmaAutoFirmaRadio"),
                               ("preguntar", "afirmaAskRadio")):
            self.assertIn(f'objectName: "{radio}"', body)
            self.assertIn(f'checked = configTab.afirmaEffectiveHandler === "{handler}"', body)
            self.assertIn(f'configTab.selectAfirmaHandler("{handler}")', body)
        self.assertIn('tr("protocolo.opcion.preguntar")', body)
        self.assertIn('text: tr("ayuda.protocolo.preguntar")', body)
        effective = function("afirmaEffective")
        self.assertIn("status.effective", effective)
        self.assertIn("status.current", effective)
        self.assertIn('case "preguntar": return tr("protocolo.estado.preguntar")', function("afirmaStatusText"))

    def test_options_without_autofirma_are_disabled_and_explained(self):
        body = card()
        ask = body[body.index('id: afirmaAskRadio'):]
        ask = ask[:ask.index("onToggled")]
        self.assertIn("autofirmaInstalled === true", ask)
        self.assertIn("enabled: available && !configTab.afirmaLocked", ask)
        self.assertIn("configTab.afirmaOptionDescription(available)", ask)
        self.assertIn('tr("protocolo.opciones_sin_autofirma")', function("afirmaOptionDescription"))
        self.assertRegex(body, r'objectName: "afirmaAutoFirmaMissingText"\s*\n\s*visible: configTab\.afirmaStatus\.autofirmaInstalled !== true')

    def test_policy_makes_the_setting_read_only_and_says_why(self):
        self.assertIn('readonly property bool afirmaLocked: afirmaPolicy !== ""', QML)
        self.assertIn("afirmaLocked", function("selectAfirmaHandler"))
        body = card()
        # Tres opciones, el aviso de que falta AutoFirma y su descarga.
        self.assertEqual(body.count("!configTab.afirmaLocked"), 5)
        policy = function("afirmaPolicyText")
        for key in ("protocolo.politica.grxfirma", "protocolo.politica.autofirma",
                    "protocolo.politica.preguntar", "protocolo.politica.sin_autofirma"):
            self.assertIn(f'tr("{key}")', policy)
        self.assertIn('objectName: "afirmaHandlerPolicyText"', body)
        self.assertIn('tr("protocolo.politica_ilegible")', body)
        self.assertIn('configTab.afirmaStatus.policyUnreadable === true', body)
        self.assertIn('case "afirma_handler_policy": return tr("protocolo.error.politica")', QML)

    def test_bridge_accepts_ask_and_the_policy_error_only(self):
        select = IPC[IPC.index("void IpcBridge::selectAfirmaHandler"):]
        select = select[:select.index("\n}\n")]
        for handler in ("grxfirma", "autofirma", "preguntar"):
            self.assertIn(f'QStringLiteral("{handler}")', select)
        self.assertEqual(len(re.findall(r'handler != QStringLiteral', select)), 3)
        self.assertIn('QStringLiteral("afirma_handler_policy")};', IPC)

    def test_every_key_is_in_every_catalog(self):
        keys = set(re.findall(r'tr\("((?:protocolo|ayuda\.protocolo)[\w.]*)"\)', card() + QML[QML.index("function afirmaEffective("):QML.index("Timer {\n                    id: statusRetryTimer")]))
        self.assertIn("protocolo.opciones_sin_autofirma", keys)
        for path in sorted(LOCALES.glob("*.json")):
            catalog = json.loads(path.read_text(encoding="utf-8"))
            for key in keys:
                self.assertTrue(catalog.get(key), f"{path.name}: falta {key}")


if __name__ == "__main__":
    unittest.main()
