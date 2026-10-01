// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.RegularExpressions;
using GrxFirma.WinUI.Core.Ipc;

namespace GrxFirma.WinUI.Core.Diagnostics;

/// <summary>
/// Transforma resultados y fallos del canal local en el modelo que consume la
/// interfaz básica. El mapeo no inspecciona los datos funcionales de la
/// operación ni propaga mensajes de excepciones no controladas.
/// </summary>
public static class OperationDiagnosticMapper
{
    private const string RedactedText = "[contenido protegido]";

    private static readonly Regex SensitiveAssignmentPattern = new(
        """
        \b(?:password|passwd|contrase(?:ñ|n)a|clave|secret(?:_b64)?|token|bearer|authorization|api[_-]?key|pin|passphrase|payload(?:_b64)?|data(?:_base64|_b64)?|credential(?:_b64)?|certificate(?:_b64)?|signature(?:_b64)?)\b
        \s*[:=]\s*
        (?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)
        """,
        RegexOptions.IgnoreCase |
        RegexOptions.CultureInvariant |
        RegexOptions.IgnorePatternWhitespace,
        TimeSpan.FromMilliseconds(100));

    private static readonly Regex FileUriPattern = new(
        @"\bfile:(?://)?[^\r\n]*",
        RegexOptions.IgnoreCase | RegexOptions.CultureInvariant,
        TimeSpan.FromMilliseconds(100));

    private static readonly Regex WindowsPathPattern = new(
        @"(?<![A-Za-z0-9])[A-Za-z]:[\\/][^\r\n]*",
        RegexOptions.CultureInvariant,
        TimeSpan.FromMilliseconds(100));

    private static readonly Regex UncPathPattern = new(
        @"(?<![\\])\\\\[^\r\n]*",
        RegexOptions.CultureInvariant,
        TimeSpan.FromMilliseconds(100));

    private static readonly Regex RelativePathPattern = new(
        @"(?<![A-Za-z0-9])\.\.?[\\/][^\r\n]*",
        RegexOptions.CultureInvariant,
        TimeSpan.FromMilliseconds(100));

    private static readonly Regex PosixPathPattern = new(
        @"(?<![A-Za-z0-9:/])/(?!/)[^\r\n]*",
        RegexOptions.CultureInvariant,
        TimeSpan.FromMilliseconds(100));

    private static readonly Regex Base64Pattern = new(
        @"(?<![A-Za-z0-9+/])(?:[A-Za-z0-9+/]{24,}={0,2})(?![A-Za-z0-9+/=])",
        RegexOptions.CultureInvariant,
        TimeSpan.FromMilliseconds(100));

    public static OperationDiagnostic FromResult<TData>(
        IpcCallResult<TData> result)
    {
        ArgumentNullException.ThrowIfNull(result);

        var diagnostic = result.Diagnostic is not null
            ? SanitizeBackendDiagnostic(result.Diagnostic)
            : BuildFromResultMetadata(result);

        return diagnostic with
        {
            RequestId = NormalizeOpaqueIdentifier(result.RequestId, 128),
            TraceId = NormalizeOpaqueIdentifier(result.TraceId, 128),
            Action = NormalizeOpaqueIdentifier(result.Action, 64),
        };
    }

    /// <summary>
    /// Reaplica los límites y la redacción del contrato seguro antes de
    /// persistir o presentar un diagnóstico recibido por otra vía.
    /// </summary>
    public static OperationDiagnostic Sanitize(
        OperationDiagnostic diagnostic)
    {
        ArgumentNullException.ThrowIfNull(diagnostic);

        return SanitizeBackendDiagnostic(diagnostic) with
        {
            RequestId = NormalizeOpaqueIdentifier(
                diagnostic.RequestId,
                128),
            TraceId = NormalizeOpaqueIdentifier(
                diagnostic.TraceId,
                128),
            Action = NormalizeOpaqueIdentifier(
                diagnostic.Action,
                64),
        };
    }

    /// <summary>
    /// Convierte una excepción sin usar su mensaje, traza, tipo concreto de
    /// datos ni propiedades ajenas al contrato seguro de
    /// <see cref="IpcClientException"/>.
    /// </summary>
    /// <param name="exception">Fallo capturado por la interfaz.</param>
    /// <param name="userCancellationToken">
    /// Token de cancelación controlado por el usuario. Una cancelación solo se
    /// presenta como voluntaria cuando este token confirma que fue solicitada.
    /// </param>
    public static OperationDiagnostic FromException(
        Exception exception,
        CancellationToken userCancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(exception);

        if (exception is IpcClientException ipcException)
        {
            return BuildFromIpcException(ipcException);
        }

        if (exception is OperationCanceledException &&
            userCancellationToken.IsCancellationRequested)
        {
            return FromUserCancellation();
        }

        return BuildUnexpectedFailure();
    }

    public static OperationDiagnostic FromUserCancellation() =>
        new()
        {
            Category = "unknown",
            FailureCode = "USER_CANCELLED",
            UserMessage = "La operación se canceló antes de completarse.",
            ExpertMessage =
                "La interfaz recibió una cancelación solicitada por el usuario.",
            LikelyOwner = "unknown",
            ResponsibilityMessage =
                "La operación se detuvo por decisión del usuario; no se atribuye un fallo a ningún sistema.",
            SuggestedAction =
                "Si desea continuar, inicie de nuevo la operación.",
            UserCanResolveDirectly = true,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = "USER_CANCELLED",
                    Label = "Ejecución de la operación",
                    Status = DiagnosticStepStatus.Skipped,
                    Owner = "unknown",
                    UserMessage =
                        "La operación se detuvo antes de finalizar.",
                    SuggestedAction =
                        "Inicie de nuevo la operación cuando quiera continuar.",
                    EvidenceRef = "phase:operation",
                },
            ],
        };

    private static OperationDiagnostic SanitizeBackendDiagnostic(
        OperationDiagnostic diagnostic)
    {
        var steps = (diagnostic.Steps ?? [])
            .Take(64)
            .Select(step => new OperationDiagnosticStep
            {
                Code = NormalizeTechnicalCode(step.Code) ?? "unknown",
                Label = CleanVisibleText(
                    step.Label,
                    160,
                    "Paso observado"),
                Status = step.Status,
                Owner = NormalizeOwner(step.Owner),
                UserMessage = CleanOptionalVisibleText(
                    step.UserMessage,
                    512),
                SuggestedAction = CleanOptionalVisibleText(
                    step.SuggestedAction,
                    512),
                EvidenceRef = NormalizeEvidenceReference(step.EvidenceRef),
            })
            .ToArray();

        return new OperationDiagnostic
        {
            Category = NormalizeCategory(diagnostic.Category),
            FailureCode = NormalizeTechnicalCode(diagnostic.FailureCode),
            UserMessage = CleanVisibleText(
                diagnostic.UserMessage,
                512,
                "La operación no se pudo completar."),
            ExpertMessage = CleanVisibleText(
                diagnostic.ExpertMessage,
                4096,
                "No hay detalle técnico disponible."),
            LikelyOwner = NormalizeOwner(diagnostic.LikelyOwner),
            ResponsibilityMessage = CleanVisibleText(
                diagnostic.ResponsibilityMessage,
                512,
                "No se ha podido determinar el responsable probable."),
            SuggestedAction = CleanVisibleText(
                diagnostic.SuggestedAction,
                512,
                "Vuelva a intentarlo o solicite soporte si el problema continúa."),
            UserCanResolveDirectly = diagnostic.UserCanResolveDirectly,
            Steps = steps,
        };
    }

    private static OperationDiagnostic BuildFromResultMetadata<TData>(
        IpcCallResult<TData> result)
    {
        var phase = NormalizePhase(result.Phase);
        var outcome = NormalizeOutcome(result.IsSuccess, result.Outcome);
        var successful = outcome == "success";
        var partial = outcome == "partial";
        var cause = successful
            ? "La operación se completó correctamente."
            : partial
                ? "La operación terminó parcialmente."
                : "La operación no se pudo completar.";
        var action = successful
            ? "Puede continuar con el trámite."
            : partial
                ? "Revise el resultado antes de continuar y repita la operación si falta información."
                : "Vuelva a intentarlo. Si el problema continúa, conserve el diagnóstico y solicite soporte.";

        return new OperationDiagnostic
        {
            Category = "unknown",
            FailureCode = successful
                ? null
                : NormalizeTechnicalCode(result.ErrorCode),
            UserMessage = cause,
            ExpertMessage = BuildMetadataExpertMessage(outcome, phase),
            LikelyOwner = "unknown",
            ResponsibilityMessage = successful
                ? "La operación finalizó sin un fallo que atribuir."
                : "Con la información disponible no se puede determinar si el origen es local o remoto.",
            SuggestedAction = action,
            UserCanResolveDirectly = false,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = $"{phase ?? "unknown"}_result",
                    Label = PhaseLabel(phase),
                    Status = successful
                        ? DiagnosticStepStatus.Success
                        : DiagnosticStepStatus.Failure,
                    Owner = "unknown",
                    UserMessage = cause,
                    SuggestedAction = action,
                    EvidenceRef = phase is null
                        ? null
                        : $"phase:{phase}",
                },
            ],
        };
    }

    private static OperationDiagnostic BuildFromIpcException(
        IpcClientException exception)
    {
        var phase = NormalizePhase(exception.Phase);
        var owner = NormalizeOwner(exception.LikelyOwner);
        var action = SuggestedActionForPhase(phase, exception.Retryable);
        var userMessage = CleanVisibleText(
            exception.UserMessage,
            512,
            "No se pudo completar la comunicación segura con el motor local.");

        return new OperationDiagnostic
        {
            Category = owner,
            FailureCode = NormalizeTechnicalCode(exception.Code),
            UserMessage = userMessage,
            ExpertMessage = phase switch
            {
                "admission" =>
                    "El canal local se detuvo durante la admisión segura, antes de habilitar la operación.",
                "protocol" when exception.Code == "connection_closed" =>
                    "La interfaz envió la petición y no recibió respuesta del motor local.",
                "protocol" =>
                    "El canal local se detuvo al validar el protocolo de comunicación.",
                _ =>
                    "El canal local se detuvo durante la operación.",
            },
            LikelyOwner = owner,
            ResponsibilityMessage = ResponsibilityForOwner(owner),
            SuggestedAction = action,
            UserCanResolveDirectly =
                owner is "environment" or "computer",
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = $"{phase ?? "unknown"}_ipc",
                    Label = PhaseLabel(phase),
                    Status = DiagnosticStepStatus.Failure,
                    Owner = owner,
                    UserMessage = userMessage,
                    SuggestedAction = action,
                    EvidenceRef = phase is null
                        ? null
                        : $"phase:{phase}",
                },
            ],
        };
    }

    private static OperationDiagnostic BuildUnexpectedFailure() =>
        new()
        {
            Category = "unknown",
            FailureCode = "UNEXPECTED_FAILURE",
            UserMessage =
                "La aplicación encontró un problema inesperado y no pudo completar la operación.",
            ExpertMessage =
                "La interfaz detuvo la operación sin incorporar el mensaje, la traza ni los datos de la excepción.",
            LikelyOwner = "unknown",
            ResponsibilityMessage =
                "Con la evidencia disponible no se puede atribuir el origen del problema.",
            SuggestedAction =
                "Vuelva a intentarlo. Si se repite, conserve el código del diagnóstico y solicite soporte.",
            UserCanResolveDirectly = false,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = "UNEXPECTED_OPERATION",
                    Label = "Ejecución de la operación",
                    Status = DiagnosticStepStatus.Failure,
                    Owner = "unknown",
                    UserMessage =
                        "La operación se detuvo por un problema no identificado.",
                    SuggestedAction =
                        "Vuelva a intentarlo y solicite soporte si se repite.",
                    EvidenceRef = "phase:operation",
                },
            ],
        };

    private static string NormalizeOutcome(bool isSuccess, string? outcome)
    {
        var normalized = outcome?.Trim().ToLowerInvariant();
        if (!isSuccess || normalized == "failure")
        {
            return "failure";
        }

        return normalized == "partial" ? "partial" : "success";
    }

    private static string? NormalizePhase(string? phase) =>
        phase?.Trim().ToLowerInvariant() switch
        {
            "admission" => "admission",
            "protocol" => "protocol",
            "operation" => "operation",
            _ => null,
        };

    private static string PhaseLabel(string? phase) => phase switch
    {
        "admission" => "Preparación segura de la operación",
        "protocol" => "Comunicación con el motor local",
        "operation" => "Ejecución de la operación",
        _ => "Resultado de la operación",
    };

    private static string BuildMetadataExpertMessage(
        string outcome,
        string? phase) =>
        phase is null
            ? $"El motor local comunicó un resultado {OutcomeLabel(outcome)} sin una fase reconocida."
            : $"El motor local comunicó un resultado {OutcomeLabel(outcome)} en la fase {PhaseTechnicalLabel(phase)}.";

    private static string OutcomeLabel(string outcome) => outcome switch
    {
        "success" => "correcto",
        "partial" => "parcial",
        _ => "fallido",
    };

    private static string PhaseTechnicalLabel(string phase) => phase switch
    {
        "admission" => "de admisión",
        "protocol" => "de protocolo",
        _ => "de operación",
    };

    private static string SuggestedActionForPhase(
        string? phase,
        bool retryable) =>
        phase switch
        {
            "protocol" =>
                "Compruebe que la interfaz y el motor local pertenecen a la misma versión instalada. Si se repite, repare la instalación.",
            "admission" when retryable =>
                "Cierre la aplicación, ábrala de nuevo desde el lanzador y repita la operación.",
            "admission" =>
                "Abra la aplicación desde el lanzador. Si se repite, repare la instalación o solicite soporte.",
            _ when retryable =>
                "Vuelva a abrir la aplicación y repita la operación.",
            _ =>
                "Vuelva a intentarlo. Si se repite, conserve el diagnóstico y solicite soporte.",
        };

    private static string ResponsibilityForOwner(string owner) => owner switch
    {
        "app_local" =>
            "El fallo se ha localizado en la aplicación o el motor local.",
        "environment" or "computer" =>
            "El fallo se ha localizado en el entorno de este equipo.",
        "browser" or "browser_integration" or "local_web_service" =>
            "El fallo se ha localizado en la integración local con el navegador.",
        "remote_service" or "portal" or "website" =>
            "El fallo se ha localizado en el portal o servicio remoto.",
        "government_afirma" or "afirma" or "@firma" =>
            "El fallo se ha localizado en la plataforma @firma.",
        _ =>
            "Con la evidencia disponible no se puede determinar el responsable.",
    };

    private static string NormalizeCategory(string? category) =>
        NormalizeClassification(category);

    private static string NormalizeOwner(string? owner) =>
        NormalizeClassification(owner);

    private static string NormalizeClassification(string? value) =>
        value?.Trim().ToLowerInvariant() switch
        {
            "app_local" => "app_local",
            "certificate_store" => "certificate_store",
            "certificate_or_device" => "certificate_or_device",
            "environment" => "environment",
            "network_proxy" => "network_proxy",
            "computer" => "computer",
            "browser" => "browser",
            "browser_integration" => "browser_integration",
            "local_web_service" => "local_web_service",
            "navegador" => "navegador",
            "remote_service" => "remote_service",
            "portal" => "portal",
            "website" => "website",
            "government_afirma" => "government_afirma",
            "afirma" => "afirma",
            "@firma" => "@firma",
            _ => "unknown",
        };

    private static string? NormalizeTechnicalCode(string? value)
    {
        var normalized = value?.Trim().ToUpperInvariant();
        if (string.IsNullOrEmpty(normalized) ||
            normalized.Length > 64 ||
            normalized.Any(character =>
                character is not (
                    '_' or '-' or
                    >= 'A' and <= 'Z' or
                    >= '0' and <= '9')))
        {
            return null;
        }

        return normalized;
    }

    private static string? NormalizeOpaqueIdentifier(
        string? value,
        int maximumCharacters)
    {
        var normalized = value?.Trim();
        if (string.IsNullOrEmpty(normalized) ||
            normalized.Length > maximumCharacters ||
            normalized.Any(character =>
                character is not (
                    '.' or '_' or '-' or ':' or
                    >= 'A' and <= 'Z' or
                    >= 'a' and <= 'z' or
                    >= '0' and <= '9')))
        {
            return null;
        }

        return normalized;
    }

    private static string? NormalizeEvidenceReference(string? value) =>
        value?.Trim().ToLowerInvariant() switch
        {
            "phase:admission" => "phase:admission",
            "phase:protocol" => "phase:protocol",
            "phase:operation" => "phase:operation",
            _ => null,
        };

    private static string? CleanOptionalVisibleText(
        string? value,
        int maximumCharacters)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return null;
        }

        return CleanVisibleText(value, maximumCharacters, string.Empty);
    }

    private static string CleanVisibleText(
        string? value,
        int maximumCharacters,
        string fallback)
    {
        var clean = SafeIpcText.Clean(
            value,
            maximumCharacters,
            fallback);
        if (string.IsNullOrEmpty(clean))
        {
            return fallback;
        }

        clean = SensitiveAssignmentPattern.Replace(clean, RedactedText);
        clean = FileUriPattern.Replace(clean, RedactedText);
        clean = WindowsPathPattern.Replace(clean, RedactedText);
        clean = UncPathPattern.Replace(clean, RedactedText);
        clean = RelativePathPattern.Replace(clean, RedactedText);
        clean = PosixPathPattern.Replace(clean, RedactedText);
        clean = Base64Pattern.Replace(clean, RedactedText);

        return SafeIpcText.Clean(clean, maximumCharacters, fallback);
    }
}
