// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import Foundation

struct SecureDocument: Identifiable, Equatable, Sendable {
    let id: UUID
    let url: URL
    let displayName: String
    let mimeType: String
    let byteCount: Int64
    let sha256: String
}

struct SignedArtifact: Identifiable, Equatable, Sendable {
    let id: UUID
    let url: URL
    let displayName: String
    let format: String
    let algorithm: String
    let certificateID: String
    let byteCount: Int64
}

struct CertificateSummary: Codable, Equatable, Sendable {
    let certificateID: String
    let subject: String
    let issuer: String
    let fingerprint: String?
    let confirmed: Bool

    enum CodingKeys: String, CodingKey {
        case certificateID = "certificate_id"
        case subject
        case issuer
        case fingerprint
        case confirmed
    }
}

struct VerificationSigner: Codable, Equatable, Sendable, Identifiable {
    let id: String
    let subject: String
    let issuer: String
    let fingerprint: String
}

struct VerificationSummary: Equatable, Sendable {
    let valid: Bool
    let reason: String
    let details: [String]
    let format: String
    let coverage: String
    let integrityStatus: String
    let certificateStatus: String
    let trustStatus: String
    let signers: [VerificationSigner]
    let warnings: [String]
    let errors: [String]
}

struct CoreReadiness: Equatable, Sendable {
    let available: Bool
    let code: String
    let detail: String

    static let checking = CoreReadiness(
        available: false,
        code: "checking",
        detail: "Comprobando el núcleo criptográfico."
    )
}

struct CoreCapabilities: Equatable, Sendable {
    let remoteExchange: Bool
    let persistentIdentity: Bool

    static let unavailable = CoreCapabilities(remoteExchange: false, persistentIdentity: false)
}

enum SignatureAction: String, CaseIterable, Identifiable, Sendable {
    case sign
    case cosign
    case countersign

    var id: String { rawValue }

    var title: String {
        switch self {
        case .sign: return "Firmar"
        case .cosign: return "Cofirmar"
        case .countersign: return "Contrafirmar"
        }
    }
}

struct SignApproval: Identifiable, Equatable, Sendable {
    let nonce: UUID
    let documentID: UUID
    let certificateID: String
    let action: SignatureAction
    let format: String
    let algorithm: String?
    let expiresAt: Date

    var id: UUID { nonce }

    func isValid(at date: Date = Date()) -> Bool {
        expiresAt > date
    }
}

struct PendingRemoteSession: Equatable, Sendable {
    let requestID: String
    let sessionKey: String
    let retrieveEndpoint: URL
    let uploadEndpoint: URL
    let state: String
}

enum AppError: LocalizedError, Equatable, Sendable {
    case configuration(String)
    case coreUnavailable(String)
    case invalidDocument(String)
    case invalidDeepLink(String)
    case operationRejected(String)
    case operationFailed(String)
    case storage(String)

    var errorDescription: String? {
        switch self {
        case .configuration(let message),
             .coreUnavailable(let message),
             .invalidDocument(let message),
             .invalidDeepLink(let message),
             .operationRejected(let message),
             .operationFailed(let message),
             .storage(let message):
            return message
        }
    }
}

enum OperationState: Equatable, Sendable {
    case idle
    case loading(String)
    case completed(String)

    var isBusy: Bool {
        if case .loading = self { return true }
        return false
    }
}
