// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using Windows.Foundation;
using Windows.System;

namespace GrxFirma.WinUI.Services;

public sealed class WindowsFacePortalLauncherService
    : IFacePortalLauncherService
{
    private const string PortalHost = "proveedores.face.gob.es";
    private const string ValidatorPath =
        "/proveedores/validar-factura";
    private const string OrganisationDirectoryPath =
        "/administraciones_y_organismos/buscador-de-organismos-y-relaciones";
    private const string SubmissionPath =
        "/proveedores/remitir-factura";
    private const string InvoiceStatusPath =
        "/proveedores/consultar-facturas";
    private const string ReceiptVerificationPath =
        "/proveedores/verificar-codigo-csv";

    public Task<HelpLaunchResult> OpenValidatorAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(ValidatorPath),
            "Se ha abierto el validador oficial de FACe.",
            cancellationToken);

    public Task<HelpLaunchResult> OpenOrganisationDirectoryAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(OrganisationDirectoryPath),
            "Se ha abierto el buscador oficial de organismos y relaciones DIR3.",
            cancellationToken);

    public Task<HelpLaunchResult> OpenSubmissionAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(SubmissionPath),
            "Se ha abierto el trámite oficial de remisión de facturas.",
            cancellationToken);

    public Task<HelpLaunchResult> OpenInvoiceStatusAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(InvoiceStatusPath),
            "Se ha abierto la consulta oficial de facturas.",
            cancellationToken);

    public Task<HelpLaunchResult> OpenReceiptVerificationAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(ReceiptVerificationPath),
            "Se ha abierto la verificación oficial del CSV del justificante.",
            cancellationToken);

    private static async Task<HelpLaunchResult> LaunchAsync(
        Uri trustedUri,
        string successMessage,
        CancellationToken cancellationToken)
    {
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            var operation = Launcher.LaunchUriAsync(trustedUri);
            using var registration = cancellationToken.Register(
                static state => ((IAsyncInfo)state!).Cancel(),
                operation);
            return await operation
                ? new(true, successMessage)
                : new(
                    false,
                    "Windows no tiene un navegador disponible para abrir FACe.");
        }
        catch (OperationCanceledException)
        {
            throw;
        }
        catch
        {
            return new(
                false,
                "El portal oficial de FACe no se pudo abrir. Compruebe la conexión y vuelva a intentarlo.");
        }
    }

    private static Uri TrustedPortalUri(string expectedPath)
    {
        var uri = new Uri(
            $"https://{PortalHost}{expectedPath}",
            UriKind.Absolute);
        if (!string.Equals(
                uri.Scheme,
                Uri.UriSchemeHttps,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.Host,
                PortalHost,
                StringComparison.OrdinalIgnoreCase) ||
            !string.Equals(
                uri.AbsolutePath.TrimEnd('/'),
                expectedPath,
                StringComparison.Ordinal) ||
            !string.IsNullOrEmpty(uri.Query) ||
            !string.IsNullOrEmpty(uri.Fragment))
        {
            throw new InvalidOperationException(
                "El destino oficial de FACe configurado no es válido.");
        }

        return uri;
    }
}
