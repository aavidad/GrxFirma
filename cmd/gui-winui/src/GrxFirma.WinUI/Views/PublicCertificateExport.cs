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
    private const string ShareMessage =
        "Es su certificado público: puede enviarlo sin riesgo. Quien lo reciba podrá proteger archivos que solo usted podrá abrir con GrxFirma (Desproteger).";

    internal static async Task ExportAsync(
        DesktopOperationSession session,
        IFilePickerService picker,
        XamlRoot xamlRoot,
        string? certificateId,
        CancellationToken cancellationToken)
    {
        if (!session.TryGetOperations(DesktopOperationActions.CertificateExportPublic, out var operations))
        {
            await NoticeAsync(xamlRoot, "El motor local no permite exportar certificados públicos.");
            return;
        }
        if (string.IsNullOrWhiteSpace(certificateId))
        {
            var listed = await operations.GetCertificatesAsync(cancellationToken);
            if (!listed.IsSuccess || listed.Data is null || listed.Data.Count == 0)
            {
                await NoticeAsync(xamlRoot, "No hay certificados propios disponibles.");
                return;
            }
            var candidates = listed.Data.Where(c => c.CanSign || c.NeedsUnlock).ToArray();
            if (candidates.Length == 0)
            {
                await NoticeAsync(xamlRoot, "No hay certificados propios disponibles.");
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
                    Header = "Elija su certificado",
                    ItemsSource = candidates,
                    DisplayMemberPath = nameof(CertificateInfo.SubjectName),
                    SelectedIndex = 0,
                    MinWidth = 300,
                };
                var dialog = new ContentDialog
                {
                    XamlRoot = xamlRoot,
                    Title = "Compartir mi certificado",
                    Content = combo,
                    PrimaryButtonText = "Continuar",
                    CloseButtonText = "Cancelar",
                };
                if (await dialog.ShowAsync() != ContentDialogResult.Primary)
                    return;
                certificateId = (combo.SelectedItem as CertificateInfo)?.Id;
            }
        }
        if (string.IsNullOrWhiteSpace(certificateId)) return;
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
            await NoticeAsync(xamlRoot, "El motor no confirmó el archivo del certificado público.");
            return;
        }
        await NoticeAsync(xamlRoot, ShareMessage);
    }

    private static async Task NoticeAsync(XamlRoot xamlRoot, string message)
    {
        await new ContentDialog
        {
            XamlRoot = xamlRoot,
            Title = "Certificado público",
            Content = message,
            CloseButtonText = "Cerrar",
        }.ShowAsync();
    }
}
