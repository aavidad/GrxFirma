// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import XCTest
@testable import GrxFirma

final class AFirmaDeepLinkParserTests: XCTestCase {
    func testParsesInlineSignWithoutExecutingIt() throws {
        let url = try XCTUnwrap(URL(string: "afirma://sign?id=req-123&dat=QUJD&format=CAdES&algorithm=SHA256withRSA"))
        let request = try AFirmaDeepLinkParser.parse(url)
        XCTAssertEqual(request.operation, .sign)
        XCTAssertEqual(request.requestID, "req-123")
        XCTAssertEqual(request.inlineData, Data("ABC".utf8))
        XCTAssertNil(request.remoteSession)
    }

    func testParsesStrictHTTPSRemoteSession() throws {
        let retrieve = "https%3A%2F%2Ffirma.example.org%2Fretrieve"
        let upload = "https%3A%2F%2Ffirma.example.org%2Fupload"
        let url = try XCTUnwrap(URL(string: "afirma://sign?id=req-remote&key=secret&rtservlet=\(retrieve)&stservlet=\(upload)"))
        let request = try AFirmaDeepLinkParser.parse(url)
        XCTAssertEqual(request.portalHost, "firma.example.org")
        XCTAssertEqual(request.remoteSession?.state, "active")
    }

    func testRejectsHTTPAndLocalEndpoints() throws {
        let cases = [
            "http%3A%2F%2Ffirma.example.org%2Fretrieve",
            "https%3A%2F%2Flocalhost%2Fretrieve",
            "https%3A%2F%2F127.0.0.1%2Fretrieve"
        ]
        for endpoint in cases {
            let url = try XCTUnwrap(URL(string: "afirma://sign?id=req-1&rtservlet=\(endpoint)&stservlet=\(endpoint)"))
            XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(url), endpoint)
        }
    }

    func testRejectsMixedPortalHosts() throws {
        let url = try XCTUnwrap(URL(string:
            "afirma://sign?id=req-1&rtservlet=https%3A%2F%2Fa.example.org%2Fr&stservlet=https%3A%2F%2Fb.example.org%2Fu"
        ))
        XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(url))
    }

    func testRejectsAmbiguousInlineAndRemoteSources() throws {
        let url = try XCTUnwrap(URL(string:
            "afirma://sign?id=req-1&dat=QUJD&rtservlet=https%3A%2F%2Ffirma.example.org%2Fr&stservlet=https%3A%2F%2Ffirma.example.org%2Fu"
        ))
        XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(url))
    }

    func testRejectsRemoteSessionWithoutKeyOrActiveState() throws {
        let endpoints = "rtservlet=https%3A%2F%2Ffirma.example.org%2Fr&stservlet=https%3A%2F%2Ffirma.example.org%2Fu"
        let missingKey = try XCTUnwrap(URL(string: "afirma://sign?id=req-1&\(endpoints)"))
        let completed = try XCTUnwrap(URL(string: "afirma://sign?id=req-2&key=secret&state=completed&\(endpoints)"))
        XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(missingKey))
        XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(completed))
    }

    func testRejectsWeakAlgorithmAndDuplicateCriticalValue() throws {
        let weak = try XCTUnwrap(URL(string: "afirma://sign?id=req-1&dat=QUJD&algorithm=SHA1withRSA"))
        XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(weak))
        let duplicate = try XCTUnwrap(URL(string: "afirma://sign?id=req-1&id=req-2&dat=QUJD"))
        XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(duplicate))
    }

    func testRejectsUnsupportedPAdESDigest() throws {
        let url = try XCTUnwrap(URL(string: "afirma://sign?id=req-1&dat=QUJD&format=PAdES&algorithm=SHA512withRSA"))
        XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(url))
    }

    func testRejectsOversizedInlinePayload() throws {
        let payload = Data(repeating: 0x41, count: 5 * 1_024 + 1).base64EncodedString()
        let url = try XCTUnwrap(URL(string: "afirma://sign?id=req-1&dat=\(payload)"))
        XCTAssertThrowsError(try AFirmaDeepLinkParser.parse(url))
    }
}
