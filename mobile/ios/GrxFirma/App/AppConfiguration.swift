// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import Foundation

struct AppConfiguration: Sendable {
    enum CoreMode: String, Sendable {
        case production
        case verification
    }

    static let maximumDocumentBytes: Int64 = 25 * 1_024 * 1_024
    static let maximumCertificateBytes: Int64 = 2 * 1_024 * 1_024
    static let maximumSignedBytes: Int64 = 40 * 1_024 * 1_024
    static let maximumJSONBytes = 56 * 1_024 * 1_024
    static let maximumPasswordBytes = 1_024
    static let maximumSharedDocuments = 1

    let coreMode: CoreMode
    let appGroup: String
    let keychainGroup: String
    let applicationSupportDirectory: URL
    let sharedContainerDirectory: URL

    static func load(
        bundle: Bundle = .main,
        fileManager: FileManager = .default
    ) throws -> AppConfiguration {
        guard
            let rawMode = bundle.object(forInfoDictionaryKey: "GrxFirmaCoreMode") as? String,
            let mode = CoreMode(rawValue: rawMode)
        else {
            throw AppError.configuration("El modo del núcleo no está configurado.")
        }
        guard let appGroup = nonPlaceholder(
            bundle.object(forInfoDictionaryKey: "GrxFirmaAppGroup") as? String,
            prefix: "group."
        ) else {
            throw AppError.configuration("El App Group de iOS no es válido.")
        }
        guard let keychainGroup = nonPlaceholder(
            bundle.object(forInfoDictionaryKey: "GrxFirmaKeychainGroup") as? String,
            prefix: nil
        ) else {
            throw AppError.configuration("El grupo de Keychain de iOS no es válido.")
        }
        let support = try fileManager.url(
            for: .applicationSupportDirectory,
            in: .userDomainMask,
            appropriateFor: nil,
            create: true
        ).appendingPathComponent("GrxFirma", isDirectory: true)

        let sharedContainer: URL
        if let signedContainer = fileManager.containerURL(forSecurityApplicationGroupIdentifier: appGroup) {
            sharedContainer = signedContainer
        } else if mode == .verification {
            sharedContainer = support.appendingPathComponent("VerificationSharedContainer", isDirectory: true)
        } else {
            throw AppError.configuration("No se puede abrir el contenedor compartido firmado.")
        }

        return AppConfiguration(
            coreMode: mode,
            appGroup: appGroup,
            keychainGroup: keychainGroup,
            applicationSupportDirectory: support,
            sharedContainerDirectory: sharedContainer
        )
    }

    private static func nonPlaceholder(_ value: String?, prefix: String?) -> String? {
        guard let value = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !value.isEmpty,
              !value.contains("$("),
              !value.contains("__"),
              value.count <= 255
        else {
            return nil
        }
        if let prefix, !value.hasPrefix(prefix) {
            return nil
        }
        let allowed = CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-")
        guard value.unicodeScalars.allSatisfy(allowed.contains) else {
            return nil
        }
        return value
    }
}
