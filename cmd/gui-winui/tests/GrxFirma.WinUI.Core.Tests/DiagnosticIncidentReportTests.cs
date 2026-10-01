// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Diagnostics;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class DiagnosticIncidentReportTests
{
    [TestMethod]
    public void CreateJson_RedactsSensitiveContentAndKeepsSafeCorrelation()
    {
        var diagnostic = new OperationDiagnostic
        {
            RequestId = "request-123",
            TraceId = "trace:456",
            Action = "sign",
            FailureCode = "sign_failed",
            UserMessage = @"No se pudo leer C:\Users\persona\documento.pdf",
            ExpertMessage =
                "password=secreto payload=U0VDUkVUX1BBWUxPQUQ=",
            LikelyOwner = "app_local",
            ResponsibilityMessage = "Revise /home/persona/documento.pdf",
            SuggestedAction = "Repita la operación.",
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = "sign",
                    Label = "Firma",
                    Status = DiagnosticStepStatus.Failure,
                    Owner = "app_local",
                    UserMessage = @"Falló file:///C:/privado.pdf",
                    SuggestedAction = "Vuelva a intentarlo.",
                    EvidenceRef = @"C:\privado\evidence.json",
                },
            ],
        };

        var json = DiagnosticIncidentReport.CreateJson(
            diagnostic,
            DateTimeOffset.Parse("2026-07-28T12:34:56+02:00"));

        using var document = JsonDocument.Parse(json);
        var root = document.RootElement;
        Assert.AreEqual(
            DiagnosticIncidentReport.Schema,
            root.GetProperty("schema").GetString());
        Assert.AreEqual(
            "2026-07-28T10:34:56+00:00",
            root.GetProperty("generatedAtUtc").GetString());
        Assert.AreEqual(
            "request-123",
            root.GetProperty("correlation")
                .GetProperty("requestId")
                .GetString());
        Assert.AreEqual(
            "sign",
            root.GetProperty("correlation")
                .GetProperty("action")
                .GetString());
        Assert.AreEqual(
            1,
            root.GetProperty("steps").GetArrayLength());
        StringAssert.Contains(json, "[contenido protegido]");
        AssertDoesNotContain(json, @"C:\Users\persona");
        AssertDoesNotContain(json, "/home/persona");
        AssertDoesNotContain(json, "secreto");
        AssertDoesNotContain(json, "U0VDUkVUX1BBWUxPQUQ=");
        AssertDoesNotContain(json, "evidenceRef");
    }

    [TestMethod]
    public void CreateJson_DropsInvalidCorrelationAndLimitsSteps()
    {
        var diagnostic = new OperationDiagnostic
        {
            RequestId = "request with spaces",
            TraceId = new string('x', 129),
            Action = "../sign",
            UserMessage = "Fallo controlado.",
            ExpertMessage = "Detalle controlado.",
            LikelyOwner = "unknown",
            ResponsibilityMessage = "Responsable desconocido.",
            SuggestedAction = "Repita la operación.",
            Steps = Enumerable.Range(1, 80)
                .Select(index => new OperationDiagnosticStep
                {
                    Code = $"step_{index}",
                    Label = $"Paso {index}",
                    Status = DiagnosticStepStatus.Success,
                })
                .ToArray(),
        };

        var json = DiagnosticIncidentReport.CreateJson(
            diagnostic,
            DateTimeOffset.UtcNow);

        using var document = JsonDocument.Parse(json);
        var root = document.RootElement;
        Assert.IsFalse(root.TryGetProperty("correlation", out _));
        Assert.AreEqual(
            32,
            root.GetProperty("steps").GetArrayLength());
        Assert.IsTrue(
            System.Text.Encoding.UTF8.GetByteCount(json) < 128 * 1024);
    }

    private static void AssertDoesNotContain(
        string actual,
        string forbidden) =>
        Assert.IsFalse(
            actual.Contains(
                forbidden,
                StringComparison.OrdinalIgnoreCase),
            $"El informe contiene el valor prohibido: {forbidden}");
}
