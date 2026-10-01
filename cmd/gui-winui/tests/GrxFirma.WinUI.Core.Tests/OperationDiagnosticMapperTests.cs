// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;
using GrxFirma.WinUI.Core.Diagnostics;
using GrxFirma.WinUI.Core.Ipc;

namespace GrxFirma.WinUI.Core.Tests;

[TestClass]
public sealed class OperationDiagnosticMapperTests
{
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

        var mapped = OperationDiagnosticMapper.FromResult(result);
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
            phase: "operation"));
        var partial = OperationDiagnosticMapper.FromResult(NewResult<object>(
            isSuccess: true,
            outcome: "partial",
            errorCode: "partial_result",
            phase: "operation"));

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

        var mapped = OperationDiagnosticMapper.FromException(exception);
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
