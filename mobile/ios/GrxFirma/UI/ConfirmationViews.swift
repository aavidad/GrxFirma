// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import SwiftUI

struct SignatureConfirmationView: View {
    let approval: SignApproval
    let document: SecureDocument?
    let certificate: CertificateSummary?
    let onConfirm: () -> Void
    let onCancel: () -> Void

    var body: some View {
        NavigationStack {
            Form {
                Section("Documento") {
                    LabeledContent("Nombre", value: document?.displayName ?? "No disponible")
                    LabeledContent("Tamaño", value: document.map {
                        ByteCountFormatter.string(fromByteCount: $0.byteCount, countStyle: .file)
                    } ?? "No disponible")
                    if let hash = document?.sha256 {
                        LabeledContent("SHA-256", value: String(hash.prefix(20)) + "…")
                            .fontDesign(.monospaced)
                    }
                }
                Section("Firma") {
                    LabeledContent("Acción", value: approval.action.title)
                    LabeledContent("Formato", value: approval.format)
                    if let algorithm = approval.algorithm {
                        LabeledContent("Algoritmo solicitado", value: algorithm)
                    }
                    LabeledContent("Certificado", value: certificate?.subject ?? "No disponible")
                }
                Section {
                    Label(
                        "La firma se ejecutará solo al pulsar el botón inferior. Revise el documento y el certificado.",
                        systemImage: "hand.raised.fill"
                    )
                }
            }
            .navigationTitle("Confirmar firma")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancelar", role: .cancel, action: onCancel)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Firmar documento", action: onConfirm)
                        .bold()
                        .accessibilityIdentifier("signature.confirm")
                }
            }
        }
    }
}

struct DeepLinkReviewView: View {
    let request: AFirmaDeepLinkRequest
    let supportsRemoteExchange: Bool
    let onAccept: () -> Void
    let onCancel: () -> Void

    var body: some View {
        NavigationStack {
            Form {
                Section("Solicitud") {
                    LabeledContent("Operación", value: request.operation.rawValue)
                    LabeledContent("Identificador", value: request.requestID)
                    LabeledContent("Formato", value: request.format)
                    LabeledContent("Algoritmo", value: request.algorithm)
                    if let portal = request.portalHost {
                        LabeledContent("Portal", value: portal)
                    } else {
                        LabeledContent("Origen", value: "Documento incrustado")
                    }
                }
                Section {
                    if request.remoteSession != nil && !supportsRemoteExchange {
                        Label(
                            "El núcleo instalado no admite intercambio remoto. Esta solicitud no puede aceptarse.",
                            systemImage: "exclamationmark.shield.fill"
                        )
                        .foregroundStyle(.orange)
                    } else {
                        Label(
                            "Aceptar solo prepara el documento. La firma exigirá otra confirmación explícita.",
                            systemImage: "shield.lefthalf.filled"
                        )
                    }
                }
            }
            .navigationTitle("Revisar enlace afirma://")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Rechazar", role: .cancel, action: onCancel)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Aceptar solicitud", action: onAccept)
                        .disabled(request.remoteSession != nil && !supportsRemoteExchange)
                        .accessibilityIdentifier("deeplink.accept")
                }
            }
        }
    }
}

struct CertificatePasswordView: View {
    let fileName: String
    let onImport: (SensitiveText) -> Void
    let onCancel: () -> Void
    @State private var password = ""

    var body: some View {
        NavigationStack {
            Form {
                Section("Certificado") {
                    LabeledContent("Fichero", value: fileName)
                    SecureField("Contraseña PKCS#12", text: $password)
                        .textContentType(.password)
                        .submitLabel(.done)
                        .accessibilityIdentifier("certificate.password")
                }
                Section {
                    Text("La contraseña no se guarda. El núcleo actual mantiene la identidad importada solo durante la sesión y la elimina al pasar a segundo plano.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle("Importar certificado")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancelar", role: .cancel) {
                        password = ""
                        onCancel()
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Importar") {
                        let protected = SensitiveText(password)
                        password = ""
                        onImport(protected)
                    }
                    .disabled(password.utf8.count > AppConfiguration.maximumPasswordBytes)
                    .accessibilityIdentifier("certificate.password.confirm")
                }
            }
        }
    }
}
