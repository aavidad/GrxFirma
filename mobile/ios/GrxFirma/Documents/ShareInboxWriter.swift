// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import Foundation

enum ShareInboxWriter {
    static func copy(
        source: URL,
        appGroup: String,
        fileManager: FileManager = .default
    ) throws -> URL {
        guard let container = fileManager.containerURL(forSecurityApplicationGroupIdentifier: appGroup) else {
            throw AppError.configuration("La extensión no puede abrir el App Group firmado.")
        }
        let values = try source.resourceValues(forKeys: [
            .isRegularFileKey, .isSymbolicLinkKey, .isDirectoryKey, .fileSizeKey
        ])
        guard values.isRegularFile == true,
              values.isSymbolicLink != true,
              values.isDirectory != true,
              let size = values.fileSize,
              size > 0,
              Int64(size) <= AppConfiguration.maximumDocumentBytes
        else {
            throw AppError.invalidDocument("El elemento compartido no es un documento admitido.")
        }

        let inbox = container.appendingPathComponent("Inbox", isDirectory: true)
        try fileManager.createDirectory(
            at: inbox,
            withIntermediateDirectories: true,
            attributes: [.protectionKey: FileProtectionType.complete]
        )
        try fileManager.setAttributes([.protectionKey: FileProtectionType.complete], ofItemAtPath: inbox.path)
        var inboxValues = URLResourceValues()
        inboxValues.isExcludedFromBackup = true
        var mutableInbox = inbox
        try mutableInbox.setResourceValues(inboxValues)

        let safeExtension = source.pathExtension.lowercased().filter { $0.isLetter || $0.isNumber }
        let suffix = safeExtension.isEmpty ? "" : ".\(String(safeExtension.prefix(12)))"
        let pending = inbox.appendingPathComponent(".\(UUID().uuidString).pending", isDirectory: false)
        let destination = inbox.appendingPathComponent("\(UUID().uuidString)\(suffix)", isDirectory: false)
        do {
            try fileManager.copyItem(at: source, to: pending)
            let copied = try pending.resourceValues(forKeys: [.isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey])
            guard copied.isRegularFile == true,
                  copied.isSymbolicLink != true,
                  let copiedSize = copied.fileSize,
                  copiedSize == size,
                  Int64(copiedSize) <= AppConfiguration.maximumDocumentBytes
            else {
                throw AppError.invalidDocument("El documento cambió durante la copia compartida.")
            }
            try fileManager.setAttributes([.protectionKey: FileProtectionType.complete], ofItemAtPath: pending.path)
            try fileManager.moveItem(at: pending, to: destination)
            var resource = URLResourceValues()
            resource.isExcludedFromBackup = true
            var mutableDestination = destination
            try mutableDestination.setResourceValues(resource)
            return destination
        } catch {
            try? fileManager.removeItem(at: pending)
            try? fileManager.removeItem(at: destination)
            throw AppError.storage("No se pudo copiar el documento al contenedor protegido.")
        }
    }
}
