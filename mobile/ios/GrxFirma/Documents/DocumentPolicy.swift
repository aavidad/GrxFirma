// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import Foundation

enum DocumentPolicy {
    static func suggestedFormat(for document: SecureDocument) -> String {
        let lowerName = document.displayName.lowercased()
        if document.mimeType == "application/pdf" || lowerName.hasSuffix(".pdf") {
            return "PAdES"
        }
        if document.mimeType.contains("xml") || lowerName.hasSuffix(".xml") {
            return "XAdES"
        }
        return "CAdES"
    }

    static func isCertificateFile(_ url: URL) -> Bool {
        let ext = url.pathExtension.lowercased()
        return ext == "p12" || ext == "pfx"
    }
}
