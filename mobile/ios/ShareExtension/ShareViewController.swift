// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import UIKit
import UniformTypeIdentifiers

@MainActor
final class ShareViewController: UIViewController {
    private let statusLabel = UILabel()
    private let progress = UIActivityIndicatorView(style: .large)
    private let finishButton = UIButton(type: .system)
    private let cancelButton = UIButton(type: .system)
    private var importTask: Task<Void, Never>?
    private var copiedFiles: [URL] = []
    private var importFailed = false

    override func viewDidLoad() {
        super.viewDidLoad()
        configureView()
        importTask = Task { await importAttachments() }
    }

    deinit {
        importTask?.cancel()
    }

    private func configureView() {
        view.backgroundColor = .systemBackground
        statusLabel.text = "Preparando documentos…"
        statusLabel.font = .preferredFont(forTextStyle: .body)
        statusLabel.adjustsFontForContentSizeCategory = true
        statusLabel.numberOfLines = 0
        statusLabel.textAlignment = .center
        statusLabel.accessibilityIdentifier = "share.status"

        progress.startAnimating()
        progress.accessibilityLabel = "Importando documentos"

        finishButton.configuration = .filled()
        finishButton.setTitle("Finalizar", for: .normal)
        finishButton.isHidden = true
        finishButton.accessibilityIdentifier = "share.finish"
        finishButton.addTarget(self, action: #selector(finish), for: .touchUpInside)

        cancelButton.setTitle("Cancelar", for: .normal)
        cancelButton.accessibilityIdentifier = "share.cancel"
        cancelButton.addTarget(self, action: #selector(cancel), for: .touchUpInside)

        let stack = UIStackView(arrangedSubviews: [progress, statusLabel, finishButton, cancelButton])
        stack.axis = .vertical
        stack.alignment = .center
        stack.spacing = 20
        stack.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(stack)
        NSLayoutConstraint.activate([
            stack.leadingAnchor.constraint(greaterThanOrEqualTo: view.layoutMarginsGuide.leadingAnchor),
            stack.trailingAnchor.constraint(lessThanOrEqualTo: view.layoutMarginsGuide.trailingAnchor),
            stack.centerXAnchor.constraint(equalTo: view.centerXAnchor),
            stack.centerYAnchor.constraint(equalTo: view.centerYAnchor),
            statusLabel.widthAnchor.constraint(lessThanOrEqualToConstant: 420)
        ])
    }

    private func importAttachments() async {
        do {
            guard let appGroup = Bundle.main.object(forInfoDictionaryKey: "GrxFirmaAppGroup") as? String,
                  appGroup.hasPrefix("group."),
                  !appGroup.contains("$(")
            else {
                throw AppError.configuration("La extensión no tiene un App Group válido.")
            }
            let providers = (extensionContext?.inputItems as? [NSExtensionItem] ?? [])
                .flatMap { $0.attachments ?? [] }
                .filter { $0.hasItemConformingToTypeIdentifier(UTType.data.identifier) }
            guard !providers.isEmpty, providers.count <= AppConfiguration.maximumSharedDocuments else {
                throw AppError.invalidDocument("Seleccione un único documento por operación.")
            }
            var imported = 0
            for provider in providers {
                try Task.checkCancellation()
                copiedFiles.append(try await copy(provider: provider, appGroup: appGroup))
                try Task.checkCancellation()
                imported += 1
                statusLabel.text = "Importados \(imported) de \(providers.count)…"
            }
            progress.stopAnimating()
            progress.isHidden = true
            statusLabel.text = imported == 1
                ? "Documento protegido. Abra GrxFirma para revisarlo."
                : "\(imported) documentos protegidos. Abra GrxFirma para revisarlos."
            finishButton.isHidden = false
            cancelButton.isHidden = true
            UIAccessibility.post(notification: .announcement, argument: statusLabel.text)
        } catch is CancellationError {
            removeCopiedFiles()
            cancel()
        } catch {
            importFailed = true
            removeCopiedFiles()
            progress.stopAnimating()
            progress.isHidden = true
            statusLabel.text = Self.sanitizedMessage(error)
            finishButton.isHidden = false
            finishButton.setTitle("Cerrar", for: .normal)
            UIAccessibility.post(notification: .announcement, argument: statusLabel.text)
        }
    }

    private func copy(provider: NSItemProvider, appGroup: String) async throws -> URL {
        try await withCheckedThrowingContinuation { continuation in
            provider.loadFileRepresentation(forTypeIdentifier: UTType.data.identifier) { source, error in
                do {
                    if let error { throw error }
                    guard let source else {
                        throw AppError.invalidDocument("El proveedor no entregó un fichero.")
                    }
                    let destination = try ShareInboxWriter.copy(source: source, appGroup: appGroup)
                    continuation.resume(returning: destination)
                } catch {
                    continuation.resume(throwing: error)
                }
            }
        }
    }

    @objc private func finish() {
        if importFailed {
            cancel()
        } else {
            extensionContext?.completeRequest(returningItems: nil)
        }
    }

    @objc private func cancel() {
        importTask?.cancel()
        removeCopiedFiles()
        let error = NSError(
            domain: "es.dipgra.grxfirma.share",
            code: NSUserCancelledError,
            userInfo: [NSLocalizedDescriptionKey: "Importación cancelada por el usuario."]
        )
        extensionContext?.cancelRequest(withError: error)
    }

    private func removeCopiedFiles() {
        copiedFiles.forEach { try? FileManager.default.removeItem(at: $0) }
        copiedFiles.removeAll()
    }

    private static func sanitizedMessage(_ error: Error) -> String {
        let raw = (error as? LocalizedError)?.errorDescription ?? error.localizedDescription
        let scalars = raw.unicodeScalars.filter { !CharacterSet.controlCharacters.contains($0) }
        return String(scalars.map(String.init).joined().prefix(400))
    }
}
