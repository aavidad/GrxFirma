// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Diagnostics;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class OperationDiagnosticPresentationTests
{
    [TestMethod]
    [DataRow("app_local", DiagnosticOwner.Computer, "Este equipo o la aplicación local")]
    [DataRow("certificate_or_device", DiagnosticOwner.Computer, "Este equipo o la aplicación local")]
    [DataRow("network_proxy", DiagnosticOwner.Computer, "Este equipo o la aplicación local")]
    [DataRow("browser", DiagnosticOwner.Browser, "El navegador")]
    [DataRow("local_web_service", DiagnosticOwner.Browser, "El navegador")]
    [DataRow("remote_service", DiagnosticOwner.Portal, "El portal o sede electrónica")]
    [DataRow("government_afirma", DiagnosticOwner.GovernmentAFirma, "La plataforma @firma")]
    [DataRow("unexpected", DiagnosticOwner.Unknown, "Responsable desconocido")]
    public void OwnerMapping_IsExplicit(
        string wireOwner,
        DiagnosticOwner expectedOwner,
        string expectedLabel)
    {
        var presentation = OperationDiagnosticPresentation.From(new()
        {
            UserMessage = "Causa",
            ExpertMessage = "Detalle",
            LikelyOwner = wireOwner,
            ResponsibilityMessage = "Responsabilidad",
            SuggestedAction = "Acción",
        });

        Assert.AreEqual(expectedOwner, presentation.Owner);
        Assert.AreEqual(expectedLabel, presentation.OwnerLabel);
    }

    [TestMethod]
    [DataRow(DiagnosticStepStatus.Success, "✓", "Correcto")]
    [DataRow(DiagnosticStepStatus.Failure, "✕", "Falló")]
    [DataRow(DiagnosticStepStatus.Skipped, "—", "Omitido")]
    [DataRow(DiagnosticStepStatus.Unknown, "?", "No comprobado")]
    public void StepStatus_AlwaysProvidesIconAndText(
        DiagnosticStepStatus status,
        string expectedIcon,
        string expectedText)
    {
        Assert.AreEqual(
            expectedIcon,
            DiagnosticStepPresentation.StatusIconText(status));
        Assert.AreEqual(
            expectedText,
            DiagnosticStepPresentation.StatusDescription(status));
    }

    [TestMethod]
    public void UnknownWireStatus_RemainsUnknown()
    {
        const string json =
            """
            {
              "category": "unknown",
              "userMessage": "Sin datos",
              "expertMessage": "No observado",
              "likelyOwner": "unknown",
              "responsibilityMessage": "Sin responsable",
              "suggestedAction": "Esperar datos reales",
              "steps": [
                {
                  "code": "future",
                  "label": "Fase futura",
                  "status": "new-state"
                }
              ]
            }
            """;

        var diagnostic = JsonSerializer.Deserialize<OperationDiagnostic>(json);

        Assert.IsNotNull(diagnostic);
        Assert.AreEqual(
            DiagnosticStepStatus.Unknown,
            diagnostic!.Steps.Single().Status);
    }

    [TestMethod]
    public void FailureWithoutSteps_DoesNotInventTimelineEntries()
    {
        var presentation = OperationDiagnosticPresentation.From(new()
        {
            UserMessage = "Sin operación",
            ExpertMessage = "No conectado",
            LikelyOwner = "unknown",
            ResponsibilityMessage = "Sin responsable",
            SuggestedAction = "Abrir desde un error real",
            Steps = [],
        });

        Assert.AreEqual(0, presentation.Steps.Count);
    }

    [TestMethod]
    public void StepPresentation_IncludesOwnerActionAndAutomationSummary()
    {
        var presentation = DiagnosticStepPresentation.From(new()
        {
            Code = "SAF_26",
            Label = "Entrega al portal",
            Status = DiagnosticStepStatus.Failure,
            Owner = "remote_service",
            UserMessage = "El servicio remoto rechazó la operación.",
            SuggestedAction = "Contacte con la sede electrónica.",
        });

        Assert.AreEqual(DiagnosticOwner.Portal, presentation.Owner);
        Assert.AreEqual(
            "Responsable probable: El portal o sede electrónica",
            presentation.OwnerDisplayText);
        Assert.AreEqual(
            "Qué hacer ahora: Contacte con la sede electrónica.",
            presentation.SuggestedActionDisplayText);
        StringAssert.Contains(presentation.AutomationSummary, "Entrega al portal");
        StringAssert.Contains(presentation.AutomationSummary, "Estado: Falló");
        StringAssert.Contains(
            presentation.AutomationSummary,
            "Responsable probable: El portal o sede electrónica");
        StringAssert.Contains(
            presentation.AutomationSummary,
            "Qué hacer ahora: Contacte con la sede electrónica.");
    }

    [TestMethod]
    public void Presentation_LimitsVisibleStepsToCommonMaximum()
    {
        var presentation = OperationDiagnosticPresentation.From(new()
        {
            UserMessage = "Fallo",
            ExpertMessage = "Detalle",
            LikelyOwner = "app_local",
            ResponsibilityMessage = "Responsabilidad",
            SuggestedAction = "Acción",
            Steps = Enumerable.Range(1, 40)
                .Select(index => new OperationDiagnosticStep
                {
                    Code = $"STEP_{index}",
                    Label = $"Paso {index}",
                    Status = DiagnosticStepStatus.Success,
                })
                .ToArray(),
        });

        Assert.AreEqual(32, presentation.Steps.Count);
        Assert.AreEqual("Paso 32", presentation.Steps[^1].Label);
    }

    [TestMethod]
    public void Presentation_SanitizesControlsAndRejectsUnsafeCode()
    {
        var presentation = OperationDiagnosticPresentation.From(new()
        {
            FailureCode = "invalid code\r\n",
            UserMessage = "Causa\u0000 visible",
            ExpertMessage = "Detalle\u0001 técnico",
            LikelyOwner = "unknown",
            ResponsibilityMessage = "Responsabilidad\u0002",
            SuggestedAction = "Acción\u0003",
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = "step\u0000",
                    Label = "Paso\u0001 observado",
                    Status = DiagnosticStepStatus.Failure,
                    UserMessage = "Detalle\u0002",
                },
            ],
        });

        Assert.IsNull(presentation.FailureCode);
        Assert.IsFalse(presentation.Cause.Any(character =>
            character == '\u0000'));
        Assert.IsFalse(presentation.ExpertDetail.Any(character =>
            character == '\u0001'));
        Assert.AreEqual("step", presentation.Steps.Single().Code);
        Assert.AreEqual("Paso observado", presentation.Steps.Single().Label);
    }
}
