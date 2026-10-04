// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Services;

/// <summary>
/// Metadatos no secretos que personalizan el diálogo nativo. El título y la
/// etiqueta nunca deben incluir la contraseña, la clave ni datos derivados.
/// </summary>
public sealed record SecurePasswordPromptRequest
{
    public const int MaximumSupportedCharacters = 16_384;

    public SecurePasswordPromptRequest(
        string title,
        string label,
        int maximumCharacters)
    {
        Title = ValidateDisplayText(
            title,
            nameof(title),
            maximumLength: 160);
        Label = ValidateDisplayText(
            label,
            nameof(label),
            maximumLength: 512);
        if (maximumCharacters is < 1 or > MaximumSupportedCharacters)
        {
            throw new ArgumentOutOfRangeException(
                nameof(maximumCharacters),
                Localizer.Fill("El límite debe estar entre 1 y {maximum} caracteres.",
                    ("maximum", MaximumSupportedCharacters.ToString())));
        }

        MaximumCharacters = maximumCharacters;
    }

    public string Title { get; }

    public string Label { get; }

    public int MaximumCharacters { get; }

    private static string ValidateDisplayText(
        string value,
        string parameterName,
        int maximumLength)
    {
        ArgumentNullException.ThrowIfNull(value, parameterName);
        if (string.IsNullOrWhiteSpace(value) ||
            value.Length > maximumLength ||
            value.IndexOf('\0') >= 0 ||
            value.IndexOfAny(['\r', '\n']) >= 0)
        {
            throw new ArgumentException(
                "El texto visible del diálogo no es válido.",
                parameterName);
        }

        return value;
    }
}

/// <summary>
/// Opera de forma síncrona sobre una vista prestada del buffer UTF-16 nativo.
/// El consumidor no debe conservar el puntero después de que el delegado
/// termine ni convertir el secreto en <see cref="string"/>.
/// </summary>
public delegate TResult NativePasswordConsumer<TResult>(
    nint utf16Password,
    int characterCount);

/// <summary>
/// Solicita una contraseña o clave sin materializar el secreto como texto
/// administrado.
/// </summary>
public interface ISecurePasswordPromptService
{
    /// <summary>
    /// Devuelve un buffer nativo propiedad del llamador o <see langword="null"/>
    /// cuando el usuario cierra el diálogo. El llamador debe liberar siempre el
    /// resultado. La cancelación solicitada mediante el token conserva
    /// <see cref="OperationCanceledException"/>.
    /// </summary>
    Task<NativePasswordBuffer?> CaptureAsync(
        SecurePasswordPromptRequest request,
        CancellationToken cancellationToken = default);
}
