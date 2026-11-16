// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Globalization;
using System.Text.RegularExpressions;
using GrxFirma.WinUI.Core.Ipc;
using GrxFirma.WinUI.Core.Localization;

namespace GrxFirma.WinUI.Core.Diagnostics;

/// <summary>
/// Transforma resultados y fallos del canal local en el modelo que consume la
/// interfaz básica. El mapeo no inspecciona los datos funcionales de la
/// operación ni propaga mensajes de excepciones no controladas.
/// </summary>
public static class OperationDiagnosticMapper
{
    // Todos los textos visibles salen del catálogo de la interfaz, en el
    // idioma elegido. Si falta la clave, el catálogo devuelve la clave: la
    // redacción sigue tapando el dato aunque no haya traducción.
    private const string RedactedKey = "winui.diagnostico.contenido_protegido";

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

    // catalogText solo lo cambian las pruebas; la aplicación usa el catálogo
    // compartido.
    private static Func<string, string> Catalog(Func<string, string>? catalogText) =>
        catalogText ?? CatalogLocalizer.Shared.Text;

    public static OperationDiagnostic FromResult<TData>(
        IpcCallResult<TData> result,
        Func<string, string>? catalogText = null)
    {
        ArgumentNullException.ThrowIfNull(result);
        var text = Catalog(catalogText);

        var diagnostic = result.Diagnostic is not null
            ? SanitizeBackendDiagnostic(result.Diagnostic, text)
            : BuildFromResultMetadata(result, text);
        // Ajustes sin permiso: el texto sale del catálogo de la interfaz, no
        // del motor, y no pasa por el filtro de rutas, que cortaba la orden
        // icacls "%APPDATA%\GrxFirma" /grant … por la primera barra.
        var settingsAccess = result.IsSuccess
            ? null
            : SettingsAccessFailure.From(result.ErrorCode, text);

        return diagnostic with
        {
            RequestId = NormalizeOpaqueIdentifier(result.RequestId, 128),
            TraceId = NormalizeOpaqueIdentifier(result.TraceId, 128),
            Action = NormalizeOpaqueIdentifier(result.Action, 64),
            SettingsAccess = settingsAccess,
            UserMessage = settingsAccess?.Explanation ?? diagnostic.UserMessage,
        };
    }

    /// <summary>
    /// Reaplica los límites y la redacción del contrato seguro antes de
    /// persistir o presentar un diagnóstico recibido por otra vía.
    /// </summary>
    public static OperationDiagnostic Sanitize(
        OperationDiagnostic diagnostic,
        Func<string, string>? catalogText = null)
    {
        ArgumentNullException.ThrowIfNull(diagnostic);

        return SanitizeBackendDiagnostic(diagnostic, Catalog(catalogText)) with
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
        CancellationToken userCancellationToken = default,
        Func<string, string>? catalogText = null)
    {
        ArgumentNullException.ThrowIfNull(exception);
        var text = Catalog(catalogText);

        if (exception is IpcClientException ipcException)
        {
            return BuildFromIpcException(ipcException, text);
        }

        if (exception is OperationCanceledException &&
            userCancellationToken.IsCancellationRequested)
        {
            return FromUserCancellation(text);
        }

        return BuildUnexpectedFailure(text);
    }

    public static OperationDiagnostic FromUserCancellation(
        Func<string, string>? catalogText = null)
    {
        var text = Catalog(catalogText);
        return new()
        {
            Category = "unknown",
            FailureCode = "USER_CANCELLED",
            UserMessage = text("winui.diagnostico.cancelacion.mensaje"),
            ExpertMessage =
                text("winui.diagnostico.cancelacion.detalle"),
            LikelyOwner = "unknown",
            ResponsibilityMessage =
                text("winui.diagnostico.cancelacion.responsable"),
            SuggestedAction =
                text("winui.diagnostico.cancelacion.accion"),
            UserCanResolveDirectly = true,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = "USER_CANCELLED",
                    Label = text("winui.diagnostico.fase.operacion"),
                    Status = DiagnosticStepStatus.Skipped,
                    Owner = "unknown",
                    UserMessage =
                        text("winui.diagnostico.cancelacion.paso_mensaje"),
                    SuggestedAction =
                        text("winui.diagnostico.cancelacion.paso_accion"),
                    EvidenceRef = "phase:operation",
                },
            ],
        };
    }

    private static OperationDiagnostic SanitizeBackendDiagnostic(
        OperationDiagnostic diagnostic,
        Func<string, string> text)
    {
        var steps = (diagnostic.Steps ?? [])
            .Take(64)
            .Select(step => new OperationDiagnosticStep
            {
                Code = NormalizeTechnicalCode(step.Code) ?? "unknown",
                Label = CleanVisibleText(
                    step.Label,
                    160,
                    text("winui.diagnostico.paso_observado"), text),
                Status = step.Status,
                Owner = NormalizeOwner(step.Owner),
                UserMessage = CleanOptionalVisibleText(
                    step.UserMessage,
                    512, text),
                SuggestedAction = CleanOptionalVisibleText(
                    step.SuggestedAction,
                    512, text),
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
                text("winui.diagnostico.resultado.fallido"), text),
            ExpertMessage = CleanVisibleText(
                diagnostic.ExpertMessage,
                4096,
                text("winui.diagnostico.respaldo.detalle"), text),
            LikelyOwner = NormalizeOwner(diagnostic.LikelyOwner),
            ResponsibilityMessage = CleanVisibleText(
                diagnostic.ResponsibilityMessage,
                512,
                text("winui.diagnostico.respaldo.responsable"), text),
            SuggestedAction = CleanVisibleText(
                diagnostic.SuggestedAction,
                512,
                text("winui.diagnostico.respaldo.accion"), text),
            UserCanResolveDirectly = diagnostic.UserCanResolveDirectly,
            Steps = steps,
        };
    }

    private static OperationDiagnostic BuildFromResultMetadata<TData>(
        IpcCallResult<TData> result,
        Func<string, string> text)
    {
        var phase = NormalizePhase(result.Phase);
        var outcome = NormalizeOutcome(result.IsSuccess, result.Outcome);
        var successful = outcome == "success";
        var partial = outcome == "partial";
        var cause = successful
            ? text("winui.diagnostico.resultado.correcto")
            : partial
                ? text("winui.diagnostico.resultado.parcial")
                : text("winui.diagnostico.resultado.fallido");
        var action = successful
            ? text("winui.diagnostico.resultado.accion_correcto")
            : partial
                ? text("winui.diagnostico.resultado.accion_parcial")
                : text("winui.diagnostico.resultado.accion_fallido");

        return new OperationDiagnostic
        {
            Category = "unknown",
            FailureCode = successful
                ? null
                : NormalizeTechnicalCode(result.ErrorCode),
            UserMessage = cause,
            ExpertMessage = BuildMetadataExpertMessage(outcome, phase, text),
            LikelyOwner = "unknown",
            ResponsibilityMessage = successful
                ? text("winui.diagnostico.resultado.responsable_correcto")
                : text("winui.diagnostico.resultado.responsable_desconocido"),
            SuggestedAction = action,
            UserCanResolveDirectly = false,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = $"{phase ?? "unknown"}_result",
                    Label = PhaseLabel(phase, text),
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
        IpcClientException exception,
        Func<string, string> text)
    {
        var phase = NormalizePhase(exception.Phase);
        var owner = NormalizeOwner(exception.LikelyOwner);
        var action = SuggestedActionForPhase(phase, exception.Retryable, text);
        var userMessage = CleanVisibleText(
            string.IsNullOrEmpty(exception.UserMessageKey)
                ? exception.UserMessage
                : text(exception.UserMessageKey),
            512,
            text("winui.diagnostico.canal.mensaje"), text);

        return new OperationDiagnostic
        {
            Category = owner,
            // El motivo concreto (p. ej. SUCCESS_WITH_UNSAFE_EXCEPTION_BLOCKED)
            // dice a soporte qué forma llegó; si no hay, el código general.
            FailureCode = NormalizeTechnicalCode(exception.Reason) ??
                NormalizeTechnicalCode(exception.Code),
            UserMessage = userMessage,
            ExpertMessage = phase switch
            {
                "admission" =>
                    text("winui.diagnostico.canal.detalle_admision"),
                "protocol" when exception.Code == "connection_closed" =>
                    text("winui.diagnostico.canal.detalle_sin_respuesta"),
                "protocol" =>
                    text("winui.diagnostico.canal.detalle_protocolo"),
                _ =>
                    text("winui.diagnostico.canal.detalle_operacion"),
            },
            LikelyOwner = owner,
            ResponsibilityMessage = ResponsibilityForOwner(owner, text),
            SuggestedAction = action,
            UserCanResolveDirectly =
                owner is "environment" or "computer",
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = $"{phase ?? "unknown"}_ipc",
                    Label = PhaseLabel(phase, text),
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

    private static OperationDiagnostic BuildUnexpectedFailure(
        Func<string, string> text) =>
        new()
        {
            Category = "unknown",
            FailureCode = "UNEXPECTED_FAILURE",
            UserMessage =
                text("winui.diagnostico.inesperado.mensaje"),
            ExpertMessage =
                text("winui.diagnostico.inesperado.detalle"),
            LikelyOwner = "unknown",
            ResponsibilityMessage =
                text("winui.diagnostico.inesperado.responsable"),
            SuggestedAction =
                text("winui.diagnostico.inesperado.accion"),
            UserCanResolveDirectly = false,
            Steps =
            [
                new OperationDiagnosticStep
                {
                    Code = "UNEXPECTED_OPERATION",
                    Label = text("winui.diagnostico.fase.operacion"),
                    Status = DiagnosticStepStatus.Failure,
                    Owner = "unknown",
                    UserMessage =
                        text("winui.diagnostico.inesperado.paso_mensaje"),
                    SuggestedAction =
                        text("winui.diagnostico.inesperado.paso_accion"),
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

    private static string PhaseLabel(string? phase, Func<string, string> text) => phase switch
    {
        "admission" => text("winui.diagnostico.fase.admision"),
        "protocol" => text("winui.diagnostico.fase.protocolo"),
        "operation" => text("winui.diagnostico.fase.operacion"),
        _ => text("winui.diagnostico.fase.desconocida"),
    };

    // Dos frases completas, una para el resultado y otra para la fase, en
    // lugar de una plantilla con adjetivos sueltos que no concuerdan en
    // todos los idiomas.
    private static string BuildMetadataExpertMessage(
        string outcome,
        string? phase,
        Func<string, string> text)
    {
        var result = text(outcome switch
        {
            "success" => "winui.diagnostico.motor.resultado_correcto",
            "partial" => "winui.diagnostico.motor.resultado_parcial",
            _ => "winui.diagnostico.motor.resultado_fallido",
        });
        var reportedPhase = text(phase switch
        {
            "admission" => "winui.diagnostico.motor.fase_admision",
            "protocol" => "winui.diagnostico.motor.fase_protocolo",
            "operation" => "winui.diagnostico.motor.fase_operacion",
            _ => "winui.diagnostico.motor.sin_fase",
        });
        return string.Format(
            CultureInfo.InvariantCulture,
            text("winui.diagnostico.motor.dos_frases"),
            result,
            reportedPhase);
    }

    private static string SuggestedActionForPhase(
        string? phase,
        bool retryable,
        Func<string, string> text) =>
        phase switch
        {
            "protocol" =>
                text("winui.diagnostico.accion.protocolo"),
            "admission" when retryable =>
                text("winui.diagnostico.accion.admision_reintentable"),
            "admission" =>
                text("winui.diagnostico.accion.admision"),
            _ when retryable =>
                text("winui.diagnostico.accion.reintentable"),
            _ =>
                text("winui.diagnostico.accion.general"),
        };

    private static string ResponsibilityForOwner(string owner, Func<string, string> text) => owner switch
    {
        "app_local" =>
            text("winui.diagnostico.responsable.app_local"),
        "environment" or "computer" =>
            text("winui.diagnostico.responsable.entorno"),
        "browser" or "browser_integration" or "local_web_service" =>
            text("winui.diagnostico.responsable.navegador"),
        "remote_service" or "portal" or "website" =>
            text("winui.diagnostico.responsable.remoto"),
        "government_afirma" or "afirma" or "@firma" =>
            text("winui.diagnostico.responsable.afirma"),
        _ =>
            text("winui.diagnostico.responsable.desconocido"),
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
        int maximumCharacters,
        Func<string, string> text)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return null;
        }

        return CleanVisibleText(value, maximumCharacters, string.Empty, text);
    }

    private static string CleanVisibleText(
        string? value,
        int maximumCharacters,
        string fallback,
        Func<string, string> text)
    {
        var clean = SafeIpcText.Clean(
            value,
            maximumCharacters,
            fallback);
        if (string.IsNullOrEmpty(clean))
        {
            return fallback;
        }

        // El texto de sustitución va por evaluador: así un «$» de una
        // traducción no se interpreta como referencia a un grupo.
        var redacted = text(RedactedKey);
        string Redact(Match _) => redacted;
        clean = SensitiveAssignmentPattern.Replace(clean, Redact);
        clean = FileUriPattern.Replace(clean, Redact);
        clean = WindowsPathPattern.Replace(clean, Redact);
        clean = UncPathPattern.Replace(clean, Redact);
        clean = RelativePathPattern.Replace(clean, Redact);
        clean = PosixPathPattern.Replace(clean, Redact);
        clean = Base64Pattern.Replace(clean, Redact);

        return SafeIpcText.Clean(clean, maximumCharacters, fallback);
    }
}
