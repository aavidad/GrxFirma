// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.ViewModels;
using Microsoft.UI.Xaml.Data;

namespace GrxFirma.WinUI.Controls;

public sealed class CertificateStatusBrushConverter : IValueConverter
{
    public object Convert(
        object value,
        Type targetType,
        object parameter,
        string language)
    {
        var resourceKey = value is CertificateCardStatus status
            ? status switch
            {
                CertificateCardStatus.Valid => ThemeBrushes.Success,
                CertificateCardStatus.Invalid => ThemeBrushes.Failure,
                CertificateCardStatus.Unusable => ThemeBrushes.Failure,
                _ => ThemeBrushes.Unknown,
            }
            : ThemeBrushes.Unknown;
        // Tema elegido en la aplicación, no el de Windows.
        return ThemeBrushes.Get(resourceKey);
    }

    public object ConvertBack(
        object value,
        Type targetType,
        object parameter,
        string language) =>
        throw new NotSupportedException();
}
