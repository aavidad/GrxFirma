// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import CryptoKit
import Foundation
import Security

actor KeychainStore {
    private let service: String
    private let accessGroup: String?

    init(service: String, accessGroup: String?) {
        self.service = service
        self.accessGroup = accessGroup
    }

    func save(_ data: Data, account: String) throws {
        guard data.count <= 64 * 1_024 else {
            throw AppError.storage("El valor seguro supera el límite permitido.")
        }
        var query = baseQuery(account: account)
        let updateAttributes: [CFString: Any] = [
            kSecValueData: data,
            kSecAttrAccessible: kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        ]
        let updateStatus = SecItemUpdate(query as CFDictionary, updateAttributes as CFDictionary)
        if updateStatus == errSecSuccess { return }
        guard updateStatus == errSecItemNotFound else {
            throw keychainError(updateStatus)
        }
        for (key, value) in updateAttributes {
            query[key] = value
        }
        let addStatus = SecItemAdd(query as CFDictionary, nil)
        guard addStatus == errSecSuccess else {
            throw keychainError(addStatus)
        }
    }

    func load(account: String) throws -> Data? {
        var query = baseQuery(account: account)
        query[kSecReturnData] = kCFBooleanTrue
        query[kSecMatchLimit] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let data = result as? Data else {
            throw keychainError(status)
        }
        return data
    }

    func delete(account: String) throws {
        let status = SecItemDelete(baseQuery(account: account) as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw keychainError(status)
        }
    }

    private func baseQuery(account: String) -> [CFString: Any] {
        var query: [CFString: Any] = [
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account,
            kSecAttrSynchronizable: kCFBooleanFalse as Any
        ]
        if let accessGroup, !accessGroup.isEmpty {
            query[kSecAttrAccessGroup] = accessGroup
        }
        return query
    }

    private func keychainError(_ status: OSStatus) -> AppError {
        let message = SecCopyErrorMessageString(status, nil) as String? ?? "código \(status)"
        return .storage("Keychain no pudo completar la operación: \(message).")
    }
}

actor SecureStateStore {
    private struct ReplayEntry: Codable {
        let requestHash: String
        let acceptedAt: Date
    }

    private let keychain: KeychainStore
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()
    private let replayAccount = "accepted-deep-links-v1"
    private let certificateAccount = "selected-certificate-v1"
    private let replayLifetime: TimeInterval = 24 * 60 * 60
    private let maximumReplayEntries = 64

    init(keychain: KeychainStore) {
        self.keychain = keychain
    }

    func saveCertificate(_ certificate: CertificateSummary) async throws {
        try await keychain.save(encoder.encode(certificate), account: certificateAccount)
    }

    func loadCertificate() async throws -> CertificateSummary? {
        guard let data = try await keychain.load(account: certificateAccount) else { return nil }
        return try decoder.decode(CertificateSummary.self, from: data)
    }

    func clearCertificate() async throws {
        try await keychain.delete(account: certificateAccount)
    }

    func assertFreshAndRecord(requestID: String, now: Date = Date()) async throws {
        let requestHash = SHA256.hash(data: Data(requestID.utf8))
            .map { String(format: "%02x", $0) }
            .joined()
        let oldest = now.addingTimeInterval(-replayLifetime)
        var entries: [ReplayEntry] = []
        if let data = try await keychain.load(account: replayAccount) {
            do {
                entries = try decoder.decode([ReplayEntry].self, from: data)
            } catch {
                throw AppError.storage("El registro seguro de solicitudes no es válido.")
            }
        }
        let hexadecimal = CharacterSet(charactersIn: "0123456789abcdef")
        guard entries.allSatisfy({ entry in
            entry.requestHash.count == 64
                && entry.requestHash.unicodeScalars.allSatisfy(hexadecimal.contains)
                && entry.acceptedAt <= now.addingTimeInterval(5 * 60)
        }) else {
            throw AppError.storage("El registro seguro de solicitudes está dañado.")
        }
        entries = entries.filter { $0.acceptedAt >= oldest }
        guard !entries.contains(where: { $0.requestHash == requestHash }) else {
            throw AppError.invalidDeepLink("La solicitud ya fue aceptada anteriormente.")
        }
        entries.append(ReplayEntry(requestHash: requestHash, acceptedAt: now))
        if entries.count > maximumReplayEntries {
            entries.removeFirst(entries.count - maximumReplayEntries)
        }
        try await keychain.save(encoder.encode(entries), account: replayAccount)
    }
}
