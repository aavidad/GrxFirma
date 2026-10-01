// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import Foundation

struct SensitiveText: Sendable {
    private var storage: [UInt8]

    init(_ value: String) {
        storage = Array(value.utf8)
    }

    mutating func consume() -> String {
        defer { reset() }
        return String(decoding: storage, as: UTF8.self)
    }

    mutating func reset() {
        storage.withUnsafeMutableBytes { buffer in
            _ = buffer.initializeMemory(as: UInt8.self, repeating: 0)
        }
        storage.removeAll(keepingCapacity: false)
    }
}
