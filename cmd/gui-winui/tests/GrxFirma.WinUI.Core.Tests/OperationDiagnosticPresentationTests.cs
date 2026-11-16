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
        }, TestCatalogs.Es);

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
            DiagnosticStepPresentation.StatusDescription(status, TestCatalogs.Es));
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
        }, TestCatalogs.Es);

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
        }, TestCatalogs.Es);

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
        }, TestCatalogs.Es);

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
        }, TestCatalogs.Es);

        Assert.IsNull(presentation.FailureCode);
        Assert.IsFalse(presentation.Cause.Any(character =>
            character == '\u0000'));
        Assert.IsFalse(presentation.ExpertDetail.Any(character =>
            character == '\u0001'));
        Assert.AreEqual("step", presentation.Steps.Single().Code);
        Assert.AreEqual("Paso observado", presentation.Steps.Single().Label);
    }

    // Los textos de respaldo, los responsables, los estados y las frases
    // compuestas salen del catálogo en los 11 idiomas, sin claves a la vista.
    [TestMethod]
    public void FallbackTexts_ComeFromTheCatalogInAllLanguages()
    {
        foreach (var language in TestCatalogs.Languages)
        {
            var text = TestCatalogs.In(language);
            var presentation = OperationDiagnosticPresentation.From(new()
            {
                LikelyOwner = "unknown",
                Steps = Enum.GetValues<DiagnosticStepStatus>()
                    .Select(status => new OperationDiagnosticStep { Code = "x", Status = status, Owner = "portal" })
                    .ToArray(),
            }, text);
            var visible = new List<string>
            {
                presentation.Cause, presentation.OwnerLabel, presentation.Responsibility,
                presentation.SuggestedAction, presentation.ExpertDetail,
            };
            foreach (var owner in Enum.GetValues<DiagnosticOwner>())
                visible.Add(OperationDiagnosticPresentation.OwnerText(owner, text));
            foreach (var step in presentation.Steps)
            {
                visible.AddRange([step.Label, step.StatusText, step.StatusDisplayText, step.Detail,
                    step.OwnerDisplayText, step.SuggestedActionDisplayText, step.AutomationSummary]);
                StringAssert.Contains(step.StatusDisplayText, step.StatusText, language);
                StringAssert.Contains(step.AutomationSummary, step.OwnerLabel, language);
            }
            foreach (var value in visible)
            {
                Assert.IsFalse(string.IsNullOrWhiteSpace(value), language);
                Assert.IsFalse(value.Contains("winui.", StringComparison.Ordinal), $"{language}: «{value}»");
                Assert.IsFalse(value.Contains("{0}", StringComparison.Ordinal), $"{language}: «{value}»");
            }
        }
        Assert.AreEqual("Not checked", DiagnosticStepPresentation.StatusDescription(
            DiagnosticStepStatus.Unknown, TestCatalogs.In("en")));
    }
}
