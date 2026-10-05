// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import Foundation

struct UserFacingAlert: Identifiable, Equatable {
    let id = UUID()
    let title: String
    let message: String
}

@MainActor
final class AppModel: ObservableObject {
    @Published private(set) var readiness = CoreReadiness.checking
    @Published private(set) var document: SecureDocument?
    @Published private(set) var signedArtifact: SignedArtifact?
    @Published private(set) var certificate: CertificateSummary?
    @Published private(set) var verification: VerificationSummary?
    @Published private(set) var operationState: OperationState = .idle
    @Published var selectedAction: SignatureAction = .sign
    @Published var selectedFormat = "CAdES"
    @Published var pendingApproval: SignApproval?
    @Published var pendingDeepLink: AFirmaDeepLinkRequest?
    @Published private(set) var pendingCertificateImport: URL?
    @Published var alert: UserFacingAlert?
    @Published private(set) var hasPendingRemoteUpload = false
    @Published private(set) var supportsRemoteExchange = false

    private let core: any CoreBridging
    private let repository: DocumentRepository?
    private let secureState: SecureStateStore?
    private var activeTask: Task<Void, Never>?
    private var remoteSession: PendingRemoteSession?
    private var requestedAlgorithm: String?
    private var coreCapabilities = CoreCapabilities.unavailable

    init(core: any CoreBridging, repository: DocumentRepository?, secureState: SecureStateStore?) {
        self.core = core
        self.repository = repository
        self.secureState = secureState
    }

    static func bootstrap() -> AppModel {
        do {
            let configuration = try AppConfiguration.load()
            let repository = try DocumentRepository(configuration: configuration)
            let keychain = KeychainStore(
                service: "io.github.aavidad.grxfirma.state",
                accessGroup: nil
            )
            let secureState = SecureStateStore(keychain: keychain)
            return AppModel(
                core: GomobileCoreBridge(configuration: configuration),
                repository: repository,
                secureState: secureState
            )
        } catch {
            let message = Self.message(error)
            return AppModel(
                core: UnavailableCoreBridge(reason: message),
                repository: nil,
                secureState: nil
            )
        }
    }

    func start() {
        replaceTask { [weak self] in
            guard let self else { return }
            let readiness = await self.core.readiness()
            guard !Task.isCancelled else { return }
            let capabilities = await self.core.capabilities()
            guard !Task.isCancelled else { return }
            self.readiness = readiness
            self.coreCapabilities = capabilities
            self.supportsRemoteExchange = self.coreCapabilities.remoteExchange
            if let secureState = self.secureState {
                if self.coreCapabilities.persistentIdentity {
                    let storedCertificate = try? await secureState.loadCertificate()
                    guard !Task.isCancelled else { return }
                    self.certificate = storedCertificate
                } else {
                    try? await secureState.clearCertificate()
                    guard !Task.isCancelled else { return }
                }
            }
            if let repository = self.repository {
                try? await repository.purgeExpiredFiles()
                guard !Task.isCancelled else { return }
                if let shared = try? await repository.consumeSharedInbox(), let first = shared.first {
                    guard !Task.isCancelled else {
                        await repository.remove(first)
                        return
                    }
                    self.replaceDocument(first)
                    self.operationState = .completed("Documento recibido desde Compartir.")
                }
            }
        }
    }

    func applicationDidEnterBackground() {
        activeTask?.cancel()
        activeTask = nil
        let stagedCertificate = pendingCertificateImport
        pendingCertificateImport = nil
        pendingApproval = nil
        pendingDeepLink = nil
        remoteSession = nil
        requestedAlgorithm = nil
        hasPendingRemoteUpload = false
        operationState = .idle
        if !coreCapabilities.persistentIdentity {
            certificate = nil
        }
        let core = self.core
        let repository = self.repository
        let secureState = self.secureState
        let persistentIdentity = coreCapabilities.persistentIdentity
        Task {
            await repository?.removeStagedCertificate(stagedCertificate)
            await core.clearSession()
            if !persistentIdentity {
                try? await secureState?.clearCertificate()
            }
        }
    }

    func importDocument(from url: URL) {
        guard let repository else {
            show(AppError.storage("El almacenamiento protegido no está disponible."))
            return
        }
        replaceTask { [weak self] in
            guard let self else { return }
            self.operationState = .loading("Importando documento…")
            do {
                let imported = try await repository.importDocument(from: url)
                guard !Task.isCancelled else {
                    await repository.remove(imported)
                    return
                }
                self.replaceDocument(imported)
                self.operationState = .completed("Documento protegido e importado.")
            } catch is CancellationError {
                return
            } catch {
                self.operationState = .idle
                self.show(error)
            }
        }
    }

    func selectCertificate() {
        guard readiness.available else {
            show(AppError.coreUnavailable(readiness.detail))
            return
        }
        replaceTask { [weak self] in
            guard let self else { return }
            self.operationState = .loading("Seleccionando certificado…")
            do {
                let payload = try CoreJSONCodec.selectCertificatePayload()
                let json = try await self.core.selectCertificate(payload: payload)
                let selected = try CoreJSONCodec.decodeSelectedCertificate(json)
                if self.coreCapabilities.persistentIdentity {
                    try await self.secureState?.saveCertificate(selected)
                }
                try Task.checkCancellation()
                self.certificate = selected
                self.operationState = .completed("Certificado seleccionado.")
            } catch is CancellationError {
                return
            } catch {
                self.operationState = .idle
                self.show(error)
            }
        }
    }

    func importCertificate(from url: URL, password: SensitiveText) {
        pendingCertificateImport = nil
        guard let repository else {
            show(AppError.storage("El staging protegido no está disponible."))
            return
        }
        guard readiness.available else {
            Task { await repository.removeStagedCertificate(url) }
            show(AppError.coreUnavailable(readiness.detail))
            return
        }
        guard DocumentPolicy.isCertificateFile(url) else {
            Task { await repository.removeStagedCertificate(url) }
            show(AppError.invalidDocument("Seleccione un certificado PKCS#12 con extensión .p12 o .pfx."))
            return
        }
        replaceTask { [weak self] in
            guard let self else { return }
            var protectedPassword = password
            self.operationState = .loading("Importando certificado…")
            defer { protectedPassword.reset() }
            do {
                let data = try await repository.readStagedCertificate(url)
                try Task.checkCancellation()
                let payload = try CoreJSONCodec.importCertificatePayload(
                    data: data,
                    password: protectedPassword.consume()
                )
                let json = try await self.core.importCertificate(payload: payload)
                let imported = try CoreJSONCodec.decodeImportedCertificate(json)
                if self.coreCapabilities.persistentIdentity {
                    try await self.secureState?.saveCertificate(imported)
                }
                try Task.checkCancellation()
                self.certificate = imported
                self.operationState = .completed("Certificado importado en el almacén seguro.")
            } catch is CancellationError {
                // El núcleo borra la identidad de sesión al entrar en segundo plano.
            } catch {
                self.operationState = .idle
                self.show(error)
            }
            await repository.removeStagedCertificate(url)
        }
    }

    func prepareCertificateImport(from url: URL) {
        guard let repository else {
            show(AppError.storage("El almacenamiento protegido no está disponible."))
            return
        }
        replaceTask { [weak self] in
            guard let self else { return }
            self.operationState = .loading("Protegiendo certificado…")
            do {
                let staged = try await repository.stageCertificate(from: url)
                guard !Task.isCancelled else {
                    await repository.removeStagedCertificate(staged)
                    return
                }
                let previous = self.pendingCertificateImport
                self.pendingCertificateImport = staged
                self.operationState = .idle
                await repository.removeStagedCertificate(previous)
            } catch is CancellationError {
                return
            } catch {
                self.operationState = .idle
                self.show(error)
            }
        }
    }

    func cancelCertificateImport() {
        let staged = pendingCertificateImport
        pendingCertificateImport = nil
        Task { await repository?.removeStagedCertificate(staged) }
    }

    func requestSignatureApproval() {
        guard readiness.available else {
            show(AppError.coreUnavailable(readiness.detail))
            return
        }
        guard let document, let certificate else {
            show(AppError.operationRejected("Seleccione un documento y un certificado antes de firmar."))
            return
        }
        pendingApproval = SignApproval(
            nonce: UUID(),
            documentID: document.id,
            certificateID: certificate.certificateID,
            action: selectedAction,
            format: selectedFormat,
            algorithm: requestedAlgorithm,
            expiresAt: Date().addingTimeInterval(60)
        )
    }

    func executeApprovedSignature(_ approval: SignApproval) {
        guard pendingApproval == approval else {
            show(AppError.operationRejected("La confirmación ya fue usada o dejó de estar activa."))
            return
        }
        pendingApproval = nil
        guard approval.isValid(),
              let document,
              document.id == approval.documentID,
              let certificate,
              certificate.certificateID == approval.certificateID,
              let repository
        else {
            show(AppError.operationRejected("La confirmación caducó o ya no corresponde al documento visible."))
            return
        }
        replaceTask { [weak self] in
            guard let self else { return }
            self.operationState = .loading("Firmando documento…")
            do {
                let data = try await repository.read(document)
                try Task.checkCancellation()
                let payload = try CoreJSONCodec.signPayload(
                    document: document,
                    data: data,
                    action: approval.action,
                    format: approval.format,
                    certificateID: certificate.certificateID,
                    algorithm: approval.algorithm
                )
                let json = try await self.core.sign(payload: payload)
                let decoded = try CoreJSONCodec.decodeSigned(json)
                guard decoded.3 == certificate.certificateID else {
                    throw AppError.operationFailed("El núcleo firmó con un certificado distinto del confirmado.")
                }
                guard decoded.1.caseInsensitiveCompare(approval.format) == .orderedSame else {
                    throw AppError.operationFailed("El formato devuelto no coincide con el confirmado.")
                }
                if let requested = approval.algorithm,
                   approval.format.caseInsensitiveCompare("PAdES") != .orderedSame,
                   Self.normalizedAlgorithm(decoded.2) != Self.normalizedAlgorithm(requested) {
                    throw AppError.operationFailed("El algoritmo devuelto no coincide con el confirmado.")
                }
                try Task.checkCancellation()
                let stored = try await repository.storeSigned(
                    decoded.0,
                    sourceName: document.displayName,
                    format: decoded.1,
                    algorithm: decoded.2,
                    certificateID: decoded.3
                )
                guard !Task.isCancelled else {
                    await repository.remove(stored)
                    return
                }
                let previous = self.signedArtifact
                self.signedArtifact = stored
                self.operationState = .completed("Firma completada. Revise el resultado antes de compartirlo.")
                await repository.remove(previous)
            } catch is CancellationError {
                return
            } catch {
                self.operationState = .idle
                self.show(error)
            }
        }
    }

    func verifyDocument() {
        guard readiness.available, let document, let repository else {
            show(AppError.operationRejected("Seleccione un documento y compruebe el núcleo antes de verificar."))
            return
        }
        replaceTask { [weak self] in
            guard let self else { return }
            self.operationState = .loading("Verificando firma…")
            do {
                let data = try await repository.read(document)
                try Task.checkCancellation()
                let payload = try CoreJSONCodec.verifyPayload(document: document, data: data)
                let json = try await self.core.verify(payload: payload)
                let verification = try CoreJSONCodec.decodeVerification(json)
                try Task.checkCancellation()
                self.verification = verification
                self.operationState = .completed("Verificación completada.")
            } catch is CancellationError {
                return
            } catch {
                self.operationState = .idle
                self.show(error)
            }
        }
    }

    func receiveDeepLink(_ url: URL) {
        do {
            pendingDeepLink = try AFirmaDeepLinkParser.parse(url)
        } catch {
            show(error)
        }
    }

    func receiveExternalURL(_ url: URL) {
        if url.isFileURL {
            if DocumentPolicy.isCertificateFile(url) {
                prepareCertificateImport(from: url)
            } else {
                importDocument(from: url)
            }
            return
        }
        receiveDeepLink(url)
    }

    func acceptPendingDeepLink() {
        guard let request = pendingDeepLink, let repository, let secureState else {
            show(AppError.operationRejected("No hay una solicitud afirma:// pendiente."))
            return
        }
        pendingDeepLink = nil
        replaceTask { [weak self] in
            guard let self else { return }
            self.operationState = .loading("Preparando solicitud del portal…")
            do {
                if request.remoteSession != nil && !self.coreCapabilities.remoteExchange {
                    throw AppError.coreUnavailable(
                        "El núcleo iOS actual no permite recuperar ni subir documentos remotos."
                    )
                }
                try await secureState.assertFreshAndRecord(requestID: request.requestID)
                try Task.checkCancellation()
                let data: Data
                if let inline = request.inlineData {
                    data = inline
                } else if let session = request.remoteSession {
                    let payload = try CoreJSONCodec.retrievePayload(session)
                    let json = try await self.core.retrieve(payload: payload)
                    data = try CoreJSONCodec.decodeRetrievedDocument(json, expectedRequestID: request.requestID)
                } else {
                    throw AppError.invalidDeepLink("La solicitud no contiene un origen documental.")
                }
                let imported = try await repository.importRemoteDocument(
                    data,
                    name: request.fileName,
                    mimeType: "application/octet-stream"
                )
                guard !Task.isCancelled else {
                    await repository.remove(imported)
                    return
                }
                self.replaceDocument(imported)
                self.remoteSession = request.remoteSession
                self.hasPendingRemoteUpload = request.remoteSession != nil
                self.selectedFormat = request.format
                self.requestedAlgorithm = request.algorithm
                if let action = request.operation.signatureAction {
                    self.selectedAction = action
                    self.operationState = .completed("Solicitud preparada. La firma aún requiere confirmación explícita.")
                } else {
                    self.operationState = .completed("Solicitud preparada para verificación.")
                }
            } catch is CancellationError {
                return
            } catch {
                self.operationState = .idle
                self.show(error)
            }
        }
    }

    func uploadSignedResult() {
        guard let session = remoteSession, let artifact = signedArtifact, let repository else {
            show(AppError.operationRejected("No hay un resultado remoto listo para enviar."))
            return
        }
        replaceTask { [weak self] in
            guard let self else { return }
            self.operationState = .loading("Enviando resultado al portal…")
            do {
                let data = try await repository.read(artifact)
                try Task.checkCancellation()
                let payload = try CoreJSONCodec.uploadPayload(session, data: data)
                let json = try await self.core.upload(payload: payload)
                try CoreJSONCodec.decodeUploadResponse(json)
                try Task.checkCancellation()
                self.remoteSession = nil
                self.hasPendingRemoteUpload = false
                self.operationState = .completed("El portal confirmó la recepción del resultado.")
            } catch is CancellationError {
                return
            } catch {
                self.operationState = .idle
                self.show(error)
            }
        }
    }

    func cancelPendingApproval() {
        pendingApproval = nil
    }

    func cancelPendingDeepLink() {
        pendingDeepLink = nil
    }

    private func replaceDocument(_ newDocument: SecureDocument) {
        let previousDocument = document
        let previousArtifact = signedArtifact
        let repository = self.repository
        document = newDocument
        signedArtifact = nil
        verification = nil
        pendingApproval = nil
        remoteSession = nil
        requestedAlgorithm = nil
        hasPendingRemoteUpload = false
        selectedFormat = DocumentPolicy.suggestedFormat(for: newDocument)
        Task {
            await repository?.remove(previousDocument)
            await repository?.remove(previousArtifact)
        }
    }

    private func replaceTask(_ operation: @escaping @MainActor () async -> Void) {
        activeTask?.cancel()
        activeTask = Task { await operation() }
    }

    private func show(_ error: Error) {
        alert = UserFacingAlert(title: "No se pudo completar la operación", message: Self.message(error))
    }

    private static func message(_ error: Error) -> String {
        let raw = (error as? LocalizedError)?.errorDescription ?? error.localizedDescription
        let scalars = raw.unicodeScalars.filter { !CharacterSet.controlCharacters.contains($0) }
        return String(scalars.map(String.init).joined().prefix(500))
    }

    private static func normalizedAlgorithm(_ value: String) -> String {
        value.filter { $0.isLetter || $0.isNumber }.uppercased()
    }
}
