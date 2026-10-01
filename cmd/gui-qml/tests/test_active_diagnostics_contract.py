# Derechos de autor (C) 2026 Alberto Avidad Fernández.
# Autoría: Alberto Avidad Fernández
# Licencia: EUPL 1.2 o posterior
# SPDX-License-Identifier: EUPL-1.2

import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[3]
QML = (ROOT / "cmd/gui-qml/qml/main.qml").read_text(encoding="utf-8")
RUNNER = (ROOT / "cmd/gui-qml/activediagnostics.cpp").read_text(encoding="utf-8")
PRIVACY = (ROOT / "cmd/gui-qml/incidentprivacy.cpp").read_text(encoding="utf-8")


class ActiveDiagnosticsContractTest(unittest.TestCase):
    def test_action_is_visible_only_for_a_recorded_failure(self):
        self.assertIn(
            "visible: window.activeFailureContext !== null",
            QML,
        )
        self.assertIn(
            "function recordOperationFailure(kind, message, incidentPath)",
            QML,
        )
        self.assertIn("function clearOperationFailure()", QML)
        self.assertIn("backend.cancelActiveDiagnostics()", QML)
        self.assertIn(
            "activeDiagnosticInProgress = false\n        const diagnostic",
            QML,
        )

    def test_consent_is_enforced_in_qml_and_cpp(self):
        self.assertIn(
            'text: tr("Acepto ejecutar ahora estas comprobaciones adicionales.")',
            QML,
        )
        self.assertIn("activeDiagnosticsConsentCheck.checked", QML)
        self.assertIn("if (!explicitConsent)", RUNNER)
        self.assertIn(
            'QStringLiteral("consent_required")',
            RUNNER,
        )
        self.assertIn(
            'QStringLiteral("post_failure_required")',
            RUNNER,
        )

    def test_probes_are_bounded_and_do_not_export_targets(self):
        self.assertIn("kDNSProbeTimeoutMs = 1500", RUNNER)
        self.assertIn("kTCPProbeTimeoutMs = 2000", RUNNER)
        self.assertIn("kTLSProbeTimeoutMs = 3000", RUNNER)
        self.assertNotIn(
            'm_result.insert(QStringLiteral("endpoint")',
            RUNNER,
        )
        self.assertNotIn(
            'm_result.insert(QStringLiteral("host")',
            RUNNER,
        )
        self.assertNotIn("activeDiagnosticEndpoint", RUNNER)
        self.assertIn('QStringLiteral("127.0.0.1")', RUNNER)
        self.assertIn("address.isLoopback()", RUNNER)
        self.assertIn("m_expectedLocalTLSPins.contains(peerPin)", RUNNER)
        self.assertIn("socket->ignoreSslErrors(errors)", RUNNER)
        self.assertNotIn("VerifyNone", RUNNER)

    def test_result_is_a_separate_sanitized_incident_block(self):
        self.assertIn("payload.activeDiagnostic = activeResult", QML)
        self.assertIn(
            "requestId: String(backend.lastRequestId || \"\")",
            QML,
        )
        self.assertIn(
            'return String(activeFailureContext.requestId || "")',
            QML,
        )
        self.assertIn(
            "requestId: currentFailureRequestId()",
            QML,
        )
        self.assertIn(
            'payload.value(QStringLiteral("activeDiagnostic")).toMap()',
            PRIVACY,
        )
        self.assertIn(
            'sanitized.insert(QStringLiteral("activeDiagnostic"), activeDiagnostic)',
            PRIVACY,
        )

    def test_probe_states_are_truthful_and_details_are_expert_only(self):
        self.assertIn('case "failed":\n            return tr("Falló")', QML)
        self.assertIn('case "observed":\n            return tr("Observado")', QML)
        self.assertIn(
            'case "not_applicable":\n            return tr("No aplica")',
            QML,
        )
        self.assertIn(
            "model: window.diagnosticTechnicalExpanded",
            QML,
        )


if __name__ == "__main__":
    unittest.main()
