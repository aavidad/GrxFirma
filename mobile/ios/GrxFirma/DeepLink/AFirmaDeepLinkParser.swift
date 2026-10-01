// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import CryptoKit
import Foundation

enum AFirmaOperation: String, Equatable, Sendable {
    case sign
    case cosign
    case countersign
    case verify

    var signatureAction: SignatureAction? {
        switch self {
        case .sign: return .sign
        case .cosign: return .cosign
        case .countersign: return .countersign
        case .verify: return nil
        }
    }
}

struct AFirmaDeepLinkRequest: Identifiable, Equatable, Sendable {
    let operation: AFirmaOperation
    let requestID: String
    let format: String
    let algorithm: String
    let fileName: String
    let inlineData: Data?
    let remoteSession: PendingRemoteSession?

    var id: String { requestID }

    var portalHost: String? {
        remoteSession?.retrieveEndpoint.host
    }
}

enum AFirmaDeepLinkParser {
    private static let maximumURLBytes = 8 * 1_024
    private static let maximumInlineBytes = 5 * 1_024
    private static let maximumSessionValueBytes = 1_024
    private static let supportedFormats = ["pades": "PAdES", "cades": "CAdES", "xades": "XAdES"]
    private static let supportedAlgorithms: Set<String> = [
        "SHA256withRSA", "SHA384withRSA", "SHA512withRSA",
        "SHA256withECDSA", "SHA384withECDSA", "SHA512withECDSA"
    ]

    static func parse(_ url: URL) throws -> AFirmaDeepLinkRequest {
        let raw = url.absoluteString
        guard raw.utf8.count <= maximumURLBytes else {
            throw AppError.invalidDeepLink("El enlace afirma:// supera 8 KiB.")
        }
        guard var components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              components.scheme?.lowercased() == "afirma",
              components.user == nil,
              components.password == nil,
              components.fragment == nil
        else {
            throw AppError.invalidDeepLink("El enlace afirma:// no tiene una estructura segura.")
        }
        components.scheme = components.scheme?.lowercased()
        let operationName = normalizedOperation(components)
        guard let operation = AFirmaOperation(rawValue: operationName) else {
            throw AppError.invalidDeepLink("La operación afirma:// no está admitida en iOS.")
        }

        let query = try uniqueQuery(components.queryItems ?? [])
        let inline = try decodeInline(query["dat"] ?? query["data"])
        let retrieve = query["rtservlet"] ?? query["retrieveservlet"]
        let upload = query["stservlet"] ?? query["storageservlet"]
        guard (retrieve == nil) == (upload == nil) else {
            throw AppError.invalidDeepLink("La sesión remota debe incluir descarga y subida HTTPS.")
        }
        guard inline == nil || retrieve == nil else {
            throw AppError.invalidDeepLink("El enlace no puede mezclar documento incrustado y sesión remota.")
        }
        guard inline != nil || retrieve != nil else {
            throw AppError.invalidDeepLink("El enlace no contiene documento ni sesión remota.")
        }

        let explicitID = query["id"] ?? query["fileid"] ?? query["request_id"]
        let requestID = try validatedRequestID(explicitID ?? derivedRequestID(raw))
        let remote: PendingRemoteSession?
        if let retrieve, let upload {
            let retrieveURL = try secureRemoteURL(retrieve, label: "descarga")
            let uploadURL = try secureRemoteURL(upload, label: "subida")
            guard retrieveURL.host?.lowercased() == uploadURL.host?.lowercased() else {
                throw AppError.invalidDeepLink("La descarga y la subida deben pertenecer al mismo portal.")
            }
            let key = try boundedSessionValue(query["key"] ?? "", label: "clave de sesión")
            let state = try boundedSessionValue(query["state"] ?? "active", label: "estado")
            guard !key.isEmpty else {
                throw AppError.invalidDeepLink("La sesión remota no contiene clave.")
            }
            guard state == "active" else {
                throw AppError.invalidDeepLink("La sesión remota ya no está activa.")
            }
            remote = PendingRemoteSession(
                requestID: requestID,
                sessionKey: key,
                retrieveEndpoint: retrieveURL,
                uploadEndpoint: uploadURL,
                state: state
            )
        } else {
            remote = nil
        }

        let rawFormat = (query["format"] ?? query["signformat"] ?? "cades").lowercased()
        guard let format = supportedFormats[rawFormat] else {
            throw AppError.invalidDeepLink("El formato de firma solicitado no está admitido.")
        }
        let algorithm = query["algorithm"] ?? "SHA256withRSA"
        guard supportedAlgorithms.contains(algorithm) else {
            throw AppError.invalidDeepLink("El algoritmo solicitado no cumple la política criptográfica.")
        }
        if format == "PAdES", !algorithm.hasPrefix("SHA256with") {
            throw AppError.invalidDeepLink("El núcleo iOS actual solo admite SHA-256 para PAdES.")
        }
        let fileName = safeName(query["filename"] ?? "solicitud-\(requestID).bin")
        return AFirmaDeepLinkRequest(
            operation: operation,
            requestID: requestID,
            format: format,
            algorithm: algorithm,
            fileName: fileName,
            inlineData: inline,
            remoteSession: remote
        )
    }

    private static func normalizedOperation(_ components: URLComponents) -> String {
        if let host = components.host?.lowercased(), !host.isEmpty { return host }
        let path = components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/")).lowercased()
        if !path.isEmpty { return path }
        return components.queryItems?.first(where: { $0.name.lowercased() == "op" })?.value?.lowercased() ?? ""
    }

    private static func uniqueQuery(_ items: [URLQueryItem]) throws -> [String: String] {
        var result: [String: String] = [:]
        for item in items {
            let key = item.name.lowercased()
            guard key.utf8.count <= 64, let value = item.value else { continue }
            if result[key] != nil {
                throw AppError.invalidDeepLink("El enlace repite el parámetro '\(String(key.prefix(32)))'.")
            }
            result[key] = value
        }
        return result
    }

    private static func decodeInline(_ encoded: String?) throws -> Data? {
        guard let encoded, !encoded.isEmpty else { return nil }
        guard encoded.utf8.count <= maximumInlineBytes * 2 else {
            throw AppError.invalidDeepLink("El documento incrustado supera el límite del enlace.")
        }
        let normalized = encoded.replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        let padding = String(repeating: "=", count: (4 - normalized.count % 4) % 4)
        guard let data = Data(base64Encoded: normalized + padding, options: []),
              !data.isEmpty,
              data.count <= maximumInlineBytes
        else {
            throw AppError.invalidDeepLink("El documento incrustado no tiene Base64 válido.")
        }
        return data
    }

    private static func secureRemoteURL(_ raw: String, label: String) throws -> URL {
        guard raw.utf8.count <= 2_048,
              let components = URLComponents(string: raw),
              components.scheme?.lowercased() == "https",
              components.user == nil,
              components.password == nil,
              components.fragment == nil,
              components.port == nil || components.port == 443,
              let host = components.host?.lowercased(),
              host.count <= 253,
              host.contains("."),
              !host.hasSuffix("."),
              !host.hasSuffix(".local"),
              host != "localhost",
              !isIPAddress(host),
              let url = components.url
        else {
            throw AppError.invalidDeepLink("El endpoint de \(label) debe ser HTTPS público sin credenciales.")
        }
        return url
    }

    private static func isIPAddress(_ host: String) -> Bool {
        if host.contains(":") { return true }
        let parts = host.split(separator: ".", omittingEmptySubsequences: false)
        return parts.count == 4 && parts.allSatisfy { part in
            guard let value = Int(part) else { return false }
            return value >= 0 && value <= 255 && String(value) == part
        }
    }

    private static func validatedRequestID(_ value: String) throws -> String {
        let allowed = CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._:")
        guard !value.isEmpty,
              value.utf8.count <= 128,
              value.unicodeScalars.allSatisfy(allowed.contains)
        else {
            throw AppError.invalidDeepLink("El identificador de solicitud no es válido.")
        }
        return value
    }

    private static func derivedRequestID(_ raw: String) -> String {
        let hash = SHA256.hash(data: Data(raw.utf8))
        return "ios:" + hash.prefix(12).map { String(format: "%02x", $0) }.joined()
    }

    private static func boundedSessionValue(_ value: String, label: String) throws -> String {
        guard value.utf8.count <= maximumSessionValueBytes,
              value.unicodeScalars.allSatisfy({ !CharacterSet.controlCharacters.contains($0) })
        else {
            throw AppError.invalidDeepLink("La \(label) no es válida.")
        }
        return value
    }

    private static func safeName(_ value: String) -> String {
        let name = URL(fileURLWithPath: value).lastPathComponent
        let allowed = CharacterSet.alphanumerics.union(CharacterSet(charactersIn: " ._()-"))
        let filtered = name.unicodeScalars.map { allowed.contains($0) ? Character(String($0)) : "_" }
        let result = String(filtered).trimmingCharacters(in: .whitespacesAndNewlines)
        return result.isEmpty ? "solicitud.bin" : String(result.prefix(120))
    }
}
