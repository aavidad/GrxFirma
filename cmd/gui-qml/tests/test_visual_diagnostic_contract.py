# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import json
import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
TIMELINE = (
    ROOT / "cmd/gui-qml/qml/DiagnosticTimeline.qml"
).read_text(encoding="utf-8")
QRC = (ROOT / "cmd/gui-qml/qml.qrc").read_text(encoding="utf-8")
LOCALES = (
    ROOT / "internal/adapters/outbound/common/localizador/locales"
)


def qml_function(name: str) -> str:
    match = re.search(
        rf"    function {re.escape(name)}\([^)]*\) \{{"
        rf"(?P<body>.*?)\n    \}}",
        QML,
        re.DOTALL,
    )
    if match is None:
        raise AssertionError(f"QML function not found: {name}")
    return match.group("body")


class VisualDiagnosticContractTest(unittest.TestCase):
    def test_timeline_uses_only_observed_ipc_steps(self):
        body = qml_function("diagnosticVisualSteps")
        self.assertIn(
            "diagnostic && diagnostic.steps ? diagnostic.steps : []",
            body,
        )
        self.assertIn(
            "steps.push(diagnosticVisualStep(rawSteps[i], false))",
            body,
        )
        self.assertNotIn("admission", body)
        self.assertNotIn("protocol", body)
        self.assertNotIn("operation", body)
        self.assertIn("steps: window.diagnosticVisualSteps()", QML)
        self.assertIn(
            'emptyTitle: tr("No comprobado")',
            QML,
        )

    def test_status_is_expressed_with_icon_and_text(self):
        body = qml_function("diagnosticStatusPresentation")
        for status in ("success", "failure", "skipped"):
            self.assertIn(f'case "{status}":', body)
        for icon in ('icon: "\\u2713"', 'icon: "\\u2715"', 'icon: "\\u2014"'):
            self.assertIn(icon, body)
        for label in (
            'text: tr("Correcto")',
            'text: tr("Falló")',
            'text: tr("Omitido")',
            'text: tr("No comprobado")',
        ):
            self.assertIn(label, body)
        self.assertIn("text: modelData.statusIcon", TIMELINE)
        self.assertIn("text: modelData.statusText", TIMELINE)

    def test_owner_vocabulary_is_closed_and_user_facing(self):
        body = qml_function("diagnosticOwnerText")
        for owner in (
            "app_local",
            "browser",
            "local_web_service",
            "portal",
            "remote_service",
            "government_afirma",
        ):
            self.assertIn(f'case "{owner}":', body)
        self.assertIn('return tr("Desconocido")', body)
        self.assertIn(
            'tr("Responsabilidad probable: %1").arg(owner)',
            QML,
        )

    def test_failure_keeps_its_diagnostic_snapshot(self):
        body = qml_function("recordOperationFailure")
        self.assertIn(
            "snapshotOperationDiagnostic(backend.lastDiagnostic)",
            body,
        )
        snapshot = qml_function("snapshotOperationDiagnostic")
        self.assertIn("snapshot.steps = []", snapshot)
        self.assertIn(
            "snapshot.steps.push(Object.assign({}, rawSteps[i] || {}))",
            snapshot,
        )
        self.assertIn("diagnostic: diagnostic", body)
        self.assertIn("window.openSupportAssistant(failureGoal)", body)
        self.assertIn("!supportAssistantDialog.visible", body)
        current = qml_function("currentFailureDiagnostic")
        self.assertIn("activeFailureContext.diagnostic", current)

    def test_technical_view_is_secondary_and_collapsed(self):
        self.assertIn(
            "property bool diagnosticTechnicalExpanded: false",
            QML,
        )
        open_body = qml_function("openSupportAssistant")
        self.assertIn("diagnosticTechnicalExpanded = false", open_body)
        self.assertGreaterEqual(
            QML.count("visible: window.diagnosticTechnicalExpanded"),
            2,
        )
        self.assertIn('tr("Mostrar detalles")', QML)
        self.assertIn('tr("Ocultar detalles")', QML)

    def test_diagnostics_do_not_repeat_a_signature_or_probe_a_new_target(self):
        body = qml_function("startActiveDiagnostics")
        self.assertIn(
            "backend.runActiveDiagnostics(true, activeDiagnosticContextPayload())",
            body,
        )
        for forbidden in (
            "signFile",
            "signBatch",
            "sign_multicosign",
            "verifyFile",
            "endpoint",
            "url",
        ):
            self.assertNotIn(forbidden, body)

    def test_timeline_is_plain_text_and_accessible(self):
        self.assertIn(
            "<file>qml/DiagnosticTimeline.qml</file>",
            QRC,
        )
        self.assertIn("Accessible.role: Accessible.List", TIMELINE)
        self.assertIn("Accessible.role: Accessible.ListItem", TIMELINE)
        self.assertIn(
            "steps.length === 0\n                     ? emptyTitle : accessibleName",
            TIMELINE,
        )
        self.assertGreaterEqual(
            TIMELINE.count("textFormat: Text.PlainText"),
            5,
        )

    def test_new_visible_text_is_in_every_locale(self):
        required = {
            "Admisión de la petición",
            "Correcto",
            "Diagnóstico de la operación",
            "Ejecución de la operación",
            "Este equipo",
            "No comprobado",
            "Omitido",
            "Portal o sede",
            (
                "Solo se muestran fases realmente observadas por el motor. "
                "Lo que no se pudo comprobar queda como desconocido."
            ),
            "Validación del protocolo",
        }
        paths = sorted(LOCALES.glob("*.json"))
        self.assertEqual(11, len(paths))
        for path in paths:
            with self.subTest(locale=path.stem):
                catalog = json.loads(path.read_text(encoding="utf-8"))
                self.assertTrue(required.issubset(catalog))


if __name__ == "__main__":
    unittest.main()
