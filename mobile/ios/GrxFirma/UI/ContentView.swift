// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import SwiftUI
import UniformTypeIdentifiers

struct ContentView: View {
    @ObservedObject var model: AppModel
    @State private var importsDocument = false
    @State private var importsCertificate = false
    @State private var sharesArtifact = false
    @State private var confirmsUpload = false

    var body: some View {
        NavigationStack {
            List {
                readinessSection
                documentSection
                certificateSection
                operationSection
                verificationSection
                outputSection
            }
            .navigationTitle("GrxFirma")
            .toolbar { toolbar }
            .disabled(model.operationState.isBusy)
            .overlay { progressOverlay }
        }
        .fileImporter(
            isPresented: $importsDocument,
            allowedContentTypes: [.data],
            allowsMultipleSelection: false,
            onCompletion: handleDocumentImport
        )
        .fileImporter(
            isPresented: $importsCertificate,
            allowedContentTypes: certificateTypes,
            allowsMultipleSelection: false,
            onCompletion: handleCertificateSelection
        )
        .sheet(isPresented: certificateSheetPresented) {
            if let url = model.pendingCertificateImport {
                CertificatePasswordView(fileName: url.lastPathComponent) { password in
                    model.importCertificate(from: url, password: password)
                } onCancel: {
                    model.cancelCertificateImport()
                }
            }
        }
        .sheet(isPresented: $sharesArtifact) {
            if let artifact = model.signedArtifact {
                ShareSheet(items: [artifact.url])
                    .ignoresSafeArea()
            }
        }
        .sheet(item: $model.pendingApproval) { approval in
            SignatureConfirmationView(
                approval: approval,
                document: model.document,
                certificate: model.certificate,
                onConfirm: { model.executeApprovedSignature(approval) },
                onCancel: model.cancelPendingApproval
            )
            .interactiveDismissDisabled()
        }
        .sheet(item: $model.pendingDeepLink) { request in
            DeepLinkReviewView(
                request: request,
                supportsRemoteExchange: model.supportsRemoteExchange,
                onAccept: model.acceptPendingDeepLink,
                onCancel: model.cancelPendingDeepLink
            )
            .interactiveDismissDisabled()
        }
        .confirmationDialog(
            "Enviar la firma al portal",
            isPresented: $confirmsUpload,
            titleVisibility: .visible
        ) {
            Button("Enviar resultado", role: .destructive) { model.uploadSignedResult() }
            Button("Cancelar", role: .cancel) {}
        } message: {
            Text("El documento firmado saldrá de este dispositivo hacia el portal que originó la solicitud.")
        }
        .alert(item: $model.alert) { alert in
            Alert(title: Text(alert.title), message: Text(alert.message), dismissButton: .default(Text("Aceptar")))
        }
    }

    private var readinessSection: some View {
        Section("Estado") {
            Label {
                VStack(alignment: .leading, spacing: 3) {
                    Text(model.readiness.available ? "Núcleo disponible" : "Núcleo no disponible")
                        .font(.headline)
                    Text(model.readiness.detail)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            } icon: {
                Image(systemName: model.readiness.available ? "checkmark.shield.fill" : "exclamationmark.shield.fill")
                    .foregroundStyle(model.readiness.available ? .green : .orange)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("core.readiness")

            if case .completed(let message) = model.operationState {
                Label(message, systemImage: "checkmark.circle")
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("operation.completed")
            }
        }
    }

    private var documentSection: some View {
        Section("Documento") {
            if let document = model.document {
                LabeledContent("Nombre", value: document.displayName)
                LabeledContent("Tamaño", value: ByteCountFormatter.string(fromByteCount: document.byteCount, countStyle: .file))
                LabeledContent("SHA-256", value: String(document.sha256.prefix(16)) + "…")
                    .fontDesign(.monospaced)
                    .accessibilityLabel("Huella SHA-256 \(document.sha256)")
            } else {
                Label {
                    VStack(alignment: .leading, spacing: 3) {
                        Text("Sin documento").font(.headline)
                        Text("Importe un fichero desde Archivos o use Compartir desde otra aplicación.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                } icon: {
                    Image(systemName: "doc.badge.plus")
                }
                .accessibilityElement(children: .combine)
            }
            Button("Importar documento", systemImage: "doc.badge.plus") { importsDocument = true }
                .accessibilityHint("Abre el selector de documentos del sistema")
                .accessibilityIdentifier("document.import")
        }
    }

    private var certificateSection: some View {
        Section("Certificado") {
            if let certificate = model.certificate {
                LabeledContent("Titular", value: certificate.subject)
                LabeledContent("Emisor", value: certificate.issuer)
                if let fingerprint = certificate.fingerprint, !fingerprint.isEmpty {
                    LabeledContent("Huella", value: String(fingerprint.prefix(20)) + "…")
                        .fontDesign(.monospaced)
                }
            } else {
                Text("No se ha seleccionado un certificado.")
                    .foregroundStyle(.secondary)
            }
            Button("Seleccionar certificado", systemImage: "person.text.rectangle") {
                model.selectCertificate()
            }
            .disabled(!model.readiness.available)
            .accessibilityIdentifier("certificate.select")
            Button("Importar PKCS#12", systemImage: "key.horizontal") { importsCertificate = true }
                .disabled(!model.readiness.available)
                .accessibilityIdentifier("certificate.import")
        }
    }

    private var operationSection: some View {
        Section("Operación") {
            Picker("Acción", selection: $model.selectedAction) {
                ForEach(SignatureAction.allCases) { action in
                    Text(action.title).tag(action)
                }
            }
            .pickerStyle(.segmented)

            Picker("Formato", selection: $model.selectedFormat) {
                Text("PAdES").tag("PAdES")
                Text("CAdES").tag("CAdES")
                Text("XAdES").tag("XAdES")
            }

            Button("Revisar y firmar", systemImage: "signature") {
                model.requestSignatureApproval()
            }
            .buttonStyle(.borderedProminent)
            .disabled(!canSign)
            .accessibilityHint("Muestra una confirmación detallada; nunca firma de forma automática")
            .accessibilityIdentifier("operation.sign.review")

            Button("Verificar firma", systemImage: "checkmark.seal") { model.verifyDocument() }
                .disabled(model.document == nil || !model.readiness.available)
                .accessibilityIdentifier("operation.verify")
        }
    }

    @ViewBuilder
    private var verificationSection: some View {
        if let verification = model.verification {
            Section("Resultado de verificación") {
                Label(
                    verificationTitle(verification),
                    systemImage: verificationIcon(verification)
                )
                .font(.headline)
                .foregroundStyle(verificationColor(verification))
                Text(verification.reason == "revocación no concluyente" ? revocationMessage() : verification.reason)
                if !verification.format.isEmpty {
                    LabeledContent("Formato", value: verification.format)
                }
                if !verification.integrityStatus.isEmpty {
                    LabeledContent("Integridad", value: verification.integrityStatus)
                }
                if !verification.certificateStatus.isEmpty {
                    LabeledContent("Certificado", value: verification.certificateStatus)
                }
                if !verification.trustStatus.isEmpty {
                    LabeledContent("Confianza", value: verification.trustStatus)
                }
                ForEach(verification.signers) { signer in
                    VStack(alignment: .leading, spacing: 3) {
                        Text(signer.subject).font(.subheadline).bold()
                        Text(signer.issuer).font(.caption).foregroundStyle(.secondary)
                    }
                    .accessibilityElement(children: .combine)
                }
                ForEach(verification.warnings.indices, id: \.self) { index in
                    Label(verification.warnings[index], systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.orange)
                }
                ForEach(verification.errors.indices, id: \.self) { index in
                    Label(verification.errors[index], systemImage: "xmark.octagon")
                        .foregroundStyle(.red)
                }
            }
        }
    }

    @ViewBuilder
    private var outputSection: some View {
        if let artifact = model.signedArtifact {
            Section("Resultado firmado") {
                LabeledContent("Fichero", value: artifact.displayName)
                LabeledContent("Formato", value: artifact.format)
                LabeledContent("Algoritmo", value: artifact.algorithm)
                Button("Compartir resultado", systemImage: "square.and.arrow.up") { sharesArtifact = true }
                    .accessibilityIdentifier("result.share")
                if model.hasPendingRemoteUpload {
                    Button("Enviar al portal", systemImage: "paperplane") { confirmsUpload = true }
                        .accessibilityHint("Solicita una segunda confirmación antes de transmitir el resultado")
                        .accessibilityIdentifier("result.upload")
                }
            }
        }
    }

    @ToolbarContentBuilder
    private var toolbar: some ToolbarContent {
        ToolbarItemGroup(placement: .primaryAction) {
            Button { importsDocument = true } label: {
                Label("Importar", systemImage: "plus")
            }
            .help("Importar documento")
        }
    }

    @ViewBuilder
    private var progressOverlay: some View {
        if case .loading(let message) = model.operationState {
            ZStack {
                Color.black.opacity(0.18).ignoresSafeArea()
                ProgressView(message)
                    .padding()
                    .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 8))
                    .accessibilityIdentifier("operation.progress")
            }
        }
    }

    private var canSign: Bool {
        model.readiness.available && model.document != nil && model.certificate != nil
    }

    private func verificationTitle(_ result: VerificationSummary) -> String {
        if result.reason == "revocación no concluyente" { return revocationTitle() }
        guard result.valid else { return "Firma no válida" }
        if hasAccreditedVerification(result) {
            return "Firma válida y confiable"
        }
        if result.integrityStatus.lowercased() == "valid" {
            return "Integridad válida; certificado o confianza no acreditados"
        }
        return "Verificación incompleta"
    }

    private func verificationIcon(_ result: VerificationSummary) -> String {
        if result.reason == "revocación no concluyente" { return "questionmark.diamond.fill" }
        guard result.valid else { return "xmark.seal.fill" }
        return hasAccreditedVerification(result)
            ? "checkmark.seal.fill"
            : "questionmark.diamond.fill"
    }

    private func verificationColor(_ result: VerificationSummary) -> Color {
        if result.reason == "revocación no concluyente" { return .orange }
        guard result.valid else { return .red }
        return hasAccreditedVerification(result) ? .green : .orange
    }

    private func revocationTitle() -> String {
        Locale.current.language.languageCode?.identifier == "en" ? "Revocation not established" : "Revocación no acreditada"
    }

    private func revocationMessage() -> String {
        Locale.current.language.languageCode?.identifier == "en"
            ? "Certificate revocation could not be checked. The signature is not considered valid."
            : "No se pudo comprobar la revocación del certificado. La firma no se considera válida."
    }

    private func hasAccreditedVerification(_ result: VerificationSummary) -> Bool {
        result.integrityStatus.lowercased() == "valid" &&
        result.certificateStatus.lowercased() == "valid" &&
        result.trustStatus.lowercased() == "valid"
    }

    private var certificateTypes: [UTType] {
        let types = [UTType(filenameExtension: "p12"), UTType(filenameExtension: "pfx")].compactMap { $0 }
        return types.isEmpty ? [.data] : types
    }

    private var certificateSheetPresented: Binding<Bool> {
        Binding(
            get: { model.pendingCertificateImport != nil },
            set: { if !$0 { model.cancelCertificateImport() } }
        )
    }

    private func handleDocumentImport(_ result: Result<[URL], Error>) {
        switch result {
        case .success(let urls):
            if let url = urls.first { model.importDocument(from: url) }
        case .failure(let error):
            if (error as NSError).code != NSUserCancelledError {
                model.alert = UserFacingAlert(title: "No se pudo importar", message: error.localizedDescription)
            }
        }
    }

    private func handleCertificateSelection(_ result: Result<[URL], Error>) {
        switch result {
        case .success(let urls):
            if let url = urls.first { model.prepareCertificateImport(from: url) }
        case .failure(let error):
            if (error as NSError).code != NSUserCancelledError {
                model.alert = UserFacingAlert(title: "No se pudo abrir el certificado", message: error.localizedDescription)
            }
        }
    }
}
