// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class OperationDiagnosticMapperTests
{
    private static readonly string[] Languages =
        ["ca", "de", "en", "es", "eu", "fr", "gl", "it", "pt", "va", "zh"];

    private static readonly Lazy<CatalogLocalizer> Catalogs =
        new(() => new CatalogLocalizer(FindLocales(), "es"));

    // Los textos del mapeo salen del catálogo real del repositorio.
    private static string Es(string key) => Catalogs.Value.Text("es", key);

    [TestMethod]
    public void BackendDiagnostic_IsPreservedSanitized_AndPresentationLimitsSteps()
    {
        var diagnostic = new OperationDiagnostic
        {
            Category = "remote_service",
            FailureCode = "saf_26",
            UserMessage =
                "El portal no completó la operación.\u0000",
            ExpertMessage =
                @"Fallo al leer C:\Users\persona\documento.pdf token=secreto",
            LikelyOwner = "remote_service",
            ResponsibilityMessage =
                "El portal debe revisar la operación.",
            SuggestedAction =
                "Contacte con el portal y conserve el diagnóstico.",
            Steps = Enumerable.Range(1, 40)
                .Select(index => new OperationDiagnosticStep
                {
                    Code = $"step_{index}",
                    Label = $"Paso {index}",
                    Status = index == 40
                        ? DiagnosticStepStatus.Failure
                        : DiagnosticStepStatus.Success,
                    Owner = "remote_service",
                    UserMessage = "Evidencia segura.",
                    SuggestedAction = "Continúe con el paso siguiente.",
                    EvidenceRef = "phase:operation",
                })
                .ToArray(),
        };
        var result = NewResult(
            isSuccess: false,
            outcome: "failure",
            errorCode: "saf_26",
            phase: "operation",
            data: new SensitiveResult
            {
                Path = @"C:\Users\persona\documento.pdf",
                PayloadBase64 = "U0VDUkVUX1BBWUxPQUQ=",
            },
            diagnostic: diagnostic);

        var mapped = OperationDiagnosticMapper.FromResult(result, Es);
        var serialized = JsonSerializer.Serialize(mapped);
        var presentation = OperationDiagnosticPresentation.From(mapped);

        Assert.AreEqual("remote_service", mapped.Category);
        Assert.AreEqual("SAF_26", mapped.FailureCode);
        Assert.AreEqual(
            "El portal no completó la operación.",
            mapped.UserMessage);
        Assert.AreEqual("remote_service", mapped.LikelyOwner);
        Assert.AreEqual(40, mapped.Steps.Count);
        Assert.AreEqual(32, presentation.Steps.Count);
        StringAssert.Contains(mapped.ExpertMessage, "[contenido protegido]");
        AssertDoesNotContainSensitiveData(serialized);
    }

    [TestMethod]
    public void MissingDiagnostic_UsesOnlySafeMetadata_WithoutInventingOwner()
    {
        var result = NewResult(
            isSuccess: false,
            outcome: "failure",
            errorCode: "private_failure",
            phase: "operation",
            data: new SensitiveResult
            {
                Path = @"/home/persona/documento.pdf",
                PayloadBase64 = "VGhpcy1pcy1hLXNlY3JldC1wYXlsb2Fk",
            });

        var mapped = OperationDiagnosticMapper.FromResult(result);
        var visibleText = VisibleText(mapped);
        var serialized = JsonSerializer.Serialize(mapped);

        Assert.AreEqual("unknown", mapped.Category);
        Assert.AreEqual("unknown", mapped.LikelyOwner);
        Assert.AreEqual("PRIVATE_FAILURE", mapped.FailureCode);
        Assert.AreEqual(DiagnosticStepStatus.Failure, mapped.Steps.Single().Status);
        Assert.AreEqual("unknown", mapped.Steps.Single().Owner);
        Assert.AreEqual(
            "phase:operation",
            mapped.Steps.Single().EvidenceRef);
        Assert.IsFalse(
            visibleText.Contains(
                "PRIVATE_FAILURE",
                StringComparison.OrdinalIgnoreCase));
        AssertDoesNotContainSensitiveData(serialized);
    }

    [TestMethod]
    public void MissingDiagnostic_SuccessAndPartialHaveExplicitVisualStates()
    {
        var success = OperationDiagnosticMapper.FromResult(NewResult<object>(
            isSuccess: true,
            outcome: "success",
            phase: "operation"), Es);
        var partial = OperationDiagnosticMapper.FromResult(NewResult<object>(
            isSuccess: true,
            outcome: "partial",
            errorCode: "partial_result",
            phase: "operation"), Es);

        Assert.IsNull(success.FailureCode);
        Assert.AreEqual(
            DiagnosticStepStatus.Success,
            success.Steps.Single().Status);
        Assert.AreEqual("unknown", success.LikelyOwner);

        Assert.AreEqual("PARTIAL_RESULT", partial.FailureCode);
        Assert.AreEqual(
            DiagnosticStepStatus.Failure,
            partial.Steps.Single().Status);
        Assert.AreEqual("unknown", partial.LikelyOwner);
        StringAssert.Contains(partial.UserMessage, "parcialmente");
    }

    [TestMethod]
    public void IpcException_UsesSafeContractPhaseOwnerAndSuggestedAction()
    {
        var exception = IpcClientException.ProtocolVersion();

        var mapped = OperationDiagnosticMapper.FromException(exception, catalogText: Es);
        var visibleText = VisibleText(mapped);

        Assert.AreEqual("UNSUPPORTED_PROTOCOL", mapped.FailureCode);
        Assert.AreEqual("app_local", mapped.LikelyOwner);
        Assert.AreEqual("phase:protocol", mapped.Steps.Single().EvidenceRef);
        Assert.AreEqual(
            DiagnosticStepStatus.Failure,
            mapped.Steps.Single().Status);
        Assert.AreEqual("app_local", mapped.Steps.Single().Owner);
        StringAssert.Contains(mapped.SuggestedAction, "misma versión");
        Assert.IsFalse(
            visibleText.Contains(
                "UNSUPPORTED_PROTOCOL",
                StringComparison.OrdinalIgnoreCase));
    }

    [TestMethod]
    public void UserCancellation_IsOnlyClaimedWithExplicitCancelledToken()
    {
        using var cancellation = new CancellationTokenSource();
        cancellation.Cancel();
        var exception = new OperationCanceledException(
            @"C:\Users\persona\documento.pdf token=secreto",
            innerException: null,
            cancellation.Token);

        var mapped = OperationDiagnosticMapper.FromException(
            exception,
            cancellation.Token);

        Assert.AreEqual("USER_CANCELLED", mapped.FailureCode);
        Assert.AreEqual("unknown", mapped.LikelyOwner);
        Assert.IsTrue(mapped.UserCanResolveDirectly);
        Assert.AreEqual(
            DiagnosticStepStatus.Skipped,
            mapped.Steps.Single().Status);
        Assert.AreEqual("phase:operation", mapped.Steps.Single().EvidenceRef);
        AssertDoesNotContainSensitiveData(JsonSerializer.Serialize(mapped));
    }

    [TestMethod]
    public void UnexpectedFailure_DoesNotExposeExceptionOrClaimResponsibility()
    {
        var exception = new InvalidOperationException(
            @"payload=U0VDUkVUX1BBWUxPQUQ= at C:\Users\persona\documento.pdf");

        var mapped = OperationDiagnosticMapper.FromException(exception);
        var serialized = JsonSerializer.Serialize(mapped);

        Assert.AreEqual("UNEXPECTED_FAILURE", mapped.FailureCode);
        Assert.AreEqual("unknown", mapped.Category);
        Assert.AreEqual("unknown", mapped.LikelyOwner);
        Assert.AreEqual("unknown", mapped.Steps.Single().Owner);
        Assert.AreEqual(
            DiagnosticStepStatus.Failure,
            mapped.Steps.Single().Status);
        AssertDoesNotContainSensitiveData(serialized);
        Assert.IsFalse(
            serialized.Contains(
                nameof(InvalidOperationException),
                StringComparison.Ordinal));
    }

    [TestMethod]
    public void UnconfirmedCancellation_IsTreatedAsUnexpectedFailure()
    {
        var mapped = OperationDiagnosticMapper.FromException(
            new OperationCanceledException("timeout interno"));

        Assert.AreEqual("UNEXPECTED_FAILURE", mapped.FailureCode);
        Assert.AreEqual(
            DiagnosticStepStatus.Failure,
            mapped.Steps.Single().Status);
    }

    // Lote 39j: los ajustes sin permiso o de otra cuenta llevan su propio
    // aviso con la orden entera; no pasan por el filtro de rutas.
    [TestMethod]
    public void SettingsAccessCodes_KeepTheWholeCommandOutsideTheRedaction()
    {
        const string icacls = "icacls \"%APPDATA%\\GrxFirma\" /grant \"%USERNAME%:(OI)(CI)F\" /T";
        var catalog = new Dictionary<string, string>
        {
            ["error.settings_sin_permiso"] = "No se pueden abrir los ajustes. Ejecute:\n" + icacls,
            ["error.settings_otro_usuario"] = "La carpeta es de otra cuenta.",
        };
        string Text(string key) => catalog.TryGetValue(key, out var value) ? value : key;
        foreach (var code in new[] { SettingsAccessFailure.NoPermissionCode, "settings_other_user" })
        {
            var engine = new OperationDiagnostic
            {
                Category = "local_configuration",
                FailureCode = "SETTINGS_ACCESS",
                UserMessage = "icacls \"%APPDATA%\\GrxFirma\" /grant",
            };
            var mapped = OperationDiagnosticMapper.FromResult(NewResult<object>(
                false, "failure", code, diagnostic: engine), Text);
            var expected = SettingsAccessFailure.From(code, Text);
            Assert.IsNotNull(mapped.SettingsAccess, code);
            Assert.AreEqual(expected!.Command, mapped.SettingsAccess.Command, code);
            Assert.AreEqual(expected.Explanation, mapped.UserMessage, code);
        }
        Assert.AreEqual(icacls, OperationDiagnosticMapper.FromResult(NewResult<object>(
            false, "failure", SettingsAccessFailure.NoPermissionCode), Text).SettingsAccess!.Command);
        Assert.IsNull(OperationDiagnosticMapper.FromResult(NewResult<object>(false, "failure", "operation_failed"), Text).SettingsAccess);
    }

    [TestMethod]
    public void EveryVisibleText_ComesFromTheCatalogInAllLanguages()
    {
        using var cancellation = new CancellationTokenSource();
        cancellation.Cancel();
        foreach (var language in Languages)
        {
            string Text(string key) => Catalogs.Value.Text(language, key);
            var diagnostics = new List<OperationDiagnostic>
            {
                OperationDiagnosticMapper.FromUserCancellation(Text),
                OperationDiagnosticMapper.FromException(new InvalidOperationException("x"), catalogText: Text),
                OperationDiagnosticMapper.FromException(
                    new OperationCanceledException(cancellation.Token), cancellation.Token, Text),
                OperationDiagnosticMapper.Sanitize(new OperationDiagnostic(), Text),
                OperationDiagnosticMapper.Sanitize(new OperationDiagnostic
                {
                    LikelyOwner = "afirma",
                    Steps = [new OperationDiagnosticStep { Code = "x" }],
                }, Text),
            };
            foreach (var exception in new[]
                     {
                         IpcClientException.ProtocolVersion(), IpcClientException.Protocol(),
                         IpcClientException.Connection(), IpcClientException.Connection(retryable: false),
                         IpcClientException.Closed(), IpcClientException.ClosedAfterRequest(),
                         IpcClientException.ServerIdentity(), IpcClientException.Configuration(),
                     })
            {
                diagnostics.Add(OperationDiagnosticMapper.FromException(exception, catalogText: Text));
            }
            foreach (var outcome in new[] { "success", "partial", "failure" })
            foreach (var phase in new[] { "admission", "protocol", "operation", null })
            {
                diagnostics.Add(OperationDiagnosticMapper.FromResult(NewResult<object>(
                    outcome != "failure", outcome, phase: phase), Text));
            }

            foreach (var diagnostic in diagnostics)
            {
                foreach (var visible in VisibleText(diagnostic).Split('\n').Append(diagnostic.ExpertMessage).Where(text => text.Length > 0))
                {
                    Assert.IsFalse(string.IsNullOrWhiteSpace(visible), language);
                    Assert.IsFalse(visible.Contains("winui.", StringComparison.Ordinal),
                        $"{language}: clave sin traducir en «{visible}»");
                }
            }
        }
    }

    [TestMethod]
    public void RedactionMarker_FollowsTheLanguage()
    {
        var diagnostic = new OperationDiagnostic { ExpertMessage = @"Fallo al leer C:\Users\persona\a.pdf" };
        foreach (var language in Languages)
        {
            var marker = Catalogs.Value.Text(language, "winui.diagnostico.contenido_protegido");
            Assert.IsFalse(marker.StartsWith("winui.", StringComparison.Ordinal), language);
            var mapped = OperationDiagnosticMapper.Sanitize(diagnostic, key => Catalogs.Value.Text(language, key));
            StringAssert.Contains(mapped.ExpertMessage, marker, language);
            Assert.IsFalse(mapped.ExpertMessage.Contains("persona", StringComparison.Ordinal), language);
        }
        Assert.AreEqual("[protected content]", Catalogs.Value.Text("en", "winui.diagnostico.contenido_protegido"));
        Assert.AreEqual("[contenido protegido]", Es("winui.diagnostico.contenido_protegido"));
    }

    // Un «$» en la traducción del marcador no se toma como referencia de
    // grupo de la expresión regular.
    [TestMethod]
    public void RedactionMarker_IsInsertedLiterally()
    {
        var mapped = OperationDiagnosticMapper.Sanitize(
            new OperationDiagnostic { ExpertMessage = "token=secreto" },
            key => key == "winui.diagnostico.contenido_protegido" ? "[$0 oculto]" : Es(key));
        Assert.AreEqual("[$0 oculto]", mapped.ExpertMessage);
    }

    // El detalle técnico se compone de dos frases completas; antes quedaba
    // «ein Ergebnis erfolgreich» en alemán y «parcial» sin traducir.
    [TestMethod]
    public void MetadataExpertMessage_UsesWholeSentences()
    {
        var partial = OperationDiagnosticMapper.FromResult(NewResult<object>(
            true, "partial", phase: "admission"), Es);
        Assert.AreEqual(
            "El motor local comunicó un resultado parcial. Fase comunicada: admisión.",
            partial.ExpertMessage);
        var failure = OperationDiagnosticMapper.FromResult(NewResult<object>(
            false, "failure"), key => Catalogs.Value.Text("de", key));
        Assert.AreEqual(
            "Die lokale Engine hat einen Fehlschlag gemeldet. Sie hat keine erkannte Phase gemeldet.",
            failure.ExpertMessage);
        var zh = OperationDiagnosticMapper.FromResult(NewResult<object>(
            true, "success", phase: "protocol"), key => Catalogs.Value.Text("zh", key));
        Assert.AreEqual("本地引擎报告的结果为成功。报告的阶段：协议。", zh.ExpertMessage);
    }

    private static string FindLocales()
    {
        for (var directory = new DirectoryInfo(AppContext.BaseDirectory); directory is not null; directory = directory.Parent)
        {
            var candidate = Path.Combine(directory.FullName, "internal", "adapters", "outbound", "common", "localizador", "locales");
            if (File.Exists(Path.Combine(candidate, "es.json")))
            {
                return candidate;
            }
        }

        Assert.Fail("No se encuentran los catálogos del repositorio.");
        return string.Empty;
    }

    private static IpcCallResult<TData> NewResult<TData>(
        bool isSuccess,
        string outcome,
        string? errorCode = null,
        string? phase = null,
        TData? data = default,
        OperationDiagnostic? diagnostic = null) =>
        new()
        {
            Protocol = DesktopIpcProtocol.Name,
            RequestId = "request-test",
            TraceId = "trace-test",
            Action = "test",
            IsSuccess = isSuccess,
            Outcome = outcome,
            ErrorCode = errorCode,
            Phase = phase,
            Retryable = false,
            Data = data,
            Diagnostic = diagnostic,
        };

    private static string VisibleText(OperationDiagnostic diagnostic) =>
        string.Join(
            "\n",
            new[]
            {
                diagnostic.UserMessage,
                diagnostic.ResponsibilityMessage,
                diagnostic.SuggestedAction,
            }.Concat(diagnostic.Steps.SelectMany(step => new[]
            {
                step.Label,
                step.UserMessage,
                step.SuggestedAction,
            })));

    private static void AssertDoesNotContainSensitiveData(string value)
    {
        Assert.IsFalse(
            value.Contains(
                @"C:\Users\persona",
                StringComparison.OrdinalIgnoreCase));
        Assert.IsFalse(
            value.Contains(
                "/home/persona",
                StringComparison.Ordinal));
        Assert.IsFalse(
            value.Contains(
                "U0VDUkVUX1BBWUxPQUQ=",
                StringComparison.Ordinal));
        Assert.IsFalse(
            value.Contains(
                "VGhpcy1pcy1hLXNlY3JldC1wYXlsb2Fk",
                StringComparison.Ordinal));
        Assert.IsFalse(
            value.Contains(
                "token=secreto",
                StringComparison.OrdinalIgnoreCase));
    }

    private sealed record SensitiveResult
    {
        public required string Path { get; init; }
        public required string PayloadBase64 { get; init; }
    }
}
