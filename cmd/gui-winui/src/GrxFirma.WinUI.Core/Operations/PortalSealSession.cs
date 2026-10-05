// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

using System.Text.Json;

namespace GrxFirma.WinUI.Core.Operations;

public sealed class PortalSealSession
{
    private int _completed;
    private PortalSealSession(string documentPath, string resultPath, string signerName)
    {
        DocumentPath = documentPath;
        ResultPath = resultPath;
        SignerName = signerName;
    }

    public string DocumentPath { get; }
    public string ResultPath { get; }
    public string SignerName { get; }

    // La decisión quedó escrita en result.json (colocar, sin sello o cancelar).
    public bool IsCompleted => Volatile.Read(ref _completed) != 0;

    // Código de salida del proceso del editor: 0 solo si dejó una decisión.
    public int ExitCode => IsCompleted ? 0 : 1;

    public static bool TryLoad(string[] arguments, out PortalSealSession? session,
        out bool requested)
    {
        session = null;
        requested = arguments.Contains("--portal-seal-request", StringComparer.Ordinal);
        if (!requested) return true;
        if (arguments.Length != 2 || arguments[0] != "--portal-seal-request") return false;
        try
        {
            var request = Path.GetFullPath(arguments[1]);
            var requestInfo = new FileInfo(request);
            if (requestInfo.Name != "request.json" || !requestInfo.Exists ||
                requestInfo.Length is < 2 or > 4096 || IsReparsePoint(requestInfo)) return false;
            using var json = JsonDocument.Parse(File.ReadAllBytes(request));
            var root = json.RootElement;
            if (root.ValueKind != JsonValueKind.Object || root.EnumerateObject().Count() != 3 ||
                !root.TryGetProperty("documentPath", out var documentValue) ||
                !root.TryGetProperty("resultPath", out var resultValue) ||
                !root.TryGetProperty("signerName", out var signerValue) ||
                documentValue.ValueKind != JsonValueKind.String ||
                resultValue.ValueKind != JsonValueKind.String ||
                signerValue.ValueKind != JsonValueKind.String) return false;
            var document = Path.GetFullPath(documentValue.GetString()!);
            var result = Path.GetFullPath(resultValue.GetString()!);
            var dir = requestInfo.DirectoryName!;
            if (!string.Equals(Path.GetDirectoryName(document), dir, StringComparison.OrdinalIgnoreCase) ||
                !string.Equals(Path.GetDirectoryName(result), dir, StringComparison.OrdinalIgnoreCase) ||
                Path.GetFileName(document) != "document.pdf" ||
                Path.GetFileName(result) != "result.json" || File.Exists(result)) return false;
            var pdfInfo = new FileInfo(document);
            if (!pdfInfo.Exists || pdfInfo.Length is < 1 or > 104857600 ||
                IsReparsePoint(pdfInfo)) return false;
            var signer = signerValue.GetString()!;
            session = new PortalSealSession(document, result,
                signer.Length <= 256 ? signer : signer[..256]);
            return true;
        }
        catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or
            JsonException or ArgumentException or NotSupportedException)
        {
            return false;
        }
    }

    private static bool IsReparsePoint(FileInfo file) =>
        (file.Attributes & FileAttributes.ReparsePoint) != 0;

    public bool Submit(string action, IReadOnlyList<VisibleSealPlacementParameters>? placements = null)
    {
        if (action is not ("place" or "without" or "cancel") ||
            (action == "place" && placements is not { Count: > 0 and <= 128 }) ||
            (action != "place" && placements is not null)) return false;
        if (Interlocked.CompareExchange(ref _completed, 1, 0) != 0) return false;
        var created = false;
        try
        {
            var payload = action == "place"
                ? JsonSerializer.SerializeToUtf8Bytes(new { action, visibleSealPlacements = placements })
                : JsonSerializer.SerializeToUtf8Bytes(new { action });
            if (payload.Length > 32768)
            {
                Interlocked.Exchange(ref _completed, 0);
                return false;
            }
            using var file = new FileStream(ResultPath, FileMode.CreateNew,
                FileAccess.Write, FileShare.None);
            created = true;
            file.Write(payload);
            file.Flush(true);
            return true;
        }
        catch (IOException)
        {
            ResetFailedSubmission(created);
            return false;
        }
        catch (UnauthorizedAccessException)
        {
            ResetFailedSubmission(created);
            return false;
        }
    }

    private void ResetFailedSubmission(bool created)
    {
        if (created)
        {
            try { File.Delete(ResultPath); }
            catch (IOException) { }
            catch (UnauthorizedAccessException) { }
        }
        Interlocked.Exchange(ref _completed, 0);
    }

    public void CancelOnClose() => Submit("cancel");
}
