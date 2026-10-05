// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.IO;
using GrxFirma.WinUI.Core.Operations;
using GrxFirma.WinUI.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace GrxFirma.WinUI.Views;

internal static class PublicCertificateExport
{
    private static string ShareMessage =>
        Localizer.Text("winui.certificados.es_su_certificado_publico_puede_enviarlo");

    internal static async Task ExportAsync(
        DesktopOperationSession session,
        IFilePickerService picker,
        XamlRoot xamlRoot,
        string? certificateId,
        CancellationToken cancellationToken)
    {
        if (!session.TryGetOperations(DesktopOperationActions.CertificateExportPublic, out var operations))
        {
            await NoticeAsync(xamlRoot, Localizer.Text("winui.certificados.el_motor_local_no_permite_exportar"));
            return;
        }
        if (string.IsNullOrWhiteSpace(certificateId))
        {
            var listed = await operations.GetCertificatesAsync(cancellationToken);
            if (!listed.IsSuccess || listed.Data is null || listed.Data.Count == 0)
            {
                await NoticeAsync(xamlRoot, Localizer.Text("winui.certificados.no_hay_certificados_propios_disponibles"));
                return;
            }
            var candidates = listed.Data.Where(c => c.CanSign || c.NeedsUnlock).ToArray();
            if (candidates.Length == 0)
            {
                await NoticeAsync(xamlRoot, Localizer.Text("winui.certificados.no_hay_certificados_propios_disponibles"));
                return;
            }
            if (candidates.Length == 1)
            {
                certificateId = candidates[0].Id;
            }
            else
            {
                var combo = new ComboBox
                {
                    Header = Localizer.Text("winui.certificados.elija_su_certificado"),
                    ItemsSource = candidates,
                    DisplayMemberPath = nameof(CertificateInfo.SubjectName),
                    SelectedIndex = 0,
                    MinWidth = 300,
                };
                var dialog = new ContentDialog
                {
                    XamlRoot = xamlRoot,
                    Title = Localizer.Text("winui.certificados.compartir_mi_certificado"),
                    Content = combo,
                    PrimaryButtonText = Localizer.Text("winui.certificados.continuar"),
                    CloseButtonText = Localizer.Text("winui.comun.cancelar"),
                };
                if (await Localizer.ShowAsync(dialog) != ContentDialogResult.Primary)
                    return;
                certificateId = (combo.SelectedItem as CertificateInfo)?.Id;
            }
        }
        if (string.IsNullOrWhiteSpace(certificateId)) return;
        // Antes de preguntar dónde guardar se comprueba que el certificado
        // sirve para recibir documentos protegidos (sin ruta no se escribe nada).
        var check = await operations.ExportPublicCertificateAsync(new()
        {
            CertificateId = certificateId,
            OutputPath = string.Empty,
            Format = "der",
        }, cancellationToken);
        if (!check.IsSuccess || check.Data is null || !check.Data.EncryptionSuitable)
        {
            await NoticeAsync(xamlRoot, check.IsSuccess
                ? Localizer.Text("winui.certificados.el_motor_no_confirmo_el_archivo_del")
                : check.SafeUserMessage);
            return;
        }
        var path = await picker.PickSaveFileAsync(
            SaveFilePickerProfile.PublicCertificate, "certificado-publico.cer", cancellationToken);
        if (string.IsNullOrWhiteSpace(path)) return;
        var format = string.Equals(Path.GetExtension(path), ".pem", StringComparison.OrdinalIgnoreCase)
            ? "pem" : "der";
        var result = await operations.ExportPublicCertificateAsync(new()
        {
            CertificateId = certificateId,
            OutputPath = path,
            Format = format,
            // Ruta elegida en el selector de guardar, que ya preguntó antes
            // de reemplazar un fichero existente.
            OverwriteConfirmed = true,
        }, cancellationToken);
        if (!result.IsSuccess)
        {
            await NoticeAsync(xamlRoot, result.SafeUserMessage);
            return;
        }
        if (result.Data is null || !result.Data.EncryptionSuitable ||
            string.IsNullOrWhiteSpace(result.Data.OutputPath) ||
            string.IsNullOrWhiteSpace(result.Data.CertificateDerBase64))
        {
            await NoticeAsync(xamlRoot, Localizer.Text("winui.certificados.el_motor_no_confirmo_el_archivo_del"));
            return;
        }
        await NoticeAsync(xamlRoot, ShareMessage);
    }

    private static async Task NoticeAsync(XamlRoot xamlRoot, string message)
    {
        var dialog = new ContentDialog
        {
            XamlRoot = xamlRoot,
            Title = Localizer.Text("winui.certificados.certificado_publico"),
            Content = message,
            CloseButtonText = Localizer.Text("winui.comun.cerrar"),
        };
        await Localizer.ShowAsync(dialog);
    }
}
