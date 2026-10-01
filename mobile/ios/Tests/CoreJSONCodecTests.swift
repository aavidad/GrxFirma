// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import XCTest
@testable import GrxFirma

final class CoreJSONCodecTests: XCTestCase {
    func testContractRequiresIOSAndEveryCriticalService() {
        let valid = """
        {"contract_version":1,"platform":"ios","services":{"sign":true,"verify":true,"select_certificate":true,"import_certificate":true},"identity_store":{"persistent":false},"approval":"native_ui_explicit_action","verification":{"cryptographic_integrity":true},"limits":{"document_bytes":33554432,"signed_output_bytes":50331648,"certificate_bytes":4194304,"password_bytes":1024}}
        """
        XCTAssertTrue(CoreContractValidator.isValidIOSContract(valid))
        XCTAssertFalse(CoreContractValidator.isValidIOSContract(valid.replacingOccurrences(of: "\"ios\"", with: "\"android\"")))
        XCTAssertFalse(CoreContractValidator.isValidIOSContract(valid.replacingOccurrences(of: "\"sign\":true", with: "\"sign\":false")))
    }

    func testContractCapabilitiesFailClosedWhenAbsentOrDisabled() throws {
        let current = """
        {"contract_version":1,"platform":"ios","services":{"sign":true,"verify":true,"select_certificate":true,"import_certificate":true,"remote_exchange":false},"identity_store":{"persistent":false},"approval":"native_ui_explicit_action","verification":{"cryptographic_integrity":true},"limits":{"document_bytes":33554432,"signed_output_bytes":50331648,"certificate_bytes":4194304,"password_bytes":1024}}
        """
        let status = try XCTUnwrap(CoreContractValidator.status(current))
        XCTAssertTrue(status.valid)
        XCTAssertFalse(status.capabilities.remoteExchange)
        XCTAssertFalse(status.capabilities.persistentIdentity)
    }

    func testContractRejectsWeakLimitsOrApproval() {
        let weak = """
        {"contract_version":1,"platform":"ios","services":{"sign":true,"verify":true,"select_certificate":true,"import_certificate":true},"approval":"core_implicit","verification":{"cryptographic_integrity":true},"limits":{"document_bytes":1,"signed_output_bytes":1,"certificate_bytes":1,"password_bytes":1}}
        """
        XCTAssertFalse(CoreContractValidator.isValidIOSContract(weak))
    }

    func testSignRequestMatchesMobilebindContract() throws {
        let data = Data("documento".utf8)
        let document = SecureDocument(
            id: UUID(),
            url: URL(fileURLWithPath: "/tmp/documento.txt"),
            displayName: "documento.txt",
            mimeType: "text/plain",
            byteCount: Int64(data.count),
            sha256: "00"
        )
        let payload = try CoreJSONCodec.signPayload(
            document: document,
            data: data,
            action: .sign,
            format: "CAdES",
            certificateID: "cert-1",
            algorithm: "SHA384withRSA"
        )
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(payload.utf8)) as? [String: Any])
        XCTAssertEqual(object["content_base64"] as? String, data.base64EncodedString())
        XCTAssertEqual(object["certificate_id"] as? String, "cert-1")
        XCTAssertEqual(object["action"] as? String, "sign")
        XCTAssertEqual((object["options"] as? [String: String])?["algorithm"], "SHA384withRSA")
    }

    func testDecodeSignedRejectsOversizedOrInvalidBase64() {
        let invalid = """
        {"format":"CAdES","algorithm":"SHA256withRSA","signed_content_base64":"%%%","certificate_id":"cert-1"}
        """
        XCTAssertThrowsError(try CoreJSONCodec.decodeSigned(invalid))
    }

    func testVerificationBoundsUntrustedText() throws {
        let long = String(repeating: "x", count: 2_000)
        let json = """
        {"valid":false,"reason":"\(long)","details":[],"signers":[],"integrity_status":"valid","certificate_status":"unknown","trust_status":"unknown","warnings":[],"errors":[]}
        """
        let result = try CoreJSONCodec.decodeVerification(json)
        XCTAssertEqual(result.reason.count, 500)
        XCTAssertEqual(result.integrityStatus, "valid")
        XCTAssertEqual(result.trustStatus, "unknown")
    }

    func testVerificationFailsClosedOnInconsistentIntegrity() throws {
        let json = """
        {"valid":true,"reason":"inconsistente","details":[],"signers":[],"integrity_status":"invalid","certificate_status":"valid","trust_status":"valid","warnings":[],"errors":[]}
        """
        let result = try CoreJSONCodec.decodeVerification(json)
        XCTAssertFalse(result.valid)
    }
}
