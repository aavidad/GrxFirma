// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import SwiftUI

@main
struct GrxFirmaApp: App {
    @Environment(\.scenePhase) private var scenePhase
    @StateObject private var model = AppModel.bootstrap()
    @State private var privacyShielded = false

    var body: some Scene {
        WindowGroup {
            ContentView(model: model)
                .onOpenURL { model.receiveExternalURL($0) }
                .task { model.start() }
                .overlay {
                    if privacyShielded {
                        Color(uiColor: .systemBackground)
                            .ignoresSafeArea()
                            .accessibilityHidden(true)
                    }
                }
        }
        .onChange(of: scenePhase) { phase in
            privacyShielded = phase != .active
            if phase == .background {
                model.applicationDidEnterBackground()
            } else if phase == .active {
                model.start()
            }
        }
    }
}
