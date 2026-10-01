// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import Foundation

enum CoreJSONCodec {
    private struct SignRequest: Encodable {
        let name: String
        let contentBase64: String
        let mimeType: String
        let format: String
        let action: String
        let certificateID: String
        let options: [String: String]

        enum CodingKeys: String, CodingKey {
            case name, format, action, options
            case contentBase64 = "content_base64"
            case mimeType = "mime_type"
            case certificateID = "certificate_id"
        }
    }

    private struct SignResponse: Decodable {
        let format: String
        let algorithm: String
        let signedContentBase64: String
        let certificateID: String

        enum CodingKeys: String, CodingKey {
            case format, algorithm
            case signedContentBase64 = "signed_content_base64"
            case certificateID = "certificate_id"
        }
    }

    private struct VerifyRequest: Encodable {
        let name: String
        let contentBase64: String
        let mimeType: String
        let originalContentBase64: String?

        enum CodingKeys: String, CodingKey {
            case name
            case contentBase64 = "content_base64"
            case mimeType = "mime_type"
            case originalContentBase64 = "original_content_base64"
        }
    }

    private struct VerifyResponse: Decodable {
        struct Signer: Decodable {
            let id: String?
            let subject: String?
            let issuer: String?
            let fingerprint: String?
        }

        let valid: Bool
        let reason: String
        let details: [String]?
        let format: String?
        let coverage: String?
        let integrityStatus: String?
        let certificateStatus: String?
        let trustStatus: String?
        let signerSummaries: [Signer]?
        let warnings: [String]?
        let errors: [String]?

        enum CodingKeys: String, CodingKey {
            case valid, reason, details, format, coverage, warnings, errors
            case integrityStatus = "integrity_status"
            case certificateStatus = "certificate_status"
            case trustStatus = "trust_status"
            case signerSummaries = "signer_summaries"
        }
    }

    private struct SelectRequest: Encodable {
        let subjectFilter: String
        let issuerFilter: String
        let onlyUnexpired: Bool

        enum CodingKeys: String, CodingKey {
            case subjectFilter = "subject_filter"
            case issuerFilter = "issuer_filter"
            case onlyUnexpired = "solo_no_caducados"
        }
    }

    private struct ImportRequest: Encodable {
        let dataBase64: String
        let password: String

        enum CodingKeys: String, CodingKey {
            case dataBase64 = "data_base64"
            case password
        }
    }

    private struct ImportResponse: Decodable {
        let certificateID: String
        let subject: String
        let issuer: String
        let fingerprint: String

        enum CodingKeys: String, CodingKey {
            case certificateID = "certificate_id"
            case subject, issuer, fingerprint
        }
    }

    private struct Session: Codable {
        let requestID: String
        let sessionKey: String
        let uploadEndpoint: String
        let retrieveEndpoint: String
        let state: String

        enum CodingKeys: String, CodingKey {
            case requestID = "request_id"
            case sessionKey = "session_key"
            case uploadEndpoint = "upload_endpoint"
            case retrieveEndpoint = "retrieve_endpoint"
            case state
        }
    }

    private struct RetrieveRequest: Encodable { let session: Session }

    private struct RetrieveResponse: Decodable {
        let requestID: String
        let dataBase64: String

        enum CodingKeys: String, CodingKey {
            case requestID = "request_id"
            case dataBase64 = "data_base64"
        }
    }

    private struct UploadRequest: Encodable {
        let session: Session
        let dataBase64: String

        enum CodingKeys: String, CodingKey {
            case session
            case dataBase64 = "data_base64"
        }
    }

    private struct UploadResponse: Decodable { let ok: Bool }

    static func signPayload(
        document: SecureDocument,
        data: Data,
        action: SignatureAction,
        format: String,
        certificateID: String,
        algorithm: String? = nil
    ) throws -> String {
        try requireSize(data, maximum: AppConfiguration.maximumDocumentBytes)
        let options: [String: String] = algorithm.map { ["algorithm": $0] } ?? [:]
        return try encode(SignRequest(
            name: document.displayName,
            contentBase64: data.base64EncodedString(),
            mimeType: document.mimeType,
            format: format,
            action: action.rawValue,
            certificateID: certificateID,
            options: options
        ))
    }

    static func decodeSigned(_ json: String) throws -> (Data, String, String, String) {
        let response: SignResponse = try decode(json)
        guard let data = Data(base64Encoded: response.signedContentBase64, options: []) else {
            throw AppError.operationFailed("El núcleo devolvió una firma Base64 inválida.")
        }
        try requireSize(data, maximum: AppConfiguration.maximumSignedBytes)
        let format = bounded(response.format, maximum: 64)
        let algorithm = bounded(response.algorithm, maximum: 128)
        let certificateID = bounded(response.certificateID, maximum: 128)
        guard !format.isEmpty, !algorithm.isEmpty, !certificateID.isEmpty else {
            throw AppError.operationFailed("La respuesta de firma está incompleta.")
        }
        return (data, format, algorithm, certificateID)
    }

    static func verifyPayload(
        document: SecureDocument,
        data: Data,
        originalData: Data? = nil
    ) throws -> String {
        try requireSize(data, maximum: AppConfiguration.maximumDocumentBytes)
        if let originalData {
            try requireSize(originalData, maximum: AppConfiguration.maximumDocumentBytes)
        }
        return try encode(VerifyRequest(
            name: document.displayName,
            contentBase64: data.base64EncodedString(),
            mimeType: document.mimeType,
            originalContentBase64: originalData?.base64EncodedString()
        ))
    }

    static func decodeVerification(_ json: String) throws -> VerificationSummary {
        let response: VerifyResponse = try decode(json)
        let signers = (response.signerSummaries ?? []).enumerated().map { index, signer in
            VerificationSigner(
                id: "\(index):\(bounded(signer.id, maximum: 128))",
                subject: bounded(signer.subject),
                issuer: bounded(signer.issuer),
                fingerprint: bounded(signer.fingerprint, maximum: 128)
            )
        }
        let integrityStatus = bounded(response.integrityStatus, maximum: 64)
        let errors = bounded(response.errors)
        let effectiveValid = response.valid
            && integrityStatus.lowercased() == "valid"
            && errors.isEmpty
        return VerificationSummary(
            valid: effectiveValid,
            reason: bounded(response.reason),
            details: bounded(response.details),
            format: bounded(response.format),
            coverage: bounded(response.coverage),
            integrityStatus: integrityStatus,
            certificateStatus: bounded(response.certificateStatus),
            trustStatus: bounded(response.trustStatus),
            signers: Array(signers.prefix(128)),
            warnings: bounded(response.warnings),
            errors: errors
        )
    }

    static func selectCertificatePayload() throws -> String {
        try encode(SelectRequest(subjectFilter: "", issuerFilter: "", onlyUnexpired: true))
    }

    static func decodeSelectedCertificate(_ json: String) throws -> CertificateSummary {
        let response: CertificateSummary = try decode(json)
        let certificateID = bounded(response.certificateID, maximum: 128)
        let subject = bounded(response.subject)
        guard response.confirmed, !certificateID.isEmpty, !subject.isEmpty else {
            throw AppError.operationRejected("No se confirmó ningún certificado válido.")
        }
        return CertificateSummary(
            certificateID: certificateID,
            subject: subject,
            issuer: bounded(response.issuer),
            fingerprint: bounded(response.fingerprint, maximum: 128),
            confirmed: true
        )
    }

    static func importCertificatePayload(data: Data, password: String) throws -> String {
        try requireSize(data, maximum: AppConfiguration.maximumCertificateBytes)
        guard password.utf8.count <= AppConfiguration.maximumPasswordBytes else {
            throw AppError.operationRejected("La contraseña del certificado es demasiado larga.")
        }
        return try encode(ImportRequest(dataBase64: data.base64EncodedString(), password: password))
    }

    static func decodeImportedCertificate(_ json: String) throws -> CertificateSummary {
        let response: ImportResponse = try decode(json)
        let certificateID = bounded(response.certificateID, maximum: 128)
        let subject = bounded(response.subject)
        guard !certificateID.isEmpty, !subject.isEmpty else {
            throw AppError.operationFailed("El certificado importado no contiene identidad utilizable.")
        }
        return CertificateSummary(
            certificateID: certificateID,
            subject: subject,
            issuer: bounded(response.issuer),
            fingerprint: bounded(response.fingerprint, maximum: 128),
            confirmed: true
        )
    }

    static func retrievePayload(_ remote: PendingRemoteSession) throws -> String {
        try encode(RetrieveRequest(session: session(remote)))
    }

    static func decodeRetrievedDocument(_ json: String, expectedRequestID: String) throws -> Data {
        let response: RetrieveResponse = try decode(json)
        guard response.requestID == expectedRequestID else {
            throw AppError.operationFailed("La respuesta remota no corresponde a la solicitud aceptada.")
        }
        guard let data = Data(base64Encoded: response.dataBase64, options: []) else {
            throw AppError.operationFailed("El documento remoto no tiene Base64 válido.")
        }
        try requireSize(data, maximum: AppConfiguration.maximumDocumentBytes)
        return data
    }

    static func uploadPayload(_ remote: PendingRemoteSession, data: Data) throws -> String {
        try requireSize(data, maximum: AppConfiguration.maximumSignedBytes)
        return try encode(UploadRequest(session: session(remote), dataBase64: data.base64EncodedString()))
    }

    static func decodeUploadResponse(_ json: String) throws {
        let response: UploadResponse = try decode(json)
        guard response.ok else {
            throw AppError.operationFailed("El portal no confirmó la recepción de la firma.")
        }
    }

    private static func session(_ remote: PendingRemoteSession) -> Session {
        Session(
            requestID: remote.requestID,
            sessionKey: remote.sessionKey,
            uploadEndpoint: remote.uploadEndpoint.absoluteString,
            retrieveEndpoint: remote.retrieveEndpoint.absoluteString,
            state: remote.state
        )
    }

    private static func encode<T: Encodable>(_ value: T) throws -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        let data = try encoder.encode(value)
        guard data.count <= AppConfiguration.maximumJSONBytes,
              let result = String(data: data, encoding: .utf8)
        else {
            throw AppError.operationRejected("La petición supera el límite seguro del núcleo.")
        }
        return result
    }

    private static func decode<T: Decodable>(_ json: String) throws -> T {
        guard let data = json.data(using: .utf8), data.count <= AppConfiguration.maximumJSONBytes else {
            throw AppError.operationFailed("La respuesta JSON no es válida o es demasiado grande.")
        }
        do {
            return try JSONDecoder().decode(T.self, from: data)
        } catch {
            throw AppError.operationFailed("El núcleo devolvió una respuesta incompatible.")
        }
    }

    private static func requireSize(_ data: Data, maximum: Int64) throws {
        guard !data.isEmpty, Int64(data.count) <= maximum else {
            throw AppError.invalidDocument("El documento está vacío o supera el límite permitido.")
        }
    }

    private static func bounded(_ value: String?, maximum: Int = 500) -> String {
        let scalars = (value ?? "").unicodeScalars.filter {
            !CharacterSet.controlCharacters.contains($0)
        }
        return String(scalars.map(String.init).joined().prefix(maximum))
    }

    private static func bounded(_ values: [String]?) -> [String] {
        (values ?? []).prefix(128).map { bounded($0) }
    }
}
