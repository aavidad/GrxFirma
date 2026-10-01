// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import XCTest
@testable import GrxFirma

final class DocumentPolicyTests: XCTestCase {
    func testSuggestedFormats() {
        XCTAssertEqual(DocumentPolicy.suggestedFormat(for: document("demo.pdf", "application/pdf")), "PAdES")
        XCTAssertEqual(DocumentPolicy.suggestedFormat(for: document("demo.xml", "application/xml")), "XAdES")
        XCTAssertEqual(DocumentPolicy.suggestedFormat(for: document("demo.bin", "application/octet-stream")), "CAdES")
    }

    func testCertificateExtensionsAreExplicit() {
        XCTAssertTrue(DocumentPolicy.isCertificateFile(URL(fileURLWithPath: "/tmp/cert.P12")))
        XCTAssertTrue(DocumentPolicy.isCertificateFile(URL(fileURLWithPath: "/tmp/cert.pfx")))
        XCTAssertFalse(DocumentPolicy.isCertificateFile(URL(fileURLWithPath: "/tmp/cert.pem")))
    }

    func testApprovalExpiresAndBindsIdentity() {
        let documentID = UUID()
        let approval = SignApproval(
            nonce: UUID(),
            documentID: documentID,
            certificateID: "cert-1",
            action: .sign,
            format: "PAdES",
            algorithm: nil,
            expiresAt: Date(timeIntervalSince1970: 100)
        )
        XCTAssertTrue(approval.isValid(at: Date(timeIntervalSince1970: 99)))
        XCTAssertFalse(approval.isValid(at: Date(timeIntervalSince1970: 100)))
        XCTAssertEqual(approval.documentID, documentID)
        XCTAssertEqual(approval.certificateID, "cert-1")
    }

    private func document(_ name: String, _ mime: String) -> SecureDocument {
        SecureDocument(
            id: UUID(),
            url: URL(fileURLWithPath: "/tmp/\(name)"),
            displayName: name,
            mimeType: mime,
            byteCount: 1,
            sha256: "00"
        )
    }
}
