// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

namespace GrxFirma.WinUI.Services;

public enum OpenFilePickerProfile
{
    SignableDocument,
    SignedOrOriginalDocument,
    Certificate,
    PublicRecipientCertificate,
    HashManifest,
    ProtectedContainer,
    SealImage,
    VeriFactuQrSource,
}

public enum SaveFilePickerProfile
{
    PublicCertificate,
    SignedPdf,
    CadesSignature,
    XadesSignature,
    XmlDsigSignature,
    FacturaeXml,
    AsicContainer,
    HashManifest,
    ProtectedContainer,
    ProtectedJson,
    CmsEnveloped,
    CmsAuthEnveloped,
    CmsSignedEnveloped,
    DiagnosticReport,
    VerificationReport,
    InvoiceReport,
    EniXml,
    SupportIncidentText,
    CmsEncrypted,
}

// La interfaz mantiene las páginas y ViewModels independientes de WinRT y
// permite sustituir el diálogo nativo en pruebas. Una cancelación del usuario
// devuelve null o una colección vacía; la cancelación solicitada mediante el
// token conserva OperationCanceledException.
public interface IFilePickerService
{
    Task<string?> PickOpenFileAsync(
        OpenFilePickerProfile profile,
        CancellationToken cancellationToken = default);

    Task<IReadOnlyList<string>> PickOpenFilesAsync(
        OpenFilePickerProfile profile,
        CancellationToken cancellationToken = default);

    Task<string?> PickSaveFileAsync(
        SaveFilePickerProfile profile,
        string? suggestedFileName = null,
        CancellationToken cancellationToken = default);

    Task<bool> PickAndSaveTextFileAsync(
        SaveFilePickerProfile profile,
        string contents,
        string? suggestedFileName = null,
        CancellationToken cancellationToken = default);

    Task<string?> PickFolderAsync(
        CancellationToken cancellationToken = default);
}
