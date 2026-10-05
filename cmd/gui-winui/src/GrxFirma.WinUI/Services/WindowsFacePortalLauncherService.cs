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
            Localizer.Text("winui.face.se_ha_abierto_el_validador_oficial_de"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenOrganisationDirectoryAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(OrganisationDirectoryPath),
            Localizer.Text("winui.face.se_ha_abierto_el_buscador_oficial_de"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenSubmissionAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(SubmissionPath),
            Localizer.Text("winui.face.se_ha_abierto_el_tramite_oficial_de"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenInvoiceStatusAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(InvoiceStatusPath),
            Localizer.Text("winui.face.se_ha_abierto_la_consulta_oficial_de"),
            cancellationToken);

    public Task<HelpLaunchResult> OpenReceiptVerificationAsync(
        CancellationToken cancellationToken = default) =>
        LaunchAsync(
            TrustedPortalUri(ReceiptVerificationPath),
            Localizer.Text("winui.face.se_ha_abierto_la_verificacion_oficial"),
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
                    Localizer.Text("winui.face.windows_no_tiene_un_navegador_disponible"));
        }
        catch (OperationCanceledException)
        {
            throw;
        }
        catch
        {
            return new(
                false,
                Localizer.Text("winui.face.el_portal_oficial_de_face_no_se_pudo"));
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
                Localizer.Text("winui.face.el_destino_oficial_de_face_configurado"));
        }

        return uri;
    }
}
