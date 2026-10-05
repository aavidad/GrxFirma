// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using GrxFirma.WinUI.Core.Operations;
using Windows.Data.Pdf;
using Windows.Storage;
using Windows.Storage.Streams;

namespace GrxFirma.WinUI.Services;

public sealed class WindowsPdfPreviewService : IPdfPreviewService
{
    private const long MaximumInputBytes = 100L * 1024 * 1024;
    private const ulong MaximumPngBytes = 2_900_000;
    private const double PdfPointsPerDip = 72d / 96d;
    private const double MaximumRenderDimension = 1_200;
    // Un QR tributario mide de 30 a 40 mm: a 1 200 px por página A4 cada
    // módulo ocuparía menos de 3 px. Con 2 000 px se lee con holgura y el PNG
    // de una factura sigue por debajo del tope de la línea IPC.
    private const double CodeReadingRenderDimension = 2_000;

    public Task<PdfPreviewResult> RenderPageAsync(
        string path,
        int page,
        CancellationToken cancellationToken) =>
        RenderPageCoreAsync(path, page, MaximumRenderDimension, cancellationToken);

    public Task<PdfPreviewResult> RenderPageForCodeReadingAsync(
        string path,
        int page,
        CancellationToken cancellationToken) =>
        RenderPageCoreAsync(path, page, CodeReadingRenderDimension, cancellationToken);

    private async Task<PdfPreviewResult> RenderPageCoreAsync(
        string path,
        int page,
        double maximumRenderDimension,
        CancellationToken cancellationToken)
    {
        var normalizedPath = ValidateInput(path);
        if (page < 1)
        {
            throw new ArgumentOutOfRangeException(
                nameof(page),
                "La página PDF debe ser positiva.");
        }

        cancellationToken.ThrowIfCancellationRequested();
        var storageFile = await StorageFile.GetFileFromPathAsync(
            normalizedPath);
        cancellationToken.ThrowIfCancellationRequested();
        var document = await PdfDocument.LoadFromFileAsync(storageFile);
        if (document.PageCount is 0 or > 1_000_000)
        {
            throw new InvalidDataException(
                "Windows no devolvió un número de páginas PDF válido.");
        }
        if ((uint)page > document.PageCount)
        {
            throw new ArgumentOutOfRangeException(
                nameof(page),
                Localizer.Fill("El PDF solo contiene {count} páginas.",
                    ("count", document.PageCount.ToString())));
        }

        using var pdfPage = document.GetPage((uint)(page - 1));
        var dimensions = pdfPage.Dimensions;
        var mediaBox = dimensions.MediaBox;
        if (!IsPositiveFinite(mediaBox.Width) ||
            !IsPositiveFinite(mediaBox.Height))
        {
            throw new InvalidDataException(
                "Windows no devolvió dimensiones PDF válidas.");
        }

        var scale = Math.Min(
            1,
            maximumRenderDimension /
                Math.Max(mediaBox.Width, mediaBox.Height));
        var destinationWidth = checked(
            (uint)Math.Max(1, Math.Round(mediaBox.Width * scale)));
        var destinationHeight = checked(
            (uint)Math.Max(1, Math.Round(mediaBox.Height * scale)));
        using var output = new InMemoryRandomAccessStream();
        var renderOptions = new PdfPageRenderOptions
        {
            DestinationWidth = destinationWidth,
            DestinationHeight = destinationHeight,
        };
        await pdfPage.RenderToStreamAsync(output, renderOptions);
        cancellationToken.ThrowIfCancellationRequested();
        if (output.Size is < 8 or > MaximumPngBytes)
        {
            throw new InvalidDataException(
                "La previsualización PDF nativa excede el límite seguro.");
        }

        output.Seek(0);
        var length = checked((uint)output.Size);
        using var reader = new DataReader(output.GetInputStreamAt(0));
        var loaded = await reader.LoadAsync(length);
        cancellationToken.ThrowIfCancellationRequested();
        if (loaded != length)
        {
            throw new EndOfStreamException(
                "Windows no devolvió la previsualización PDF completa.");
        }
        var data = new byte[checked((int)length)];
        reader.ReadBytes(data);

        return new PdfPreviewResult
        {
            Data = data,
            Width = mediaBox.Width * PdfPointsPerDip,
            Height = mediaBox.Height * PdfPointsPerDip,
            CurrentPage = page,
            TotalPages = checked((int)document.PageCount),
        };
    }

    private static string ValidateInput(string path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            throw new ArgumentException(
                "La ruta PDF no puede quedar vacía.",
                nameof(path));
        }
        var normalizedPath = Path.GetFullPath(path);
        if (!Path.IsPathFullyQualified(normalizedPath) ||
            !string.Equals(
                Path.GetExtension(normalizedPath),
                ".pdf",
                StringComparison.OrdinalIgnoreCase))
        {
            throw new ArgumentException(
                "La previsualización nativa solo admite rutas PDF locales.",
                nameof(path));
        }
        var file = new FileInfo(normalizedPath);
        if (!file.Exists || file.Length is <= 0 or > MaximumInputBytes)
        {
            throw new IOException(
                "El PDF no existe, está vacío o supera el límite seguro.");
        }
        if (
            (file.Attributes & System.IO.FileAttributes.ReparsePoint) != 0
        )
        {
            throw new IOException(
                "La previsualización no admite enlaces o puntos de reanálisis.");
        }
        return normalizedPath;
    }

    private static bool IsPositiveFinite(double value) =>
        double.IsFinite(value) &&
        value > 0;
}
