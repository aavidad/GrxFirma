// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import Foundation

protocol CoreBridging: Sendable {
    func readiness() async -> CoreReadiness
    func capabilities() async -> CoreCapabilities
    func clearSession() async
    func selectCertificate(payload: String) async throws -> String
    func importCertificate(payload: String) async throws -> String
    func sign(payload: String) async throws -> String
    func verify(payload: String) async throws -> String
    func retrieve(payload: String) async throws -> String
    func upload(payload: String) async throws -> String
}

actor UnavailableCoreBridge: CoreBridging {
    private let reason: String

    init(reason: String) {
        self.reason = reason
    }

    func readiness() -> CoreReadiness {
        CoreReadiness(available: false, code: "configuration_failed", detail: reason)
    }

    func capabilities() -> CoreCapabilities { .unavailable }
    func clearSession() {}

    func selectCertificate(payload: String) throws -> String { try unavailable(payload) }
    func importCertificate(payload: String) throws -> String { try unavailable(payload) }
    func sign(payload: String) throws -> String { try unavailable(payload) }
    func verify(payload: String) throws -> String { try unavailable(payload) }
    func retrieve(payload: String) throws -> String { try unavailable(payload) }
    func upload(payload: String) throws -> String { try unavailable(payload) }

    private func unavailable(_ payload: String) throws -> String {
        _ = payload
        throw AppError.coreUnavailable(reason)
    }
}

actor GomobileCoreBridge: CoreBridging {
    private let adapter: AFV2GomobileAdapter
    private let currentReadiness: CoreReadiness
    private let currentCapabilities: CoreCapabilities

    init(configuration: AppConfiguration) {
        adapter = AFV2GomobileAdapter(
            applicationSupportDirectory: configuration.applicationSupportDirectory.path,
            appGroupDirectory: configuration.sharedContainerDirectory.path,
            keychainAccessGroup: configuration.keychainGroup
        )
        if adapter.isAvailable {
            let contract = adapter.mobileContractJSON()
            if contract.isSuccess,
               let value = contract.value,
               let status = CoreContractValidator.status(value),
               status.valid {
                currentCapabilities = status.capabilities
                currentReadiness = CoreReadiness(
                    available: true,
                    code: "ready",
                    detail: "Núcleo criptográfico iOS disponible."
                )
            } else {
                currentCapabilities = .unavailable
                currentReadiness = CoreReadiness(
                    available: false,
                    code: contract.errorCode ?? "invalid_contract",
                    detail: contract.errorMessage ?? "El núcleo no cumple el contrato iOS v1."
                )
            }
        } else {
            currentCapabilities = .unavailable
            currentReadiness = CoreReadiness(
                available: false,
                code: adapter.readinessCode,
                detail: adapter.readinessDetail
            )
        }
    }

    func readiness() -> CoreReadiness { currentReadiness }
    func capabilities() -> CoreCapabilities { currentCapabilities }
    func clearSession() { adapter.clearSession() }

    func selectCertificate(payload: String) throws -> String {
        try result(adapter.selectCertificateJSON(payload))
    }

    func importCertificate(payload: String) throws -> String {
        try result(adapter.importCertificateJSON(payload))
    }

    func sign(payload: String) throws -> String {
        try result(adapter.signJSON(payload))
    }

    func verify(payload: String) throws -> String {
        try result(adapter.verifyJSON(payload))
    }

    func retrieve(payload: String) throws -> String {
        _ = payload
        throw AppError.coreUnavailable("El núcleo iOS no anuncia intercambio remoto operativo.")
    }

    func upload(payload: String) throws -> String {
        _ = payload
        throw AppError.coreUnavailable("El núcleo iOS no anuncia intercambio remoto operativo.")
    }

    private func result(_ call: AFV2CoreCallResult) throws -> String {
        try Task.checkCancellation()
        guard call.isSuccess, let value = call.value else {
            throw AppError.operationFailed(call.errorMessage ?? "El núcleo rechazó la operación.")
        }
        guard value.utf8.count <= AppConfiguration.maximumJSONBytes else {
            throw AppError.operationFailed("La respuesta del núcleo supera el límite permitido.")
        }
        return value
    }
}

enum CoreContractValidator {
    struct Status: Equatable {
        let valid: Bool
        let capabilities: CoreCapabilities
    }

    private struct Contract: Decodable {
        struct Services: Decodable {
            let sign: Bool
            let verify: Bool
            let selectCertificate: Bool
            let importCertificate: Bool
            let remoteExchange: Bool?

            enum CodingKeys: String, CodingKey {
                case sign, verify
                case selectCertificate = "select_certificate"
                case importCertificate = "import_certificate"
                case remoteExchange = "remote_exchange"
            }
        }

        struct IdentityStore: Decodable {
            let persistent: Bool?
        }

        struct Verification: Decodable {
            let cryptographicIntegrity: Bool

            enum CodingKeys: String, CodingKey {
                case cryptographicIntegrity = "cryptographic_integrity"
            }
        }

        struct Limits: Decodable {
            let documentBytes: Int64
            let signedOutputBytes: Int64
            let certificateBytes: Int64
            let passwordBytes: Int

            enum CodingKeys: String, CodingKey {
                case documentBytes = "document_bytes"
                case signedOutputBytes = "signed_output_bytes"
                case certificateBytes = "certificate_bytes"
                case passwordBytes = "password_bytes"
            }
        }

        let contractVersion: Int
        let platform: String
        let services: Services
        let identityStore: IdentityStore?
        let approval: String
        let verification: Verification
        let limits: Limits

        enum CodingKeys: String, CodingKey {
            case contractVersion = "contract_version"
            case platform, services
            case identityStore = "identity_store"
            case approval, verification, limits
        }
    }

    static func isValidIOSContract(_ json: String) -> Bool {
        status(json)?.valid == true
    }

    static func status(_ json: String) -> Status? {
        guard let data = json.data(using: .utf8),
              data.count <= 32 * 1_024,
              let contract = try? JSONDecoder().decode(Contract.self, from: data)
        else {
            return nil
        }
        let valid = contract.contractVersion == 1
            && contract.platform == "ios"
            && contract.services.sign
            && contract.services.verify
            && contract.services.selectCertificate
            && contract.services.importCertificate
            && contract.approval == "native_ui_explicit_action"
            && contract.verification.cryptographicIntegrity
            && contract.limits.documentBytes >= AppConfiguration.maximumDocumentBytes
            && contract.limits.signedOutputBytes >= AppConfiguration.maximumSignedBytes
            && contract.limits.certificateBytes >= AppConfiguration.maximumCertificateBytes
            && contract.limits.passwordBytes >= AppConfiguration.maximumPasswordBytes
        return Status(
            valid: valid,
            capabilities: CoreCapabilities(
                // El bridge nativo no expone transporte hasta que mobilebind
                // publique métodos bind-friendly y verificables para ello.
                remoteExchange: false,
                persistentIdentity: contract.identityStore?.persistent == true
            )
        )
    }
}
