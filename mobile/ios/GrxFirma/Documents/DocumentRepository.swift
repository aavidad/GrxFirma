// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import CryptoKit
import Foundation
import UniformTypeIdentifiers

actor DocumentRepository {
    private let fileManager: FileManager
    private let workingDirectory: URL
    private let outputDirectory: URL
    private let certificateDirectory: URL
    private let sharedInbox: URL

    init(configuration: AppConfiguration, fileManager: FileManager = .default) throws {
        self.fileManager = fileManager
        workingDirectory = configuration.applicationSupportDirectory
            .appendingPathComponent("Working", isDirectory: true)
        outputDirectory = configuration.applicationSupportDirectory
            .appendingPathComponent("Outputs", isDirectory: true)
        certificateDirectory = configuration.applicationSupportDirectory
            .appendingPathComponent("CertificateImports", isDirectory: true)
        sharedInbox = configuration.sharedContainerDirectory
            .appendingPathComponent("Inbox", isDirectory: true)
        try Self.prepareDirectory(workingDirectory, fileManager: fileManager)
        try Self.prepareDirectory(outputDirectory, fileManager: fileManager)
        try Self.prepareDirectory(certificateDirectory, fileManager: fileManager)
        try Self.prepareDirectory(sharedInbox, fileManager: fileManager)
    }

    func importDocument(from source: URL) throws -> SecureDocument {
        let accessed = source.startAccessingSecurityScopedResource()
        defer { if accessed { source.stopAccessingSecurityScopedResource() } }

        var coordinationError: NSError?
        var result: Result<SecureDocument, Error>?
        let coordinator = NSFileCoordinator()
        coordinator.coordinate(readingItemAt: source, options: [.withoutChanges], error: &coordinationError) {
            coordinatedURL in
            result = Result { try self.copyDocument(from: coordinatedURL, preferredName: source.lastPathComponent) }
        }
        if let coordinationError {
            throw AppError.invalidDocument(Self.sanitizedError(coordinationError))
        }
        guard let result else {
            throw AppError.invalidDocument("No se pudo coordinar el acceso al documento.")
        }
        return try result.get()
    }

    func importRemoteDocument(_ data: Data, name: String, mimeType: String) throws -> SecureDocument {
        guard !data.isEmpty, Int64(data.count) <= AppConfiguration.maximumDocumentBytes else {
            throw AppError.invalidDocument("El documento remoto está vacío o supera 25 MiB.")
        }
        return try store(data: data, preferredName: name, mimeType: mimeType, in: workingDirectory)
    }

    func stageCertificate(from source: URL) throws -> URL {
        guard DocumentPolicy.isCertificateFile(source) else {
            throw AppError.invalidDocument("El certificado debe tener extensión .p12 o .pfx.")
        }
        let accessed = source.startAccessingSecurityScopedResource()
        defer { if accessed { source.stopAccessingSecurityScopedResource() } }

        var coordinationError: NSError?
        var result: Result<URL, Error>?
        NSFileCoordinator().coordinate(readingItemAt: source, options: [.withoutChanges], error: &coordinationError) {
            coordinatedURL in
            result = Result {
                _ = try self.validatedValues(
                    for: coordinatedURL,
                    maximum: AppConfiguration.maximumCertificateBytes
                )
                let data = try Data(contentsOf: coordinatedURL, options: [.mappedIfSafe, .uncached])
                guard !data.isEmpty, Int64(data.count) <= AppConfiguration.maximumCertificateBytes else {
                    throw AppError.invalidDocument("El certificado cambió durante la importación.")
                }
                let ext = source.pathExtension.lowercased() == "pfx" ? "pfx" : "p12"
                let destination = self.certificateDirectory
                    .appendingPathComponent("\(UUID().uuidString).\(ext)", isDirectory: false)
                do {
                    try data.write(to: destination, options: [.atomic, .completeFileProtection])
                    try self.fileManager.setAttributes(
                        [.protectionKey: FileProtectionType.complete],
                        ofItemAtPath: destination.path
                    )
                    var values = URLResourceValues()
                    values.isExcludedFromBackup = true
                    var mutableDestination = destination
                    try mutableDestination.setResourceValues(values)
                    return destination
                } catch {
                    try? self.fileManager.removeItem(at: destination)
                    throw AppError.storage("No se pudo proteger el certificado en el contenedor de la aplicación.")
                }
            }
        }
        if let coordinationError {
            throw AppError.invalidDocument(Self.sanitizedError(coordinationError))
        }
        guard let result else {
            throw AppError.invalidDocument("No se pudo coordinar el acceso al certificado.")
        }
        return try result.get()
    }

    func removeStagedCertificate(_ url: URL?) {
        guard let url,
              url.deletingLastPathComponent().standardizedFileURL == certificateDirectory.standardizedFileURL
        else {
            return
        }
        try? fileManager.removeItem(at: url)
    }

    func readStagedCertificate(_ url: URL) throws -> Data {
        guard url.deletingLastPathComponent().standardizedFileURL == certificateDirectory.standardizedFileURL,
              DocumentPolicy.isCertificateFile(url)
        else {
            throw AppError.invalidDocument("El certificado no procede del staging protegido.")
        }
        _ = try validatedValues(for: url, maximum: AppConfiguration.maximumCertificateBytes)
        let data = try Data(contentsOf: url, options: [.mappedIfSafe, .uncached])
        guard !data.isEmpty, Int64(data.count) <= AppConfiguration.maximumCertificateBytes else {
            throw AppError.invalidDocument("El certificado cambió antes de importarlo.")
        }
        return data
    }

    func consumeSharedInbox() throws -> [SecureDocument] {
        let candidates = try fileManager.contentsOfDirectory(
            at: sharedInbox,
            includingPropertiesForKeys: [
                .isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey, .contentModificationDateKey
            ],
            options: [.skipsHiddenFiles, .skipsPackageDescendants]
        ).sorted {
            let left = (try? $0.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate)
                ?? .distantPast
            let right = (try? $1.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate)
                ?? .distantPast
            if left == right { return $0.lastPathComponent < $1.lastPathComponent }
            return left > right
        }

        var imported: [SecureDocument] = []
        for source in candidates {
            guard imported.count < AppConfiguration.maximumSharedDocuments else {
                try? fileManager.removeItem(at: source)
                continue
            }
            do {
                let document = try copyDocument(from: source, preferredName: source.lastPathComponent)
                try fileManager.removeItem(at: source)
                imported.append(document)
            } catch {
                try? fileManager.removeItem(at: source)
            }
        }
        return imported
    }

    func read(_ document: SecureDocument) throws -> Data {
        let values = try validatedValues(for: document.url, maximum: AppConfiguration.maximumDocumentBytes)
        guard let fileSize = values.fileSize, Int64(fileSize) == document.byteCount else {
            throw AppError.invalidDocument("El documento cambió desde su importación.")
        }
        let data = try Data(contentsOf: document.url, options: [.mappedIfSafe, .uncached])
        let currentHash = SHA256.hash(data: data).compactMap { String(format: "%02x", $0) }.joined()
        guard currentHash == document.sha256 else {
            throw AppError.invalidDocument("La integridad del documento no coincide.")
        }
        return data
    }

    func storeSigned(
        _ data: Data,
        sourceName: String,
        format: String,
        algorithm: String,
        certificateID: String
    ) throws -> SignedArtifact {
        guard !data.isEmpty, Int64(data.count) <= AppConfiguration.maximumSignedBytes else {
            throw AppError.storage("La firma está vacía o supera 40 MiB.")
        }
        let fileName = Self.outputName(sourceName: sourceName, format: format)
        let document = try store(data: data, preferredName: fileName, mimeType: "application/octet-stream", in: outputDirectory)
        return SignedArtifact(
            id: document.id,
            url: document.url,
            displayName: document.displayName,
            format: String(format.prefix(40)),
            algorithm: String(algorithm.prefix(80)),
            certificateID: String(certificateID.prefix(256)),
            byteCount: document.byteCount
        )
    }

    func read(_ artifact: SignedArtifact) throws -> Data {
        _ = try validatedValues(for: artifact.url, maximum: AppConfiguration.maximumSignedBytes)
        return try Data(contentsOf: artifact.url, options: [.mappedIfSafe, .uncached])
    }

    func remove(_ document: SecureDocument?) {
        guard let document else { return }
        try? fileManager.removeItem(at: document.url)
    }

    func remove(_ artifact: SignedArtifact?) {
        guard let artifact else { return }
        try? fileManager.removeItem(at: artifact.url)
    }

    func purgeExpiredFiles(now: Date = Date()) throws {
        let cutoff = now.addingTimeInterval(-24 * 60 * 60)
        for directory in [workingDirectory, outputDirectory, certificateDirectory, sharedInbox] {
            let files = try fileManager.contentsOfDirectory(
                at: directory,
                includingPropertiesForKeys: [.contentModificationDateKey, .isRegularFileKey],
                options: []
            )
            for file in files {
                let values = try file.resourceValues(forKeys: [.contentModificationDateKey, .isRegularFileKey])
                if values.isRegularFile == true, let date = values.contentModificationDate, date < cutoff {
                    try? fileManager.removeItem(at: file)
                }
            }
        }
    }

    private func copyDocument(from source: URL, preferredName: String) throws -> SecureDocument {
        _ = try validatedValues(for: source, maximum: AppConfiguration.maximumDocumentBytes)
        let data = try Data(contentsOf: source, options: [.mappedIfSafe, .uncached])
        guard !data.isEmpty, Int64(data.count) <= AppConfiguration.maximumDocumentBytes else {
            throw AppError.invalidDocument("El documento cambió de tamaño durante la importación.")
        }
        let type = (try? source.resourceValues(forKeys: [.contentTypeKey]).contentType)
        return try store(
            data: data,
            preferredName: preferredName,
            mimeType: type?.preferredMIMEType ?? "application/octet-stream",
            in: workingDirectory
        )
    }

    private func store(data: Data, preferredName: String, mimeType: String, in directory: URL) throws -> SecureDocument {
        let id = UUID()
        let safeName = Self.safeFileName(preferredName)
        let destination = directory.appendingPathComponent("\(id.uuidString)-\(safeName)", isDirectory: false)
        do {
            try data.write(to: destination, options: [.atomic, .completeFileProtection])
            try fileManager.setAttributes([.protectionKey: FileProtectionType.complete], ofItemAtPath: destination.path)
            var resource = URLResourceValues()
            resource.isExcludedFromBackup = true
            var mutableDestination = destination
            try mutableDestination.setResourceValues(resource)
        } catch {
            try? fileManager.removeItem(at: destination)
            throw AppError.storage("No se pudo proteger el documento en el contenedor de la aplicación.")
        }
        let hash = SHA256.hash(data: data).compactMap { String(format: "%02x", $0) }.joined()
        return SecureDocument(
            id: id,
            url: destination,
            displayName: safeName,
            mimeType: String(mimeType.prefix(128)),
            byteCount: Int64(data.count),
            sha256: hash
        )
    }

    private func validatedValues(for url: URL, maximum: Int64) throws -> URLResourceValues {
        let standardized = url.standardizedFileURL
        let values = try standardized.resourceValues(forKeys: [
            .isRegularFileKey, .isSymbolicLinkKey, .isDirectoryKey, .isPackageKey, .fileSizeKey
        ])
        guard values.isRegularFile == true,
              values.isSymbolicLink != true,
              values.isDirectory != true,
              values.isPackage != true,
              let size = values.fileSize,
              size > 0,
              Int64(size) <= maximum
        else {
            throw AppError.invalidDocument("Solo se admiten ficheros regulares no vacíos de hasta \(maximum / 1_024 / 1_024) MiB.")
        }
        return values
    }

    private static func prepareDirectory(_ url: URL, fileManager: FileManager) throws {
        try fileManager.createDirectory(
            at: url,
            withIntermediateDirectories: true,
            attributes: [.protectionKey: FileProtectionType.complete]
        )
        try fileManager.setAttributes(
            [.protectionKey: FileProtectionType.complete, .posixPermissions: 0o700],
            ofItemAtPath: url.path
        )
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        var mutableURL = url
        try mutableURL.setResourceValues(values)
    }

    private static func safeFileName(_ raw: String) -> String {
        let fallback = "documento.bin"
        let last = URL(fileURLWithPath: raw).lastPathComponent
        let allowed = CharacterSet.alphanumerics.union(CharacterSet(charactersIn: " ._()-"))
        let filtered = last.unicodeScalars.map { allowed.contains($0) ? Character(String($0)) : "_" }
        let compact = String(filtered).trimmingCharacters(in: .whitespacesAndNewlines)
        let bounded = String(compact.prefix(120))
        return bounded.isEmpty || bounded == "." || bounded == ".." ? fallback : bounded
    }

    private static func outputName(sourceName: String, format: String) -> String {
        let base = URL(fileURLWithPath: safeFileName(sourceName)).deletingPathExtension().lastPathComponent
        switch format.lowercased() {
        case "pades": return "\(base)-firmado.pdf"
        case "xades": return "\(base)-firmado.xml"
        case "cades": return "\(base)-firmado.p7s"
        default: return "\(base)-firmado.bin"
        }
    }

    private static func sanitizedError(_ error: Error) -> String {
        let filtered = error.localizedDescription.unicodeScalars.filter { !CharacterSet.controlCharacters.contains($0) }
        return String(filtered.map(String.init).joined().prefix(300))
    }
}
