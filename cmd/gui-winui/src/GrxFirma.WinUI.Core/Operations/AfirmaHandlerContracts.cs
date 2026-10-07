// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json.Serialization;
using GrxFirma.WinUI.Core.Ipc;

namespace GrxFirma.WinUI.Core.Operations;

/// <summary>
/// Programas que pueden abrir las firmas que piden los portales (protocolo
/// afirma://). El motor lee y cambia el registro; la interfaz solo elige.
/// </summary>
public static class AfirmaHandlerChoices
{
    public const string GrxFirma = "grxfirma";
    public const string AutoFirma = "autofirma";
    public const string Other = "other";
    public const string None = "none";

    public static bool IsSelectable(string? handler) =>
        handler is GrxFirma or AutoFirma;
}

/// <summary>Estado real leído del registro por el motor.</summary>
public sealed class AfirmaHandlerStatus
{
    [JsonPropertyName("supported")] public bool Supported { get; init; }
    [JsonPropertyName("current")] public string Current { get; init; } = AfirmaHandlerChoices.None;
    [JsonPropertyName("currentPath")] public string CurrentPath { get; init; } = "";
    [JsonPropertyName("grxfirmaInstalled")] public bool GrxFirmaInstalled { get; init; }
    [JsonPropertyName("autofirmaInstalled")] public bool AutoFirmaInstalled { get; init; }
    [JsonPropertyName("autofirmaPath")] public string AutoFirmaPath { get; init; } = "";
    [JsonPropertyName("preference")] public string Preference { get; init; } = "";
}

public sealed record AfirmaHandlerStatusParameters;

public sealed record AfirmaHandlerSelectParameters(
    [property: JsonPropertyName("handler")] string Handler);

/// <summary>Lo que la pantalla de Configuración debe mostrar.</summary>
public sealed record AfirmaHandlerView(
    bool Visible,
    bool GrxFirmaSelected,
    bool AutoFirmaSelected,
    bool CanChooseGrxFirma,
    bool CanChooseAutoFirma,
    string StatusKey,
    string? StatusArgument,
    bool ShowAutoFirmaMissing);

/// <summary>
/// Traduce el estado del motor a claves de catálogo; no contiene textos. Las
/// claves son las mismas que usa la interfaz Qt.
/// </summary>
public static class AfirmaHandlerPresentation
{
    public const string StatusChecking = "protocolo.estado.comprobando";
    public const string StatusChanging = "protocolo.estado.cambiando";
    public const string StatusError = "protocolo.estado.error";
    public const string AutoFirmaMissing = "protocolo.autofirma_no_instalada";
    public const string ReloadPortal = "protocolo.recargar_portal";

    public static AfirmaHandlerView From(AfirmaHandlerStatus? status)
    {
        if (status is null || !status.Supported)
        {
            return new AfirmaHandlerView(false, false, false, false, false, StatusError, null, false);
        }

        var current = status.Current ?? AfirmaHandlerChoices.None;
        var (key, argument) = current switch
        {
            AfirmaHandlerChoices.GrxFirma => ("protocolo.estado.grxfirma", (string?)null),
            AfirmaHandlerChoices.AutoFirma => ("protocolo.estado.autofirma", null),
            AfirmaHandlerChoices.Other => ("protocolo.estado.otro", SafeIpcText.Clean(status.CurrentPath, 260, "")),
            _ => ("protocolo.estado.ninguno", null),
        };
        return new AfirmaHandlerView(
            Visible: true,
            GrxFirmaSelected: current == AfirmaHandlerChoices.GrxFirma,
            AutoFirmaSelected: current == AfirmaHandlerChoices.AutoFirma,
            CanChooseGrxFirma: status.GrxFirmaInstalled,
            CanChooseAutoFirma: status.AutoFirmaInstalled,
            StatusKey: key,
            StatusArgument: argument,
            ShowAutoFirmaMissing: !status.AutoFirmaInstalled);
    }

    /// <summary>Mensaje claro para cada código de error estable del motor.</summary>
    public static string ErrorKey(string? errorCode) => errorCode switch
    {
        "afirma_handler_foreign" => "protocolo.error.ajeno",
        "autofirma_not_installed" => AutoFirmaMissing,
        "grxfirma_afirma_missing" => "protocolo.error.grxfirma_no_instalada",
        "afirma_handler_unsupported" => "protocolo.error.no_disponible",
        _ => "protocolo.error.generico",
    };
}
