// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Core.Diagnostics;

/// <summary>
/// Lote 39g: ajustes que no se pueden leer o guardar por permisos. El motor
/// manda un errorCode cerrado y el texto sale del catálogo de la interfaz, en
/// su idioma. Si lleva una orden para arreglarlo, va sola en la última línea y
/// se separa para enseñarla en un campo que se puede seleccionar y copiar.
/// No es una avería: no abre el diagnóstico, que taparía la orden con
/// «[redactado]» al tomarla por una ruta.
/// </summary>
public sealed record SettingsAccessFailure(string Explanation, string Command)
{
    public const string NoPermissionCode = "settings_no_permission";
    public const string OtherUserCode = "settings_other_user";

    public static SettingsAccessFailure? From(string? errorCode, Func<string, string> text)
    {
        ArgumentNullException.ThrowIfNull(text);
        var key = errorCode switch
        {
            // WinUI solo funciona en Windows: el remedio es el de icacls.
            NoPermissionCode => "error.settings_sin_permiso",
            OtherUserCode => "error.settings_otro_usuario",
            _ => null,
        };
        if (key is null)
        {
            return null;
        }

        var message = text(key);
        if (string.IsNullOrWhiteSpace(message) || message == key)
        {
            return null;
        }

        var commandBreak = message.LastIndexOf('\n');
        return commandBreak < 0
            ? new SettingsAccessFailure(message.Trim(), string.Empty)
            : new SettingsAccessFailure(
                message[..commandBreak].Trim(),
                message[(commandBreak + 1)..].Trim());
    }
}
