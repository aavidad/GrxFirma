// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import QtQuick
import QtQuick.Window
import QtQuick.Controls 2.15
import QtQuick.Layouts 1.15
import QtQuick.Dialogs
import QtQml 2.15
import Qt.labs.settings 1.1
import "ThemeContrast.js" as Contrast
import "VerificationSigners.js" as Signers

Window {
    id: window
    visible: true
    width: 1200
    height: 850
    title: tr("GrxFirma")
    color: currentTheme.backgroundColor
    // Paleta común: casillas, interruptores, campos, menús y botones heredan los
    // colores del tema y no los del sistema (texto oscuro sobre fondo oscuro).
    palette.window: currentTheme.cardColor
    palette.windowText: currentTheme.textColor
    palette.base: currentTheme.cardColor
    palette.alternateBase: currentTheme.backgroundColor
    palette.text: currentTheme.textColor
    palette.placeholderText: currentTheme.secondaryTextColor
    palette.button: currentTheme.cardColor
    palette.buttonText: currentTheme.textColor
    palette.highlight: currentTheme.focusColor
    palette.highlightedText: Contrast.readableOn(currentTheme.focusColor, currentTheme.cardColor)
    // Fondo del panel lateral del selector de ficheros de Qt: en temas oscuros,
    // un gris en el que se leen su texto negro y el texto claro del tema.
    palette.light: Contrast.luminance(currentTheme.cardColor) > 0.5
                   ? Qt.tint(currentTheme.cardColor, Qt.rgba(0.5, 0.5, 0.5, 0.18))
                   : Contrast.balancedSurface(currentTheme.textColor)
    palette.midlight: Qt.tint(currentTheme.cardColor, Qt.rgba(0.5, 0.5, 0.5, 0.3))
    palette.mid: currentTheme.secondaryTextColor
    palette.dark: currentTheme.primaryColor
    palette.disabled.text: currentTheme.secondaryTextColor
    palette.disabled.windowText: currentTheme.secondaryTextColor
    palette.disabled.buttonText: currentTheme.secondaryTextColor
    property bool portalSealMode: typeof portalSeal !== "undefined" && portalSeal && portalSeal.active
    property bool portalSealDecision: false
    function portalSealSubmit(action) {
        if (!portalSealMode || portalSealDecision) return
        let placements = []
        if (action === "place") {
            if (previewGeometryPath !== portalSeal.documentPath ||
                    previewGeometryPage !== previewCurrentPage || pdfPageImage.source === "") return
            placements = [currentSealGeometry()]
        }
        const appearance = action === "place" ? {
            logo: sealStyle === "institutional" ? "institutional" : "text",
            opacityPercent: Math.round(signSealLogoOpacityPercent)
        } : ({})
        if (portalSeal.submit(action, placements, appearance)) portalSealDecision = true
    }
    property string ipcSocketPath: ""
    property string publicCertificateExportId: ""
    property string appLanguage: (typeof i18n !== "undefined" && i18n && i18n.locale) ? i18n.locale : "es"
    property bool sidebarCollapsed: false
    property int sidebarExpandedWidth: Math.max(220, Math.min(260, Math.round(window.width * 0.19)))
    property int sidebarCollapsedWidth: 74
    property int sidebarPreferredWidth: sidebarCollapsed ? sidebarCollapsedWidth : sidebarExpandedWidth
    property bool rightSidebarCollapsed: false
    // Por debajo de 960 px la barra derecha se pliega sola para dejar sitio al contenido;
    // al volver a ensanchar se despliega si la plegó la ventana y no la persona.
    readonly property bool narrowWindow: width < 960
    property bool rightSidebarAutoCollapsed: false
    function applyNarrowRightSidebar() {
        if (narrowWindow && !rightSidebarCollapsed) {
            rightSidebarAutoCollapsed = true
            rightSidebarCollapsed = true
        } else if (!narrowWindow && rightSidebarAutoCollapsed) {
            rightSidebarAutoCollapsed = false
            rightSidebarCollapsed = false
        }
    }
    onNarrowWindowChanged: applyNarrowRightSidebar()
    property bool signCertificatePanelCollapsed: true
    property bool selectedCertificateUsable: certificateCanSign(selectedCertData)
    property int rightSidebarExpandedWidth: 320
    property int rightSidebarCollapsedWidth: 74
    property int previewCurrentPage: 1
    property int previewTotalPages: 1
    property int previewDocumentIndex: 0
    property string pendingPreviewRequestId: ""
    property string pendingPreviewPath: ""
    property int pendingPreviewPage: 1
    property int previewRequestSequence: 0
    property string sealPreviewRequestId: ""
    property string sealPreviewImage: ""
    property string sealPreviewMessage: ""
    property string sealStyle: "institutional"
    property string smartcardMessage: ""
    property real previewPageWidthPoints: 0
    property real previewPageHeightPoints: 0
    property string previewGeometryPath: ""
    property int previewGeometryPage: 0
    property string pendingLocaleLanguage: appLanguage
    property string aboutApplicationVersion: {
        const injectedVersion = (typeof appVersion !== "undefined")
                ? String(appVersion || "").trim()
                : ""
        if (injectedVersion !== "" && injectedVersion.toLowerCase() !== "dev")
            return injectedVersion
        const qtVersion = String(Qt.application.version || "").trim()
        return qtVersion.toLowerCase() === "dev" ? "" : qtVersion
    }
    property string pendingReleaseNotes: ""
    property bool releaseNotesAcknowledge: false

    function openReleaseNotes(history) {
        releaseNotesAcknowledge = !history
        releaseNotesDialog.notesText = history ? releaseNotesText : pendingReleaseNotes
        releaseNotesDialog.open()
    }
    property bool checkForUpdates: appSettings.checkForUpdatesCached &&
                                  officialUpdateChecker.savedCheckForUpdates()
    property bool updateCheckAttempted: false
    property bool updateCheckInProgress: false
    property bool updateAvailable: false
    property bool updateEngineUnavailable: false
    property bool updateManualRequest: false
    property bool updateUsingDirect: false
    property double updateAttemptStartedAt: 0
    property string updateLatestVersion: ""
    property string updateReleaseUrl: ""
    property string updateDismissedVersion: ""
    // «Ahora no» oculta solo esa versión hasta el próximo arranque.
    readonly property bool updateNoticeDismissed: updateLatestVersion !== "" && updateLatestVersion === updateDismissedVersion
    property string updateNoticeMessage: ""
    property string updateStatusMessage: tr("Todavía no se ha comprobado si existe una versión nueva.")
    property string localTLSStartupNoticeState: ""
    property bool localTLSStartupNoticeShown: false

    function tr(key) {
        if (typeof i18n === "undefined" || !i18n) return key
        const currentLocale = i18n.locale
        if (currentLocale === "__noop__") return key
        return i18n.t(key)
    }

    function startPublicCertificateExport(fromProtect) {
        if (!isIpcMode) return
        if (!fromProtect) {
            if (!selectedCertData) return
            publicCertificateExportId = certificateId(selectedCertData)
            publicCertificateSaveDialog.open()
            return
        }
        const own = (certificates || []).filter(function(cert) {
            return cert.canSign || cert.needsUnlock
        })
        if (own.length === 0) {
            statusMessage = tr("No hay certificados propios disponibles.")
        } else if (own.length === 1) {
            publicCertificateExportId = certificateId(own[0])
            publicCertificateSaveDialog.open()
        } else {
            publicCertificateSelectDialog.open()
        }
    }

    // Como el QR: sin esquema se entiende https://.
    function normalizedCsvUrl(raw) {
        return portalSeal.normalizeVerificationUrl(String(raw || ""))
    }

    // Un mensaje por cada problema de la leyenda CSV, para saber qué corregir.
    function csvLegendError(field) {
        const code = signCSVCode.trim()
        const url = normalizedCsvUrl(signCSVUrl)
        if (!field || field === "csvCode") {
            if (code === "") return "csv.error.code_missing"
            if (code.length > 128 || portalSeal.hasControlOrFormat(code)) return "csv.error.code_invalid"
        }
        if (!field || field === "csvUrl") {
            if (signCSVUrl.trim() === "") return "csv.error.url_missing"
            if (url === "") return "csv.error.url_invalid"
            if (url.length > 2048 || / /.test(url) || !/^https:\/\//i.test(url) ||
                    !validQrUrl(url.replace("{csv}", encodeURIComponent(code))))
                return "csv.error.url_invalid"
        }
        if ((!field || field === "csvText") &&
                (signCSVText.length > 512 || portalSeal.hasControlOrFormat(signCSVText)))
            return "csv.error.text_invalid"
        return ""
    }

    property var signFieldErrors: ({})
    property string firstSignFieldError: ""
    onSignCSVEnabledChanged: { if (!signCSVEnabled) { validateSignField("csvCode"); validateSignField("csvUrl"); validateSignField("csvText") } }
    property var settingsFieldErrors: ({})
    onTsaEnabledChanged: { if (settingsFieldError("tsa")) validateSettingsField("tsa") }
    onProxyEnabledChanged: { if (settingsFieldError("proxyHost") || settingsFieldError("proxyPort")) { validateSettingsField("proxyHost"); validateSettingsField("proxyPort") } }
    onProxyTypeChanged: { if (settingsFieldError("proxyHost") || settingsFieldError("proxyPort")) { validateSettingsField("proxyHost"); validateSettingsField("proxyPort") } }

    function settingsFieldError(field) { return settingsFieldErrors[field] || "" }

    function validateSettingsField(field) {
        const errors = Object.assign({}, settingsFieldErrors)
        let key = ""
        if (field === "tsa" && tsaEnabled && !validTsaUrl(tsaUrl))
            key = "winui.parity.tsa.invalid"
        if (proxyEnabled && proxyType === "manual") {
            if (field === "proxyHost" && (!proxyHost.trim() || proxyHost.trim().length > 512 || /[\s\/@\\]/.test(proxyHost.trim()) || proxyHost.indexOf("://") >= 0))
                key = "validacion.proxy.host"
            if (field === "proxyPort" && (!Number.isInteger(proxyPort) || proxyPort < 1 || proxyPort > 65535))
                key = "validacion.proxy.puerto"
        }
        if (key) errors[field] = key
        else delete errors[field]
        settingsFieldErrors = errors
        return key === ""
    }

    function signFieldError(field) {
        return signFieldErrors[field] || ""
    }

    function validateSignField(field) {
        const errors = Object.assign({}, signFieldErrors)
        let key = ""
        if (signVisibleSeal && supportsVisibleSeal()) {
            if (field === "qr" && signQREnabled && normalizedQrUrl(signQRContent) === "")
                key = "sign.seal.qr_https_error"
            else if (field === "image" && sealStyle === "image" && localPathFromUrl(signSealImagePath) === "")
                key = "validacion.sello.imagen"
            else if (field === "pages" && !signSealAllPages && !parsePageSelection(signSealPages).ok)
                key = "validacion.sello.paginas"
            else if (signCSVEnabled && (field === "csvCode" || field === "csvUrl" || field === "csvText")) {
                key = csvLegendError(field)
            }
        }
        if (key) errors[field] = key
        else {
            delete errors[field]
            if (field === "pages") signSealPagesError = ""
        }
        signFieldErrors = errors
        firstSignFieldError = Object.keys(errors)[0] || ""
        return key === ""
    }

    function validateSignFields() {
        const fields = ["image", "qr", "csvCode", "csvUrl", "csvText", "pages"]
        for (let i = 0; i < fields.length; i++) validateSignField(fields[i])
        return firstSignFieldError === ""
    }

    function focusSignField(field) {
        activeTab = "firmar"
        Qt.callLater(function() {
            const target = field === "qr" ? signQRContentField
                    : field === "csvCode" ? csvCodeField
                    : field === "csvUrl" ? csvUrlField
                    : field === "csvText" ? csvTextField
                    : field === "pages" ? signSealPagesField : signSealImageButton
            if (!target) return
            const position = target.mapToItem(signMainOuterContent, 0, 0)
            signMainOuterScroll.ScrollBar.vertical.position = Math.max(0,
                    Math.min(1, (position.y - 24) / Math.max(1, signMainOuterContent.height - signMainOuterScroll.height)))
            target.forceActiveFocus()
        })
    }

    function showLocalTLSStartupNotice(status) {
        // El editor del sello del portal no conecta con navegadores: el aviso
        // solo taparía el editor.
        if (!status || localTLSStartupNoticeShown || portalSealMode)
            return
        const state = String(status.state || "")
        if (state !== "error" && !(state === "ready" && status.changed === true))
            return
        localTLSStartupNoticeState = state
        localTLSStartupNoticeShown = true
        localTLSStartupNoticeDialog.open()
    }

    function startDirectUpdateCheck() {
        updateEngineBudgetTimer.stop()
        updateEngineUnavailable = true
        updateUsingDirect = true
        const remaining = Math.max(1, 10000 - (Date.now() - updateAttemptStartedAt))
        if (remaining <= 1) {
            updateCheckInProgress = false
            console.warn("update: automatic check timed out")
            return
        }
        officialUpdateChecker.check(aboutApplicationVersion, remaining)
    }

    function requestUpdateCheck(manual) {
        if (updateCheckInProgress || (!manual && !checkForUpdates)) return
        updateCheckAttempted = true
        updateCheckInProgress = true
        updateManualRequest = manual
        updateAttemptStartedAt = Date.now()
        updateEngineUnavailable = !(backend && backend.checkUpdates &&
                                    (!isIpcMode || (backend.updateEngineAvailable &&
                                                    backend.updateEngineAvailable())))
        updateUsingDirect = updateEngineUnavailable
        if (manual) updateStatusMessage = tr("Consultando la última versión publicada en GitHub…")
        if (updateEngineUnavailable) officialUpdateChecker.check(aboutApplicationVersion, 10000)
        else {
            updateEngineBudgetTimer.start()
            backend.checkUpdates()
        }
    }

    function acceptUpdateResult(result) {
        const latest = String(result.ultima_version || result.version || "")
        const current = String(aboutApplicationVersion || "")
        const target = String(result.url || "")
        const newer = officialUpdateChecker.newer(current, latest)
        if (!newer) {
            if (updateManualRequest)
                updateStatusMessage = current === ""
                        ? tr("La última versión publicada es %1, pero este build de desarrollo no se puede comparar automáticamente.").arg(latest)
                        : tr("GrxFirma está actualizado (%1).").arg(current)
            return
        }
        if (!officialUpdateChecker.isOfficialReleaseUrl(target) ||
                !target.endsWith("/" + latest)) {
            console.warn("update: invalid release destination")
            return
        }
        updateLatestVersion = latest
        updateReleaseUrl = target
        updateAvailable = true
        updateNoticeMessage = tr("Hay una versión nueva de GrxFirma (%1). Tienes la %2.")
                .arg(latest).arg(current)
        if (updateEngineUnavailable)
            updateNoticeMessage += "\n" + tr("Además, el motor local no responde; instalar la versión nueva puede resolverlo.")
        updateStatusMessage = updateNoticeMessage
        if (!updateNoticeDismissed && residentAgent.hidden)
            residentAgent.notifyUpdate(updateNoticeMessage)
    }

    function openOfficialUpdateRelease() {
        const target = String(updateReleaseUrl || "").trim()
        if (!/^https:\/\/github\.com\/aavidad\/GrxFirma\/releases(?:$|\/tag\/[^/?#]+$)/.test(target) ||
                !officialUpdateChecker.isOfficialReleaseUrl(target)) {
            updateStatusMessage = tr("El enlace recibido no pertenece al repositorio oficial y se ha bloqueado. Repita la comprobación o reinstale la aplicación desde GitHub.")
            return
        }
        // GitHub publica SHA256SUMS; con firma de código se podrá automatizar la instalación.
        backend.openExternal(target)
    }

    function themeLabel(index) {
        return tr(themes[index].name)
    }

    function hashAlgorithmOptions() {
        return [
            { texto: tr("SHA-256"), valor: "SHA-256" },
            { texto: tr("SHA-1"), valor: "SHA-1" },
            { texto: tr("SHA-384"), valor: "SHA-384" },
            { texto: tr("SHA-512"), valor: "SHA-512" }
        ]
    }

    function hashFileFormatOptions() {
        return [
            { texto: tr("Hexadecimal"), valor: "hex" },
            { texto: tr("Base64"), valor: "base64" },
            { texto: tr("Binario"), valor: "bin" }
        ]
    }

    function hashDirectoryFormatOptions() {
        return [
            { texto: tr("XML"), valor: "xml" },
            { texto: tr("Texto"), valor: "txt" },
            { texto: tr("CSV"), valor: "csv" }
        ]
    }

    function proxySecretStoreBackendLabel(value) {
        const raw = String(value || "").trim().toLowerCase()
        switch (raw) {
        case "":
        case "none":
            return tr("Ninguno")
        case "libsecret":
            return tr("libsecret")
        case "secret-service":
            return tr("Secret Service")
        case "keychain":
            return tr("Keychain")
        case "credential-manager-dpapi":
            return tr("Administrador de credenciales + DPAPI")
        case "dpapi":
        case "dpapi-user":
            return tr("DPAPI de usuario")
        default:
            return value
        }
    }

    function proxySecretStorePlatformLabel(value) {
        const raw = String(value || "").trim().toLowerCase()
        switch (raw) {
        case "":
            return ""
        case "linux":
            return tr("Linux")
        case "windows":
            return tr("Windows")
        case "darwin":
        case "macos":
            return tr("macOS")
        case "android":
            return tr("Android")
        case "ios":
            return tr("iOS")
        default:
            return value
        }
    }

    function proxyRuntimeModeLabel(value) {
        const raw = String(value || "").trim().toLowerCase()
        switch (raw) {
        case "":
            return ""
        case "manual-secure-store":
            return tr("Proxy manual con credenciales protegidas")
        case "fail-closed":
            return tr("Proxy obligatorio para esta conexión")
        case "system":
            return tr("Configuración automática del sistema")
        case "manual":
            return tr("Configuración manual del proxy")
        case "direct":
        case "none":
        case "disabled":
            return tr("Sin proxy")
        default:
            return value
        }
    }

    function copyTextToClipboard(value) {
        const text = String(value || "")
        if (text === "") return
        clipboardProxy.text = text
        clipboardProxy.forceActiveFocus()
        clipboardProxy.selectAll()
        clipboardProxy.copy()
        clipboardProxy.text = ""
    }

    function scheduleLocaleMutation(language) {
        const normalized = String(language || "").trim()
        pendingLocaleLanguage = normalized !== "" ? normalized : "es"
        console.log("QML: locale diferido solicitado", pendingLocaleLanguage)
        localeMutationTimer.restart()
    }

    // --- TEMAS ---
    property int currentThemeIndex: 0
    property var themes: [
        {
            name: "Cristal Oscuro",
            backgroundColor: "#12141a",
            sidebarColor: "#0a0c10",
            cardColor: "#1c1f26",
            primaryColor: "#3498db",
            accentColor: "#2ecc71",
            textColor: "#ffffff",
            secondaryTextColor: "#bdc3c7",
            // Error y foco con contraste AA sobre cardColor y backgroundColor.
            errorColor: "#ff8a80",
            focusColor: "#3498db",
            borderOpacity: 0.1
        },
        {
            name: "Minimalista Luz",
            backgroundColor: "#f5f6fa",
            sidebarColor: "#ffffff",
            cardColor: "#ffffff",
            primaryColor: "#2980b9",
            accentColor: "#e74c3c",
            textColor: "#2c3e50",
            secondaryTextColor: "#5f6c6d",
            errorColor: "#b42318",
            focusColor: "#2980b9",
            borderOpacity: 0.2
        },
        {
            name: "Futurista",
            backgroundColor: "#050505",
            sidebarColor: "#000000",
            cardColor: "#0d0d0d",
            primaryColor: "#00f2ff",
            accentColor: "#bc00ff",
            textColor: "#ffffff",
            secondaryTextColor: "#00f2ff",
            errorColor: "#ff8a80",
            focusColor: "#00f2ff",
            borderOpacity: 0.3
        },
        {
            name: "Corporativo",
            backgroundColor: "#0d1b2a",
            sidebarColor: "#1b263b",
            cardColor: "#415a77",
            primaryColor: "#d98841",
            accentColor: "#778da9",
            textColor: "#ffffff",
            secondaryTextColor: "#e0e1dd",
            errorColor: "#ffc4bc",
            focusColor: "#ffd166",
            borderOpacity: 0.1
        },
        {
            name: "Neón Cyber",
            backgroundColor: "#0b0c10",
            sidebarColor: "#1f2833",
            cardColor: "#12141a",
            primaryColor: "#66fcf1",
            accentColor: "#c5c6c7",
            textColor: "#ffffff",
            secondaryTextColor: "#45a29e",
            errorColor: "#ff8a80",
            focusColor: "#66fcf1",
            borderOpacity: 0.2
        },
        {
            name: "Bosque Profundo",
            backgroundColor: "#131a13",
            sidebarColor: "#0f140f",
            cardColor: "#1a241a",
            primaryColor: "#2ecc71",
            accentColor: "#f1c40f",
            textColor: "#ecf0f1",
            secondaryTextColor: "#95a5a6",
            errorColor: "#ff8a80",
            focusColor: "#2ecc71",
            borderOpacity: 0.15
        },
        {
            name: "Atardecer Cálido",
            backgroundColor: "#2c191e",
            sidebarColor: "#1a0f12",
            cardColor: "#3a2228",
            primaryColor: "#ff6b6b",
            accentColor: "#feca57",
            textColor: "#fff9f9",
            secondaryTextColor: "#f6b9b9",
            errorColor: "#ff8a80",
            focusColor: "#feca57",
            borderOpacity: 0.2
        },
        {
            name: "Océano Profundo",
            backgroundColor: "#0a192f",
            sidebarColor: "#020c1b",
            cardColor: "#112240",
            primaryColor: "#64ffda",
            accentColor: "#ccd6f6",
            textColor: "#e6f1ff",
            secondaryTextColor: "#8892b0",
            errorColor: "#ff8a80",
            focusColor: "#64ffda",
            borderOpacity: 0.1
        },
        {
            name: "Vampiro Elegante",
            backgroundColor: "#110b0b",
            sidebarColor: "#000000",
            cardColor: "#1e0f0f",
            primaryColor: "#e81c4f",
            accentColor: "#8e0020",
            textColor: "#ffffff",
            secondaryTextColor: "#a68a8a",
            errorColor: "#ff8a80",
            focusColor: "#7fd4ff",
            borderOpacity: 0.25
        },
        {
            name: "Aurora Boreal",
            backgroundColor: "#1a1025",
            sidebarColor: "#0f0817",
            cardColor: "#241738",
            primaryColor: "#00ffcc",
            accentColor: "#b366ff",
            textColor: "#ffffff",
            secondaryTextColor: "#c2a3ff",
            errorColor: "#ff8a80",
            focusColor: "#00ffcc",
            borderOpacity: 0.15
        },
        {
            name: "Perla Lujosa",
            backgroundColor: "#faf9f7",
            sidebarColor: "#ffffff",
            cardColor: "#f0ebe1",
            primaryColor: "#d4af37",
            accentColor: "#b39030",
            textColor: "#2c2a26",
            secondaryTextColor: "#686359",
            errorColor: "#b42318",
            focusColor: "#1f5fa8",
            borderOpacity: 0.1
        },
        {
            name: "Ametista",
            backgroundColor: "#1f182b",
            sidebarColor: "#15101f",
            cardColor: "#2a213a",
            primaryColor: "#9b5de5",
            accentColor: "#f15bb5",
            textColor: "#f8f5fd",
            secondaryTextColor: "#baabcf",
            errorColor: "#ff8a80",
            focusColor: "#9b5de5",
            borderOpacity: 0.2
        },
        {
            name: "Terminal Hacker",
            backgroundColor: "#050a05",
            sidebarColor: "#000000",
            cardColor: "#0a140a",
            primaryColor: "#00ff00",
            accentColor: "#008800",
            textColor: "#00ff00",
            secondaryTextColor: "#00aa00",
            errorColor: "#ff8a80",
            focusColor: "#00ff00",
            borderOpacity: 0.3
        },
        {
            name: "Desierto Terracota",
            backgroundColor: "#2a1c18",
            sidebarColor: "#1f120e",
            cardColor: "#3a2822",
            primaryColor: "#e07a5f",
            accentColor: "#3d405b",
            textColor: "#f4f1de",
            secondaryTextColor: "#eab69f",
            errorColor: "#ff8a80",
            focusColor: "#8ecae6",
            borderOpacity: 0.15
        }
    ]

    property var currentTheme: themes[currentThemeIndex]
    property string activeTab: "firmar"
    property var certificates: []
    property int selectedCertIndex: -1
    property var selectedCertData: null
    property var certificateAccessOptions: ({ managers: [], importTargets: [] })
    property var temporaryCertificateIds: []
    property string pendingTemporaryCertificateId: ""
    property string pendingTemporaryRemovalId: ""
    property bool residentCredentialPurgePending: false
    property bool showNoCertificateHelp: true
    property string preferredCertificateId: ""
    property string defaultCertificateId: ""
    property string certificateFilterText: ""
    property bool rememberCertificateFilter: false
    property string currentFilePath: ""
    property var currentBatchPaths: []
    property string currentBatchDirectory: ""
    property string currentBatchOutputDir: ""
    property var currentBatchResults: []
    property string statusMessage: tr("Iniciando...")
    property string signAction: "sign"
    property string signFormat: ""
    property string signProfile: "baseline"
    property bool signStrictCompat: false
    property string signOverwrite: "rename"
    property string autoFormatPdf: "pades"
    property string autoFormatOoxml: "ooxml"
    property string autoFormatFacturae: "facturae"
    property string autoFormatOdf: "odf"
    property string autoFormatXml: "xades"
    property string autoFormatBinary: "cades"
    property bool multiCosignEnabled: false
    property string multiCosignPrimaryCertificateId: ""
    property var multiCosignCertificateIds: []
    property bool signVisibleSeal: false
    property int signSealPage: 1
    property string signSealPages: "1"
    property string signSealPagesError: ""
    property bool signSealAllPages: false
    property real signSealX: 0.62
    property real signSealY: 0.04
    property real signSealW: 0.34
    property real signSealH: 0.12
    property int signSealRotation: 0
    property bool sealRotationDragging: false
    property bool signSealPerPage: false
    property var signSealPlacements: ({})
    property bool loadingPageSeal: false
    property bool signSealKeepText: true
    property string signSealImagePath: ""
    property int signSealLogoOpacityPercent: 100
    // Idioma de los rótulos del sello: "" sigue a la interfaz; un código fijo
    // (p. ej. "es") se envía en sealLanguage y el motor lo impone.
    property string signSealLanguage: ""
    readonly property var sealLanguageCodes: ["es", "ca", "va", "eu", "gl", "en", "de", "fr", "pt", "it", "zh"]
    property var batchGlobalSealConfig: null
    property var batchSealOverrides: ({})
    property bool batchSealOverrideEnabled: false
    property bool applyingBatchSealConfig: false
    property string bundledSealLogoPath: (typeof bundledSealLogoResolvedPath !== "undefined" && bundledSealLogoResolvedPath && bundledSealLogoResolvedPath !== "")
                                           ? bundledSealLogoResolvedPath
                                           : "../assets/logo_firma_grxfirma_final.png"
    property string signQRContent: ""
    property bool signQREnabled: false
    property bool signCSVEnabled: false
    property string signCSVCode: ""
    property string signCSVUrl: ""
    property string signCSVText: ""
    property bool signCSVQR: false
    property string signReason: ""
    property string signLocation: ""
    property string signContactInfo: ""
    property string padesSubFilter: "etsi"
    property string facturaePolicyVersion: "3.1"
    property string facturaePolicyIdentifier: ""
    property string facturaePolicyIdentifierHash: ""
    property string facturaePolicyQualifier: ""
    property string facturaeSignerRole: "emisor"
    property string facturaeSignatureCity: ""
    property string facturaeSignatureProvince: ""
    property string facturaeSignaturePostalCode: ""
    property string facturaeSignatureCountry: ""
    property string currentOutputPath: ""
    property string currentOutputVerificationMessage: ""
    property var currentOutputVerificationDetails: null
    property string signResultKind: ""
    property string signResultPath: ""
    property string signResultCause: ""
    property bool signResultVerificationPending: false
    property int signResultGeneration: 0
    property string sessionLastDocumentPath: ""
    property var protectionRecipients: []
    property var protectSelectedRecipientIds: []
    property string protectInputPath: ""
    property string protectOutputPath: ""
    property string protectProfile: "compat"
    property string protectContainer: "json"
    property var protectResult: null
    property bool protectionInProgress: false
    property string unprotectInputPath: ""
    property string unprotectOutputPath: ""
    property var unprotectResult: null
    property bool unprotectionInProgress: false
    property bool signingInProgress: false
    // Firma remota CSC: el motor decide si está permitida y guarda la sesión.
    property bool cscAllowed: false
    // La política de la organización la prohíbe: se explica, no se ofrece.
    property bool cscProhibited: false
    property var cscState: ({})
    property var cscDiscovery: null
    property bool cscBusy: false
    property string cscMessage: ""
    property var cscPendingSecrets: null
    property var cscSecretCertificate: null
    property int cscSecretCertIndex: -1
    // Operación que espera el PIN/OTP remoto: "sign" (Firmar) o "protect"
    // («Proteger y firmar»).
    property string cscSecretPurpose: "sign"
    property int verificationPendingCount: 0
    property int hashPendingCount: 0
    // Se conserva el estado local hasta que terminen operaciones y diálogos.
    property bool restartBlocked: signingInProgress || verificationPendingCount > 0 ||
        hashPendingCount > 0 || autoVerificationInProgress || pendingAutoVerificationQueue.length > 0 ||
        protectionInProgress || unprotectionInProgress || activeDiagnosticInProgress ||
        settingsSaveInFlight || backendSettingsDirty || residentCredentialPurgePending ||
        fileDialog.visible || multiFileDialog.visible || batchDirectoryDialog.visible || batchOutputDirectoryDialog.visible ||
        saveFileDialog.visible || verifyFileDialog.visible || verifyOriginalFileDialog.visible || protectFileDialog.visible ||
        unprotectFileDialog.visible || verifyReportSaveDialog.visible || verifySummarySaveDialog.visible || verifyHtmlReportSaveDialog.visible || hashInputFileDialog.visible ||
        hashInputDirectoryDialog.visible || hashReferenceDialog.visible || localTLSStartupNoticeDialog.visible || aboutDialog.visible || releaseNotesDialog.visible ||
        signValidationErrorDialog.visible || signConfirmDialog.visible || multiCosignDialog.visible ||
        signedDocumentWarningDialog.visible || certificateValidationDialog.visible || certificateValidationSaveDialog.visible || supportIncidentSaveDialog.visible ||
        activeDiagnosticsConsentDialog.visible || supportIncidentSendDialog.visible || supportAssistantDialog.visible || unsavedSettingsDialog.visible ||
        p12FileDialog.visible || temporaryCertificateFileDialog.visible || guidedImportCertificateFileDialog.visible || sealImageFileDialog.visible ||
        recipientPublicFileDialog.visible || publicCertificateSaveDialog.visible || publicCertificateSelectDialog.visible || publicCertificateResultDialog.visible ||
        importPasswordDialog.visible || temporaryCertificatePasswordDialog.visible || guidedImportPasswordDialog.visible || certificateAccessDialog.visible

    property var pendingAutoVerificationQueue: []
    property bool autoVerificationInProgress: false
    property var autoVerificationContext: null
    property var pendingSealSignerSummaryContext: null
    property var pendingSignedDocumentWarningContext: null
    property bool skipSignedDocumentWarningOnce: false
    property string signedDocumentWarningSummary: ""
    property string supportAssistantMode: "usuario"
    property string supportAssistantGoal: "sign"
    property bool facturaeToolsEnabled: false
    property bool startupWithSession: Qt.platform.os === "linux" && isIpcMode && backend.startupEnabled()
    onFacturaeToolsEnabledChanged: if (!facturaeToolsEnabled && activeTab === "facturae") activeTab = "firmar"
    property string lastIncidentReportPath: ""
    property var activeFailureContext: null
    property var activeDiagnosticResult: null
    property bool activeDiagnosticInProgress: false
    property bool diagnosticTechnicalExpanded: false

    function clearSignResult() {
        signResultGeneration++
        signResultKind = ""
        signResultPath = ""
        signResultCause = ""
        signResultVerificationPending = false
        currentOutputVerificationMessage = ""
        currentOutputVerificationDetails = null
    }

    property bool verifactuInput: false
    Connections {
        target: (typeof isIpcMode !== "undefined" && isIpcMode) ? backend : null
        ignoreUnknownSignals: true
        function onVerifactuDetected(ok, result) {
            if (ok && result.inputPath === window.currentFilePath)
                window.verifactuInput = result.isVerifactu === true
        }
    }
    onCurrentFilePathChanged: {
        verifactuInput = false
        if (signFormat === "verifactu") signFormat = ""
        if (isIpcMode && currentFilePath.toLowerCase().endsWith(".xml")) backend.detectVeriFactu(currentFilePath)
        if (portalSealMode) {
            pendingPreviewRequestId = ""
            return
        }
        if (signResultKind !== "") clearSignResult()
        if (currentFilePath !== "" && !isBatchMode()) {
            rememberSessionDocumentPath(currentFilePath)
        }
        if (multiCosignEnabled && !supportsGuidedMultiCosignFormat()) {
            multiCosignEnabled = false
        }
        if (!isBatchMode() && currentFilePath !== "" && (currentOutputPath === "" || currentOutputPath.includes("_firmado"))) {
            suggestOutputPath(currentFilePath)
        }
        pendingPreviewRequestId = ""
        pendingPreviewPath = ""
        pendingPreviewPage = 1
        previewPageWidthPoints = 0
        previewPageHeightPoints = 0
        previewGeometryPath = ""
        previewGeometryPage = 0
        if (typeof pdfPageImage !== "undefined" && pdfPageImage) {
            pdfPageImage.source = ""
        }
        requestPdfPreview()
    }
    onCurrentBatchPathsChanged: { if (signResultKind !== "") clearSignResult() }
    onCurrentBatchDirectoryChanged: { if (signResultKind !== "") clearSignResult() }
    onCurrentOutputPathChanged: {
        if (signResultKind !== "" && currentOutputPath !== signResultPath)
            clearSignResult()
    }

    onActiveTabChanged: {
        seedSessionDocumentForActiveTab()
        if (activeTab !== "cifrar")
            clearTransientProtectionSecrets()
    }

    function localPathFromUrl(urlValue) {
        let path = String(urlValue === undefined || urlValue === null ? "" : urlValue)
        if (!path.startsWith("file://")) return path
        path = Qt.platform.os === "windows" ? path.substring(8) : path.substring(7)
        // Qt deja codificados «%», «#» o «?» en la URL: se deshace la codificación.
        try {
            return decodeURIComponent(path)
        } catch (e) {
            return path
        }
    }

    // URL file:// bien formada para una ruta local: cada tramo va codificado,
    // así que espacios, «#» o acentos no rompen la propuesta del diálogo.
    function fileUrlFromLocalPath(path) {
        const normalized = String(path || "").replace(/\\/g, "/")
        if (normalized === "") return ""
        const drive = /^[A-Za-z]:(\/|$)/.test(normalized)
        const encoded = normalized.split("/").map(function(part, index) {
            return drive && index === 0 ? part : encodeURIComponent(part)
        }).join("/")
        if (drive) return "file:///" + encoded
        return "file://" + (encoded.charAt(0) === "/" ? "" : "/") + encoded
    }

    // Carpeta en la que se propone guardar: la del documento si se conoce;
    // si no, Documentos del usuario.
    function suggestedSaveFolder(documentPath) {
        const folder = String(documentPath || "") !== "" ? dirname(documentPath) : ""
        if (folder !== "" && (folder.charAt(0) === "/" || /^[A-Za-z]:/.test(folder)))
            return folder
        return (typeof documentsFolderPath !== "undefined" && documentsFolderPath) ? String(documentsFolderPath) : ""
    }

    function suggestedSaveUrl(documentPath, fileName) {
        const folder = suggestedSaveFolder(documentPath)
        const name = basename(fileName)
        if (name === "") return folder !== "" ? fileUrlFromLocalPath(folder) : ""
        return fileUrlFromLocalPath(folder !== "" ? folder.replace(/\/$/, "") + "/" + name : name)
    }

    // Abre un diálogo de guardar con carpeta y nombre propuestos.
    function openSaveDialog(dialog, documentPath, fileName) {
        const folder = suggestedSaveFolder(documentPath)
        if (folder !== "")
            dialog.currentFolder = fileUrlFromLocalPath(folder)
        const url = suggestedSaveUrl(documentPath, fileName)
        if (url !== "" && basename(fileName) !== "")
            dialog.selectedFile = url
        dialog.open()
    }

    function fileStem(path) {
        const name = basename(path || "")
        const dot = name.lastIndexOf(".")
        return dot > 0 ? name.substring(0, dot) : name
    }

    function localPathsFromUrls(urls) {
        let paths = []
        for (let i = 0; i < urls.length; i++) {
            const path = localPathFromUrl(urls[i])
            if (path !== "") paths.push(path)
        }
        return paths
    }

    function basename(path) {
        if (!path || path === "") return ""
        let normalized = String(path).replace(/\\/g, "/")
        const idx = normalized.lastIndexOf("/")
        return idx === -1 ? normalized : normalized.substring(idx + 1)
    }

    function dirname(path) {
        if (!path || path === "") return ""
        let normalized = String(path).replace(/\\/g, "/")
        const idx = normalized.lastIndexOf("/")
        return idx === -1 ? "" : normalized.substring(0, idx)
    }

    function signedResultFolder(path) {
        const folder = dirname(path)
        if (/^[A-Za-z]:$/.test(folder)) return folder + "/"
        return folder !== "" ? folder : (String(path).startsWith("/") ? "/" : "")
    }

    function suggestVerificationReportName(path) {
        const stem = fileStem(path)
        return stem !== "" ? stem + ".verify.json" : ""
    }

    function suggestVerificationReportPath(path) {
        if (!path || path === "") return ""
        return suggestedSaveUrl(path, suggestVerificationReportName(path))
    }

    // Informe imprimible del motor: «documento-informe-verificacion.html».
    function suggestVerificationHtmlReportName(path) {
        const stem = fileStem(path)
        const suffix = tr("winui.parity.verify.filename")
        return (stem !== "" ? stem + "-" : "") + suffix + ".html"
    }

    function normalizeSealImagePath(path) {
        const raw = String(path || "")
        if (raw === "" || raw === bundledSealLogoPath) return raw
        const normalized = raw.replace(/\\/g, "/")
        if (normalized === "../assets/logo_firma_grxfirma_final.png"
                || normalized.endsWith("/assets/logo_firma_grxfirma_final.png")
                || normalized.endsWith("/logo_firma_grxfirma_final.png")) {
            return bundledSealLogoPath
        }
        return raw
    }

    function supportAssistantGoalOptions() {
        return [
            { texto: tr("Quiero firmar"), valor: "sign" },
            { texto: tr("Quiero validar"), valor: "verify" },
            { texto: tr("No veo mi certificado"), valor: "certificate" },
            { texto: tr("La firma ha fallado"), valor: "sign-failure" },
            { texto: tr("La validación ha fallado"), valor: "verify-failure" },
            { texto: tr("Quiero preparar una incidencia"), valor: "support" }
        ]
    }

    function supportAssistantKnownIssue() {
        const diag = currentFailureDiagnostic()
        const failureCode = String(diag.failureCode || "").trim().toUpperCase()
        const text = (String(statusMessage || "") + " " +
                      String(diag.technicalDetail || "") + " " +
                      String(diag.userMessage || "")).toLowerCase()
        if (text.indexOf("macoskeychain") !== -1) {
            return {
                known: true,
                title: tr("Incidencia conocida"),
                summary: tr("Ese error ya quedó corregido en versiones recientes de Linux/Windows."),
                fixedIn: "59d6832"
            }
        }
        if (text.indexOf("ya estaba firmado") !== -1 || text.indexOf("cofirm") !== -1) {
            return {
                known: true,
                title: tr("Incidencia conocida"),
                summary: tr("El flujo de documentos ya firmados se ha ido afinando para redirigir hacia cofirma."),
                fixedIn: ""
            }
        }
        if (failureCode === "SAF_26" || failureCode === "SAF_27") {
            return {
                known: true,
                title: tr("Incidencia conocida"),
                summary: tr("Ese código suele indicar un problema remoto o del servicio intermedio, no de la app local."),
                fixedIn: ""
            }
        }
        return { known: false }
    }

    function supportAssistantDefaultGoal() {
        if (activeTab === "verificar") {
            if (verificationOutcomeKind(verifyTab.verifyDetails) === "invalid") return "verify-failure"
            return "verify"
        }
        if (String(statusMessage || "").toLowerCase().indexOf("cert") !== -1) return "certificate"
        if (String(statusMessage || "").toLowerCase().indexOf("error") !== -1) return "sign-failure"
        return "sign"
    }

    function supportAssistantOperationLabel() {
        if (activeTab === "verificar") {
            if (verifyTab.verifyFilePath !== "") return basename(verifyTab.verifyFilePath)
            return tr("Ningún fichero de validación seleccionado")
        }
        if (currentBatchPaths.length > 0) return tr("%1 documento(s) en lote").arg(currentBatchPaths.length)
        if (currentFilePath !== "") return basename(currentFilePath)
        return tr("Ningún documento seleccionado")
    }

    function supportAssistantCurrentStep() {
        if (signingInProgress) return tr("Procesando firma")
        if (autoVerificationInProgress) return tr("Verificando resultado")
        if (activeTab === "verificar" && verifyTab.verifyFilePath !== "" && verifyTab.verifyDetails === null) return tr("Documento preparado para validar")
        if (activeTab === "verificar" && verifyTab.verifyDetails !== null) return tr("Validación completada")
        if (currentBatchPaths.length > 0 || currentFilePath !== "") return tr("Documento preparado")
        return tr("Preparación")
    }

    function localizeVisibleDiagnosticText(value) {
        const text = String(value || "").trim()
        if (text === "") return ""
        switch (text) {
        case "app_local":
            return tr("Aplicación en este equipo")
        case "certificate_store":
        case "certificate_or_device":
            return tr("Certificado o dispositivo de firma")
        case "network_proxy":
        case "environment":
            return tr("Conexión, proxy o configuración del equipo")
        case "local_web_service":
            return tr("Canal local entre navegador y aplicación")
        case "remote_service":
            return tr("Servicio externo")
        case "government_afirma":
            return tr("Plataforma pública @firma")
        case "Admisión de la petición":
            return tr("Admisión de la petición")
        case "Validación del protocolo":
            return tr("Validación del protocolo")
        case "Ejecución de la operación":
            return tr("Ejecución de la operación")
        case "unknown":
            return tr("Origen todavía no clasificado")
        default:
            break
        }
        return tr(text)
    }

    function activeDiagnosticCodeText(value) {
        const code = String(value || "")
        switch (code) {
        case "app_local":
            return tr("Aplicación en este equipo")
        case "certificate_or_device":
        case "certificate_store":
            return tr("Certificado o dispositivo de firma")
        case "network_proxy":
            return tr("Red, proxy o configuración del equipo")
        case "local_web_service":
            return tr("Canal local entre navegador y aplicación")
        case "network_or_remote":
            return tr("Parece un problema de conexión o de un servicio externo.")
        case "remote_service":
            return tr("Servicio externo")
        case "government_afirma":
            return tr("Plataforma pública @firma")
        case "local_backend_unavailable":
            return tr("Sin conexión con el motor")
        case "dns_unavailable":
            return tr("Error de conexión")
        case "remote_or_network_unreachable":
            return tr("Parece depender de red o de un servicio externo.")
        case "tls_validation_failed":
            return tr("Error de conexión")
        case "certificate_context":
            return tr("Certificado o dispositivo de firma")
        case "endpoint_reachable":
            return tr("Aún no hay evidencia suficiente para determinar el origen exacto.")
        case "insufficient_evidence":
            return tr("Aún no hay evidencia suficiente para determinar el origen exacto.")
        case "restart_local_backend":
            return tr("Reiniciar Backend")
        case "review_network_proxy":
            return tr("Revisa el último mensaje y comprueba si el problema parece de conexión, del certificado o de la app.")
        case "retry_or_contact_remote_service":
            return tr("Si el fallo depende de un servicio externo, documenta la incidencia y reintenta más tarde.")
        case "review_tls_proxy_or_service":
            return tr("Revisa el último mensaje y comprueba si el problema parece de conexión, del certificado o de la app.")
        case "review_certificate_or_device":
            return tr("Revisa los filtros, la vigencia y si el certificado sirve para firmar.")
        case "retry_and_export_incident":
            return tr("Si persiste, abre ayuda o exporta diagnóstico para soporte.")
        case "proxy_configuration":
            return tr("Configuración de proxy")
        case "local_channel":
            return tr("Canal con el motor local")
        case "dns_resolution":
            return tr("Resolución DNS")
        case "tcp_reachability":
            return tr("Conectividad TCP")
        case "tls_handshake":
            return tr("Negociación TLS")
        case "ok":
            return tr("OK")
        case "failed":
            return tr("Falló")
        case "observed":
            return tr("Observado")
        case "not_applicable":
            return tr("No aplica")
        default:
            return tr("No disponible")
        }
    }

    function currentFailureDiagnostic() {
        if (activeFailureContext !== null && activeFailureContext.diagnostic)
            return activeFailureContext.diagnostic
        return backend.lastDiagnostic || {}
    }

    function currentFailureRequestId() {
        if (activeFailureContext !== null)
            return String(activeFailureContext.requestId || "")
        return String(backend.lastRequestId || "")
    }

    function currentFailureTraceId() {
        if (activeFailureContext !== null)
            return String(activeFailureContext.traceId || "")
        return String(backend.lastTraceId || "")
    }

    function snapshotOperationDiagnostic(value) {
        const source = value || {}
        const snapshot = Object.assign({}, source)
        const rawSteps = source.steps || []
        snapshot.steps = []
        for (let i = 0; i < Math.min(rawSteps.length, 32); ++i)
            snapshot.steps.push(Object.assign({}, rawSteps[i] || {}))
        return snapshot
    }

    function diagnosticOwnerText(value) {
        const owner = String(value || "").trim().toLowerCase()
        switch (owner) {
        case "app_local":
        case "local_machine":
        case "team":
            return tr("Aplicación en este equipo")
        case "certificate_store":
        case "certificate_or_device":
        case "network_proxy":
        case "environment":
            return tr("Este equipo")
        case "browser":
        case "local_web_service":
            return tr("Canal local entre navegador y aplicación")
        case "portal":
        case "remote_service":
            return tr("Portal o sede")
        case "government_afirma":
        case "afirma":
            return tr("Plataforma pública @firma")
        default:
            return tr("Desconocido")
        }
    }

    function diagnosticStatusPresentation(value) {
        const status = String(value || "").trim().toLowerCase()
        switch (status) {
        case "success":
        case "ok":
            return {
                icon: "\u2713",
                text: tr("Correcto"),
                color: "#18794e",
                badge: Qt.rgba(24 / 255, 121 / 255, 78 / 255, 0.18)
            }
        case "failure":
        case "failed":
            return {
                icon: "\u2715",
                text: tr("Falló"),
                color: currentTheme.errorColor,
                badge: Qt.rgba(180 / 255, 35 / 255, 24 / 255, 0.18)
            }
        case "skipped":
        case "not_applicable":
            return {
                icon: "\u2014",
                text: tr("Omitido"),
                color: "#596579",
                badge: Qt.rgba(89 / 255, 101 / 255, 121 / 255, 0.18)
            }
        case "observed":
            return {
                icon: "\u2022",
                text: tr("Observado"),
                color: "#1769aa",
                badge: Qt.rgba(23 / 255, 105 / 255, 170 / 255, 0.18)
            }
        default:
            return {
                icon: "?",
                text: tr("No comprobado"),
                color: "#7a5d00",
                badge: Qt.rgba(122 / 255, 93 / 255, 0, 0.18)
            }
        }
    }

    function diagnosticVisualStep(step, activeProbe) {
        const raw = step || {}
        const presentation = diagnosticStatusPresentation(raw.status || raw.state)
        const owner = activeProbe ? "" : diagnosticOwnerText(raw.owner)
        const action = activeProbe ? "" : localizeVisibleDiagnosticText(raw.suggestedAction)
        return {
            code: String(raw.code || raw.detailCode || ""),
            label: activeProbe
                   ? activeDiagnosticCodeText(raw.kind)
                   : localizeVisibleDiagnosticText(raw.label || ""),
            status: String(raw.status || raw.state || "unknown"),
            statusIcon: presentation.icon,
            statusText: presentation.text,
            statusColor: presentation.color,
            statusBadgeColor: presentation.badge,
            statusBorderColor: presentation.color,
            statusTextColor: "#ffffff",
            ownerText: owner === ""
                       ? ""
                       : tr("Responsabilidad probable: %1").arg(owner),
            message: activeProbe
                     ? ""
                     : localizeVisibleDiagnosticText(raw.userMessage),
            actionText: action === ""
                        ? ""
                        : tr("Qué hacer ahora: %1").arg(action)
        }
    }

    function diagnosticVisualSteps() {
        const diagnostic = currentFailureDiagnostic()
        const rawSteps = diagnostic && diagnostic.steps ? diagnostic.steps : []
        const steps = []
        for (let i = 0; i < Math.min(rawSteps.length, 32); ++i)
            steps.push(diagnosticVisualStep(rawSteps[i], false))
        return steps
    }

    function activeDiagnosticVisualSteps() {
        const rawProbes = activeDiagnosticResult && activeDiagnosticResult.probes
                        ? activeDiagnosticResult.probes : []
        const steps = []
        for (let i = 0; i < Math.min(rawProbes.length, 16); ++i)
            steps.push(diagnosticVisualStep(rawProbes[i], true))
        return steps
    }

    function recordOperationFailure(kind, message, incidentPath) {
        if (activeDiagnosticInProgress
                || (backend.activeDiagnosticsRunning === true)) {
            backend.cancelActiveDiagnostics()
        }
        activeDiagnosticInProgress = false
        const diagnostic = snapshotOperationDiagnostic(backend.lastDiagnostic)
        diagnosticTechnicalExpanded = false
        activeFailureContext = {
            postFailure: true,
            kind: String(kind || "operation-failure"),
            message: String(message || ""),
            incidentPath: String(incidentPath || ""),
            incidentFileName: basename(String(incidentPath || "")),
            failureCategory: String(diagnostic.category || "unknown"),
            diagnostic: diagnostic,
            requestId: String(backend.lastRequestId || ""),
            traceId: String(backend.lastTraceId || "")
        }
        activeDiagnosticResult = null
        const failureGoal = String(kind || "").indexOf("verify") !== -1
                          ? "verify-failure"
                          : (String(kind || "").indexOf("sign") !== -1
                             ? "sign-failure" : "support")
        Qt.callLater(function() {
            if (window.activeFailureContext !== null
                    && !supportAssistantDialog.visible) {
                window.openSupportAssistant(failureGoal)
            }
        })
    }

    function clearOperationFailure() {
        if (activeDiagnosticInProgress
                || (backend.activeDiagnosticsRunning === true)) {
            backend.cancelActiveDiagnostics()
        }
        activeFailureContext = null
        activeDiagnosticResult = null
        activeDiagnosticInProgress = false
        diagnosticTechnicalExpanded = false
    }

    function activeDiagnosticContextPayload() {
        if (activeFailureContext === null)
            return ({ postFailure: false })
        return {
            postFailure: true,
            failureCategory: activeFailureContext.failureCategory || "unknown"
        }
    }

    function startActiveDiagnostics() {
        if (activeFailureContext === null || activeDiagnosticInProgress)
            return
        activeDiagnosticResult = null
        activeDiagnosticInProgress = true
        backend.runActiveDiagnostics(true, activeDiagnosticContextPayload())
        statusMessage = tr("Ejecutando diagnóstico activo acotado...")
    }

    function localizedStatusMessage() {
        return localizeVisibleDiagnosticText(window.statusMessage)
    }

    function supportAssistantResponsibility() {
        const diag = currentFailureDiagnostic()
        if (diag.responsibilityMessage) return localizeVisibleDiagnosticText(diag.responsibilityMessage)
        const status = String(statusMessage || "").toLowerCase()
        const verifyReason = verifyTab.verifyDetails && verifyTab.verifyDetails.reason ? String(verifyTab.verifyDetails.reason).toLowerCase() : ""
        const joined = status + " " + verifyReason
        if (joined.indexOf("proxy") !== -1 || joined.indexOf("tls") !== -1 || joined.indexOf("servidor") !== -1 || joined.indexOf("@firma") !== -1) {
            return tr("Parece un problema de conexión o de un servicio externo.")
        }
        if (joined.indexOf("cert") !== -1 || joined.indexOf("almac") !== -1 || joined.indexOf("caduc") !== -1) {
            return tr("Parece un problema con el certificado o con su acceso en este equipo.")
        }
        if (joined.indexOf("ipc") !== -1 || joined.indexOf("backend") !== -1 || joined.indexOf("conexión cerrada") !== -1) {
            return tr("Parece un problema interno de la aplicación en este equipo.")
        }
        return tr("Todavía no está claro dónde está el problema.")
    }

    function supportAssistantOwnerLabel() {
        const diag = currentFailureDiagnostic()
        if (diag && diag.likelyOwner)
            return diagnosticOwnerText(diag.likelyOwner)
        return tr("Desconocido")
    }

    function supportAssistantSuggestedAction() {
        const diag = currentFailureDiagnostic()
        if (diag.suggestedAction) return localizeVisibleDiagnosticText(diag.suggestedAction)
        switch (supportAssistantGoal) {
        case "verify":
            return tr("Valida el documento y revisa si falta el original o aparece algún aviso.")
        case "certificate":
            return tr("Revisa los filtros, la vigencia y si el certificado sirve para firmar.")
        case "sign-failure":
            return tr("Si sigue fallando, abre ayuda o guarda un resumen para soporte.")
        case "verify-failure":
            return tr("Si parece un fallo externo, anota lo ocurrido y vuelve a intentarlo más tarde.")
        case "support":
            return tr("Copia o guarda el resumen después de revisarlo.")
        default:
            return tr("Haz la operación y vuelve aquí si algo no queda claro.")
        }
    }

    function supportAssistantResolutionScope() {
        const diag = currentFailureDiagnostic()
        if (diag.userCanResolveDirectly === true) {
            return tr("Parece un problema que puedes revisar desde este equipo.")
        }
        if (diag.userCanResolveDirectly === false && diag.likelyOwner) {
            return tr("Parece un problema que no depende solo de tu equipo.")
        }
        return tr("Todavía no está claro si depende de tu equipo o de un servicio externo.")
    }

    function supportAssistantFriendlySummary() {
        const diag = currentFailureDiagnostic()
        if (isIpcMode
                && (supportAssistantGoal === "sign-failure" || supportAssistantGoal === "support")
                && activeFailureContext !== null
                && String(activeFailureContext.message || "").trim() !== "") {
            return localizeVisibleDiagnosticText(activeFailureContext.message)
        }
        if (diag.userMessage && (supportAssistantGoal === "sign-failure" || supportAssistantGoal === "verify-failure" || supportAssistantGoal === "support")) {
            return localizeVisibleDiagnosticText(diag.userMessage)
        }
        switch (supportAssistantGoal) {
        case "verify":
            if (verifyTab.verifyFilePath === "") return tr("Primero selecciona el fichero firmado que quieres validar.")
            if (verificationOutcomeKind(verifyTab.verifyDetails) === "trusted") return tr("La validación actual parece correcta. Si necesitas más detalle, puedes exportar el informe.")
            if (verificationOutcomeKind(verifyTab.verifyDetails) === "invalid") return tr("La validación ha encontrado problemas. Revisa el motivo y comprueba si falta el original o necesitas ayuda.")
            if (verificationOutcomeKind(verifyTab.verifyDetails) === "untrusted") return tr("Integridad válida; confianza no evaluada.")
            if (verifyTab.verifyDetails) return verificationOutcomeLabel(verifyTab.verifyDetails)
            return tr("Ya tienes preparado el flujo de validación. El siguiente paso es lanzar la comprobación.")
        case "certificate":
            if (window.certificates.length === 0) return tr("Ahora mismo no hay certificados disponibles. Revisa tu almacén, tu fichero P12 o el acceso al dispositivo.")
            return tr("Sí hay certificados disponibles, pero el actual puede no servir para esta operación.")
        case "sign-failure":
            return String(statusMessage || "") !== ""
                   ? tr("El último estado visible indica: %1").arg(localizeVisibleDiagnosticText(statusMessage))
                   : tr("No hay un fallo de firma concreto registrado todavía. Selecciona documento y certificado para reproducirlo.")
        case "verify-failure":
            if (verifyTab.verifyDetails && verifyTab.verifyDetails.reason) {
                return tr("La validación ha fallado o tiene incidencias: %1").arg(localizeVisibleDiagnosticText(verifyTab.verifyDetails.reason))
            }
            return tr("Todavía no hay una razón de validación disponible. Ejecuta la validación para obtener contexto.")
        case "support":
            return tr("Este asistente puede resumir el contexto actual y orientarte antes de exportar una incidencia.")
        default:
            if (currentFilePath === "" && currentBatchPaths.length === 0) return tr("Empieza seleccionando un documento o un lote de documentos para firmar.")
            if (selectedCertIndex < 0 && window.certificates.length > 0) return tr("Ya tienes documento, pero falta seleccionar un certificado adecuado para firmar.")
            if (window.certificates.length === 0) return tr("Falta un certificado utilizable para completar la firma.")
            return tr("El flujo de firma está preparado. Revisa operación, formato y certificado antes de continuar.")
        }
    }

    function supportAssistantSteps() {
        switch (supportAssistantGoal) {
        case "verify":
            return [
                tr("Selecciona el fichero firmado que quieres validar."),
                tr("Añade el original solo si la firma es detached o la validación lo pide."),
                tr("Lanza la validación y revisa si falta el original o aparece algún aviso.")
            ]
        case "certificate":
            return [
                tr("Comprueba si hay certificados cargados en la columna derecha."),
                tr("Revisa los filtros, la vigencia y si el certificado sirve para firmar."),
                tr("Si hace falta, importa tu certificado o abre el gestor de certificados.")
            ]
        case "sign-failure":
            return [
                tr("Confirma que el documento y el certificado siguen seleccionados."),
                tr("Revisa el último mensaje y comprueba si el problema parece de conexión, del certificado o de la app."),
                tr("Si sigue fallando, abre ayuda o guarda un resumen para soporte.")
            ]
        case "verify-failure":
            return [
                tr("Revisa la razón de validación y si falta el original."),
                tr("Comprueba si falta el original o si aparece algún aviso sobre confianza o certificado."),
                tr("Si parece un fallo externo, anota lo ocurrido y vuelve a intentarlo más tarde.")
            ]
        case "support":
            return [
                tr("Resume qué querías hacer y en qué paso te has quedado."),
                tr("Copia o guarda el resumen solo después de revisarlo."),
                tr("Si estás en modo experto, añade detalles técnicos útiles para soporte.")
            ]
        default:
            return [
                tr("Selecciona uno o varios documentos."),
                tr("Elige un certificado adecuado y revisa el formato de firma."),
                tr("Haz la operación y vuelve aquí si algo no queda claro.")
            ]
        }
    }

    function supportAssistantExportText() {
        const lines = []
        lines.push(tr("Resumen de incidencia"))
        lines.push(tr("Paso actual: %1").arg(window.supportAssistantCurrentStep()))
        lines.push(tr("Operación o fichero: %1").arg(window.supportAssistantOperationLabel()))
        lines.push(tr("Qué está pasando: %1").arg(window.supportAssistantFriendlySummary()))
        lines.push(tr("Responsabilidad probable: %1").arg(window.supportAssistantResponsibility()))
        lines.push(tr("Ámbito de resolución: %1").arg(window.supportAssistantResolutionScope()))
        lines.push(tr("Qué hacer ahora: %1").arg(window.supportAssistantSuggestedAction()))
        const failureDiagnostic = currentFailureDiagnostic()
        if (failureDiagnostic && failureDiagnostic.failureCode) {
            lines.push(tr("Código estable: %1").arg(failureDiagnostic.failureCode))
        }
        const failureRequestId = currentFailureRequestId()
        const failureTraceId = currentFailureTraceId()
        if (failureRequestId !== "") {
            lines.push(tr("RequestId: %1").arg(failureRequestId))
        }
        if (failureTraceId !== "") {
            lines.push(tr("TraceId: %1").arg(failureTraceId))
        }
        if (activeDiagnosticResult !== null) {
            lines.push("")
            lines.push(tr("Resultado del diagnóstico activo"))
            lines.push(tr("Qué está pasando: %1").arg(
                           activeDiagnosticCodeText(
                               activeDiagnosticResult.probableCause)))
            lines.push(tr("Responsabilidad probable: %1").arg(
                           activeDiagnosticCodeText(
                               activeDiagnosticResult.likelyOwner)))
            lines.push(tr("Siguiente acción recomendada: %1").arg(
                           activeDiagnosticCodeText(
                               activeDiagnosticResult.suggestedAction)))
        }
        const knownIssue = window.supportAssistantKnownIssue()
        if (knownIssue.known) {
            lines.push(tr("Problema conocido: %1").arg(knownIssue.summary))
            if (knownIssue.fixedIn && knownIssue.fixedIn !== "") {
                lines.push(tr("Corregido en: %1").arg(knownIssue.fixedIn))
            }
        }
        const steps = window.supportAssistantSteps()
        if (steps && steps.length) {
            lines.push("")
            lines.push(tr("Siguientes pasos"))
            for (let i = 0; i < steps.length; i++) {
                lines.push(String(i + 1) + ". " + steps[i])
            }
        }
        return lines.join("\n")
    }

    function supportIncidentIncludedData() {
        const included = [
            tr("Resumen de la operación"),
            tr("Versión y plataforma"),
            tr("Metadatos del certificado seleccionado"),
            tr("Nombres de fichero sin ruta completa"),
            tr("Cola de log redactada")
        ]
        if (activeDiagnosticResult !== null)
            included.push(tr("Diagnóstico activo"))
        return included
    }

    function supportIncidentOmittedData() {
        return [
            tr("Documentos originales"),
            tr("Rutas completas del perfil de usuario"),
            tr("Claves privadas"),
            tr("Certificados DER/PEM completos"),
            tr("Secretos, tokens y cabeceras sensibles")
        ]
    }

    function supportIncidentBundleFiles() {
        const txtPath = localPathFromUrl(suggestSupportIncidentPath())
        const base = txtPath !== "" ? txtPath : "incidencia_grxfirma.incident.txt"
        return [
            base,
            base.replace(/\.txt$/i, ".json"),
            base.replace(/\.txt$/i, ".logtail.txt"),
            base.replace(/\.txt$/i, ".preview.json"),
            base.replace(/\.txt$/i, ".manifest.json")
        ]
    }

    function supportIncidentPreviewPayload() {
        return {
            title: tr("Paquete de soporte listo para exportar"),
            summary: tr("Se incluirá un resumen saneado de la incidencia y un extracto técnico mínimo."),
            includedData: supportIncidentIncludedData(),
            omittedData: supportIncidentOmittedData(),
            consent: {
                required: true,
                previewShown: true,
                remoteSend: false
            }
        }
    }

    function currentOperationContextForIncident() {
        return {
            activeTab: activeTab,
            signAction: signAction,
            signFormat: signFormat,
            signProfile: signProfile,
            selectedCertIndex: selectedCertIndex,
            certificateSelected: selectedCertData !== null,
            inputFileName: basename(currentFilePath),
            outputFileName: basename(currentOutputPath),
            batchCount: currentBatchPaths.length,
            verifyFileName: basename(verifyTab.verifyFilePath),
            verifyOriginalFileName: basename(verifyTab.verifyOriginalPath),
            protectInputFileName: basename(protectInputPath),
            protectOutputFileName: basename(protectOutputPath),
            unprotectInputFileName: basename(unprotectInputPath),
            unprotectOutputFileName: basename(unprotectOutputPath),
            runtimeProxyMode: window.proxySecretStoreRuntimeMode || "",
            expertMode: backend.expertMode
        }
    }

    function currentEnvironmentContextForIncident() {
        return {
            platform: Qt.platform.os,
            appVersion: String(appVersion || Qt.application.version || "dev"),
            language: appLanguage,
            currentStep: supportAssistantCurrentStep(),
            operationLabel: supportAssistantOperationLabel(),
            requestId: currentFailureRequestId(),
            traceId: currentFailureTraceId(),
            diagnostic: currentFailureDiagnostic()
        }
    }

    function buildIncidentPayload(kind, userMessage, extra) {
        const knownIssue = supportAssistantKnownIssue()
        const payload = {
            schema: "grxfirma-incident-v1",
            generatedAt: new Date().toISOString(),
            kind: String(kind || "general"),
            userMessage: String(userMessage || ""),
            friendlySummary: supportAssistantFriendlySummary(),
            probableResponsibility: supportAssistantResponsibility(),
            resolutionScope: supportAssistantResolutionScope(),
            suggestedAction: supportAssistantSuggestedAction(),
            summaryText: supportAssistantExportText(),
            supportPreview: supportIncidentPreviewPayload(),
            environment: currentEnvironmentContextForIncident(),
            operation: currentOperationContextForIncident()
        }
        payload.verificationAvailable = verifyTab.verifyDetails !== null
        payload.lastOutputVerificationAvailable =
                window.currentOutputVerificationDetails !== null
        if (knownIssue.known) {
            payload.knownIssue = knownIssue
        }
        if (extra !== undefined && extra !== null) {
            payload.extra = extra
        }
        if (activeDiagnosticResult !== null && activeFailureContext !== null) {
            const activeResult = Object.assign({}, activeDiagnosticResult)
            activeResult.originIncidentFileName =
                    activeFailureContext.incidentFileName || ""
            payload.activeDiagnostic = activeResult
        }
        return payload
    }

    function persistIncidentReport(kind, userMessage, extra) {
        const payload = buildIncidentPayload(kind, userMessage, extra)
        const path = backend.saveIncidentReport(payload, kind)
        if (path && path !== "") {
            lastIncidentReportPath = path
        }
        return path
    }

    function currentSupportIncidentKind() {
        return supportAssistantGoal === "support" ? "support" : "sign-failure"
    }

    function currentSupportIncidentPayload() {
        return buildIncidentPayload(
                    currentSupportIncidentKind(),
                    supportAssistantFriendlySummary(),
                    { source: "supportAssistantSendDialog" })
    }

    function supportAssistantPrimaryActionLabel() {
        switch (supportAssistantGoal) {
        case "verify":
            if (activeTab !== "verificar") return tr("Ir a validar")
            if (verifyTab.verifyFilePath === "") return tr("Preparar validación")
            return tr("Validar ahora")
        case "certificate":
            if (selectedCertData !== null) return tr("Revisar certificado")
            return tr("Recargar certificados")
        case "sign-failure":
            return tr("Preparar incidencia")
        case "verify-failure":
            if (activeTab !== "verificar") return tr("Ir a validar")
            if (verifyTab.verifyFilePath !== "") return tr("Reintentar validación")
            return tr("Preparar validación")
        case "support":
            return tr("Preparar incidencia")
        default:
            if (activeTab !== "firmar") return tr("Ir a firmar")
            return tr("Abrir ayuda")
        }
    }

    function supportAssistantPrimaryActionHint() {
        switch (supportAssistantGoal) {
        case "verify":
            if (activeTab !== "verificar") return tr("Te lleva a la pestaña de validación.")
            if (verifyTab.verifyFilePath === "") return tr("Te deja en la validación lista para seleccionar el fichero.")
            return tr("Lanza la validación con el fichero actual.")
        case "certificate":
            if (selectedCertData !== null) return tr("Abre el resumen del certificado seleccionado.")
            return tr("Vuelve a consultar el catálogo de certificados.")
        case "sign-failure":
            return tr("Prepara un resumen útil para soporte.")
        case "verify-failure":
            if (activeTab !== "verificar") return tr("Te lleva a la validación para revisar el problema.")
            if (verifyTab.verifyFilePath !== "") return tr("Vuelve a ejecutar la validación con el contexto actual.")
            return tr("Te lleva a la validación para seleccionar el fichero.")
        case "support":
            return tr("Prepara un resumen útil para soporte.")
        default:
            if (activeTab !== "firmar") return tr("Te lleva a la pestaña de firma.")
            return tr("Abre la ayuda local de la aplicación.")
        }
    }

    function runSupportAssistantPrimaryAction() {
        switch (supportAssistantGoal) {
        case "verify":
            activeTab = "verificar"
            if (verifyTab.verifyFilePath !== "") {
                verifyTab.resetVerificationResult()
                if (verifyTab.verifyOriginalPath !== "") {
                    window.requestVerification(verifyTab.verifyFilePath, verifyTab.verifyOriginalPath)
                } else {
                    window.requestVerification(verifyTab.verifyFilePath)
                }
                window.statusMessage = tr("Validación lanzada desde el asistente.")
            } else {
                window.statusMessage = tr("Selecciona primero el fichero que quieres validar.")
            }
            supportAssistantDialog.close()
            return
        case "certificate":
            if (selectedCertData !== null) {
                openCurrentCertificateValidation()
            } else {
                backend.refreshCertificates()
                window.statusMessage = tr("Recargando certificados desde el asistente.")
            }
            supportAssistantDialog.close()
            return
        case "sign-failure":
        case "support":
            copyTextToClipboard(window.supportAssistantExportText())
            backend.exportDiagnosticReport()
            window.persistIncidentReport(supportAssistantGoal === "support" ? "support" : "sign-failure",
                                         window.statusMessage,
                                         { source: "assistant" })
            window.statusMessage = tr("Resumen de incidencia preparado desde el asistente.")
            return
        case "verify-failure":
            activeTab = "verificar"
            if (verifyTab.verifyFilePath !== "") {
                verifyTab.resetVerificationResult()
                if (verifyTab.verifyOriginalPath !== "") {
                    window.requestVerification(verifyTab.verifyFilePath, verifyTab.verifyOriginalPath)
                } else {
                    window.requestVerification(verifyTab.verifyFilePath)
                }
                window.statusMessage = tr("Reintentando la validación desde el asistente.")
            } else {
                window.statusMessage = tr("Selecciona el fichero firmado antes de reintentar la validación.")
            }
            supportAssistantDialog.close()
            return
        default:
            if (activeTab !== "firmar") {
                activeTab = "firmar"
                supportAssistantDialog.close()
            } else {
                backend.openHelpManual()
            }
            return
        }
    }

    function openSupportAssistant(goal) {
        supportAssistantMode = backend.expertMode ? "experto" : "usuario"
        supportAssistantGoal = goal && goal !== "" ? goal : supportAssistantDefaultGoal()
        diagnosticTechnicalExpanded = false
        supportAssistantDialog.open()
    }

    function openCurrentCertificateValidation() {
        window.openCertificateValidation(window.selectedCertData, false)
    }

    function previewInputPath() {
        if (currentBatchPaths.length > 0) {
            const idx = Math.max(0, Math.min(previewDocumentIndex, currentBatchPaths.length - 1))
            return currentBatchPaths[idx] || currentFilePath
        }
        return currentFilePath
    }

    function previewDocumentLabel() {
        const path = previewInputPath()
        return path !== "" ? basename(path) : tr("Sin documento")
    }

    function resetPreviewNavigation() {
        signSealPerPage = false
        signSealPlacements = ({})
        previewDocumentIndex = 0
        previewCurrentPage = previewPageFromSelection(signSealPages)
        previewTotalPages = 1
        previewPageWidthPoints = 0
        previewPageHeightPoints = 0
        previewGeometryPath = ""
        previewGeometryPage = 0
    }

    function captureSealConfig() {
        return {
            allPages: signSealAllPages,
            pages: String(signSealPages || "1"),
            page: signSealPage,
            x: clamp01(Number(signSealX)),
            y: clamp01(Number(signSealY)),
            w: clamp01(Number(signSealW)),
            h: clamp01(Number(signSealH)),
            rotation: signSealRotation,
            perPage: signSealPerPage,
            placements: signSealPlacements,
            keepText: signSealKeepText,
            imagePath: String(signSealImagePath || ""),
            logoOpacityPercent: signSealLogoOpacityPercent,
            pageWidth: previewPageWidthPoints,
            pageHeight: previewPageHeightPoints,
            previewPage: previewGeometryPage,
            totalPages: previewTotalPages,
            geometryPath: previewGeometryPath
        }
    }

    function applySealConfig(config) {
        if (!config) return
        applyingBatchSealConfig = true
        signSealAllPages = !!config.allPages
        signSealPages = String(config.pages || "1")
        signSealPage = Number(config.page || previewPageFromSelection(signSealPages))
        signSealX = clamp01(Number(config.x))
        signSealY = clamp01(Number(config.y))
        signSealW = clamp01(Number(config.w))
        signSealH = clamp01(Number(config.h))
        signSealRotation = Number(config.rotation || 0)
        signSealPerPage = !!config.perPage
        signSealPlacements = config.placements || ({})
        signSealKeepText = config.keepText === undefined ? true : !!config.keepText
        signSealImagePath = String(config.imagePath || "")
        signSealLogoOpacityPercent = config.logoOpacityPercent === undefined ? 100 : Number(config.logoOpacityPercent)
        signSealPagesError = ""
        const activePath = previewInputPath()
        if (String(config.geometryPath || "") === activePath
                && Number(config.pageWidth) > 0 && Number(config.pageHeight) > 0) {
            previewPageWidthPoints = Number(config.pageWidth)
            previewPageHeightPoints = Number(config.pageHeight)
            previewGeometryPath = activePath
            previewGeometryPage = Number(config.previewPage || 1)
            previewCurrentPage = previewGeometryPage
            previewTotalPages = Math.max(1, Number(config.totalPages || 1))
        } else {
            previewPageWidthPoints = 0
            previewPageHeightPoints = 0
            previewGeometryPath = ""
            previewGeometryPage = 0
            previewCurrentPage = previewPageFromSelection(signSealPages)
            previewTotalPages = 1
        }
        applyingBatchSealConfig = false
        Qt.callLater(syncPreviewFromSeal)
    }

    function resetBatchSealOverrides() {
        batchGlobalSealConfig = null
        batchSealOverrides = ({})
        batchSealOverrideEnabled = false
    }

    function saveCurrentBatchSealConfig() {
        if (currentBatchPaths.length <= 1 || applyingBatchSealConfig) return
        const path = previewInputPath()
        if (path === "") return
        const config = captureSealConfig()
        if (batchSealOverrideEnabled) {
            const updated = Object.assign({}, batchSealOverrides)
            updated[path] = config
            batchSealOverrides = updated
        } else {
            batchGlobalSealConfig = config
        }
    }

    function loadBatchSealConfigForCurrentDocument() {
        if (currentBatchPaths.length <= 1) return
        const path = previewInputPath()
        const override = batchSealOverrides[path]
        if (override) {
            batchSealOverrideEnabled = true
            applySealConfig(override)
        } else {
            batchSealOverrideEnabled = false
            applySealConfig(batchGlobalSealConfig)
        }
    }

    function setBatchSealOverrideEnabled(enabled) {
        if (currentBatchPaths.length <= 1) return
        if (!!enabled === batchSealOverrideEnabled) return
        const path = previewInputPath()
        if (enabled) {
            batchGlobalSealConfig = captureSealConfig()
            batchSealOverrideEnabled = true
            const existing = batchSealOverrides[path]
            applySealConfig(existing ? existing : batchGlobalSealConfig)
        } else {
            const updated = Object.assign({}, batchSealOverrides)
            delete updated[path]
            batchSealOverrides = updated
            batchSealOverrideEnabled = false
            applySealConfig(batchGlobalSealConfig)
        }
        if (signVisibleSeal) Qt.callLater(requestPdfPreview)
    }

    function goToPreviewDocument(index) {
        if (currentBatchPaths.length === 0) return
        const clamped = Math.max(0, Math.min(index, currentBatchPaths.length - 1))
        if (previewDocumentIndex === clamped && previewCurrentPage === 1) return
        saveCurrentBatchSealConfig()
        previewDocumentIndex = clamped
        previewCurrentPage = 1
        previewTotalPages = 1
        loadBatchSealConfigForCurrentDocument()
        Qt.callLater(requestPdfPreview)
    }

    function goToPreviewPage(page) {
        const maxPage = Math.max(1, previewTotalPages)
        const clamped = Math.max(1, Math.min(page, maxPage))
        if (previewCurrentPage === clamped) return
        sealDrawArea.cancel()
        savePageSeal()
        previewCurrentPage = clamped
        requestPdfPreview()
    }

    function selectedSealPages() {
        if (previewTotalPages < 1 || previewTotalPages > 128) return []
        if (signSealAllPages) return Array.from({length: previewTotalPages}, function(_, i) { return i + 1 })
        const result = []
        const parsed = parsePageSelection(signSealPages)
        if (!parsed.ok) return result
        String(parsed.normalized).split(",").forEach(function(part) {
            const bounds = part.split("-").map(Number)
            for (let page = bounds[0]; page <= (bounds[1] || bounds[0]) && result.length < 128; page++) {
                if (page >= 1 && page <= previewTotalPages && result.indexOf(page) < 0) result.push(page)
            }
        })
        return result
    }

    function currentSealGeometry() {
        return {page: previewCurrentPage, rect: {x: signSealX, y: signSealY, w: signSealW, h: signSealH}, rotation: signSealRotation}
    }

    function savePageSeal() {
        if (!signSealPerPage || loadingPageSeal || !signSealPlacements[String(previewCurrentPage)]) return
        const updated = Object.assign({}, signSealPlacements)
        updated[String(previewCurrentPage)] = currentSealGeometry()
        signSealPlacements = updated
    }

    function loadPageSeal() {
        if (!signSealPerPage) return
        const item = signSealPlacements[String(previewCurrentPage)]
        if (!item) return
        loadingPageSeal = true
        signSealX = item.rect.x; signSealY = item.rect.y
        signSealW = item.rect.w; signSealH = item.rect.h
        signSealRotation = item.rotation
        loadingPageSeal = false
        Qt.callLater(syncPreviewFromSeal)
    }

    function enablePerPageSeal() {
        const pages = selectedSealPages()
        if (pages.length === 0) { statusMessage = tr("sign.seal.page_limit"); return }
        const geometry = currentSealGeometry()
        const updated = ({})
        pages.forEach(function(page) { updated[String(page)] = {page: page, rect: Object.assign({}, geometry.rect), rotation: geometry.rotation} })
        signSealPlacements = updated
        signSealPerPage = true
    }

    function applySealToAllPages() {
        if (!signSealPerPage) enablePerPageSeal()
        if (previewTotalPages < 1 || previewTotalPages > 128) { statusMessage = tr("sign.seal.page_limit"); return }
        const geometry = currentSealGeometry()
        const updated = ({})
        for (let page = 1; page <= previewTotalPages; page++) updated[String(page)] = {page: page, rect: Object.assign({}, geometry.rect), rotation: geometry.rotation}
        signSealPlacements = updated
        signSealAllPages = true
    }

    function removeSealFromPage() {
        const updated = Object.assign({}, signSealPlacements)
        delete updated[String(previewCurrentPage)]
        signSealPlacements = updated
    }

    function addSealToPage() {
        const updated = Object.assign({}, signSealPlacements)
        updated[String(previewCurrentPage)] = currentSealGeometry()
        signSealPlacements = updated
    }

    function isBatchMode() {
        return currentBatchDirectory !== "" || currentBatchPaths.length > 1
    }

    // Un lote con OTP solo se firma si el prestador autoriza varias firmas
    // con un código (multisign) y el lote cabe. Con una carpeta el motor
    // cuenta los documentos y lo rechaza antes de firmar si no caben.
    function remoteBatchOtpBlockKey(certificate) {
        if (!window.isBatchMode() || !certificate || certificate.remoteOtp !== true)
            return ""
        const capacity = Number(certificate.remoteMultiSign || 1)
        if (window.multiCosignEnabled || !(capacity > 1))
            return "csc.error.otp_lote"
        if (currentBatchDirectory === "" && currentBatchPaths.length > capacity)
            return "csc.error.otp_lote_excede"
        return ""
    }

    function rememberSessionDocumentPath(path) {
        const normalized = String(path || "").trim()
        if (normalized === "")
            return
        sessionLastDocumentPath = normalized
    }

    function seedSessionDocumentForActiveTab() {
        if (sessionLastDocumentPath === "")
            return
        if (activeTab === "verificar" && verifyTab && verifyTab.verifyFilePath === "") {
            verifyTab.verifyFilePath = sessionLastDocumentPath
            verifyTab.verifyDetails = null
            return
        }
        if (activeTab === "cifrar" && protectInputPath === "") {
            protectInputPath = sessionLastDocumentPath
            protectResult = null
        }
    }

    function hasSigningSelection() {
        return currentFilePath !== "" || currentBatchDirectory !== "" || currentBatchPaths.length > 0
    }

    function clearBatchSelection() {
        currentBatchPaths = []
        currentBatchDirectory = ""
        currentBatchOutputDir = ""
        currentBatchResults = []
        resetBatchSealOverrides()
        resetPreviewNavigation()
    }

    function useSingleSelection(path) {
        clearSignResult()
        clearBatchSelection()
        currentFilePath = path
        resetPreviewNavigation()
    }

    function useMultiSelection(paths) {
        clearSignResult()
        currentBatchResults = []
        if (!paths || paths.length === 0) {
            clearBatchSelection()
            currentFilePath = ""
            return
        }
        if (paths.length === 1) {
            useSingleSelection(paths[0])
            return
        }
        currentBatchPaths = paths
        currentBatchDirectory = ""
        currentBatchOutputDir = ""
        resetBatchSealOverrides()
        currentFilePath = paths[0]
        currentOutputPath = ""
        resetPreviewNavigation()
    }

    function useDirectorySelection(path) {
        clearSignResult()
        currentBatchResults = []
        currentBatchPaths = []
        currentBatchDirectory = path
        currentBatchOutputDir = ""
        resetBatchSealOverrides()
        currentFilePath = ""
        currentOutputPath = ""
        resetPreviewNavigation()
    }

    function selectedInputsSummary() {
        if (currentBatchDirectory !== "") return tr("Carpeta seleccionada: %1").arg(basename(currentBatchDirectory))
        if (currentBatchPaths.length > 1) return tr("%1 ficheros seleccionados").arg(currentBatchPaths.length)
        if (currentFilePath !== "") return basename(currentFilePath)
        return tr("Arrastra o selecciona documentos")
    }

    function hasSuffix(path, suffixes) {
        for (let i = 0; i < suffixes.length; ++i) {
            if (path.endsWith(suffixes[i])) return true
        }
        return false
    }

    function detectInputDocumentKind(path) {
        if (!path || path === "") return "binary"
        const lower = path.toLowerCase()
        if (lower.endsWith(".pdf")) return "pdf"
        if (hasSuffix(lower, [".docx", ".docm", ".dotx", ".dotm", ".xlsx", ".xlsm", ".xltx", ".xltm", ".pptx", ".pptm", ".ppsx", ".ppsm"])) return "ooxml"
        if (hasSuffix(lower, [".odt", ".ods", ".odp", ".odg", ".odf"])) return "odf"
        if (lower.endsWith(".facturae.xml") || (lower.endsWith(".xml") && lower.indexOf("facturae") !== -1)) return "facturae"
        if (hasSuffix(lower, [".xml", ".xsig", ".dsig", ".xmlsig"])) return "xml"
        return "binary"
    }

    function autoFormatForInputPath(path) {
        switch (detectInputDocumentKind(path)) {
        case "pdf":
            return autoFormatPdf
        case "ooxml":
            return autoFormatOoxml
        case "facturae":
            return autoFormatFacturae
        case "odf":
            return autoFormatOdf
        case "xml":
            return autoFormatXml
        default:
            return autoFormatBinary
        }
    }

    function normalizedEnumSetting(value, fallback, allowed) {
        const raw = String(value === undefined || value === null ? "" : value).trim()
        if (raw === "") return fallback
        for (let i = 0; i < allowed.length; ++i) {
            if (raw === allowed[i]) return raw
        }
        return fallback
    }

    function effectiveSignFormat() {
        if (signFormat !== "") return signFormat
        const basePath = previewInputPath()
        return autoFormatForInputPath(basePath)
    }

    function supportsGuidedMultiCosignFormat() {
        const format = String(effectiveSignFormat() || "").toLowerCase()
        return format === "pades" || format === "odf" || format === "ooxml"
    }

    function suggestOutputPath(inputPath) {
        if (!inputPath || inputPath === "") return
        let idx = inputPath.lastIndexOf('.')
        let base = idx !== -1 ? inputPath.substring(0, idx) : inputPath
        let formato = effectiveSignFormat()
        if (formato === "pades") {
            currentOutputPath = base + "_firmado.pdf"
        } else if (formato === "xmldsig") {
            currentOutputPath = base + "_firmado.dsig"
        } else if (formato === "xades") {
            currentOutputPath = base + "_firmado.xsig"
        } else {
            currentOutputPath = base + "_firmado.p7s"
        }
    }

    function certificateId(cert) {
        if (!cert) return ""
        return String(cert.id || cert.fingerprint || cert.serialNumber || cert.subjectName || cert.subject || "")
    }

    function isDefaultCertificate(cert) {
        const id = certificateId(cert)
        return id !== "" && id === String(defaultCertificateId || "")
    }

    function certificateDisplayName(cert) {
        return cert ? (cert.subjectName || cert.subject || tr("Certificado")) : tr("Certificado")
    }

    function guidedCertificateAccessAvailable() {
        return (typeof isIpcMode !== "undefined" && isIpcMode
                && backend && backend.requestCertificateAccessOptions)
    }

    function isTemporaryCertificate(cert) {
        const id = certificateId(cert)
        return id !== "" && temporaryCertificateIds.indexOf(id) !== -1
    }

    function rememberTemporaryCertificate(id) {
        id = String(id || "")
        if (id === "" || temporaryCertificateIds.indexOf(id) !== -1) return
        let ids = temporaryCertificateIds.slice(0)
        ids.push(id)
        temporaryCertificateIds = ids
    }

    function forgetTemporaryCertificate(id) {
        const ids = temporaryCertificateIds || []
        let remaining = []
        for (let i = 0; i < ids.length; ++i) {
            if (ids[i] !== id) remaining.push(ids[i])
        }
        temporaryCertificateIds = remaining
    }

    function openGuidedCertificateAccess() {
        if (!guidedCertificateAccessAvailable()) {
            p12FileDialog.open()
            return
        }
        certificateAccessDialog.open()
        backend.requestCertificateAccessOptions()
    }

    function optionIndexById(items, id) {
        const target = String(id || "")
        const values = items || []
        for (let i = 0; i < values.length; ++i) {
            if (String(values[i].id || "") === target) return i
        }
        return values.length > 0 ? 0 : -1
    }

    function certificateMatchesFilter(cert) {
        const filter = String(certificateFilterText || "").trim().toLowerCase()
        if (filter === "") return true
        const haystack = [
            cert.subjectName || cert.subject || "",
            cert.issuerName || cert.issuer || "",
            cert.serialNumber || cert.nif || "",
            cert.fingerprint || "",
            cert.status || ""
        ].join(" ").toLowerCase()
        return haystack.indexOf(filter) !== -1
    }

    function certificateMatchesStructuredFilters(cert) {
        const selectedTypes = window.certificateTypeFilter || []
        if (selectedTypes.length > 0) {
            const certType = String(cert.tipo || "desconocido").trim().toLowerCase()
            if (selectedTypes.indexOf(certType) === -1) return false
        }
        if (window.certificateRequireNIF) {
            const nif = String(cert.nif || cert.serialNumber || "").trim()
            if (nif === "") return false
        }
        if (window.certificateRequireOrganization) {
            const org = String(cert.organizacion || "").trim()
            if (org === "") return false
        }
        if (window.activeTab !== "firmar" && window.useOnlySignatureCertificates && !cert.canSign && !cert.needsUnlock) {
            return false
        }
        return true
    }

    function filteredCertificates() {
        const items = window.certificates || []
        let out = []
        for (let i = 0; i < items.length; ++i) {
            const cert = items[i]
            if (window.activeTab !== "firmar" && !window.certsExpiredShow && cert.caducado) continue
            if (window.activeTab !== "firmar" && !window.certsInvalidShow && !cert.canSign && !cert.needsUnlock) continue
            if (!certificateMatchesStructuredFilters(cert)) continue
            if (certificateMatchesFilter(cert)) out.push(cert)
        }
        if (window.showDefaultCertificateFirst || window.showValidCertificatesFirst || window.showUsableCertificatesFirst || window.activeTab === "firmar") {
            out.sort(function(a, b) {
                const aSign = window.certificateCanSign(a) ? 1 : 0
                const bSign = window.certificateCanSign(b) ? 1 : 0
                if (aSign !== bSign) return bSign - aSign
                const aDefault = window.isDefaultCertificate(a) ? 1 : 0
                const bDefault = window.isDefaultCertificate(b) ? 1 : 0
                if (window.showDefaultCertificateFirst && aDefault !== bDefault) {
                    return bDefault - aDefault
                }
                const aValid = a && !a.caducado ? 1 : 0
                const bValid = b && !b.caducado ? 1 : 0
                if (window.showValidCertificatesFirst && aValid !== bValid) {
                    return bValid - aValid
                }
                const aUsable = a && a.canSign ? 1 : 0
                const bUsable = b && b.canSign ? 1 : 0
                if (window.showUsableCertificatesFirst && aUsable !== bUsable) {
                    return bUsable - aUsable
                }
                return 0
            })
        }
        return out
    }

    function signingCertificates() {
        const items = (window.certificates || []).slice(0)
        items.sort(function(a, b) {
            return Number(window.certificateCanSign(b)) - Number(window.certificateCanSign(a))
        })
        return items
    }

    function syncSelectionWithCurrentCertificateFilters() {
        const visible = filteredCertificates()
        if (!selectedCertData) {
            if (visible.length === 0) {
                clearCertificateSelection()
                sanitizeMultiCosignCertificates()
            }
            return
        }
        const currentVisibleIndex = findCertificateIndexById(certificateId(selectedCertData), visible)
        if (currentVisibleIndex !== -1) return
        if (visible.length === 0) {
            clearCertificateSelection()
            sanitizeMultiCosignCertificates()
            return
        }
        const firstVisibleIndex = findCertificateIndexById(certificateId(visible[0]), window.certificates)
        if (firstVisibleIndex !== -1 && !visible[0].needsUnlock) {
            selectCertificateIndex(firstVisibleIndex, false)
        } else {
            clearCertificateSelection()
        }
    }

    function setSelectedCertificateAsDefault() {
        if (!selectedCertData) return
        defaultCertificateId = certificateId(selectedCertData)
        markBackendSettingsDirty()
        saveBackendSettings()
        window.statusMessage = tr("Se ha establecido un certificado predeterminado.")
    }

    function clearDefaultCertificate() {
        defaultCertificateId = ""
        markBackendSettingsDirty()
        saveBackendSettings()
        window.statusMessage = tr("Se ha borrado el certificado predeterminado.")
    }

    function clearCertificateSelection() {
        selectedCertIndex = -1
        selectedCertData = null
    }

    function findCertificateIndexById(id, certs) {
        const target = String(id || "")
        if (target === "") return -1
        const items = certs || window.certificates || []
        for (let i = 0; i < items.length; ++i) {
            if (certificateId(items[i]) === target) return i
        }
        return -1
    }

    function selectCertificateIndex(index, persistPreference) {
        if (index < 0 || index >= window.certificates.length) {
            clearCertificateSelection()
            return
        }
        selectedCertIndex = index
        selectedCertData = window.certificates[index]
        if (window.multiCosignEnabled) {
            window.multiCosignPrimaryCertificateId = certificateId(selectedCertData)
        }
        normalizeMultiCosignPrimaryCertificate()
        sanitizeMultiCosignCertificates()
        if (persistPreference === true && stickySigner) {
            preferredCertificateId = certificateId(selectedCertData)
        }
    }

    function sanitizeMultiCosignCertificates() {
        const primaryId = effectiveMultiCosignPrimaryId()
        const items = window.certificates || []
        let seen = {}
        let cleaned = []
        for (let i = 0; i < multiCosignCertificateIds.length; ++i) {
            const id = String(multiCosignCertificateIds[i] || "")
            if (id === "" || id === primaryId || seen[id]) continue
            const index = findCertificateIndexById(id, items)
            if (index === -1 || !certificateCanSign(items[index])) continue
            seen[id] = true
            cleaned.push(id)
        }
        // Reasignar una lista igual dispararía el aviso de «Cambios sin guardar»
        // al cargar los certificados aunque el usuario no haya tocado nada.
        if (JSON.stringify(cleaned) !== JSON.stringify(multiCosignCertificateIds))
            multiCosignCertificateIds = cleaned
    }

    function sanitizeMultiCosignIdsForPrimary(primaryId, ids, certs) {
        const normalizedPrimary = String(primaryId || "")
        const items = certs || window.certificates || []
        const source = ids || []
        let seen = {}
        let cleaned = []
        for (let i = 0; i < source.length; ++i) {
            const id = String(source[i] || "")
            if (id === "" || id === normalizedPrimary || seen[id]) continue
            const index = findCertificateIndexById(id, items)
            if (index === -1 || !certificateCanSign(items[index])) continue
            seen[id] = true
            cleaned.push(id)
        }
        return cleaned
    }

    function availableAdditionalCertificates() {
        const primaryId = (typeof multiCosignDialog !== "undefined" && multiCosignDialog.visible)
                          ? String(multiCosignDialog.draftPrimaryId || "")
                          : effectiveMultiCosignPrimaryId()
        const items = window.certificates || []
        let out = []
        for (let i = 0; i < items.length; ++i) {
            if (certificateId(items[i]) === primaryId || !certificateCanSign(items[i])) continue
            out.push(items[i])
        }
        return out
    }

    function additionalCoSignerNames() {
        const items = window.certificates || []
        let names = []
        for (let i = 0; i < multiCosignCertificateIds.length; ++i) {
            const idx = findCertificateIndexById(multiCosignCertificateIds[i], items)
            if (idx !== -1) names.push(certificateDisplayName(items[idx]))
        }
        return names
    }

    function effectiveMultiCosignPrimaryId() {
        const items = window.certificates || []
        const explicitId = String(window.multiCosignPrimaryCertificateId || "")
        const explicitIndex = findCertificateIndexById(explicitId, items)
        if (explicitIndex !== -1 && certificateCanSign(items[explicitIndex])) return explicitId
        return certificateCanSign(selectedCertData) ? certificateId(selectedCertData) : ""
    }

    function effectiveMultiCosignPrimaryIndex() {
        const items = window.certificates || []
        const primaryId = effectiveMultiCosignPrimaryId()
        if (primaryId === "") return -1
        return findCertificateIndexById(primaryId, items)
    }

    function normalizeMultiCosignPrimaryCertificate() {
        const items = window.certificates || []
        const explicitId = String(window.multiCosignPrimaryCertificateId || "")
        const explicitIndex = findCertificateIndexById(explicitId, items)
        if (explicitIndex !== -1 && certificateCanSign(items[explicitIndex])) return
        window.multiCosignPrimaryCertificateId = certificateCanSign(selectedCertData) ? certificateId(selectedCertData) : ""
    }

    function multiCosignSummaryText() {
        const primaryId = effectiveMultiCosignPrimaryId()
        const primaryIndex = findCertificateIndexById(primaryId, window.certificates || [])
        const primaryName = primaryIndex !== -1 ? certificateDisplayName(window.certificates[primaryIndex]) : tr("No definido")
        const names = additionalCoSignerNames()
        if (names.length === 0) return tr("Principal: %1. Ningún certificado adicional seleccionado.").arg(primaryName)
        return tr("Principal: %1. %2 certificado(s) adicional(es): %3").arg(primaryName).arg(names.length).arg(names.join(", "))
    }

    function effectiveMultiCosignPrimaryName() {
        const items = window.certificates || []
        const primaryIndex = effectiveMultiCosignPrimaryIndex()
        if (primaryIndex !== -1) return certificateDisplayName(items[primaryIndex])
        return tr("No definido")
    }

    function multiCosignVisibleSealSummary() {
        if (!window.multiCosignEnabled) return ""
        const names = additionalCoSignerNames()
        if (names.length === 0) return ""
        return tr("Cofirmantes: %1").arg(names.join(", "))
    }

    function verificationSignerNames(details) {
        let names = []
        if (details && details.signers && details.signers.length > 0) {
            for (let i = 0; i < details.signers.length; i++) {
                const value = Signers.readableName(String(details.signers[i] || "").trim())
                if (value !== "" && names.indexOf(value) === -1)
                    names.push(value)
            }
        }
        if (details && details.signerSummaries && details.signerSummaries.length > 0) {
            for (let j = 0; j < details.signerSummaries.length; j++) {
                const item = details.signerSummaries[j]
                const value = Signers.readableName(String((item && item.subject) || "").trim())
                if (value !== "" && names.indexOf(value) === -1)
                    names.push(value)
            }
        }
        return names
    }

    function truncateSignerList(names, maxCount) {
        if (!names || names.length === 0)
            return ""
        const limit = Math.max(1, Number(maxCount || 3))
        const slice = names.slice(0, limit)
        if (names.length > limit)
            slice.push(tr("+%1 más").arg(names.length - limit))
        return slice.join(", ")
    }

    function currentCoSignSealSummary(previousSignerNames) {
        let rows = []
        const previous = previousSignerNames || []
        if (previous.length > 0)
            rows.push(tr("Previos: %1").arg(truncateSignerList(previous, 2)))

        if (window.multiCosignEnabled) {
            const primaryName = effectiveMultiCosignPrimaryName()
            if (primaryName && primaryName !== tr("No definido"))
                rows.push(tr("Cofirma: %1").arg(primaryName))
            const additional = additionalCoSignerNames()
            if (additional.length > 0)
                rows.push(tr("Después: %1").arg(truncateSignerList(additional, 2)))
        } else if (window.selectedCertData) {
            rows.push(tr("Cofirma: %1").arg(certificateDisplayName(window.selectedCertData)))
        }

        return rows.join(" | ")
    }

    function buildSignedDocumentWarningSummary(details) {
        let parts = [tr("El documento seleccionado ya contiene una firma electrónica.")]
        if (details && details.format)
            parts.push(tr("Formato detectado: %1").arg(details.format))
        const names = verificationSignerNames(details)
        if (names.length > 0)
            parts.push(tr("Firmantes detectados: %1").arg(names.join(", ")))
        parts.push(tr("Si quieres añadir otra firma sobre el mismo documento, lo correcto normalmente es usar Cofirmar."))
        return parts.join("\n")
    }

    function syncCertificateSelection(certs) {
        const items = certs || []
        if (items.length === 0) {
            clearCertificateSelection()
            sanitizeMultiCosignCertificates()
            return
        }

        const currentId = certificateId(selectedCertData)
        let idx = findCertificateIndexById(currentId, items)
        if (idx === -1 && multiCosignEnabled && multiCosignPrimaryCertificateId !== "") {
            idx = findCertificateIndexById(multiCosignPrimaryCertificateId, items)
        }
        if (idx === -1 && autoSelectSingleCertificate && items.length === 1) {
            idx = 0
        }
        if (idx === -1 && preferDefaultCertificate && defaultCertificateId !== "") {
            idx = findCertificateIndexById(defaultCertificateId, items)
        }
        if (idx === -1 && stickySigner && preferredCertificateId !== "") {
            idx = findCertificateIndexById(preferredCertificateId, items)
        }
        if (idx === -1 && !preferDefaultCertificate && defaultCertificateId !== "") {
            idx = findCertificateIndexById(defaultCertificateId, items)
        }

        // A remembered/default token is not fresh consent. Preserve an already
        // selected identity, but never silently pick a locked device on refresh.
        if (idx !== -1 && items[idx].needsUnlock && certificateId(items[idx]) !== currentId) {
            idx = -1
        }
        if (idx !== -1) {
            selectCertificateIndex(idx, false)
            normalizeMultiCosignPrimaryCertificate()
            return
        }

        clearCertificateSelection()
        multiCosignPrimaryCertificateId = ""
        sanitizeMultiCosignCertificates()
    }

    // Salvo en la comprobación automática tras firmar, se pide también el
    // informe imprimible (HTML) del motor en el idioma de la aplicación.
    function requestVerification(path, original, withReport) {
        verificationPendingCount += 1
        const originalPath = (original !== undefined && original !== null) ? String(original) : ""
        if (withReport !== false && backend && typeof backend.verifyFileWithReport === "function")
            backend.verifyFileWithReport(path, originalPath)
        else if (originalPath !== "")
            backend.verifyFileWithOriginal(path, originalPath)
        else
            backend.verifyFile(path)
    }

    function jumpToVerify(path) {
        if (!path || path === "") return
        rememberSessionDocumentPath(path)
        verifyTab.verifyFilePath = path
        activeTab = "verificar"
        window.requestVerification(path)
    }

    function buildCertificateValidationSummary(cert) {
        if (!cert) {
            return {
                title: tr("Verificación de certificado"),
                valid: false,
                message: tr("No hay ningún certificado seleccionado."),
                details: {
                    valid: false,
                    reason: tr("No hay ningún certificado seleccionado.")
                }
            }
        }

        const titular = cert.subjectName || cert.subject || tr("Certificado")
        const emisor = cert.issuerName || cert.issuer || "---"
        const estado = certificateStatusText(cert)
        const validoHasta = cert.validTo || cert.notAfter || "---"
        const dias = Number(cert.diasCaducidad || 0)
        const diasCaducado = cert.caducado ? Math.abs(dias) : 0
        const tipo = cert.tipo || tr("desconocido")
        const nif = cert.nif || cert.serialNumber || "---"
        let resumen = ""

        if (cert.caducado) {
            resumen = tr("El certificado está caducado y no debe usarse para firmar.")
        } else if (cert.needsUnlock) {
            resumen = tr("La tarjeta requiere autorización. Al firmar se pedirá el PIN o la confirmación en el lector; todavía no se ha comprobado el acceso a la clave privada.")
        } else if (!cert.canSign) {
            resumen = tr("El certificado no está marcado como utilizable para firma.")
        } else if (dias >= 0 && dias <= 60) {
            resumen = tr("El certificado es válido para firma, pero caduca pronto.")
        } else {
            resumen = tr("El certificado es válido y apto para firma.")
        }

        const detalle = [
            resumen,
            "",
            tr("Titular: ") + titular,
            tr("Emisor: ") + emisor,
            tr("Tipo: ") + tipo,
            tr("NIF/Serie: ") + nif,
            tr("Estado: ") + estado,
            tr("Válido hasta: ") + validoHasta
        ]

        if (cert.caducado) {
            detalle.push(tr("Días caducado: ") + diasCaducado)
        } else if (cert.canSign) {
            detalle.push(tr("Días restantes: ") + dias)
        }
        if (cert.fingerprint && cert.fingerprint !== "") {
            detalle.push(tr("Huella SHA-256: ") + formatFingerprintForDisplay(cert.fingerprint))
        }

        return {
            title: tr("Verificación de certificado"),
            valid: (!cert.caducado && cert.canSign),
            details: {
                valid: (!cert.caducado && cert.canSign),
                reason: resumen,
                certificate: (!cert.caducado && cert.canSign) ? tr("válido") : (cert.caducado ? tr("caducado") : (cert.needsUnlock ? tr("pendiente de autorización") : tr("no utilizable"))),
                trust: cert.status || estado,
                statusText: estado,
                validTo: validoHasta,
                daysRemaining: (!cert.caducado && cert.canSign) ? dias : 0,
                expiredDays: cert.caducado ? diasCaducado : 0,
                signerSummaries: [titular],
                details: detalle
            },
            message: detalle.join("\n"),
            reportText: [
                tr("Verificación de certificado"),
                ( !cert.caducado && cert.canSign ) ? tr("Certificado válido para firma") : tr("Certificado con incidencias"),
                "",
                detalle.join("\n")
            ].join("\n")
        }
    }

    function formatFingerprintForDisplay(value) {
        const raw = String(value || "").trim()
        if (raw === "") {
            return ""
        }
        const compact = raw.replace(/[^0-9a-fA-F]/g, "")
        if (compact.length < 16) {
            return raw
        }
        return compact.toUpperCase().match(/.{1,4}/g).join(" ")
    }

    function compactCertificateTooltip(cert) {
        if (!cert) {
            return tr("Certificado")
        }
        const subject = cert.subjectName || cert.subject || tr("Certificado")
        const issuer = cert.issuerName || cert.issuer || "---"
        const serial = cert.serialNumber || cert.nif || "---"
        const validTo = cert.validTo || cert.notAfter || tr("Desconocida")
        const fingerprint = cert.fingerprint ? formatFingerprintForDisplay(cert.fingerprint) : "---"
        const status = certificateStatusText(cert)
        return tr("<b>Titular:</b> ") + subject
            + "<br/>" + tr("<b>Emisor:</b> ") + issuer
            + "<br/>" + tr("<b>Nº Serie:</b> ") + serial
            + "<br/>" + tr("<b>Válido hasta:</b> ") + validTo
            + "<br/>" + tr("<b>Estado:</b> ") + status
            + "<br/>" + tr("<b>Huella:</b> ") + fingerprint
    }

    function certificateStatusText(cert) {
        const remoteMark = cert && cert.remote === true ? " · " + tr("csc.gui.remoto") : ""
        if (!certificateCanSign(cert)) return "⚠ " + tr("No válido") + remoteMark
        if (cert.validTo && Number(cert.diasCaducidad) >= 0 && Number(cert.diasCaducidad) <= 60)
            return tr("Caduca pronto") + remoteMark
        return tr("Válido") + remoteMark
    }

    function certificateNeedsRemoteSecrets(cert) {
        return !!cert && cert.remote === true && (cert.remotePin === true || cert.remoteOtp === true)
    }

    // El PIN y el OTP se entregan al bridge una sola vez y se olvidan aquí.
    function attachRemoteSecrets(payload) {
        const secrets = window.cscPendingSecrets
        window.cscPendingSecrets = null
        if (!secrets) return
        if (secrets.pin) payload.remotePin = secrets.pin
        if (secrets.otp) payload.remoteOtp = secrets.otp
    }

    function openRemoteSigningDialog() {
        window.cscMessage = ""
        cscServiceUrlField.text = String(window.cscState.serviceUrl || "")
        cscClientIdField.text = String(window.cscState.clientId || "")
        cscRemoteDialog.open()
        backend.cscStatus()
    }

    function openRemoteSecretDialog(selectedCertIndex, cert, purpose) {
        window.cscSecretCertIndex = selectedCertIndex
        window.cscSecretPurpose = purpose === "protect" ? "protect" : "sign"
        window.cscSecretCertificate = cert
        cscPinField.text = ""
        cscOtpField.text = ""
        cscSecretStatus.text = ""
        cscRemoteSecretDialog.open()
    }

    // «Proteger y firmar» con un certificado remoto que pide PIN u OTP los
    // pide con el mismo diálogo que Firmar y los entrega una sola vez.
    function executeProtectSignRequest() {
        const index = window.selectedCertIndex
        const cert = index >= 0 && index < window.certificates.length
                ? window.certificates[index] : null
        // Unos secretos escritos para otro certificado no se reutilizan nunca.
        if (window.cscPendingSecrets !== null && (!cert
                || String(window.cscPendingSecrets.certificateId) !== String(cert.id || ""))) {
            window.cscPendingSecrets = null
        }
        if (window.certificateNeedsRemoteSecrets(cert) && window.cscPendingSecrets === null) {
            window.openRemoteSecretDialog(index, cert, "protect")
            return
        }
        const payload = {
            profile: window.protectProfile,
            overwrite: "rename",
            saveToDisk: true,
            signToo: true,
            options: { container: "signedandenvelopeddata" }
        }
        if (cert && String(cert.id || "") !== "")
            payload.certificateId = String(cert.id)
        window.protectResult = null
        window.protectionInProgress = true
        window.attachRemoteSecrets(payload)
        backend.protectFileAdvanced(
            window.protectInputPath,
            window.protectOutputPath,
            window.protectSelectedRecipientIds,
            index,
            payload)
    }

    function clearRemoteSecretFields() {
        cscPinField.text = ""
        cscOtpField.text = ""
    }

    function certificateStatusReason(cert) {
        if (!cert) return tr("No hay certificado seleccionado.")
        if (cert.caducado || String(cert.status || "").toLowerCase() === "caducado")
            return tr("Caducado el %1").arg(certificateExpiry(cert))
        if (cert.needsUnlock) return tr("Requiere autorización de la tarjeta")
        const status = String(cert.status || "").trim()
        if (status !== "" && status.toLowerCase() !== "válido" && status.toLowerCase() !== "valido")
            return tr(status)
        if (!cert.canSign) return tr("No dispone de clave utilizable para firmar")
        return ""
    }

    function certificateCanSign(cert) {
        if (!cert || cert.caducado || !cert.canSign || cert.needsUnlock) return false
        const status = String(cert.status || "").trim().toLowerCase()
        return status === "" || status === "válido" || status === "valido"
    }

    function certificateStatusColor(cert) {
        return certificateStatusColorForBackground(cert, currentTheme.cardColor)
    }

    function certificateSummaryStatusColor(cert) {
        return certificateStatusColorForBackground(cert, currentTheme.sidebarColor)
    }

    function certificateStatusColorForBackground(cert, surface) {
        function luminance(color) {
            const hex = String(color || "#000000").replace("#", "")
            const rgb = [0, 2, 4].map(function(offset) {
                const value = parseInt(hex.substring(offset, offset + 2), 16) / 255
                return value <= 0.04045 ? value / 12.92 : Math.pow((value + 0.055) / 1.055, 2.4)
            })
            return 0.2126 * rgb[0] + 0.7152 * rgb[1] + 0.0722 * rgb[2]
        }
        const invalid = !certificateCanSign(cert)
        const expiresSoon = cert && cert.validTo && Number(cert.diasCaducidad) >= 0
                            && Number(cert.diasCaducidad) <= 60
        const dark = invalid ? "#750010" : expiresSoon ? "#5c3900" : "#064c2a"
        const light = invalid ? "#ffd9d5" : expiresSoon ? "#ffe7a0" : "#a8f5c1"
        const bg = luminance(surface)
        const a = luminance(dark)
        const b = luminance(light)
        const darkRatio = (Math.max(bg, a) + 0.05) / (Math.min(bg, a) + 0.05)
        const lightRatio = (Math.max(bg, b) + 0.05) / (Math.min(bg, b) + 0.05)
        return darkRatio >= lightRatio ? dark : light
    }

    function certificateExpiry(cert) {
        const raw = String((cert && (cert.validTo || cert.notAfter)) || "")
        const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(raw)
        return match ? match[3] + "/" + match[2] + "/" + match[1] : tr("Desconocida")
    }

    function canRenewFnmt(cert) {
        if (!cert || cert.caducado || !cert.canSign || !cert.validTo || cert.needsUnlock) return false
        const status = String(cert.status || "").trim().toLowerCase()
        if (status !== "" && status !== "válido" && status !== "valido") return false
        const days = Number(cert.diasCaducidad)
        const issuer = String(cert.issuerName || cert.issuer || "").toUpperCase()
        return days >= 0 && days <= 60 && issuer.indexOf("FNMT") >= 0
                && String(cert.tipo || "").toLowerCase() === "fisica"
    }

    function normalizedQrUrl(value) {
        return portalSeal.normalizeVerificationUrl(String(value || ""))
    }

    // Igual que el motor: RFC 3161 por http o https, sin credenciales ni fragmento.
    function validTsaUrl(rawValue) {
        const raw = String(rawValue || "").trim()
        if (raw === "" || raw.length > 2048 || /[\s\\\u0000-\u001f\u007f]/.test(raw)) return false
        return /^https?:\/\/[^/?#@\s]+(?:[/?][^#]*)?$/i.test(raw)
    }

    function validQrUrl(rawValue) {
        const raw = String(rawValue || "").trim()
        if (raw === "") return true
        return /^https:\/\//i.test(raw) && portalSeal.normalizeVerificationUrl(raw) !== ""
    }

    function certificateSelectionColor() {
        return currentThemeIndex === 1 || currentThemeIndex === 10 ? "#dcecf6" : "#243c54"
    }

    function certificateSelectionTextColor() {
        return currentThemeIndex === 1 || currentThemeIndex === 10 ? "#17344d" : "#ffffff"
    }

    function certificateDividerColor() {
        return currentThemeIndex === 1 || currentThemeIndex === 10 ? "#8294a6" : "#71869c"
    }

    function compactCertificateBadge(cert) {
        const subject = String((cert && (cert.subjectName || cert.subject)) || "").trim()
        if (subject === "") {
            return "CT"
        }
        const cleaned = subject
            .replace(/[,_;:.()\\-]+/g, " ")
            .replace(/\s+/g, " ")
            .trim()
        const parts = cleaned.split(" ").filter(Boolean)
        if (parts.length === 0) {
            return "CT"
        }
        let initials = ""
        for (let i = 0; i < parts.length && initials.length < 3; i++) {
            const token = parts[i]
            if (!token)
                continue
            initials += token.charAt(0).toUpperCase()
        }
        return initials || "CT"
    }

    function suggestCertificateValidationReportPath() {
        const cert = window.selectedCertData
        const rawName = cert ? basename(window.certificateId(cert)) : ""
        if (rawName === "") return ""
        return suggestedSaveUrl("", rawName + "_validacion_certificado.json")
    }

    function buildVerificationUserSummary(details, filePath, originalPath) {
        const info = details || {}
        const lines = [
            tr("Resumen de validación"),
            ""
        ]

        const fileName = basename(filePath || "")
        if (fileName !== "") {
            lines.push(tr("Documento: ") + fileName)
        }
        const originalName = basename(originalPath || "")
        if (originalName !== "") {
            lines.push(tr("Original: ") + originalName)
        }

        lines.push(tr("Estado: ") + verificationOutcomeLabel(info))

        if (info.reason && info.reason !== "") {
            lines.push(tr("Motivo: ") + localizeVisibleDiagnosticText(info.reason))
        }
        if (info.format && info.format !== "") {
            lines.push(tr("Formato: ") + info.format)
        }
        if (info.coverage && info.coverage !== "") {
            lines.push(tr("Cobertura: ") + verificationCoverageText(info.coverage))
        }

        const aspects = [
            { label: tr("Integridad"), value: info.integrity },
            { label: tr("Certificado"), value: info.certificate },
            { label: tr("Confianza"), value: info.trust }
        ]
        for (let i = 0; i < aspects.length; ++i) {
            const aspect = aspects[i]
            if (!aspect.value) continue
            const status = aspect.value.status ? verificationStatusText(aspect.value.status) : tr("No disponible")
            let line = aspect.label + ": " + status
            if (aspect.value.reason && aspect.value.reason !== "") {
                line += " (" + localizeVisibleDiagnosticText(aspect.value.reason) + ")"
            }
            lines.push(line)
        }

        if (verificationSignerEntries(info).length > 0) {
            lines.push("")
            lines.push(tr("Firmantes: ").trim())
            lines.push(verificationSignerSummariesText(info))
        }

        if (info.warnings && info.warnings.length > 0) {
            lines.push("")
            lines.push(tr("Avisos:"))
            for (let i = 0; i < info.warnings.length; ++i) {
                lines.push("- " + localizeVisibleDiagnosticText(info.warnings[i]))
            }
        }

        if (info.errors && info.errors.length > 0) {
            lines.push("")
            lines.push(tr("Errores:"))
            for (let i = 0; i < info.errors.length; ++i) {
                lines.push("- " + localizeVisibleDiagnosticText(info.errors[i]))
            }
        }

        const technical = verificationSignerTechnicalText(info)
        if (technical !== "") {
            lines.push("")
            lines.push(tr("report.section.details"))
            lines.push(technical)
        }

        return lines.join("\n")
    }

    function certificateValidationMessageBody(message) {
        const text = String(message || "")
        if (text === "")
            return ""
        return text
            .split("\n")
            .filter(function(line) {
                const trimmed = String(line || "").trim()
                return trimmed !== "" &&
                    !trimmed.startsWith(tr("Estado: ")) &&
                    !trimmed.startsWith(tr("Días restantes: ")) &&
                    !trimmed.startsWith(tr("Días caducado: "))
            })
            .join("\n")
    }

    function certificateOnlineStatusText(result) {
        const status = String((result && result.status) || "").trim()
        if (status === "valid") return tr("Válido")
        if (status === "revoked") return tr("Revocado")
        if (status === "inconclusive") return tr("No concluyente")
        if (status === "unavailable") return tr("No se pudo comprobar")
        return tr("Sin comprobar")
    }

    function certificateOnlineStatusColor(result) {
        const status = String((result && result.status) || "").trim()
        if (status === "valid") return "#2ecc71"
        if (status === "revoked") return "#e74c3c"
        if (status === "inconclusive") return "#f39c12"
        if (status === "unavailable") return "#e67e22"
        return currentTheme.secondaryTextColor
    }

    function certificateOnlineResultReport(result) {
        if (!result || Object.keys(result).length === 0)
            return ""
        const lines = [
            tr("Comprobación online del certificado"),
            tr("Estado: ") + certificateOnlineStatusText(result)
        ]
        if (result.userMessage) lines.push(result.userMessage)
        if (result.reason) lines.push(tr("Detalle: ") + result.reason)
        if (result.method) lines.push(tr("Método: ") + result.method)
        if (result.checkedAt) lines.push(tr("Comprobado: ") + result.checkedAt)
        if (result.revokedAt) lines.push(tr("Revocado desde: ") + result.revokedAt)
        if (result.ocspUrl) lines.push(tr("OCSP: ") + result.ocspUrl)
        if (result.crlUrl) lines.push(tr("CRL: ") + result.crlUrl)
        return lines.join("\n")
    }

    function certificateOnlineStatusUserHint(result) {
        const status = String((result && result.status) || "").trim()
        if (status === "valid") {
            return tr("No consta revocación del certificado en este momento.")
        }
        if (status === "revoked") {
            return tr("El certificado figura como revocado por su entidad emisora.")
        }
        if (status === "inconclusive") {
            return tr("El servicio de revocación no ha dado un estado concluyente para este certificado.")
        }
        if (status === "unavailable") {
            return tr("No se ha podido consultar el estado online del certificado.")
        }
        return tr("Todavía no se ha lanzado la comprobación online.")
    }

    function updateCertificateValidationReport() {
        const base = certificateValidationDialog.validationBaseReport || certificateValidationDialog.validationMessage || ""
        const online = certificateOnlineResultReport(certificateValidationDialog.onlineCheckResult)
        certificateValidationDialog.validationReport = online !== "" ? (base + "\n\n" + online) : base
    }

    function openCertificateValidation(cert, triggerOnlineRefresh) {
        if (!cert)
            return
        const result = buildCertificateValidationSummary(cert)
        certificateValidationDialog.validationMessage = result.message
        certificateValidationDialog.validationOk = result.valid
        certificateValidationDialog.validationBaseReport = result.reportText || result.message
        certificateValidationDialog.validationReport = result.reportText || result.message
        certificateValidationDialog.validationDetails = result.details || {}
        certificateValidationDialog.onlineCheckInProgress = false
        certificateValidationDialog.onlineCheckResult = ({})
        certificateValidationDialog.requestedCertificateId = window.certificateId(cert)
        certificateValidationDialog.open()
        if (triggerOnlineRefresh === true && certificateValidationDialog.requestedCertificateId !== "") {
            certificateValidationDialog.onlineCheckInProgress = true
            backend.checkCertificateOnline(certificateValidationDialog.requestedCertificateId)
        }
    }

    // «documento_resumen_validacion.txt» (el sufijo, en el idioma de la
    // aplicación), en la carpeta del documento verificado o en Documentos.
    function suggestVerificationSummaryName(path) {
        const stem = fileStem(path)
        const suffix = tr("verificacion.resumen.nombre_fichero")
        return (stem !== "" ? stem + "_" : "") + suffix + ".txt"
    }

    function suggestVerificationSummaryPath(path) {
        return suggestedSaveUrl(path, suggestVerificationSummaryName(path))
    }

    function suggestSupportIncidentPath() {
        let stem = "incidencia_grxfirma"
        const path = supportAssistantOperationLabel()
        const base = basename(path)
        if (base !== "") {
            const dot = base.lastIndexOf(".")
            stem = dot > 0 ? base.substring(0, dot) : base
        }
        return suggestedSaveUrl("", stem + ".incident.txt")
    }

    Settings {
        id: appSettings
        category: "General"
        property int themeIndex: 0
        property bool expertMode: false
        property string language: ""
        property string lastSeenVersion: ""
        property bool checkForUpdatesCached: true
        property string lastUpdateCheckAt: ""
        property string supportIncidentEndpoint: ""
        property bool leftSidebarCollapsed: false
        property bool rightSidebarCollapsed: false
        property bool signCertificatePanelCollapsed: true
    }

    // --- Ajustes del Backend ---
    property bool autoClose: false
    property bool stickySigner: false
    property bool autoSelectSingleCertificate: true
    property bool preferDefaultCertificate: true
    property bool showDefaultCertificateFirst: true
    property bool showUsableCertificatesFirst: true
    property bool showValidCertificatesFirst: true
    property bool certsExpiredShow: false
    property bool certsInvalidShow: false
    property bool useOnlySignatureCertificates: false
    property var certificateTypeFilter: []
    property bool certificateRequireNIF: false
    property bool certificateRequireOrganization: false
    property bool tsaEnabled: false
    property string tsaUrl: ""
    property bool proxyEnabled: false
    property string proxyType: "none"
    property string proxyHost: ""
    property int proxyPort: 8080
    property var proxyExcludedUrls: []
    property bool proxySecretStoreAvailable: false
    property string proxySecretStorePlatform: ""
    property string proxySecretStoreBackend: "none"
    property string proxySecretStoreReason: ""
    property string proxyRuntimeMode: ""
    property bool proxyCredentialsConfigured: false
    property string proxyCredentialRealm: ""
    property string proxyCredentialUsername: ""
    property bool proxyCredentialBusy: false
    property string defaultHashAlgorithm: "SHA-256"
    property bool defaultHashCopyToClipboard: true
    property string defaultHashFormatFile: "hex"
    property string defaultHashFormatDirectory: "xml"
    property bool defaultHashRecursive: true
    property bool defaultHashSaveReport: false
    property bool confirmToSign: true
    property bool omitAskOnClose: false
    property string closeBehavior: "resident"
    property int webCompatibilityDurationMinutes: 30

    property bool settingsLoaded: false
    property bool applyingLoadedSettings: false
    property bool backendSettingsDirty: false
    property bool settingsSaveInFlight: false
    property bool bypassUnsavedClosePrompt: false
    property bool pendingDiscardAndClose: false
    property bool pendingCloseAfterSettingsSave: false
    property bool explicitExitRequested: false

    Timer {
        id: settingsSaveDebounceTimer
        interval: 300
        repeat: false
        onTriggered: window.saveBackendSettings()
    }

    Timer {
        id: settingsRoundtripTimeoutTimer
        interval: 900
        repeat: false
        onTriggered: {
            const hadPendingSaveClose = window.pendingCloseAfterSettingsSave
            const hadPendingDiscardClose = window.pendingDiscardAndClose
            window.settingsSaveInFlight = false
            window.pendingCloseAfterSettingsSave = false
            window.pendingDiscardAndClose = false
            if (hadPendingSaveClose || hadPendingDiscardClose) {
                window.statusMessage = tr("No se pudo confirmar el guardado de preferencias. Cerrando igualmente.")
                window.continueWindowClose(true)
                return
            }
            window.statusMessage = tr("No se pudo confirmar el guardado de preferencias.")
        }
    }

    component SettingsRowHighlight: Rectangle {
        id: settingsRowFrame
        property real rowSpacing: 10
        default property alias contentData: settingsRow.data
        Layout.fillWidth: true
        implicitHeight: settingsRow.implicitHeight + 12
        radius: 8
        color: settingsRowHover.containsMouse ? Qt.alpha(currentTheme.primaryColor, 0.10) : "transparent"
        border.width: settingsRowHover.containsMouse ? 1 : 0
        border.color: Qt.alpha(currentTheme.primaryColor, 0.35)

        MouseArea {
            id: settingsRowHover
            anchors.fill: parent
            acceptedButtons: Qt.NoButton
            hoverEnabled: true
        }

        RowLayout {
            id: settingsRow
            anchors.fill: parent
            anchors.margins: 10
            spacing: settingsRowFrame.rowSpacing
        }
    }

    function keepResidentAfterClose() {
        flushSettingsNow()
        if (temporaryCertificateIds.length > 0) {
            if (residentCredentialPurgePending) {
                window.statusMessage = tr("Error")
                return
            }
            if (!backend || !backend.clearTemporaryCertificates) {
                window.statusMessage = tr("Error")
                return
            }
            residentCredentialPurgePending = true
            backend.clearTemporaryCertificates()
        }
        const hiddenToNativeTray =
            (typeof residentAgent !== "undefined" && residentAgent)
                ? residentAgent.hideMainWindow()
                : false
        if (!hiddenToNativeTray) {
            // Sin bandeja nativa se mantiene accesible en la barra de tareas.
            window.visibility = Window.Minimized
        }
        window.statusMessage = tr("GrxFirma sigue en segundo plano.")
    }

    function syncResidentAgentUi() {
        if (typeof residentAgent === "undefined" || !residentAgent) return
        residentAgent.configureLabels(
            tr("Abrir GrxFirma"),
            tr("Firmas desde portales: activo"),
            tr("Ajustes"),
            tr("Ayuda"),
            tr("Manual de ayuda"),
            tr("Novedades"),
            tr("Acerca de GrxFirma"),
            tr("Salir"),
            tr("GrxFirma se ha ocultado en la bandeja. Abra su icono para volver a la ventana."),
            tr("Operación completada"),
            tr("Error"))
        residentAgent.setEnabled(window.closeBehavior === "resident")
    }

    function markBackendSettingsDirty() {
        if (!settingsLoaded || applyingLoadedSettings) return
        if (!backendSettingsDirty) {
            console.log("QML: Preferencias marcadas como pendientes de guardar")
        }
        backendSettingsDirty = true
    }

    function scheduleSettingsSave() {
        if (portalSealMode) return
        markBackendSettingsDirty()
    }

    function flushSettingsNow() {
        if (settingsSaveDebounceTimer.running) {
            settingsSaveDebounceTimer.stop()
        }
    }

    function defaultSealState() {
        return {
            pages: "1",
            allPages: false,
            x: 0.62,
            y: 0.04,
            w: 0.34,
            h: 0.12,
            rotation: 0,
            keepText: true,
            imagePath: "",
            logoOpacityPercent: 100
        }
    }

    function applySealState(state) {
        if (state.visibleSeal !== undefined) signVisibleSeal = state.visibleSeal
        if (state.pages !== undefined) signSealPages = String(state.pages)
        if (state.allPages !== undefined) signSealAllPages = !!state.allPages
        if (state.x !== undefined) signSealX = clamp01(Number(state.x))
        if (state.y !== undefined) signSealY = clamp01(Number(state.y))
        if (state.w !== undefined) signSealW = clamp01(Number(state.w))
        if (state.h !== undefined) signSealH = clamp01(Number(state.h))
        if (state.rotation !== undefined) signSealRotation = ((Number(state.rotation) % 360) + 360) % 360
        if (state.keepText !== undefined) signSealKeepText = !!state.keepText
        if (state.imagePath !== undefined) signSealImagePath = state.imagePath
        if (state.logoOpacityPercent !== undefined) signSealLogoOpacityPercent = Number(state.logoOpacityPercent)
        if (state.qrContent !== undefined) signQRContent = state.qrContent
        applyPageSelectionValidity()
        syncPreviewFromSeal()
        if (signVisibleSeal && supportsVisibleSeal()) requestPdfPreview()
    }

    function syncSealVisibilityControls() {
        if (typeof signVisibleSealCheckBox !== "undefined" && signVisibleSealCheckBox) {
            signVisibleSealCheckBox.checked = window.signVisibleSeal
        }
        if (typeof settingsSignVisibleSealSwitch !== "undefined" && settingsSignVisibleSealSwitch) {
            settingsSignVisibleSealSwitch.checked = window.signVisibleSeal
        }
    }

    function applySealPreset(presetKey) {
        let label = ""
        switch (presetKey) {
        case "compact":
            label = tr("Compacto")
            sealStyle = "text"
            applySealState({
                visibleSeal: true,
                x: 0.70,
                y: 0.04,
                w: 0.24,
                h: 0.08,
                rotation: 0,
                keepText: true,
                imagePath: ""
            })
            break
        case "institutional":
            label = tr("Institucional")
            sealStyle = "institutional"
            applySealState({
                visibleSeal: true,
                x: 0.62,
                y: 0.04,
                w: 0.34,
                h: 0.12,
                rotation: 0,
                keepText: true,
                imagePath: ""
            })
            break
        case "logo":
            label = tr("Solo logo")
            sealStyle = "image"
            applySealState({
                visibleSeal: true,
                x: 0.74,
                y: 0.04,
                w: 0.18,
                h: 0.10,
                rotation: 0,
                keepText: false,
                imagePath: bundledSealLogoPath
            })
            break
        case "logoqr":
            label = tr("Logo + datos + QR")
            sealStyle = "image"
            signQREnabled = true
            applySealState({
                visibleSeal: true,
                x: 0.58,
                y: 0.04,
                w: 0.38,
                h: 0.12,
                rotation: 0,
                keepText: true,
                imagePath: bundledSealLogoPath
            })
            break
        }
        if (label !== "") window.statusMessage = tr("Preset aplicado: %1").arg(label)
    }

    function restoreDefaultSealSettings() {
        sealStyle = "institutional"
        signQREnabled = false
        signSealPerPage = false
        signSealPlacements = ({})
        const defaults = defaultSealState()
        applySealState({
            pages: defaults.pages,
            allPages: defaults.allPages,
            x: defaults.x,
            y: defaults.y,
            w: defaults.w,
            h: defaults.h,
            rotation: defaults.rotation,
            keepText: defaults.keepText,
            imagePath: defaults.imagePath,
            logoOpacityPercent: defaults.logoOpacityPercent,
            qrContent: ""
        })
        window.statusMessage = tr("Sello restaurado a valores por defecto.")
    }

    function signActionIndex() {
        switch (signAction) {
        case "cosign": return 1
        case "countersign": return 2
        default: return 0
        }
    }

    function signFormatIndex() {
        switch (signFormat) {
        case "pades": return 1
        case "cades": return 2
        case "xades": return 3
        case "xmldsig": return 4
        case "odf": return 5
        case "ooxml": return 6
        case "facturae": return 7
        case "asic-xades": return 8
        case "verifactu": return verifactuInput ? 9 : 0
        default: return 0
        }
    }

    function signOverwriteIndex() {
        switch (signOverwrite) {
        case "fail": return 1
        case "force": return 2
        default: return 0
        }
    }

    function optionIndexByValue(options, value) {
        for (let i = 0; i < options.length; ++i) {
            if (options[i].valor === value) return i
        }
        return 0
    }

    function parseProxyExcludedUrlsText(text) {
        if (text === undefined || text === null) return []
        const raw = String(text).split(/[\n,;]/)
        const out = []
        const seen = {}
        for (let i = 0; i < raw.length; ++i) {
            const item = String(raw[i]).trim()
            if (item === "" || seen[item]) continue
            seen[item] = true
            out.push(item)
        }
        return out
    }

    function proxyExcludedUrlsText() {
        if (!window.proxyExcludedUrls || window.proxyExcludedUrls.length === 0) return ""
        return window.proxyExcludedUrls.join("\n")
    }

    function certificateTypeFilterContains(tipo) {
        return (window.certificateTypeFilter || []).indexOf(tipo) !== -1
    }

    function normalizeCertificateTypeFilter(raw) {
        const allowed = ["fisica", "representacion", "sello", "empleado_publico", "desconocido"]
        let values = []
        if (Array.isArray(raw)) {
            values = raw
        } else if (raw !== undefined && raw !== null && String(raw).trim() !== "") {
            values = [raw]
        }
        let out = []
        for (let i = 0; i < values.length; ++i) {
            const value = String(values[i] === undefined || values[i] === null ? "" : values[i]).trim().toLowerCase()
            if (value === "" || allowed.indexOf(value) === -1 || out.indexOf(value) !== -1) continue
            out.push(value)
        }
        return out
    }

    function toggleCertificateTypeFilter(tipo, enabled) {
        const current = (window.certificateTypeFilter || []).slice()
        const idx = current.indexOf(tipo)
        if (enabled) {
            if (idx === -1) current.push(tipo)
        } else if (idx !== -1) {
            current.splice(idx, 1)
        }
        window.certificateTypeFilter = current
    }

    function visibleProtectionRecipients() {
        const targetProfile = String(window.protectProfile || "compat").toLowerCase()
        if (targetProfile === "compat"
                && String(window.protectContainer || "json").toLowerCase() === "cms-encrypted")
            return []
        const authEnveloped = targetProfile === "compat"
                              && String(window.protectContainer || "json").toLowerCase() === "authenvelopeddata"
        return (window.protectionRecipients || []).filter(function(recipient) {
            const profile = String(recipient.profile || recipient.Profile || "compat").toLowerCase()
            if (profile !== targetProfile) return false
            if (!authEnveloped) return true
            return recipient.authEnvelopedDataCompatible === true
                   || recipient.AuthEnvelopedDataCompatible === true
        })
    }

    function protectionContainerSuffix() {
        if (window.protectProfile === "alto") return ".afp"
        if (window.protectContainer === "cms") return ".enveloped"
        if (window.protectContainer === "cms-encrypted") return ".encrypted.p7m"
        if (window.protectContainer === "authenvelopeddata") return ".authenveloped.p7m"
        return ".afp"
    }

    function protectedContainerLabel(path) {
        const lower = String(path || "").toLowerCase()
        if (lower.endsWith(".encrypted.p7m"))
            return tr("CMS EncryptedData (.encrypted.p7m)")
        if (lower.endsWith(".authenveloped.p7m"))
            return tr("CMS AuthEnvelopedData (.authenveloped.p7m)")
        if (lower.endsWith(".enveloped"))
            return tr("CMS EnvelopedData (.enveloped)")
        if (lower.endsWith(".p7m"))
            return tr("CMS/PKCS#7 (.p7m)")
        if (lower.endsWith(".afp"))
            return tr("JSON (.afp)")
        return tr("Detección automática por contenido")
    }

    function isEncryptedDataProtection() {
        return window.protectProfile === "compat"
                && window.protectContainer === "cms-encrypted"
    }

    function isEncryptedDataPath(path) {
        return String(path || "").toLowerCase().endsWith(".encrypted.p7m")
    }

    function isGenericCMSPath(path) {
        const lower = String(path || "").toLowerCase()
        return lower.endsWith(".p7m")
                && !lower.endsWith(".encrypted.p7m")
                && !lower.endsWith(".authenveloped.p7m")
                && !lower.endsWith(".signedenveloped.p7m")
    }

    function usesTransientUnprotectionSecret() {
        if (isEncryptedDataPath(window.unprotectInputPath))
            return true
        return isGenericCMSPath(window.unprotectInputPath)
                && typeof unprotectEncryptedDataToggle !== "undefined"
                && unprotectEncryptedDataToggle.checked
    }

    function looksLikeCanonicalAES256Secret(value) {
        return /^[A-Za-z0-9+\/]{42}[AEIMQUYcgkosw048]=$/.test(String(value || ""))
    }

    function clearTransientProtectionSecrets() {
        if (typeof protectEncryptedSecretField !== "undefined")
            protectEncryptedSecretField.clear()
        if (typeof protectEncryptedSecretConfirmField !== "undefined")
            protectEncryptedSecretConfirmField.clear()
        if (typeof unprotectEncryptedSecretField !== "undefined")
            unprotectEncryptedSecretField.clear()
        if (typeof unprotectEncryptedDataToggle !== "undefined"
                && isGenericCMSPath(window.unprotectInputPath))
            unprotectEncryptedDataToggle.checked = false
    }

    function pruneProtectionSelection() {
        const visible = visibleProtectionRecipients()
        const allowed = {}
        for (let i = 0; i < visible.length; ++i) {
            allowed[String(visible[i].id || visible[i].ID || "")] = true
        }
        const next = []
        for (let i = 0; i < window.protectSelectedRecipientIds.length; ++i) {
            const id = String(window.protectSelectedRecipientIds[i] || "")
            if (id !== "" && allowed[id]) next.push(id)
        }
        window.protectSelectedRecipientIds = next
        if (window.protectProfile === "alto") {
            window.protectContainer = "json"
        }
    }

    function protectionRecipientLabel(recipient) {
        if (!recipient) return ""
        const label = String(recipient.label || recipient.Label || "")
        const rid = String(recipient.id || recipient.ID || "")
        if (label !== "" && rid !== "" && label !== rid) return label + " (" + rid + ")"
        return label !== "" ? label : rid
    }

    function saveBackendSettings() {
        if (!settingsLoaded || settingsSaveInFlight) return
        for (const field of ["tsa", "proxyHost", "proxyPort"]) validateSettingsField(field)
        const firstError = Object.keys(settingsFieldErrors)[0]
        if (firstError) {
            const target = firstError === "tsa" ? tsaCombo : firstError === "proxyHost" ? proxyHostField : proxyPortField
            const position = target.mapToItem(configMainOuterContent, 0, 0)
            configMainOuterScroll.ScrollBar.vertical.position = Math.max(0,
                    Math.min(1, (position.y - 24) / Math.max(1, configMainOuterContent.height - configMainOuterScroll.height)))
            target.forceActiveFocus()
            return
        }
        console.log("QML: Guardando preferencias en backend")
        const s = {
            expertMode: backend.expertMode,
            themeIndex: window.currentThemeIndex,
            autoClose: window.autoClose,
            closeBehavior: window.closeBehavior,
            stickySigner: window.stickySigner,
            autoSelectSingleCertificate: window.autoSelectSingleCertificate,
            preferDefaultCertificate: window.preferDefaultCertificate,
            signCertificatePanelExpanded: !window.signCertificatePanelCollapsed,
            showDefaultCertificateFirst: window.showDefaultCertificateFirst,
            showUsableCertificatesFirst: window.showUsableCertificatesFirst,
            showValidCertificatesFirst: window.showValidCertificatesFirst,
            certsExpiredShow: window.certsExpiredShow,
            certsInvalidShow: window.certsInvalidShow,
            useOnlySignatureCertificates: window.useOnlySignatureCertificates,
            certificateTypeFilter: window.certificateTypeFilter,
            certificateRequireNIF: window.certificateRequireNIF,
            certificateRequireOrganization: window.certificateRequireOrganization,
            tsaEnabled: window.tsaEnabled,
            tsaUrl: window.tsaUrl,
            proxyEnabled: window.proxyEnabled,
            proxyType: window.proxyType,
            proxyHost: window.proxyHost,
            proxyPort: window.proxyPort,
            proxyExcludedUrls: window.proxyExcludedUrls,
            defaultHashAlgorithm: window.defaultHashAlgorithm,
            defaultHashCopyToClipboard: window.defaultHashCopyToClipboard,
            defaultHashFormatFile: window.defaultHashFormatFile,
            defaultHashFormatDirectory: window.defaultHashFormatDirectory,
            defaultHashRecursive: window.defaultHashRecursive,
            defaultHashSaveReport: window.defaultHashSaveReport,
            confirmToSign: window.confirmToSign,
            omitAskOnClose: window.omitAskOnClose,
            webCompatibilityDurationMinutes: window.webCompatibilityDurationMinutes,
            facturaeToolsEnabled: window.facturaeToolsEnabled,
            checkForUpdates: window.checkForUpdates,
            idioma: window.appLanguage,
            preferredCertificateId: window.preferredCertificateId,
            defaultCertificateId: window.defaultCertificateId,
            rememberCertificateFilter: window.rememberCertificateFilter,
            certificateFilterText: window.rememberCertificateFilter ? window.certificateFilterText : "",
            signAction: window.signAction,
            signFormat: window.signFormat,
            signProfile: window.signProfile,
            signStrictCompat: window.signStrictCompat,
            signOverwrite: window.signOverwrite,
            autoFormatPdf: window.autoFormatPdf,
            autoFormatOoxml: window.autoFormatOoxml,
            autoFormatFacturae: window.autoFormatFacturae,
            autoFormatOdf: window.autoFormatOdf,
            autoFormatXml: window.autoFormatXml,
            autoFormatBinary: window.autoFormatBinary,
            multiCosignEnabled: window.multiCosignEnabled,
            multiCosignPrimaryCertificateId: window.multiCosignPrimaryCertificateId,
            multiCosignCertificateIds: window.multiCosignCertificateIds,
            signVisibleSeal: window.signVisibleSeal,
            signSealPages: window.signSealPages,
            signSealAllPages: window.signSealAllPages,
            signSealX: window.signSealX,
            signSealY: window.signSealY,
            signSealW: window.signSealW,
            signSealH: window.signSealH,
            signSealRotation: window.signSealRotation,
            signSealPerPage: window.signSealPerPage,
            signSealPlacements: window.signSealPlacements,
            signSealKeepText: window.signSealKeepText,
            signSealImagePath: window.signSealImagePath,
            signSealLogoOpacityPercent: window.signSealLogoOpacityPercent,
            signSealLanguage: window.signSealLanguage === "" ? "interface" : window.signSealLanguage,
            signSealStyle: window.sealStyle,
            signQRContent: window.signQRContent,
            signQREnabled: window.signQREnabled,
            signReason: window.signReason,
            signLocation: window.signLocation,
            signContactInfo: window.signContactInfo,
            padesSubFilter: window.padesSubFilter,
            facturaePolicyVersion: window.facturaePolicyVersion,
            policyIdentifier: window.facturaePolicyIdentifier,
            policyIdentifierHash: window.facturaePolicyIdentifierHash,
            policyQualifier: window.facturaePolicyQualifier,
            signerClaimedRole: window.facturaeSignerRole
            ,
            signatureProductionCity: window.facturaeSignatureCity,
            signatureProductionProvince: window.facturaeSignatureProvince,
            signatureProductionPostalCode: window.facturaeSignaturePostalCode,
            signatureProductionCountry: window.facturaeSignatureCountry
        }
        settingsSaveInFlight = true
        settingsRoundtripTimeoutTimer.restart()
        backend.saveSettings(s)
        window.statusMessage = tr("Guardando preferencias...")
    }

    function saveBackendSettingsAndClose() {
        console.log("QML: Solicitud de guardar preferencias y cerrar")
        explicitExitRequested = true
        pendingCloseAfterSettingsSave = true
        if (!backendSettingsDirty) {
            pendingCloseAfterSettingsSave = false
            continueWindowClose(true)
            return
        }
        if (settingsSaveInFlight) {
            settingsRoundtripTimeoutTimer.restart()
            return
        }
        saveBackendSettings()
    }

    function discardBackendSettingsChanges(closeAfter) {
        if (!settingsLoaded) return
        console.log("QML: Solicitud de descartar preferencias", closeAfter === true ? "cerrando" : "sin cerrar")
        if (closeAfter === true) {
            pendingDiscardAndClose = false
            pendingCloseAfterSettingsSave = false
            settingsSaveInFlight = false
            settingsRoundtripTimeoutTimer.stop()
            explicitExitRequested = true
            backendSettingsDirty = false
            window.statusMessage = tr("Cambios descartados.")
            continueWindowClose(true)
            return
        }
        pendingDiscardAndClose = closeAfter === true
        explicitExitRequested = closeAfter === true
        settingsRoundtripTimeoutTimer.restart()
        backend.getSettings()
    }

    function continueWindowClose(forceExit) {
        console.log("QML: continueWindowClose", forceExit === true ? "forzado" : "normal")
        const mustExit = forceExit === true || window.explicitExitRequested
        window.explicitExitRequested = false
        if (!mustExit && window.closeBehavior === "resident") {
            window.keepResidentAfterClose()
            return
        }
        bypassUnsavedClosePrompt = true
        Qt.quit()
    }

    function executeSignRequest(selectedCertIndex) {
        const resumedAfterPreview = signAfterPreviewResumed
        signAfterPreviewResumed = false
        const certificate = selectedCertIndex >= 0 && selectedCertIndex < window.certificates.length
                ? window.certificates[selectedCertIndex] : null
        if (!window.certificateCanSign(certificate)) {
            signValidationErrorDialog.errorMessage = window.certificateStatusReason(certificate)
            signValidationErrorDialog.open()
            return
        }
        const remotePrimaryIndex = window.multiCosignEnabled ? window.effectiveMultiCosignPrimaryIndex() : selectedCertIndex
        const remoteCertificate = remotePrimaryIndex >= 0 && remotePrimaryIndex < window.certificates.length
                ? window.certificates[remotePrimaryIndex] : null
        // Unos secretos escritos para otro certificado no se reutilizan nunca.
        if (window.cscPendingSecrets !== null && (!remoteCertificate
                || String(window.cscPendingSecrets.certificateId) !== String(remoteCertificate.id || ""))) {
            window.cscPendingSecrets = null
        }
        if (remoteCertificate && remoteCertificate.remote === true) {
            const batchOtpBlock = window.remoteBatchOtpBlockKey(remoteCertificate)
            if (batchOtpBlock !== "") {
                window.cscPendingSecrets = null
                signValidationErrorDialog.errorMessage = batchOtpBlock === "csc.error.otp_lote_excede"
                        ? tr("csc.error.otp_lote_excede") : tr("csc.error.otp_lote")
                signValidationErrorDialog.open()
                return
            }
            if (window.certificateNeedsRemoteSecrets(remoteCertificate) && window.cscPendingSecrets === null) {
                window.openRemoteSecretDialog(selectedCertIndex, remoteCertificate)
                return
            }
        }
        if (window.multiCosignEnabled) {
            const primaryIndex = window.effectiveMultiCosignPrimaryIndex()
            const additional = window.sanitizeMultiCosignIdsForPrimary(
                window.effectiveMultiCosignPrimaryId(), window.multiCosignCertificateIds, window.certificates)
            if (primaryIndex === -1 || !window.certificateCanSign(window.certificates[primaryIndex])
                    || additional.length !== window.multiCosignCertificateIds.length) {
                signValidationErrorDialog.errorMessage = tr("Seleccione certificados válidos para la cofirma múltiple")
                signValidationErrorDialog.open()
                return
            }
        }
        if (!validateSignFields()) {
            focusSignField(firstSignFieldError)
            return
        }
        signPayloadNeedsPreview = false
        const payload = buildSignPayload()
        if (payload === null && signPayloadNeedsPreview && resumedAfterPreview) {
            window.statusMessage = ""
            signValidationErrorDialog.heading = tr("sign.seal.preview_unavailable_heading")
            signValidationErrorDialog.errorMessage = tr("sign.seal.preview_unavailable_before_sign")
            signValidationErrorDialog.open()
            return
        }
        if (payload === null && signPayloadNeedsPreview) {
            // Falta la vista del PDF para situar el sello: se carga y la
            // firma sigue sola al llegar; si no llega a tiempo, se explica.
            signAfterPreviewCertIndex = selectedCertIndex
            signAfterPreviewTimeout.restart()
            window.statusMessage = tr("sign.seal.preview_loading_before_sign")
            return
        }
        if (payload === null) {
            signValidationErrorDialog.errorMessage = tr("Selección de páginas inválida. Usa 1, 1,3-5 o all.")
            signValidationErrorDialog.open()
            return
        }
        window.clearOperationFailure()
        window.clearSignResult()
        if (skipSignedDocumentWarningOnce) {
            skipSignedDocumentWarningOnce = false
        } else if (!window.isBatchMode() && String(payload.action || signAction) === "sign" && currentFilePath !== "") {
            pendingSignedDocumentWarningContext = {
                selectedCertIndex: selectedCertIndex,
                path: currentFilePath
            }
            window.statusMessage = tr("Comprobando si el documento ya estaba firmado...")
            window.requestVerification(currentFilePath)
            return
        }
        if (shouldPrecheckSealSignerSummary(payload) && pendingSealSignerSummaryContext === null) {
            pendingSealSignerSummaryContext = {
                selectedCertIndex: selectedCertIndex,
                payload: payload,
                path: currentFilePath
            }
            window.statusMessage = tr("Leyendo firmantes previos para completar el sello...")
            window.requestVerification(currentFilePath)
            return
        }
        if (pendingSealSignerSummaryContext !== null && pendingSealSignerSummaryContext.payload) {
            payload.extraOptions = pendingSealSignerSummaryContext.payload.extraOptions || payload.extraOptions
            pendingSealSignerSummaryContext = null
        }
        window.signingInProgress = true
        window.statusMessage = tr("Procesando firma...")
        window.currentBatchResults = []
        if (window.multiCosignEnabled && !window.isBatchMode()) {
            const primaryIndex = window.effectiveMultiCosignPrimaryIndex()
            if (primaryIndex === -1) {
                window.signingInProgress = false
                signValidationErrorDialog.errorMessage = tr("Debe definir un firmante principal válido para la cofirma múltiple guiada.")
                signValidationErrorDialog.open()
                return
            }
            if (primaryIndex !== selectedCertIndex) {
                selectCertificateIndex(primaryIndex, false)
            }
            payload.certificateId = certificateId(window.certificates[primaryIndex])
            window.attachRemoteSecrets(payload); backend.signFileMultiAdvanced(window.currentFilePath, window.currentOutputPath, primaryIndex, window.multiCosignCertificateIds, payload)
        } else if (window.isBatchMode()) {
            if (window.multiCosignEnabled) {
                const primaryIndex = window.effectiveMultiCosignPrimaryIndex()
                if (primaryIndex === -1) {
                    window.signingInProgress = false
                    signValidationErrorDialog.errorMessage = tr("Debe definir un firmante principal válido para la cofirma múltiple guiada.")
                    signValidationErrorDialog.open()
                    return
                }
                if (primaryIndex !== selectedCertIndex) {
                    selectCertificateIndex(primaryIndex, false)
                }
                payload.certificateId = certificateId(window.certificates[primaryIndex])
                payload.additionalCertificateIds = window.multiCosignCertificateIds.slice()
                window.attachRemoteSecrets(payload); backend.signBatchAdvanced(window.currentBatchPaths, window.currentBatchDirectory, window.currentBatchOutputDir, primaryIndex, payload)
                return
            }
            payload.certificateId = certificateId(window.certificates[selectedCertIndex])
            window.attachRemoteSecrets(payload); backend.signBatchAdvanced(window.currentBatchPaths, window.currentBatchDirectory, window.currentBatchOutputDir, selectedCertIndex, payload)
        } else {
            payload.certificateId = certificateId(window.certificates[selectedCertIndex])
            window.attachRemoteSecrets(payload); backend.signFileAdvanced(window.currentFilePath, window.currentOutputPath, selectedCertIndex, payload)
        }
    }

    function requestSignConfirmation(selectedCertIndex) {
        if (!window.confirmToSign) {
            executeSignRequest(selectedCertIndex)
            return
        }
        if (window.multiCosignEnabled) {
            const primaryIndex = window.effectiveMultiCosignPrimaryIndex()
            signConfirmDialog.selectedCertIndex = primaryIndex !== -1 ? primaryIndex : selectedCertIndex
        } else {
            signConfirmDialog.selectedCertIndex = selectedCertIndex
        }
        signConfirmDialog.open()
    }

    Timer {
        id: localeMutationTimer
        interval: 0
        repeat: false
        onTriggered: {
            if (typeof i18n !== "undefined" && i18n && i18n.locale !== pendingLocaleLanguage) {
                console.log("QML: Aplicando locale", pendingLocaleLanguage)
                i18n.locale = pendingLocaleLanguage
            }
            if (typeof languageCombo !== "undefined" && languageCombo) {
                languageCombo.syncCurrentIndex()
            }
        }
    }

    Timer {
        id: backendStartupQueriesTimer
        interval: 250
        repeat: false
        onTriggered: {
            if (backend && backend.getSettings) backend.getSettings()
            if (backend && backend.getProxySecretStoreStatus) backend.getProxySecretStoreStatus()
        }
    }

    Component.onCompleted: {
        window.currentThemeIndex = appSettings.themeIndex
        backend.expertMode = appSettings.expertMode
        window.sidebarCollapsed = appSettings.leftSidebarCollapsed
        window.rightSidebarCollapsed = appSettings.rightSidebarCollapsed
        window.applyNarrowRightSidebar()
        window.signCertificatePanelCollapsed = appSettings.signCertificatePanelCollapsed
        if (appSettings.language !== "") {
            window.appLanguage = appSettings.language
        }
        if (typeof ipcSocketPath !== "undefined") window.ipcSocketPath = ipcSocketPath
        if (!portalSealMode) window.syncResidentAgentUi()
        if (typeof isIpcMode !== "undefined" && isIpcMode && backend)
            window.showLocalTLSStartupNotice(backend.localTLSStartupStatus)
        if (!portalSealMode) {
            backendStartupQueriesTimer.start()
            updateStartupTimer.start()
            updateCycleTimer.start()
        }
        settingsLoaded = true
        if (!portalSealMode) releaseNotesStartupTimer.start()
        if (portalSealMode) {
            window.currentFilePath = portalSeal.documentPath
            window.signFormat = "pades"
            window.signVisibleSeal = true
            window.signSealPages = "1"
            window.signSealPerPage = false
            window.signSealPlacements = ({})
            portalSealEditor.parent = portalSealCanvas
            portalSealEditor.anchors.fill = portalSealCanvas
            portalSealStartupTimer.start()
            portalSealPreviewRetryTimer.start()
            portalSealPreviewTimeout.start()
        }
    }

    Timer {
        id: updateEngineBudgetTimer
        interval: 4000
        repeat: false
        onTriggered: {
            if (window.updateCheckInProgress && !window.updateUsingDirect)
                window.startDirectUpdateCheck()
        }
    }
    Timer {
        id: updateStartupTimer
        // Margen para que el motor termine de conectar: si no, la primera
        // comprobación lo daría por caído y avisaría de un problema inexistente.
        interval: 15000
        repeat: false
        onTriggered: window.requestUpdateCheck(false)
    }
    Timer {
        id: updateCycleTimer
        interval: 5 * 60 * 60 * 1000
        repeat: true
        onTriggered: window.requestUpdateCheck(false)
    }
    Connections {
        target: officialUpdateChecker
        function onFinished(ok, result) {
            if (!window.updateCheckInProgress) return
            window.updateCheckInProgress = false
            if (ok && result.estado === "sin_publicaciones") {
                if (window.updateManualRequest)
                    window.updateStatusMessage = tr("Todavía no hay versiones publicadas en el canal oficial.")
            } else if (ok) window.acceptUpdateResult(result)
            else {
                console.warn("update: automatic check unavailable")
                if (window.updateManualRequest)
                    window.updateStatusMessage = tr("No se pudo conectar para comprobar versiones. Revise su conexión a Internet y vuelva a intentarlo.")
            }
        }
    }

    Timer {
        id: releaseNotesStartupTimer
        interval: 700
        repeat: false
        onTriggered: {
            const current = window.aboutApplicationVersion
            if (!/^\d+\.\d+\.\d+$/.test(current)) return
            const previous = appSettings.lastSeenVersion
            if (previous === "") {
                appSettings.lastSeenVersion = current
                return
            }
            if (previous === current) return
            window.pendingReleaseNotes = releaseNotes.since(current, previous)
            if (window.pendingReleaseNotes === "") {
                appSettings.lastSeenVersion = current
                return
            }
            if (residentAgent.hidden)
                residentAgent.notifyReleaseNotes(tr("Actualizado a %1: ver novedades").arg(current))
            else
                window.openReleaseNotes(false)
        }
    }

    Connections {
        target: (typeof isIpcMode !== "undefined" && isIpcMode) ? backend : null
        ignoreUnknownSignals: true
        function onLocalTLSStartupStatusChanged() {
            window.showLocalTLSStartupNotice(backend.localTLSStartupStatus)
        }
    }

    Connections {
        target: (typeof i18n !== "undefined" && i18n) ? i18n : null
        function onLocaleChanged() {
            window.syncResidentAgentUi()
        }
    }

    Connections {
        target: (typeof residentAgent !== "undefined" && residentAgent) ? residentAgent : null
        function onSettingsRequested() {
            residentAgent.showMainWindow()
            window.activeTab = "config"
        }
        function onHelpRequested() {
            backend.openHelpManual()
        }
        function onAboutRequested() {
            residentAgent.showMainWindow()
            aboutDialog.open()
        }
        function onReleaseNotesRequested() {
            residentAgent.showMainWindow()
            window.openReleaseNotes(true)
        }
        function onPendingReleaseNotesRequested() {
            residentAgent.showMainWindow()
            window.openReleaseNotes(false)
        }
        function onExitRequested() {
            window.continueWindowClose(true)
        }
    }

    onCurrentThemeIndexChanged: {
        if (settingsLoaded) {
            appSettings.themeIndex = currentThemeIndex
            markBackendSettingsDirty()
        }
    }

    onAppLanguageChanged: {
        console.log("QML: Cambio de idioma solicitado", appLanguage)
        scheduleLocaleMutation(appLanguage)
        if (settingsLoaded) {
            appSettings.language = appLanguage
        }
    }

    onCloseBehaviorChanged: {
        window.syncResidentAgentUi()
    }

    onSidebarCollapsedChanged: {
        if (settingsLoaded) {
            appSettings.leftSidebarCollapsed = sidebarCollapsed
        }
    }

    onRightSidebarCollapsedChanged: {
        if (settingsLoaded && !rightSidebarAutoCollapsed) {
            appSettings.rightSidebarCollapsed = rightSidebarCollapsed
        }
    }

    onSignCertificatePanelCollapsedChanged: {
        if (settingsLoaded) appSettings.signCertificatePanelCollapsed = signCertificatePanelCollapsed
        if (!signCertificatePanelCollapsed) {
            rightSidebarAutoCollapsed = false
            rightSidebarCollapsed = false
        }
    }

    onSignReasonChanged: { scheduleSettingsSave(); scheduleSealPreview() }
    onSignLocationChanged: { scheduleSettingsSave(); scheduleSealPreview() }
    onSignContactInfoChanged: { scheduleSettingsSave(); scheduleSealPreview() }
    onCertificateFilterTextChanged: {
        if (rememberCertificateFilter) scheduleSettingsSave()
    }
    onSignActionChanged: {
        scheduleSettingsSave()
        if (signAction === "countersign" && multiCosignEnabled) {
            multiCosignEnabled = false
        }
    }
    onSignFormatChanged: {
        if (portalSealMode) return
        scheduleSettingsSave()
        if (signVisibleSeal && supportsVisibleSeal()) Qt.callLater(requestPdfPreview)
        if (multiCosignEnabled && !supportsGuidedMultiCosignFormat()) {
            multiCosignEnabled = false
        }
        if (currentFilePath !== "" && (currentOutputPath === "" || currentOutputPath.includes("_firmado"))) {
            suggestOutputPath(currentFilePath)
        }
    }
    onSignStrictCompatChanged: scheduleSettingsSave()
    onSignOverwriteChanged: scheduleSettingsSave()
    onAutoFormatPdfChanged: {
        if (signFormat === "" && currentFilePath !== "" && (currentOutputPath === "" || currentOutputPath.includes("_firmado"))) {
            suggestOutputPath(currentFilePath)
        }
    }
    onAutoFormatOoxmlChanged: {
        if (signFormat === "" && currentFilePath !== "" && (currentOutputPath === "" || currentOutputPath.includes("_firmado"))) {
            suggestOutputPath(currentFilePath)
        }
    }
    onAutoFormatFacturaeChanged: {
        if (signFormat === "" && currentFilePath !== "" && (currentOutputPath === "" || currentOutputPath.includes("_firmado"))) {
            suggestOutputPath(currentFilePath)
        }
    }
    onAutoFormatOdfChanged: {
        if (signFormat === "" && currentFilePath !== "" && (currentOutputPath === "" || currentOutputPath.includes("_firmado"))) {
            suggestOutputPath(currentFilePath)
        }
    }
    onAutoFormatXmlChanged: {
        if (signFormat === "" && currentFilePath !== "" && (currentOutputPath === "" || currentOutputPath.includes("_firmado"))) {
            suggestOutputPath(currentFilePath)
        }
    }
    onAutoFormatBinaryChanged: {
        if (signFormat === "" && currentFilePath !== "" && (currentOutputPath === "" || currentOutputPath.includes("_firmado"))) {
            suggestOutputPath(currentFilePath)
        }
    }
    onMultiCosignEnabledChanged: scheduleSettingsSave()
    onMultiCosignCertificateIdsChanged: scheduleSettingsSave()
    onSignVisibleSealChanged: {
        syncSealVisibilityControls()
        scheduleSettingsSave()
        scheduleSealPreview()
        if (firstSignFieldError) validateSignFields()
    }
    onSignSealPagesChanged: { scheduleSettingsSave(); scheduleSealPreview() }
    onSignSealAllPagesChanged: { scheduleSettingsSave(); scheduleSealPreview() }
    onSealStyleChanged: { scheduleSettingsSave(); scheduleSealPreview(); if (signFieldError("image")) validateSignField("image") }
    onSignSealXChanged: { savePageSeal(); scheduleSettingsSave(); if (!sealRotationDragging) scheduleSealPreview() }
    onSignSealYChanged: { savePageSeal(); scheduleSettingsSave(); if (!sealRotationDragging) scheduleSealPreview() }
    onSignSealWChanged: { savePageSeal(); scheduleSettingsSave(); scheduleSealPreview() }
    onSignSealHChanged: { savePageSeal(); scheduleSettingsSave(); scheduleSealPreview() }
    onSignSealRotationChanged: { savePageSeal(); scheduleSettingsSave(); if (!sealRotationDragging) scheduleSealPreview() }
    onSignSealKeepTextChanged: { scheduleSettingsSave(); scheduleSealPreview() }
    onSignSealImagePathChanged: { scheduleSettingsSave(); scheduleSealPreview(); if (signFieldError("image")) validateSignField("image") }
    onSignSealLogoOpacityPercentChanged: { scheduleSettingsSave(); scheduleSealPreview() }
    onSignQRContentChanged: { scheduleSettingsSave(); scheduleSealPreview() }
    onSignQREnabledChanged: { scheduleSettingsSave(); scheduleSealPreview(); if (signFieldError("qr")) validateSignField("qr") }
    onSignSealPerPageChanged: scheduleSettingsSave()
    onSignSealPlacementsChanged: scheduleSettingsSave()
    onPreviewCurrentPageChanged: { sealDrawArea.cancel(); loadPageSeal() }
    onPreferredCertificateIdChanged: scheduleSettingsSave()

    Connections {
        target: backend
        function onExpertModeChanged() {
            if (settingsLoaded) {
                appSettings.expertMode = backend.expertMode
            }
        }
    }

    function clamp01(v) {
        if (isNaN(v)) return 0.0
        if (v < 0.0) return 0.0
        if (v > 1.0) return 1.0
        return v
    }

    property bool signPayloadNeedsPreview: false
    property int signAfterPreviewCertIndex: -1
    property bool signAfterPreviewResumed: false
    // Mientras se espera la vista del PDF, «Firmar ahora» queda desactivado
    // y el aviso de espera se ve y se anuncia junto al botón.
    readonly property bool signWaitingForPreview: signAfterPreviewCertIndex >= 0

    Timer {
        id: signAfterPreviewTimeout
        interval: 20000
        onTriggered: window.resumeSignAfterPreview(false)
    }

    // La firma pedida sin vista del PDF continúa al llegar la vista; nunca
    // termina en un fallo de firma ni en un mensaje de páginas inválidas.
    function resumeSignAfterPreview(ok) {
        if (signAfterPreviewCertIndex < 0) return
        const certIndex = signAfterPreviewCertIndex
        signAfterPreviewCertIndex = -1
        signAfterPreviewTimeout.stop()
        if (ok) {
            Qt.callLater(function() {
                window.signAfterPreviewResumed = true
                window.executeSignRequest(certIndex)
            })
            return
        }
        window.statusMessage = ""
        signValidationErrorDialog.heading = tr("sign.seal.preview_unavailable_heading")
        signValidationErrorDialog.errorMessage = tr("sign.seal.preview_unavailable_before_sign")
        signValidationErrorDialog.open()
    }

    function requestPdfPreview() {
        const previewPath = previewInputPath()
        if (!signVisibleSeal || !supportsVisibleSeal() || previewPath === "") return;
        sealPreviewRequestId = ""
        sealPreviewImage = ""
        if (!applyPageSelectionValidity(false)) return;
        const page = previewCurrentPage > 0 ? previewCurrentPage : previewPageFromSelection(signSealPages)
        previewRequestSequence += 1
        const requestId = "preview-qml-" + Date.now() + "-" + previewRequestSequence
        pendingPreviewRequestId = requestId
        pendingPreviewPath = previewPath
        pendingPreviewPage = page
        backend.getPdfPreview(previewPath, page, requestId);
    }

    function normalizePageSelection(raw) {
        let value = String(raw === undefined || raw === null ? "" : raw).trim()
        if (value === "") return "1"
        return value.replace(/\s+/g, "")
    }

    function parsePageSelection(raw) {
        const normalized = normalizePageSelection(raw)
        if (normalized === "1") return { ok: true, normalized: "1", previewPage: 1 }
        if (/^(all|todas|todos|\*)$/i.test(normalized)) {
            return { ok: true, normalized: "all", previewPage: 1 }
        }
        const parts = normalized.split(",")
        let previewPage = 1
        let previewAssigned = false
        for (let i = 0; i < parts.length; i++) {
            const part = parts[i]
            if (!/^\d+(?:-\d+)?$/.test(part)) {
                return { ok: false, normalized: normalized, previewPage: 1 }
            }
            if (part.indexOf("-") !== -1) {
                const bounds = part.split("-", 2)
                const start = Number(bounds[0])
                const end = Number(bounds[1])
                if (!Number.isInteger(start) || !Number.isInteger(end) || start < 1 || end < 1 || start > end) {
                    return { ok: false, normalized: normalized, previewPage: 1 }
                }
                if (!previewAssigned) {
                    previewPage = start
                    previewAssigned = true
                }
                continue
            }
            const page = Number(part)
            if (!Number.isInteger(page) || page < 1) {
                return { ok: false, normalized: normalized, previewPage: 1 }
            }
            if (!previewAssigned) {
                previewPage = page
                previewAssigned = true
            }
        }
        return { ok: true, normalized: normalized, previewPage: previewPage }
    }

    function applyPageSelectionValidity(syncPreviewPage) {
        const shouldSyncPreviewPage = syncPreviewPage === undefined ? true : !!syncPreviewPage
        if (signSealAllPages) {
            signSealPagesError = ""
            return true
        }
        const parsed = parsePageSelection(signSealPages)
        if (!parsed.ok) {
            signSealPagesError = tr("Selección de páginas inválida. Usa 1, 1,3-5 o todas.")
            return false
        }
        signSealPages = parsed.normalized
        signSealPage = parsed.previewPage
        if (!signSealAllPages && shouldSyncPreviewPage) previewCurrentPage = parsed.previewPage
        signSealPagesError = ""
        return true
    }

    function previewPageFromSelection(raw) {
        const parsed = parsePageSelection(raw)
        return parsed.ok ? parsed.previewPage : 1
    }

    Connections {
        target: backend
        function onPdfPreviewReceived(requestId, requestedPath, requestedPage, ok, data, width, height, currentPage, totalPages) {
            if (requestId !== window.pendingPreviewRequestId
                    || requestedPath !== window.pendingPreviewPath
                    || requestedPage !== window.pendingPreviewPage) {
                console.log(tr("QML: Preview obsoleta ignorada para"), requestedPath)
                return
            }
            if (window.previewInputPath() !== requestedPath) {
                console.log(tr("QML: Preview obsoleta ignorada para"), requestedPath)
                return
            }
            window.pendingPreviewRequestId = ""
            if (ok && width > 0 && height > 0 && currentPage > 0
                    && totalPages >= currentPage) {
                console.log(tr("QML: Previsualización recibida OK, tamaño:"), width, "x", height);
                previewPageWidthPoints = width
                previewPageHeightPoints = height
                previewGeometryPath = requestedPath
                previewGeometryPage = currentPage
                pagePreview.pageRatio = height / width;
                previewCurrentPage = currentPage
                previewTotalPages = totalPages
                scheduleSealPreview()
                pdfPageImage.source = "data:image/png;base64," + data;
                window.resumeSignAfterPreview(true)
            } else {
                previewPageWidthPoints = 0
                previewPageHeightPoints = 0
                previewGeometryPath = ""
                previewGeometryPage = 0
                console.log(tr("Error de previsualización: ") + data);
                window.resumeSignAfterPreview(false)
            }
        }
    }

    function isCurrentPdf() {
        const path = previewInputPath()
        if (!path || path === "") return false
        return path.toLowerCase().endsWith(".pdf")
    }

    function supportsVisibleSeal() {
        return effectiveSignFormat() === "pades" && isCurrentPdf()
    }

    function syncSealFromPreview() {
        if (pagePreview.width <= 0 || pagePreview.height <= 0) return
        constrainSealCardToPage()
        signSealX = clamp01(sealRect.x / pagePreview.width)
        signSealW = clamp01(sealRect.width / pagePreview.width)
        const topY = sealRect.y / pagePreview.height
        signSealY = clamp01(1.0 - topY - (sealRect.height / pagePreview.height))
        signSealH = clamp01(sealRect.height / pagePreview.height)
    }

    function syncPreviewFromSeal() {
        if (pagePreview.width <= 0 || pagePreview.height <= 0) return
        sealRect.width = Math.max(20, clamp01(signSealW) * pagePreview.width)
        sealRect.height = Math.max(20, clamp01(signSealH) * pagePreview.height)
        sealRect.x = Math.max(0, Math.min(pagePreview.width - sealRect.width, clamp01(signSealX) * pagePreview.width))
        const topY = (1.0 - clamp01(signSealY) - clamp01(signSealH)) * pagePreview.height
        sealRect.y = Math.max(0, Math.min(pagePreview.height - sealRect.height, topY))
        constrainSealCardToPage()
        signSealX = clamp01(sealRect.x / pagePreview.width)
        signSealY = clamp01(1.0 - (sealRect.y + sealRect.height) / pagePreview.height)
    }

    // x/y/w/h describen la tarjeta antes del giro; solo su caja envolvente
    // determina cuánto se puede mover dentro de la página.
    function constrainSealCardToPage() {
        const r = signSealRotation * Math.PI / 180
        const bw = Math.abs(sealRect.width * Math.cos(r)) + Math.abs(sealRect.height * Math.sin(r))
        const bh = Math.abs(sealRect.width * Math.sin(r)) + Math.abs(sealRect.height * Math.cos(r))
        if (sealRect.width > pagePreview.width || sealRect.height > pagePreview.height ||
                bw > pagePreview.width || bh > pagePreview.height) return
        const cxMin = Math.max(bw, sealRect.width) / 2
        const cyMin = Math.max(bh, sealRect.height) / 2
        const cx = Math.max(cxMin, Math.min(pagePreview.width - cxMin, sealRect.x + sealRect.width / 2))
        const cy = Math.max(cyMin, Math.min(pagePreview.height - cyMin, sealRect.y + sealRect.height / 2))
        sealRect.x = cx - sealRect.width / 2
        sealRect.y = cy - sealRect.height / 2
    }

    function rotationIndexForSeal() {
        switch (signSealRotation % 360) {
        case 90: return 1
        case 180: return 2
        case 270: return 3
        default: return signSealRotation % 360 === 0 ? 0 : -1
        }
    }

    function applySealRotation(newRotation) {
        const oldRotation = ((signSealRotation % 360) + 360) % 360
        const nextRotation = ((newRotation % 360) + 360) % 360
        if (oldRotation === nextRotation) return

        signSealRotation = nextRotation
        syncPreviewFromSeal()
    }

    function rotationFromSealPointer(point, modifiers) {
        const cx = sealRect.x + sealRect.width / 2
        const cy = sealRect.y + sealRect.height / 2
        const base = Math.atan2(-sealRect.height / 2, sealRect.width / 2)
        let degrees = ((Math.atan2(point.y - cy, point.x - cx) - base) * 180 / Math.PI + 360) % 360
        if (modifiers & Qt.ShiftModifier) degrees = Math.round(degrees / 15) * 15
        else {
            const nearest = Math.round(degrees / 90) * 90
            if (Math.abs(degrees - nearest) <= 4) degrees = nearest
        }
        return (Math.round(degrees) + 360) % 360
    }

    function sealPayloadFromConfig(config) {
        if (!config) return null
        const parsed = config.allPages
            ? { ok: true, normalized: "all" }
            : parsePageSelection(config.pages)
        const pageWidth = Number(config.pageWidth || 0)
        const pageHeight = Number(config.pageHeight || 0)
        if (!parsed.ok || pageWidth <= 0 || pageHeight <= 0) return null
        const payload = {
            page: parsed.normalized,
            x: clamp01(Number(config.x)),
            y: clamp01(Number(config.y)),
            w: clamp01(Number(config.w)),
            h: clamp01(Number(config.h)),
            pageWidth: pageWidth,
            pageHeight: pageHeight,
            previewPage: Number(config.previewPage || 1),
            totalPages: Math.max(1, Number(config.totalPages || 1)),
            rotation: Number(config.rotation || 0),
            keepText: config.keepText === undefined ? true : !!config.keepText,
            logoOpacityPercent: config.logoOpacityPercent === undefined ? 100 : Number(config.logoOpacityPercent)
        }
        const imagePath = localPathFromUrl(String(config.imagePath || ""))
        if (sealStyle === "image" && imagePath !== "") payload.imagePath = imagePath
        if (config.perPage) {
            payload.placements = Object.keys(config.placements || {}).map(function(key) { return config.placements[key] })
                .filter(function(item) { return item && Number.isInteger(item.page) && item.page >= 1 && item.page <= payload.totalPages })
            if (payload.placements.length < 1 || payload.placements.length > 128) return null
        }
        return payload
    }

    function normalizedSealLanguage(value) {
        const code = String(value === undefined || value === null ? "" : value).trim().toLowerCase()
        return sealLanguageCodes.indexOf(code) >= 0 ? code : ""
    }

    function sealLanguageOptions() {
        const options = [{ code: "", name: tr("settings.seal_language.interface") }]
        const languages = (typeof i18n !== "undefined" && i18n) ? i18n.languages : []
        for (let i = 0; i < languages.length; ++i) {
            if (sealLanguageCodes.indexOf(languages[i].code) >= 0) options.push(languages[i])
        }
        return options
    }

    function sealAppearanceOptions() {
        const options = {}
        // Sin idioma fijo, ipcbridge añade el de la interfaz.
        const fixedSealLanguage = normalizedSealLanguage(window.signSealLanguage)
        if (fixedSealLanguage !== "") options.sealLanguage = fixedSealLanguage
        if (sealStyle === "institutional") options.visibleSealLogo = "institucional"
        if (window.multiCosignEnabled) {
            const summary = multiCosignVisibleSealSummary()
            if (summary.trim() !== "") options.visibleSealSignerSummary = summary.trim()
        }
        return options
    }

    function requestSealPreview() {
        if (!signVisibleSeal || !supportsVisibleSeal()) return
        if (!isIpcMode) {
            sealPreviewMessage = tr("La vista real del sello requiere el motor IPC local.")
            return
        }
        if (sealStyle === "image" && localPathFromUrl(signSealImagePath) === "") {
            sealPreviewMessage = tr("Selecciona una imagen PNG o JPEG para el sello.")
            sealPreviewImage = ""
            return
        }
        if (signQREnabled && normalizedQrUrl(signQRContent) === "") {
            sealPreviewMessage = tr("sign.seal.qr_https_error")
            sealPreviewImage = ""
            return
        }
        const seal = sealPayloadFromConfig(captureSealConfig())
        if (!seal) {
            sealPreviewMessage = tr("Abre la vista del PDF y elige una página para preparar el sello.")
            return
        }
        // El editor gira la tarjeta; solicita la composición a 0° para no girarla dos veces.
        seal.rotation = 0
        const id = "seal-qml-" + Date.now() + "-" + (++previewRequestSequence)
        sealPreviewRequestId = id
        sealPreviewImage = ""
        sealPreviewMessage = tr("Preparando vista real del sello…")
        const options = {
            certificateId: certificateId(selectedCertData),
            signerName: portalSealMode ? portalSeal.signerName : "",
            visibleSeal: seal,
            extraOptions: sealAppearanceOptions(),
            qrContent: signQREnabled ? normalizedQrUrl(signQRContent) : "",
            reason: signReason.trim(),
            location: signLocation.trim(),
            contactInfo: signContactInfo.trim()
        }
        backend.getSealPreview(options, id)
    }

    Timer {
        id: sealPreviewDelay
        interval: 350
        repeat: false
        onTriggered: window.requestSealPreview()
    }

    function scheduleSealPreview() {
        sealPreviewRequestId = ""
        sealPreviewImage = ""
        sealPreviewMessage = tr("Preparando vista real del sello…")
        if (typeof sealPreviewDelay !== "undefined" && sealPreviewDelay)
            sealPreviewDelay.restart()
    }

    onSelectedCertDataChanged: scheduleSealPreview()

    Connections {
        target: backend
        function onSealPreviewReceived(requestId, ok, image, message) {
            if (requestId !== window.sealPreviewRequestId) return
            window.sealPreviewImage = ok && image !== "" ? "data:image/png;base64," + image : ""
            window.sealPreviewMessage = ok && image !== "" ? "" : (message || tr("No se pudo generar la vista del sello."))
        }
        function onCscFinished(action, ok, data, message) {
            if (action === "csc_status") {
                window.cscState = ok ? data : ({})
                window.cscAllowed = ok && data.allowed === true
                // Prohibida: el botón solo aparece, para explicarlo, a quien ya
                // la tenía configurada; no a toda la organización.
                window.cscProhibited = ok && data.prohibitedByPolicy === true && data.userConfigured === true
                if (!window.cscAllowed && !window.cscProhibited && cscRemoteDialog.opened) cscRemoteDialog.close()
                if (ok && data.discovered === true)
                    window.cscDiscovery = { serviceHost: data.serviceHost, oauthHost: data.oauthHost, serviceName: data.serviceName }
                else if (ok)
                    window.cscDiscovery = null
                return
            }
            if (action === "csc_send_otp") {
                cscSecretStatus.text = ok ? tr("csc.gui.codigo_enviado") : message
                return
            }
            window.cscBusy = false
            if (!ok) {
                window.cscMessage = message
                if (action === "csc_configure") window.cscDiscovery = null
                backend.cscStatus()
                return
            }
            if (action === "csc_configure") {
                window.cscDiscovery = data
                window.cscMessage = ""
            } else if (action === "csc_connect") {
                const credentials = data.credentials || []
                window.cscMessage = credentials.length === 0 ? tr("csc.gui.sin_credenciales")
                        : (Number(data.omitted || 0) > 0 ? tr("csc.gui.conectada") + " " + tr("csc.gui.omitidas")
                                                         : tr("csc.gui.conectada"))
            } else if (action === "csc_disconnect") {
                window.cscDiscovery = null
                window.cscMessage = ""
            }
            backend.cscStatus()
        }
        function onSmartcardStatusReceived(ok, readers, message) {
            if (!ok) {
                window.smartcardMessage = tr("No se pudo consultar la tarjeta: %1. Comprueba pcscd, el lector y el módulo PKCS#11.").arg(String(message || "").indexOf("smartcard.") === 0 ? tr(message) : message)
            } else if (!readers || readers.length === 0) {
                window.smartcardMessage = tr("No se detectan lectores. Comprueba que pcscd esté activo, conecta el lector y configura el módulo PKCS#11.")
            } else {
                const present = readers.filter(function(r) { return r.present }).length
                const dnie = readers.some(function(r) { return r.present && r.isDnie })
                window.smartcardMessage = dnie
                    ? tr("facturae.smartcard_dnie")
                    : (present > 0
                       ? tr("Tarjeta detectada. Se actualiza la lista de certificados; puede que necesites configurar el módulo PKCS#11.")
                       : tr("Lector detectado sin tarjeta. Introduce el DNIe o tarjeta y vuelve a consultar."))
            }
            backend.refreshCertificates()
        }
        function onProtectionRecipientChanged(ok, message) {
            window.statusMessage = ok
                ? tr("Libro de destinatarios actualizado. La clave privada no se ha importado.")
                : tr("No se pudo cambiar el destinatario: %1").arg(message)
        }
    }

    function buildSignPayload() {
        const csvError = (signVisibleSeal && supportsVisibleSeal() && signCSVEnabled) ? csvLegendError() : ""
        if (csvError !== "") {
            statusMessage = tr(csvError)
            return null
        }
        if (signVisibleSeal && supportsVisibleSeal() && signQREnabled && normalizedQrUrl(signQRContent) === "") {
            statusMessage = tr("sign.seal.qr_https_error")
            return null
        }
        if (signVisibleSeal && supportsVisibleSeal() && sealStyle === "image" && localPathFromUrl(signSealImagePath) === "") {
            statusMessage = tr("Selecciona una imagen PNG o JPEG para el sello.")
            return null
        }
        const body = {
            action: signAction,
            format: effectiveSignFormat(),
            strictCompat: signStrictCompat,
            overwrite: signOverwrite,
            saveToDisk: true,
            returnSignatureB64: false
        }
        if (signVisibleSeal && body.format === "pades") {
            if (!applyPageSelectionValidity(false)) return null
            if (previewPageWidthPoints <= 0 || previewPageHeightPoints <= 0
                    || previewGeometryPath !== previewInputPath()
                    || previewGeometryPage !== previewCurrentPage) {
                statusMessage = tr("Previsualización")
                signPayloadNeedsPreview = true
                requestPdfPreview()
                return null
            }
            if (currentBatchPaths.length > 1) {
                saveCurrentBatchSealConfig()
                const globalConfig = batchGlobalSealConfig || captureSealConfig()
                body.visibleSeal = sealPayloadFromConfig(globalConfig)
                if (!body.visibleSeal) {
                    statusMessage = tr("Previsualización")
                    return null
                }
                const documentOverrides = []
                const overridePaths = Object.keys(batchSealOverrides)
                for (let i = 0; i < overridePaths.length; i++) {
                    const path = overridePaths[i]
                    if (currentBatchPaths.indexOf(path) < 0) continue
                    const overrideSeal = sealPayloadFromConfig(batchSealOverrides[path])
                    if (!overrideSeal) {
                        statusMessage = tr("Previsualización") + ": " + basename(path)
                        return null
                    }
                    documentOverrides.push({
                        inputPath: path,
                        visibleSeal: overrideSeal
                    })
                }
                if (documentOverrides.length > 0) body.documentOverrides = documentOverrides
            } else {
                body.visibleSeal = sealPayloadFromConfig(captureSealConfig())
                if (!body.visibleSeal) {
                    statusMessage = tr("sign.seal.no_pages_error")
                    return null
                }
            }
        }
        if (body.format === "pades" || body.format === "cades" || body.format === "xades") {
            const extraOptions = body.extraOptions ? body.extraOptions : {}
            if (signProfile.trim() !== "") extraOptions.profile = signProfile.trim()
            if (tsaEnabled && tsaUrl.trim() !== "") extraOptions.tsaURL = tsaUrl.trim()
            if (Object.keys(extraOptions).length > 0) body.extraOptions = extraOptions
        }
        if (body.format === "pades") {
            const extraOptions = body.extraOptions ? body.extraOptions : {}
            Object.assign(extraOptions, sealAppearanceOptions())
            if (signVisibleSeal && signCSVEnabled) {
                extraOptions.csv = signCSVCode.trim()
                extraOptions.csvUrl = normalizedCsvUrl(signCSVUrl)
                extraOptions.csvText = signCSVText.trim()
                extraOptions.csvQR = signCSVQR ? "true" : "false"
            }
            if (padesSubFilter.trim() !== "") extraOptions.subfilter = padesSubFilter.trim()
            if (window.multiCosignEnabled) {
                const signerSummary = multiCosignVisibleSealSummary()
                if (signerSummary.trim() !== "") extraOptions.visibleSealSignerSummary = signerSummary.trim()
            }
            if (signQREnabled && signQRContent.trim() !== "") body.qrContent = normalizedQrUrl(signQRContent)
            if (signReason.trim() !== "") body.reason = signReason.trim()
            if (signLocation.trim() !== "") body.location = signLocation.trim()
            if (signContactInfo.trim() !== "") body.contactInfo = signContactInfo.trim()
            if (Object.keys(extraOptions).length > 0) body.extraOptions = extraOptions
        }
        if (body.format === "facturae") {
            const extraOptions = {}
            if (facturaePolicyVersion.trim() !== "") extraOptions.facturaePolicyVersion = facturaePolicyVersion.trim()
            if (facturaePolicyIdentifier.trim() !== "") extraOptions.policyIdentifier = facturaePolicyIdentifier.trim()
            if (facturaePolicyIdentifierHash.trim() !== "") extraOptions.policyIdentifierHash = facturaePolicyIdentifierHash.trim()
            if (facturaePolicyQualifier.trim() !== "") extraOptions.policyQualifier = facturaePolicyQualifier.trim()
            if (facturaeSignerRole.trim() !== "") extraOptions.signerClaimedRole = facturaeSignerRole.trim()
            if (facturaeSignatureCity.trim() !== "") extraOptions.signatureProductionCity = facturaeSignatureCity.trim()
            if (facturaeSignatureProvince.trim() !== "") extraOptions.signatureProductionProvince = facturaeSignatureProvince.trim()
            if (facturaeSignaturePostalCode.trim() !== "") extraOptions.signatureProductionPostalCode = facturaeSignaturePostalCode.trim()
            if (facturaeSignatureCountry.trim() !== "") extraOptions.signatureProductionCountry = facturaeSignatureCountry.trim()
            if (Object.keys(extraOptions).length > 0) body.extraOptions = extraOptions
        }
        return body
    }

    function shouldPrecheckSealSignerSummary(payload) {
        if (!payload)
            return false
        if (!signVisibleSeal)
            return false
        if (String(payload.action || signAction) !== "cosign")
            return false
        if (String(payload.format || effectiveSignFormat()) !== "pades")
            return false
        return currentFilePath !== ""
    }

    function enqueueAutoVerification(context) {
        let queue = pendingAutoVerificationQueue.slice()
        queue.push(context)
        pendingAutoVerificationQueue = queue
        processNextAutoVerification()
    }

    function processNextAutoVerification() {
        if (autoVerificationInProgress || pendingAutoVerificationQueue.length === 0) return
        let queue = pendingAutoVerificationQueue.slice()
        let next = queue.shift()
        pendingAutoVerificationQueue = queue
        autoVerificationContext = next
        autoVerificationInProgress = true
        window.requestVerification(next.path, "", false)
    }

    function verificationPayload(success, message, details) {
        if (details && typeof details === "object" && Object.keys(details).length > 0) {
            const payload = Object.assign({}, details)
            if (!success)
                payload.valid = false
            else if (payload.valid === undefined)
                payload.valid = true
            if ((!payload.reason || payload.reason === "") && message)
                payload.reason = message
            return payload
        }
        return {
            valid: false,
            reason: message,
            details: [],
            signers: [],
            format: "",
            coverage: "",
            integrity: { status: "invalid", reason: message, details: [] },
            certificate: { status: "unknown", reason: "", details: [] },
            trust: { status: "unknown", reason: "", details: [] },
            signerSummaries: [],
            warnings: [],
            errors: [message],
            evidence: []
        }
    }

    function verificationStatusText(status) {
        const key = String(status || "").toLowerCase()
        if (key === "valid") return tr("Válida")
        if (key === "invalid") return tr("Inválida")
        if (key === "warning") return tr("Con advertencias")
        return tr("Desconocido")
    }

    function verificationAspectColor(status) {
        const key = String(status || "").toLowerCase()
        if (key === "valid") return "#2ecc71"
        if (key === "invalid") return "#e74c3c"
        if (key === "warning") return "#f39c12"
        return "#bdc3c7"
    }

    function verificationAspectStatus(details, name) {
        if (!details || !details[name])
            return ""
        return String(details[name].status || "").trim().toLowerCase()
    }

    function verificationOutcomeKind(details) {
        if (!details)
            return "incomplete"

        const integrity = verificationAspectStatus(details, "integrity")
        const certificate = verificationAspectStatus(details, "certificate")
        const trust = verificationAspectStatus(details, "trust")

        if (details.valid === false
                || integrity === "invalid"
                || certificate === "invalid"
                || trust === "invalid") {
            return "invalid"
        }
        if (details.valid === true
                && integrity === "valid"
                && certificate === "valid"
                && trust === "valid") {
            return "trusted"
        }
        if (details.valid === true
                && integrity === "valid"
                && (trust === "" || trust === "unknown" || trust === "warning")) {
            return "untrusted"
        }
        return "incomplete"
    }

    function verificationOutcomeLabel(details) {
        const outcome = verificationOutcomeKind(details)
        if (outcome === "trusted") return tr("Válida y confiable")
        if (outcome === "invalid") return tr("No válida")
        if (outcome === "untrusted") return tr("Integridad válida; confianza no evaluada")
        return tr("Verificación incompleta")
    }

    function verificationOutcomeDisplay(details) {
        const outcome = verificationOutcomeKind(details)
        if (outcome === "trusted") return "✅ " + verificationOutcomeLabel(details)
        if (outcome === "invalid") return "❌ " + verificationOutcomeLabel(details)
        return "⚠️ " + verificationOutcomeLabel(details)
    }

    function verificationOutcomeColor(details) {
        const outcome = verificationOutcomeKind(details)
        if (outcome === "trusted") return "#2ecc71"
        if (outcome === "invalid") return "#e74c3c"
        return "#f39c12"
    }

    // Igual, pero legible sobre un fondo del tema: en temas claros usa tonos
    // oscuros para mantener contraste AA.
    function verificationOutcomeColorOn(details, background) {
        const outcome = verificationOutcomeKind(details)
        const light = Contrast.luminance(background) > 0.5
        if (outcome === "trusted") return light ? "#18794e" : "#2ecc71"
        if (outcome === "invalid") return light ? currentTheme.errorColor : "#ff8a80"
        return light ? "#8a5300" : "#f39c12"
    }

    function verificationAutoMessage(details) {
        const outcome = verificationOutcomeKind(details)
        if (outcome === "trusted") return tr("Firma completada y verificada correctamente.")
        if (outcome === "untrusted") return tr("Firma completada; integridad válida, confianza no evaluada.")
        return tr("Firma completada, pero la verificación ha devuelto incidencias.")
    }

    function verificationAspectDetailsText(aspect) {
        if (!aspect || !aspect.details || aspect.details.length === 0)
            return tr("No disponible")
        return aspect.details.map((item) => verificationDetailText(item)).join("\n")
    }

    // Sustituye %s y %d de los textos del catálogo compartido con el motor, en orden.
    function catalogFormat(key, args) {
        let index = 0
        return tr(key).replace(/%[sd]/g, function(match) {
            return index < args.length ? String(args[index++]) : match
        })
    }

    // Igual que informeverificacion.TraducirDetalle: «formato_detectado=PAdES»
    // pasa a «Formato detectado: PAdES» en el idioma de la aplicación.
    function verificationDetailText(line) {
        const text = String(line || "").trim()
        const prefix = "verificacion.detalle."
        const revocation = text.match(/^cert\[\d+\] (.+): (bueno|revocado|desconocido)(?: vía (\w+))?(?: \((.*)\))?$/)
        if (revocation) {
            let state = tr(prefix + "revocacion." + revocation[2])
            if (revocation[3]) state += " (" + revocation[3].toUpperCase() + ")"
            return catalogFormat(prefix + "formato", [revocation[1], state])
        }
        const cut = text.indexOf("=")
        if (cut <= 0 || /[ \t]/.test(text.substring(0, cut)))
            return localizeVisibleDiagnosticText(text)
        const key = text.substring(0, cut)
        const value = text.substring(cut + 1)
        let label = ""
        const coverage = key.match(/^cobertura_firma_pdf_(\d{1,4})$/)
        if (coverage) {
            label = catalogFormat(prefix + "cobertura_firma_pdf", [coverage[1]])
        } else if (tr(prefix + key) !== prefix + key) {
            label = tr(prefix + key)
        } else {
            return text
        }
        let shown = value
        const revision = value.match(/^revision_hasta_(\d{1,12})_de_(\d{1,12})$/)
        if (revision) {
            shown = catalogFormat(prefix + "valor.revision_hasta", [revision[1], revision[2]])
        } else if (value !== "" && tr(prefix + "valor." + value) !== prefix + "valor." + value) {
            shown = tr(prefix + "valor." + value)
        }
        return catalogFormat(prefix + "formato", [label, shown])
    }

    function verificationArrayText(values) {
        if (!values || values.length === 0)
            return tr("No disponible")
        return values.map((item) => localizeVisibleDiagnosticText(item)).join("\n")
    }

    // Firmantes como los lee una persona (igual que el informe del motor y
    // WinUI): nombre, NIF, organización, emisor y fecha con su origen. Los DN
    // completos quedan en verificationSignerTechnicalText.
    function verificationSignerEntries(details) {
        let entries = []
        let seen = []
        const summaries = (details && details.signerSummaries) ? details.signerSummaries : []
        for (let i = 0; i < summaries.length; i++) {
            const item = summaries[i] || {}
            const subject = String(item.subject || "").trim()
            const issuer = String(item.issuer || "").trim()
            const parsed = Signers.parseDistinguishedName(subject)
            const date = Signers.signingTimeText(item.signingTime, item.signingTimeSource, tr("format.datetime_seconds"))
            let dateText = ""
            if (date !== "")
                dateText = catalogFormat(item.signingTimeSource === "timestamp"
                                         ? "report.signer.time_from_timestamp"
                                         : "report.signer.time_declared", [date])
            entries.push({
                name: subject !== "" ? Signers.readableName(subject) : String(item.id || "").trim(),
                identifier: parsed.identifier,
                organization: parsed.organization,
                issuer: issuer !== "" ? Signers.readableIssuer(issuer) : "",
                date: date,
                dateText: dateText,
                subjectDn: subject,
                issuerDn: issuer,
                fingerprint: String(item.fingerprint || "").trim()
            })
            if (subject !== "") seen.push(subject)
        }
        const signers = (details && details.signers) ? details.signers : []
        for (let j = 0; j < signers.length; j++) {
            const dn = String(signers[j] || "").trim()
            if (dn === "" || seen.indexOf(dn) !== -1) continue
            seen.push(dn)
            const parsed = Signers.parseDistinguishedName(dn)
            entries.push({ name: Signers.readableName(dn), identifier: parsed.identifier,
                           organization: parsed.organization, issuer: "", date: "", dateText: "",
                           subjectDn: dn, issuerDn: "", fingerprint: "" })
        }
        return entries
    }

    function verificationSignerLabelLine(labelKey, value) {
        return catalogFormat("verificacion.detalle.formato", [tr(labelKey), value])
    }

    function verificationSignerSummariesText(details) {
        const entries = verificationSignerEntries(details)
        if (entries.length === 0)
            return tr("No disponible")
        let blocks = []
        for (let i = 0; i < entries.length; i++) {
            const entry = entries[i]
            let lines = ["• " + entry.name]
            if (entry.identifier !== "") lines.push("   " + verificationSignerLabelLine("report.signer.identifier", entry.identifier))
            if (entry.organization !== "" && entry.organization !== entry.name)
                lines.push("   " + verificationSignerLabelLine("report.signer.organization", entry.organization))
            if (entry.issuer !== "") lines.push("   " + verificationSignerLabelLine("report.signer.issuer", entry.issuer))
            if (entry.dateText !== "") lines.push("   " + verificationSignerLabelLine("report.signer.signing_time", entry.dateText))
            blocks.push(lines.join("\n"))
        }
        return blocks.join("\n")
    }

    // Una línea por firmante para los resúmenes breves: nombre y fecha.
    function verificationSignerShortText(details) {
        const entries = verificationSignerEntries(details)
        if (entries.length === 0)
            return tr("No disponible")
        return entries.map(function(entry) {
            return entry.date !== "" ? tr("verificacion.firmante_con_fecha").arg(entry.name).arg(entry.date) : entry.name
        }).join("; ")
    }

    // Los DN completos y la huella, para los detalles técnicos.
    function verificationSignerTechnicalText(details) {
        const entries = verificationSignerEntries(details)
        let lines = []
        for (let i = 0; i < entries.length; i++) {
            const entry = entries[i]
            if (entry.subjectDn !== "") lines.push(catalogFormat("report.technical.subject_dn", [entry.subjectDn]))
            if (entry.issuerDn !== "") lines.push(catalogFormat("report.technical.issuer_dn", [entry.issuerDn]))
            if (entry.fingerprint !== "") lines.push(verificationSignerLabelLine("report.signer.fingerprint", entry.fingerprint))
        }
        return lines.join("\n")
    }

    // Cobertura que devuelve el motor (full, partial, chain, unknown).
    function verificationHasHtmlReport(details) {
        return !!(details && typeof details.reportHtml === "string" && details.reportHtml !== "")
    }

    function verificationCoverageText(value) {
        const key = String(value || "").trim().toLowerCase()
        if (key === "full") return tr("winui.verificar.completa")
        if (key === "partial") return tr("winui.verificar.parcial")
        if (key === "chain") return tr("verificacion.cobertura.cadena")
        return tr("winui.verificar.no_determinada")
    }

    function verificationEvidenceText(details) {
        if (!details || !details.evidence || details.evidence.length === 0)
            return tr("No disponible")
        let rows = []
        for (let i = 0; i < details.evidence.length; i++) {
            const item = details.evidence[i]
            const type = item && item.type ? item.type : tr("No disponible")
            const summary = item && item.summary ? item.summary : ""
            rows.push(tr("Tipo: ") + type + (summary !== "" ? " · " + summary : ""))
        }
        return rows.join("\n")
    }

    function applySingleAutoVerificationResult(success, message, details, path) {
        if (signResultKind !== "success" || signResultPath !== path || currentOutputPath !== path)
            return
        currentOutputVerificationDetails = verificationPayload(success, message, details)
        currentOutputVerificationMessage = verificationAutoMessage(currentOutputVerificationDetails)
        verifyTab.verifyFilePath = path
        verifyTab.verifyDetails = currentOutputVerificationDetails
        statusMessage = currentOutputVerificationMessage
        if (signResultKind === "success" && signResultPath === path) {
            signResultVerificationPending = false
            signResultHeading.Accessible.announce(signResultAnnouncement())
        }
    }

    function signResultAnnouncement() {
        if (signResultKind === "success") {
            let result = tr("Documento firmado correctamente") + ". " + basename(signResultPath)
            if (signResultVerificationPending) return result + ". " + tr("Comprobando la firma...")
            if (currentOutputVerificationDetails !== null && signResultPath === currentOutputPath)
                return result + ". " + signVerificationResultText()
            return result
        }
        if (signResultKind === "error")
            return tr("No se pudo firmar el documento") + ". " + signResultCause + ". "
                    + tr("Comprueba que el documento siga disponible y que el certificado permita firmar; después vuelve a intentarlo.")
        return ""
    }

    function signVerificationResultText() {
        if (verificationOutcomeKind(currentOutputVerificationDetails) === "trusted")
            return tr("Firma verificada")
        const reason = currentOutputVerificationDetails && currentOutputVerificationDetails.reason
                       ? localizeVisibleDiagnosticText(currentOutputVerificationDetails.reason) : ""
        if (reason === "")
            return currentOutputVerificationMessage
        // Plantilla con el motivo: el mensaje ya acaba en punto y no se le
        // pegan «: » detrás.
        const template = verificationOutcomeKind(currentOutputVerificationDetails) === "untrusted"
                         ? "verificacion.auto.sin_confianza_motivo"
                         : "verificacion.auto.incidencias_motivo"
        return tr(template).arg(reason)
    }

    function applyBatchAutoVerificationResult(index, success, message, details) {
        let items = currentBatchResults.slice()
        if (index < 0 || index >= items.length) return
        let item = Object.assign({}, items[index])
        item.verifyDetails = verificationPayload(success, message, details)
        item.verifyDone = true
        item.verifyOutcome = verificationOutcomeKind(item.verifyDetails)
        item.verifyMessage = message
        item.verifyReason = item.verifyDetails && item.verifyDetails.reason ? item.verifyDetails.reason : message
        items[index] = item
        currentBatchResults = items
    }

    function refreshBatchVerificationSummary() {
        if (currentBatchResults.length === 0) return
        let pending = 0
        let untrusted = 0
        let incomplete = 0
        let invalid = 0
        for (let i = 0; i < currentBatchResults.length; i++) {
            const item = currentBatchResults[i]
            if (!item.ok || !item.outputPath) continue
            if (!item.verifyDone) {
                pending++
                continue
            }
            const outcome = item.verifyOutcome || verificationOutcomeKind(item.verifyDetails)
            if (outcome === "invalid") invalid++
            else if (outcome === "untrusted") untrusted++
            else if (outcome !== "trusted") incomplete++
        }
        if (pending > 0) {
            statusMessage = tr("Lote firmado. Verificando resultados...")
            return
        }
        if (invalid > 0) {
            statusMessage = tr("Lote firmado, con incidencias de verificación en algunos resultados.")
        } else if (incomplete > 0) {
            statusMessage = tr("Verificación incompleta")
        } else if (untrusted > 0) {
            statusMessage = tr("Lote firmado. Integridad válida, pero confianza no evaluada en algunos resultados.")
        } else {
            statusMessage = tr("Lote firmado y verificado correctamente.")
        }
    }

    function firstBatchFailureMessage(results) {
        if (!results || results.length === 0) return ""
        for (let i = 0; i < results.length; i++) {
            const item = results[i]
            if (item && !item.ok && item.error) return String(item.error)
        }
        return ""
    }

    function handleAutoVerificationFinished(success, message, details) {
        const ctx = autoVerificationContext
        autoVerificationContext = null
        autoVerificationInProgress = false
        if (!ctx) return
        if (ctx.kind === "single") {
            applySingleAutoVerificationResult(success, message, details, ctx.path)
        } else if (ctx.kind === "batch") {
            applyBatchAutoVerificationResult(ctx.index, success, message, details)
            refreshBatchVerificationSummary()
        }
        processNextAutoVerification()
        if (!autoVerificationInProgress && pendingAutoVerificationQueue.length === 0
                && autoClose && ctx.kind !== "single" && signResultKind === "") {
            if (closeBehavior === "resident") {
                keepResidentAfterClose()
            } else {
                flushSettingsNow()
                Qt.quit()
            }
        }
    }

    // --- DIALOGOS ---
    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: fileDialog
        title: tr("Seleccionar documento")
        nameFilters: [tr("Todos los archivos (*)")]
        onAccepted: {
            window.useSingleSelection(localPathFromUrl(selectedFile))
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: multiFileDialog
        title: tr("Seleccionar varios documentos")
        fileMode: FileDialog.OpenFiles
        nameFilters: [tr("Todos los archivos (*)")]
        onAccepted: {
            window.useMultiSelection(localPathsFromUrls(selectedFiles))
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: batchDirectoryDialog
        title: tr("Seleccionar cualquier fichero de la carpeta de entrada")
        nameFilters: [tr("Todos los archivos (*)")]
        onAccepted: {
            const selectedPath = localPathFromUrl(selectedFile)
            const dir = dirname(selectedPath)
            if (dir !== "") window.useDirectorySelection(dir)
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: batchOutputDirectoryDialog
        title: tr("Seleccionar cualquier fichero de la carpeta de salida del lote")
        nameFilters: [tr("Todos los archivos (*)")]
        onAccepted: {
            const selectedPath = localPathFromUrl(selectedFile)
            const dir = dirname(selectedPath)
            if (dir !== "") window.currentBatchOutputDir = dir
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: saveFileDialog
        title: tr("Seleccionar destino del PDF firmado")
        currentFile: window.fileUrlFromLocalPath(window.currentOutputPath)
        fileMode: FileDialog.SaveFile
        nameFilters: [tr("Archivos PDF (*.pdf)")]
        onAccepted: {
            window.currentOutputPath = localPathFromUrl(selectedFile)
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: verifyFileDialog
        title: tr("Seleccionar documento firmado")
        nameFilters: [tr("Documentos firmados (*.pdf *.p7s *.xsig *.xml)"), tr("Todos los archivos (*)")]
        onAccepted: {
            let path = localPathFromUrl(selectedFile)
            window.rememberSessionDocumentPath(path)
            verifyTab.verifyFilePath = path
            verifyTab.verifyDetails = null
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: verifyOriginalFileDialog
        title: tr("Seleccionar documento original de referencia")
        nameFilters: [tr("Todos los archivos (*)")]
        onAccepted: {
            verifyTab.verifyOriginalPath = localPathFromUrl(selectedFile)
            verifyTab.verifyDetails = null
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: protectFileDialog
        title: tr("Seleccionar fichero a proteger")
        nameFilters: [tr("Todos los archivos (*)")]
        onAccepted: {
            window.clearTransientProtectionSecrets()
            const path = localPathFromUrl(selectedFile)
            window.rememberSessionDocumentPath(path)
            window.protectInputPath = path
            window.protectResult = null
        }
        onRejected: window.clearTransientProtectionSecrets()
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: unprotectFileDialog
        title: tr("Seleccionar fichero protegido")
        nameFilters: [
            tr("Contenedores protegidos (*.afp *.enveloped *.p7m)"),
            tr("Todos los archivos (*)")
        ]
        onAccepted: {
            window.clearTransientProtectionSecrets()
            window.unprotectInputPath = localPathFromUrl(selectedFile)
            window.unprotectResult = null
        }
        onRejected: window.clearTransientProtectionSecrets()
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: verifyReportSaveDialog
        title: tr("Guardar informe de verificación")
        fileMode: FileDialog.SaveFile
        currentFile: suggestVerificationReportPath(verifyTab.verifyFilePath)
        nameFilters: [tr("Informe JSON (*.json)"), tr("Todos los archivos (*)")]
        onAccepted: {
            backend.exportVerificationReport(
                localPathFromUrl(selectedFile),
                verifyTab.verifyDetails || {},
                verifyTab.verifyFilePath,
                verifyTab.verifyOriginalPath)
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: verifyHtmlReportSaveDialog
        title: tr("winui.parity.verify.export_html")
        fileMode: FileDialog.SaveFile
        nameFilters: [tr("verificacion.informe_html.filtro"), tr("Todos los archivos (*)")]
        onAccepted: {
            backend.saveTextReport(localPathFromUrl(selectedFile),
                                   String((verifyTab.verifyDetails && verifyTab.verifyDetails.reportHtml) || ""))
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: verifySummarySaveDialog
        title: tr("Guardar resumen de validación")
        fileMode: FileDialog.SaveFile
        currentFile: suggestVerificationSummaryPath(verifyTab.verifyFilePath)
        nameFilters: [tr("Resumen de validación (*.txt)"), tr("Todos los archivos (*)")]
        onAccepted: {
            backend.saveTextReport(
                localPathFromUrl(selectedFile),
                window.buildVerificationUserSummary(
                    verifyTab.verifyDetails || {},
                    verifyTab.verifyFilePath,
                    verifyTab.verifyOriginalPath))
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: hashInputFileDialog
        title: tr("Seleccionar fichero para huella")
        nameFilters: [tr("Todos los archivos (*)")]
        onAccepted: {
            verifyTab.hashInputPath = localPathFromUrl(selectedFile)
            verifyTab.hashInputIsDirectory = false
            verifyTab.hashCreateResult = null
            verifyTab.hashCheckResult = null
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: hashInputDirectoryDialog
        title: tr("Seleccionar cualquier fichero de la carpeta a comprobar")
        nameFilters: [tr("Todos los archivos (*)")]
        onAccepted: {
            const selectedPath = localPathFromUrl(selectedFile)
            const dir = dirname(selectedPath)
            if (dir !== "") {
                verifyTab.hashInputPath = dir
                verifyTab.hashInputIsDirectory = true
                verifyTab.hashCreateResult = null
                verifyTab.hashCheckResult = null
            }
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: hashReferenceDialog
        title: tr("Seleccionar huella o manifiesto")
        nameFilters: [tr("Huellas y manifiestos (*.hexhash *.hashb64 *.hash *.hashfiles *.txthashfiles *.csv)"), tr("Todos los archivos (*)")]
        onAccepted: {
            verifyTab.hashReferencePath = localPathFromUrl(selectedFile)
            verifyTab.hashCheckResult = null
        }
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: localTLSStartupNoticeDialog
        title: tr("Conexión segura con navegadores")
        modal: true
        anchors.centerIn: parent
        width: Math.min(560, window.width - 48)
        standardButtons: Dialog.Close
        accessibleName: title
        accessibleDescription: localTLSStartupNoticeText.text

        Text {
            id: localTLSStartupNoticeText
            width: localTLSStartupNoticeDialog.availableWidth
            wrapMode: Text.WordWrap
            color: currentTheme.textColor
            text: window.localTLSStartupNoticeState === "error"
                  ? tr("No se pudo instalar la CA local de GrxFirma en todos los navegadores. Cierra Firefox por completo y vuelve a abrir GrxFirma; si persiste, comprueba que certutil está instalado.")
                  : tr("GrxFirma ha instalado su CA local para conectar de forma segura con los portales desde el navegador. Cierra Firefox por completo y vuelve a abrirlo antes de firmar.")
        }
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: aboutDialog
        title: tr("Acerca de")
        modal: true
        anchors.centerIn: parent
        width: Math.min(560, window.width - 48)
        height: Math.min(720, window.height - 48)
        standardButtons: Dialog.Close
        accessibleName: tr("Acerca de GrxFirma")
        accessibleDescription: tr("Información de versión, autoría y licencia de la aplicación")

        // Con pantallas bajas el contenido no cabe: se desplaza en vez de cortarse.
        ScrollView {
            id: aboutScroll
            anchors.fill: parent
            clip: true
            contentWidth: availableWidth

        ColumnLayout {
            width: Math.max(320, aboutScroll.availableWidth)
            spacing: 14

            Rectangle {
                Layout.alignment: Qt.AlignHCenter
                Layout.preferredWidth: Math.min(400, aboutScroll.availableWidth)
                Layout.preferredHeight: 150
                color: "#ffffff"
                radius: 12
                border.color: Qt.rgba(0, 0, 0, 0.12)
                border.width: 1

                Image {
                    anchors.fill: parent
                    anchors.margins: 16
                    source: "../assets/Logo-Horizontal-Color.png"
                    fillMode: Image.PreserveAspectFit
                    smooth: true
                    Accessible.role: Accessible.Graphic
                    Accessible.name: tr("Logotipo de GrxFirma")
                }
            }

            Text {
                text: tr("GrxFirma")
                color: currentTheme.textColor
                font.pixelSize: 24
                font.bold: true
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
                Accessible.role: Accessible.StaticText
                Accessible.name: text
            }

            Text {
                visible: window.aboutApplicationVersion !== ""
                text: tr("Versión %1").arg(window.aboutApplicationVersion)
                color: currentTheme.secondaryTextColor
                font.pixelSize: 15
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
                Accessible.role: Accessible.StaticText
                Accessible.name: text
            }

            ThemedButton {
                Layout.alignment: Qt.AlignHCenter
                text: tr("Novedades")
                onClicked: {
                    aboutDialog.close()
                    window.openReleaseNotes(true)
                }
                Accessible.name: text
            }

            Rectangle {
                Layout.fillWidth: true
                Layout.preferredHeight: 1
                color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity + 0.12)
            }

            Text {
                text: tr("Autoría: Alberto Avidad Fernández")
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
                Accessible.role: Accessible.StaticText
                Accessible.name: text
            }

            Text {
                text: tr("Derechos de autor (C) 2026 Alberto Avidad Fernández.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
                Accessible.role: Accessible.StaticText
                Accessible.name: text
            }

            Text {
                text: tr("Licencia: EUPL 1.2 o posterior")
                color: currentTheme.secondaryTextColor
                font.bold: true
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
                Accessible.role: Accessible.StaticText
                Accessible.name: text
            }

            Rectangle {
                Layout.fillWidth: true
                Layout.preferredHeight: 1
                color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity + 0.12)
            }

            Text {
                text: window.updateStatusMessage
                color: window.updateAvailable
                       ? currentTheme.primaryColor
                       : currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignHCenter
                Layout.fillWidth: true
                Accessible.role: Accessible.StaticText
                Accessible.name: text
            }

            RowLayout {
                Layout.alignment: Qt.AlignHCenter
                spacing: 10

                ThemedButton {
                    text: window.updateCheckInProgress
                          ? tr("Comprobando…")
                          : tr("Comprobar actualizaciones")
                    enabled: !window.updateCheckInProgress
                    onClicked: window.requestUpdateCheck(true)
                    Accessible.name: text
                    Accessible.description: tr("Consulta GitHub sin descargar ni instalar archivos.")
                }

                ThemedButton {
                    visible: window.updateAvailable && window.updateReleaseUrl !== ""
                    text: tr("Ver versión en GitHub")
                    onClicked: window.openOfficialUpdateRelease()
                    Accessible.name: text
                    Accessible.description: tr("Abre la página oficial de la versión; la aplicación no descarga ni ejecuta archivos.")
                }
            }
        }
        }
    }

    ThemedDialog {
        theme: currentTheme
        id: releaseNotesDialog
        property string notesText: ""
        title: tr("Novedades de GrxFirma %1").arg(window.aboutApplicationVersion)
        modal: true
        anchors.centerIn: parent
        width: Math.min(640, window.width - 48)
        height: Math.min(620, window.height - 48)
        standardButtons: Dialog.NoButton
        accessibleName: title
        ScrollView {
            anchors.fill: parent
            clip: true
            TextArea {
                readOnly: true
                wrapMode: TextEdit.Wrap
                textFormat: TextEdit.MarkdownText
                text: releaseNotesDialog.notesText !== ""
                      ? releaseNotesDialog.notesText
                      : tr("Novedades no disponibles en esta instalación.")
                Accessible.name: tr("Novedades instaladas")
            }
        }
        footer: ThemedButton {
            text: tr("Entendido")
            onClicked: {
                if (window.releaseNotesAcknowledge)
                    appSettings.lastSeenVersion = window.aboutApplicationVersion
                releaseNotesDialog.close()
            }
        }
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: signValidationErrorDialog
        title: tr("Atención")
        modal: true
        anchors.centerIn: parent
        standardButtons: Dialog.Ok
        property string errorMessage: ""
        // Titular propio para avisos que no son requisitos sin completar.
        property string heading: ""
        onClosed: heading = ""
        ColumnLayout {
            spacing: 10
            Text {
                text: signValidationErrorDialog.heading !== "" ? signValidationErrorDialog.heading : tr("⚠️ Requisitos faltantes")
                color: currentTheme.textColor
                font.bold: true
            }
            Text {
                text: signValidationErrorDialog.errorMessage
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.preferredWidth: 300
            }
        }
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: signConfirmDialog
        title: tr("Confirmar firma")
        modal: true
        anchors.centerIn: parent
        standardButtons: Dialog.Ok | Dialog.Cancel
        property int selectedCertIndex: -1
        onAccepted: window.executeSignRequest(selectedCertIndex)

        ColumnLayout {
            spacing: 10
            Text {
                text: window.isBatchMode()
                      ? tr("Se va a iniciar una firma por lote con el certificado seleccionado. Revise la selección antes de continuar.")
                      : tr("Se va a iniciar la firma del documento actual con el certificado seleccionado. Revise la selección antes de continuar.")
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
                Layout.preferredWidth: 360
            }
            Text {
                text: signConfirmDialog.selectedCertIndex >= 0 && signConfirmDialog.selectedCertIndex < certificates.length
                      ? tr("Certificado: %1").arg(window.certificateDisplayName(certificates[signConfirmDialog.selectedCertIndex]))
                      : tr("Certificado: no disponible")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.preferredWidth: 360
            }
        }
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: multiCosignDialog
        title: tr("Cofirma múltiple guiada")
        modal: true
        anchors.centerIn: parent
        standardButtons: Dialog.Ok | Dialog.Cancel
        width: 560
        property var draftIds: []
        property string draftPrimaryId: ""

        onOpened: {
            draftPrimaryId = window.effectiveMultiCosignPrimaryId()
            draftIds = window.sanitizeMultiCosignIdsForPrimary(
                draftPrimaryId,
                window.multiCosignCertificateIds.slice(),
                window.certificates)
        }
        onAccepted: {
            window.multiCosignPrimaryCertificateId = draftPrimaryId
            window.multiCosignCertificateIds = window.sanitizeMultiCosignIdsForPrimary(
                draftPrimaryId,
                draftIds.slice(),
                window.certificates)
            const primaryIndex = window.findCertificateIndexById(draftPrimaryId, window.certificates)
            if (primaryIndex !== -1) {
                window.selectCertificateIndex(primaryIndex, false)
            }
            window.statusMessage = tr("Firmantes adicionales actualizados.")
        }
        onDraftPrimaryIdChanged: {
            draftIds = window.sanitizeMultiCosignIdsForPrimary(
                draftPrimaryId,
                draftIds,
                window.certificates)
        }

        ColumnLayout {
            width: 520
            spacing: 10

            Text {
                text: tr("El firmante principal se aplicará primero. Los certificados marcados aquí se aplicarán después como cofirmas secuenciales.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            Text {
                text: tr("Orden aplicado: primero el firmante principal y después esta lista en el orden mostrado.")
                color: currentTheme.secondaryTextColor
                font.pixelSize: 11
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 10

                Text {
                    text: tr("Firmante principal")
                    color: currentTheme.textColor
                    Layout.preferredWidth: 160
                }

                ComboBox {
                    id: multiCosignPrimaryCombo
                    Layout.fillWidth: true
                    model: window.signingCertificates().filter(window.certificateCanSign).map(function(cert) {
                        return { texto: window.certificateDisplayName(cert), valor: window.certificateId(cert) }
                    })
                    textRole: "texto"
                    valueRole: "valor"
                    currentIndex: optionIndexByValue(model, multiCosignDialog.draftPrimaryId)
                    onActivated: function(index) {
                        if (index >= 0 && index < model.length) {
                            multiCosignDialog.draftPrimaryId = model[index].valor
                        }
                    }
                    onCurrentValueChanged: {
                        const value = String(currentValue || "")
                        if (value !== "" && value !== multiCosignDialog.draftPrimaryId) {
                            multiCosignDialog.draftPrimaryId = value
                        }
                    }
                    ToolTip.visible: hovered
                    ToolTip.text: tr("El certificado elegido aquí firmará primero. Los demás se añadirán como cofirmas en secuencia.")
                }
            }

            Rectangle {
                Layout.fillWidth: true
                Layout.preferredHeight: 280
                radius: 8
                color: currentTheme.cardColor
                border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)

                ScrollView {
                    anchors.fill: parent
                    anchors.margins: 8
                    clip: true

                    ColumnLayout {
                        width: parent.width
                        spacing: 8

                        Repeater {
                            model: window.availableAdditionalCertificates()
                            delegate: ThemedCheckBox {
                                Layout.fillWidth: true
                                text: window.certificateDisplayName(modelData)
                                      + " · "
                                      + (modelData.issuerName || modelData.issuer || "")
                                checked: multiCosignDialog.draftIds.indexOf(window.certificateId(modelData)) !== -1
                                onToggled: {
                                    const id = window.certificateId(modelData)
                                    let next = multiCosignDialog.draftIds.slice()
                                    const pos = next.indexOf(id)
                                    if (checked && pos === -1) {
                                        next.push(id)
                                    } else if (!checked && pos !== -1) {
                                        next.splice(pos, 1)
                                    }
                                    multiCosignDialog.draftIds = next
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    ThemedDialog {
        theme: currentTheme
        id: signedDocumentWarningDialog
        title: tr("Documento ya firmado")
        modal: true
        anchors.centerIn: parent

        ColumnLayout {
            spacing: 10

            Text {
                text: signedDocumentWarningSummary
                color: currentTheme.textColor
                wrapMode: Text.Wrap
                Layout.preferredWidth: 460
                Layout.maximumWidth: 460
            }
            RowLayout {
                Layout.alignment: Qt.AlignRight
                spacing: 8

                ThemedButton {
                    text: tr("Cambiar a Cofirmar")
                    onClicked: {
                        pendingSignedDocumentWarningContext = null
                        signAction = "cosign"
                        window.statusMessage = tr("Se ha cambiado la operación a Cofirmar.")
                        signedDocumentWarningDialog.close()
                    }
                }
                ThemedButton {
                    text: tr("Firmar igualmente")
                    onClicked: {
                        const ctx = pendingSignedDocumentWarningContext
                        pendingSignedDocumentWarningContext = null
                        skipSignedDocumentWarningOnce = true
                        signedDocumentWarningDialog.close()
                        if (ctx)
                            Qt.callLater(function() { window.executeSignRequest(ctx.selectedCertIndex) })
                    }
                }
            }
        }
    }

    ThemedDialog {
        theme: currentTheme
        id: certificateValidationDialog
        title: tr("Verificación de certificado")
        modal: true
        anchors.centerIn: parent
        property string validationMessage: ""
        property bool validationOk: false
        property string validationBaseReport: ""
        property string validationReport: ""
        property var validationDetails: ({})
        property bool onlineCheckInProgress: false
        property var onlineCheckResult: ({})
        property string requestedCertificateId: ""

        ColumnLayout {
            spacing: 10
            Text {
                text: certificateValidationDialog.validationOk ? tr("Certificado válido para firma") : tr("Certificado con incidencias")
                color: currentTheme.textColor
                font.bold: true
            }
            RowLayout {
                Layout.preferredWidth: 420
                Layout.maximumWidth: 420
                spacing: 6

                Text {
                    text: tr("Estado: ")
                    color: currentTheme.textColor
                    font.bold: true
                }
                Text {
                    text: (certificateValidationDialog.validationDetails && certificateValidationDialog.validationDetails.statusText)
                          ? certificateValidationDialog.validationDetails.statusText
                          : (certificateValidationDialog.validationOk ? tr("Válido") : tr("Con incidencias"))
                    color: currentTheme.textColor
                    font.bold: true
                    Layout.fillWidth: true
                    wrapMode: Text.Wrap
                }
            }
            RowLayout {
                visible: !!(certificateValidationDialog.validationDetails && (certificateValidationDialog.validationDetails.expiredDays > 0 || certificateValidationDialog.validationDetails.daysRemaining !== undefined))
                Layout.preferredWidth: 420
                Layout.maximumWidth: 420
                spacing: 6

                Text {
                    text: (certificateValidationDialog.validationDetails && certificateValidationDialog.validationDetails.expiredDays > 0)
                          ? tr("Días caducado: ")
                          : tr("Días restantes: ")
                    color: currentTheme.textColor
                    font.bold: true
                }
                Text {
                    text: (certificateValidationDialog.validationDetails && certificateValidationDialog.validationDetails.expiredDays > 0)
                          ? String(certificateValidationDialog.validationDetails.expiredDays)
                          : String((certificateValidationDialog.validationDetails && certificateValidationDialog.validationDetails.daysRemaining !== undefined)
                                   ? certificateValidationDialog.validationDetails.daysRemaining
                                   : 0)
                    color: currentTheme.textColor
                    font.bold: true
                }
            }
            Text {
                text: certificateValidationMessageBody(certificateValidationDialog.validationMessage)
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WrapAnywhere
                Layout.preferredWidth: 420
                Layout.maximumWidth: 420
            }
            RowLayout {
                Layout.preferredWidth: 420
                Layout.maximumWidth: 420
                spacing: 6

                Text {
                    text: tr("Revocación online: ")
                    color: currentTheme.textColor
                    font.bold: true
                }
                Text {
                    text: certificateValidationDialog.onlineCheckInProgress
                          ? tr("Comprobando...")
                          : window.certificateOnlineStatusText(certificateValidationDialog.onlineCheckResult)
                    color: currentTheme.secondaryTextColor
                    font.bold: true
                    Layout.fillWidth: true
                    wrapMode: Text.Wrap
                }
            }
            Text {
                visible: !!(certificateValidationDialog.onlineCheckResult && Object.keys(certificateValidationDialog.onlineCheckResult).length > 0)
                text: certificateValidationDialog.onlineCheckResult.userMessage
                      || certificateValidationDialog.onlineCheckResult.reason
                      || ""
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WrapAnywhere
                Layout.preferredWidth: 420
                Layout.maximumWidth: 420
            }
            Text {
                text: window.certificateOnlineStatusUserHint(certificateValidationDialog.onlineCheckResult)
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WrapAnywhere
                Layout.preferredWidth: 420
                Layout.maximumWidth: 420
            }
            RowLayout {
                Layout.alignment: Qt.AlignRight
                spacing: 8

                ThemedButton {
                    text: certificateValidationDialog.onlineCheckInProgress
                          ? tr("Comprobando...")
                          : tr("Comprobar revocación online")
                    enabled: !certificateValidationDialog.onlineCheckInProgress
                             && window.selectedCertData !== null
                             && certificateValidationDialog.requestedCertificateId !== ""
                    onClicked: {
                        certificateValidationDialog.onlineCheckInProgress = true
                        certificateValidationDialog.onlineCheckResult = ({})
                        window.statusMessage = tr("Comprobando online el estado del certificado...")
                        backend.checkCertificateOnline(certificateValidationDialog.requestedCertificateId)
                    }
                }
                ThemedButton {
                    text: tr("Copiar informe")
                    enabled: certificateValidationDialog.validationReport !== ""
                    onClicked: {
                        window.copyTextToClipboard(certificateValidationDialog.validationReport)
                        window.statusMessage = tr("Informe de validación del certificado copiado al portapapeles.")
                    }
                }
                ThemedButton {
                    text: tr("Guardar informe JSON")
                    enabled: certificateValidationDialog.validationReport !== ""
                    onClicked: certificateValidationSaveDialog.open()
                }
                ThemedButton {
                    text: tr("Cerrar")
                    onClicked: certificateValidationDialog.close()
                }
            }
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: certificateValidationSaveDialog
        title: tr("Guardar informe de validación del certificado")
        fileMode: FileDialog.SaveFile
        currentFile: suggestCertificateValidationReportPath()
        nameFilters: [tr("Informe JSON (*.json)"), tr("Todos los archivos (*)")]
        onAccepted: {
            backend.exportVerificationReport(
                localPathFromUrl(selectedFile),
                certificateValidationDialog.validationDetails || {},
                window.certificateId(window.selectedCertData),
                "")
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: supportIncidentSaveDialog
        title: tr("Guardar incidencia preparada")
        fileMode: FileDialog.SaveFile
        currentFile: suggestSupportIncidentPath()
        nameFilters: [tr("Informe de incidencia (*.txt)"), tr("Todos los archivos (*)")]
        onAccepted: {
            backend.saveTextReport(localPathFromUrl(selectedFile), window.supportAssistantExportText())
        }
    }

    ThemedDialog {
        theme: currentTheme
        id: activeDiagnosticsConsentDialog
        title: tr("Diagnóstico activo")
        modal: true
        anchors.centerIn: parent
        width: 620
        closePolicy: Popup.CloseOnEscape

        onOpened: {
            activeDiagnosticsConsentCheck.checked = false
            activeDiagnosticsConsentCheck.forceActiveFocus()
        }
        onClosed: activeDiagnosticsConsentCheck.checked = false

        ColumnLayout {
            anchors.fill: parent
            anchors.margins: 18
            spacing: 12

            Text {
                Layout.fillWidth: true
                text: tr("La operación ya ha fallado. Si aceptas, GrxFirma hará ahora comprobaciones acotadas para orientar la causa probable.")
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
            }

            Text {
                Layout.fillWidth: true
                text: tr("Según el contexto se comprobarán el canal local, la configuración de proxy, DNS, TCP y TLS. Cada prueba tiene un tiempo máximo y no se ejecuta durante una operación correcta.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
            }

            Rectangle {
                Layout.fillWidth: true
                radius: 8
                color: currentTheme.cardColor
                border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                implicitHeight: activeDiagnosticsPrivacyColumn.implicitHeight + 20

                ColumnLayout {
                    id: activeDiagnosticsPrivacyColumn
                    anchors.fill: parent
                    anchors.margins: 10
                    spacing: 6

                    Text {
                        Layout.fillWidth: true
                        text: tr("El informe no incluirá documentos, certificados, rutas, direcciones IP, credenciales ni errores crudos. Las comprobaciones solo contactarán con el motor local configurado.")
                        color: currentTheme.textColor
                        wrapMode: Text.WordWrap
                    }

                    Text {
                        Layout.fillWidth: true
                        text: tr("El resultado usará categorías limitadas y se añadirá como bloque separado a una nueva versión de la incidencia local.")
                        color: currentTheme.secondaryTextColor
                        wrapMode: Text.WordWrap
                    }
                }
            }

            ThemedCheckBox {
                id: activeDiagnosticsConsentCheck
                Layout.fillWidth: true
                text: tr("Acepto ejecutar ahora estas comprobaciones adicionales.")
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 8

                Item { Layout.fillWidth: true }

                ThemedButton {
                    text: tr("Cancelar")
                    onClicked: activeDiagnosticsConsentDialog.close()
                }

                ThemedButton {
                    text: tr("Diagnosticar ahora")
                    highlighted: true
                    enabled: activeDiagnosticsConsentCheck.checked
                             && window.activeFailureContext !== null
                             && !window.activeDiagnosticInProgress
                    onClicked: {
                        activeDiagnosticsConsentDialog.close()
                        window.startActiveDiagnostics()
                    }
                }
            }
        }
    }

    ThemedDialog {
        theme: currentTheme
        id: supportIncidentSendDialog
        title: tr("Enviar incidencia")
        modal: true
        anchors.centerIn: parent
        width: 620

        property bool consentAccepted: false
        property string sourceIncidentPath: ""

        ColumnLayout {
            anchors.fill: parent
            anchors.margins: 18
            spacing: 12

            Text {
                text: tr("La incidencia ya está guardada en local y se enviará solo al destino HTTPS que indiques.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }

            Text {
                text: tr("Las incidencias locales se eliminan automáticamente a los 30 días y solo se conservan las 50 más recientes.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }

            Text {
                text: tr("Destino HTTPS")
                color: currentTheme.textColor
                font.bold: true
                Layout.fillWidth: true
            }

            ThemedTextField {
                id: supportIncidentEndpointField
                Layout.fillWidth: true
                placeholderText: tr("https://soporte.ejemplo/incidents")
                text: appSettings.supportIncidentEndpoint
            }

            Text {
                text: tr("Se enviará la incidencia estructurada, la previsualización del paquete, el manifiesto y una cola de log redactada. No se enviarán documentos originales, rutas completas del perfil ni secretos.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }

            ThemedCheckBox {
                id: supportIncidentConsentCheck
                text: tr("Confirmo que quiero enviar esta incidencia al destino indicado.")
                checked: supportIncidentSendDialog.consentAccepted
                onToggled: supportIncidentSendDialog.consentAccepted = checked
                Layout.fillWidth: true
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 8

                Item { Layout.fillWidth: true }

                ThemedButton {
                    text: tr("Cancelar")
                    onClicked: supportIncidentSendDialog.close()
                }

                ThemedButton {
                    text: tr("Enviar incidencia")
                    highlighted: true
                    enabled: supportIncidentSendDialog.consentAccepted
                             && supportIncidentSendDialog.sourceIncidentPath !== ""
                             && supportIncidentEndpointField.text.trim() !== ""
                    onClicked: {
                        appSettings.supportIncidentEndpoint = supportIncidentEndpointField.text.trim()
                        const payload = window.currentSupportIncidentPayload()
                        payload.supportPreview.consent.remoteSend = true
                        backend.sendIncidentReport(payload,
                                                   appSettings.supportIncidentEndpoint,
                                                   supportIncidentSendDialog.sourceIncidentPath)
                        supportIncidentSendDialog.close()
                    }
                }
            }
        }
    }

    ThemedDialog {
        theme: currentTheme
        id: supportAssistantDialog
        title: tr("Asistente guiado")
        modal: true
        anchors.centerIn: parent
        width: 700
        height: 700

        ColumnLayout {
            anchors.fill: parent
            anchors.margins: 18
            spacing: 12

            Text {
                text: tr("Ayuda paso a paso para la operación actual")
                color: currentTheme.textColor
                font.bold: true
                font.pixelSize: 18
                Layout.fillWidth: true
            }

            Text {
                text: tr("Este asistente resume el contexto actual, sugiere los siguientes pasos y deja a mano el soporte técnico sin abrir un chat libre.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 10

                Text {
                    text: tr("Modo")
                    color: currentTheme.textColor
                }

                ThemedButton {
                    text: tr("Usuario")
                    highlighted: supportAssistantMode === "usuario"
                    onClicked: {
                        supportAssistantMode = "usuario"
                        diagnosticTechnicalExpanded = false
                    }
                }

                ThemedButton {
                    text: tr("Experto")
                    highlighted: supportAssistantMode === "experto"
                    onClicked: {
                        supportAssistantMode = "experto"
                        diagnosticTechnicalExpanded = true
                    }
                }

                Item { Layout.fillWidth: true }
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 10

                Text {
                    text: tr("Necesito ayuda con")
                    color: currentTheme.textColor
                    Layout.preferredWidth: 170
                }

                ComboBox {
                    Layout.fillWidth: true
                    model: window.supportAssistantGoalOptions()
                    textRole: "texto"
                    valueRole: "valor"
                    currentIndex: optionIndexByValue(model, window.supportAssistantGoal)
                    onActivated: function(index) {
                        if (index >= 0 && index < model.length) {
                            window.supportAssistantGoal = model[index].valor
                        }
                    }
                }
            }

            ScrollView {
                id: supportAssistantBodyScroll
                Layout.fillWidth: true
                Layout.fillHeight: true
                clip: true
                contentWidth: availableWidth

                Column {
                    id: supportAssistantBody
                    width: supportAssistantBodyScroll.availableWidth
                    spacing: 12

                    Rectangle {
                        width: parent.width
                        radius: 10
                        color: currentTheme.cardColor
                        border.color: currentTheme.primaryColor
                        border.width: 1
                        implicitHeight: supportAssistantSummaryColumn.implicitHeight + 28

                        ColumnLayout {
                            id: supportAssistantSummaryColumn
                            anchors.fill: parent
                            anchors.margins: 14
                            spacing: 8

                            Text {
                                text: tr("Qué está pasando")
                                color: currentTheme.textColor
                                font.bold: true
                                Layout.fillWidth: true
                            }

                            Text {
                                text: window.supportAssistantFriendlySummary()
                                color: currentTheme.secondaryTextColor
                                textFormat: Text.PlainText
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            RowLayout {
                                Layout.fillWidth: true
                                spacing: 8

                                Text {
                                    text: "\u2302"
                                    color: currentTheme.textColor
                                    font.bold: true
                                    font.pixelSize: 18
                                    Accessible.ignored: true
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 2

                                    Text {
                                        Layout.fillWidth: true
                                        text: tr("Responsable probable: %1").arg(
                                                  window.supportAssistantOwnerLabel())
                                        textFormat: Text.PlainText
                                        color: currentTheme.textColor
                                        font.bold: true
                                        wrapMode: Text.WordWrap
                                    }

                                    Text {
                                        Layout.fillWidth: true
                                        text: window.supportAssistantResponsibility()
                                        textFormat: Text.PlainText
                                        color: currentTheme.secondaryTextColor
                                        font.pixelSize: 11
                                        wrapMode: Text.WordWrap
                                    }
                                }
                            }

                            RowLayout {
                                Layout.fillWidth: true
                                spacing: 8

                                Text {
                                    text: "\u2192"
                                    color: currentTheme.textColor
                                    font.bold: true
                                    font.pixelSize: 18
                                    Accessible.ignored: true
                                }

                                Text {
                                    Layout.fillWidth: true
                                    text: tr("Qué hacer ahora: %1").arg(
                                              window.supportAssistantSuggestedAction())
                                    textFormat: Text.PlainText
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                }
                            }

                            Text {
                                text: tr("Ámbito de resolución: %1").arg(window.supportAssistantResolutionScope())
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                font.pixelSize: 11
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                visible: window.supportAssistantKnownIssue().known
                                text: window.supportAssistantKnownIssue().fixedIn && window.supportAssistantKnownIssue().fixedIn !== ""
                                      ? tr("Problema conocido: %1 (corregido en %2)")
                                            .arg(window.supportAssistantKnownIssue().summary)
                                            .arg(window.supportAssistantKnownIssue().fixedIn)
                                      : tr("Problema conocido: %1")
                                            .arg(window.supportAssistantKnownIssue().summary)
                                color: currentTheme.secondaryTextColor
                                font.pixelSize: 11
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }
                        }
                    }

                    Rectangle {
                        width: parent.width
                        visible: window.activeFailureContext !== null
                        radius: 10
                        color: currentTheme.cardColor
                        border.color: currentTheme.primaryColor
                        border.width: 1
                        implicitHeight: operationDiagnosticColumn.implicitHeight + 28

                        ColumnLayout {
                            id: operationDiagnosticColumn
                            anchors.fill: parent
                            anchors.margins: 14
                            spacing: 10

                            Text {
                                Layout.fillWidth: true
                                text: tr("Diagnóstico de la operación")
                                textFormat: Text.PlainText
                                color: currentTheme.textColor
                                font.bold: true
                                font.pixelSize: 16
                                wrapMode: Text.WordWrap
                                Accessible.role: Accessible.Heading
                                Accessible.name: text
                            }

                            Text {
                                Layout.fillWidth: true
                                text: tr("Solo se muestran fases realmente observadas por el motor. Lo que no se pudo comprobar queda como desconocido.")
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                            }

                            DiagnosticTimeline {
                                Layout.fillWidth: true
                                steps: window.diagnosticVisualSteps()
                                theme: currentTheme
                                accessibleName: tr("Diagnóstico de la operación")
                                emptyTitle: tr("No comprobado")
                                emptyMessage: tr("Aún no hay evidencia suficiente para determinar el origen exacto.")
                            }

                            ThemedButton {
                                Layout.fillWidth: true
                                text: window.diagnosticTechnicalExpanded
                                      ? tr("Ocultar detalles")
                                      : tr("Mostrar detalles")
                                onClicked: window.diagnosticTechnicalExpanded =
                                           !window.diagnosticTechnicalExpanded
                                Accessible.name: text
                                Accessible.description:
                                    tr("El resultado es orientativo y no se presenta como una certeza.")
                            }
                        }
                    }

                    Rectangle {
                        width: parent.width
                        radius: 10
                        color: currentTheme.cardColor
                        border.color: currentTheme.primaryColor
                        border.width: 1
                        implicitHeight: supportAssistantActionColumn.implicitHeight + 28

                        ColumnLayout {
                            id: supportAssistantActionColumn
                            anchors.fill: parent
                            anchors.margins: 14
                            spacing: 10

                            Text {
                                text: tr("Qué puede hacer por ti ahora")
                                color: currentTheme.textColor
                                font.bold: true
                                Layout.fillWidth: true
                            }

                            Text {
                                text: window.supportAssistantPrimaryActionHint()
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            ThemedButton {
                                Layout.fillWidth: true
                                text: window.activeDiagnosticInProgress
                                      ? tr("Diagnosticando...")
                                      : tr("Diagnosticar ahora")
                                visible: window.activeFailureContext !== null
                                enabled: !window.activeDiagnosticInProgress
                                highlighted: true
                                onClicked: activeDiagnosticsConsentDialog.open()
                            }

                            RowLayout {
                                Layout.fillWidth: true
                                spacing: 10

                                ThemedButton {
                                    text: window.supportAssistantPrimaryActionLabel()
                                    highlighted: true
                                    onClicked: window.runSupportAssistantPrimaryAction()
                                }

                                ThemedButton {
                                    visible: window.firstSignFieldError !== "" &&
                                             (window.supportAssistantGoal === "sign-failure" || window.activeTab === "firmar")
                                    text: tr("validacion.corregir")
                                    onClicked: {
                                        supportAssistantDialog.close()
                                        window.focusSignField(window.firstSignFieldError)
                                    }
                                }

                                ThemedButton {
                                    text: tr("Abrir ayuda")
                                    // No repetir el botón cuando la acción principal ya es abrir la ayuda.
                                    visible: window.supportAssistantPrimaryActionLabel() !== tr("Abrir ayuda")
                                    onClicked: backend.openHelpManual()
                                }

                                ThemedButton {
                                    text: tr("Abrir incidencias")
                                    visible: window.lastIncidentReportPath !== ""
                                    onClicked: backend.openIncidentFolder()
                                }

                                Item { Layout.fillWidth: true }
                            }

                            Text {
                                visible: window.lastIncidentReportPath !== ""
                                text: tr("Última incidencia guardada: %1").arg(window.lastIncidentReportPath)
                                color: currentTheme.secondaryTextColor
                                font.pixelSize: 11
                                wrapMode: Text.WrapAnywhere
                                Layout.fillWidth: true
                            }
                        }
                    }

                    Rectangle {
                        width: parent.width
                        visible: window.activeDiagnosticResult !== null
                        radius: 10
                        color: currentTheme.cardColor
                        border.color: currentTheme.primaryColor
                        border.width: 1
                        implicitHeight: activeDiagnosticResultColumn.implicitHeight + 28

                        ColumnLayout {
                            id: activeDiagnosticResultColumn
                            anchors.fill: parent
                            anchors.margins: 14
                            spacing: 8

                            Text {
                                id: activeDiagnosticResultHeading
                                Layout.fillWidth: true
                                text: tr("Resultado del diagnóstico activo")
                                color: currentTheme.textColor
                                font.bold: true
                                wrapMode: Text.WordWrap
                                activeFocusOnTab: true
                                Accessible.role: Accessible.StaticText
                                Accessible.name: text
                            }

                            Text {
                                Layout.fillWidth: true
                                text: tr("Qué está pasando: %1").arg(
                                          window.activeDiagnosticCodeText(
                                              window.activeDiagnosticResult
                                              ? window.activeDiagnosticResult.probableCause
                                              : ""))
                                textFormat: Text.PlainText
                                color: currentTheme.textColor
                                wrapMode: Text.WordWrap
                            }

                            Text {
                                Layout.fillWidth: true
                                text: tr("Responsabilidad probable: %1").arg(
                                          window.activeDiagnosticCodeText(
                                              window.activeDiagnosticResult
                                              ? window.activeDiagnosticResult.likelyOwner
                                              : ""))
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                            }

                            Text {
                                Layout.fillWidth: true
                                text: tr("Siguiente acción recomendada: %1").arg(
                                          window.activeDiagnosticCodeText(
                                              window.activeDiagnosticResult
                                              ? window.activeDiagnosticResult.suggestedAction
                                              : ""))
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                            }

                            Text {
                                Layout.fillWidth: true
                                text: tr("El resultado es orientativo y no se presenta como una certeza.")
                                color: currentTheme.secondaryTextColor
                                font.italic: true
                                wrapMode: Text.WordWrap
                            }

                            DiagnosticTimeline {
                                Layout.fillWidth: true
                                steps: window.activeDiagnosticVisualSteps()
                                theme: currentTheme
                                accessibleName: tr("Diagnóstico de la operación")
                                emptyTitle: tr("No comprobado")
                                emptyMessage: tr("Aún no hay evidencia suficiente para determinar el origen exacto.")
                            }

                            Repeater {
                                model: window.diagnosticTechnicalExpanded
                                       && window.activeDiagnosticResult
                                       && window.activeDiagnosticResult.probes
                                       ? window.activeDiagnosticResult.probes : []
                                delegate: Text {
                                    Layout.fillWidth: true
                                    text: window.activeDiagnosticCodeText(
                                              modelData.kind) + ": "
                                          + window.activeDiagnosticCodeText(
                                              modelData.state)
                                    textFormat: Text.PlainText
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 11
                                    wrapMode: Text.WordWrap
                                }
                            }
                        }
                    }

                    Rectangle {
                        width: parent.width
                        radius: 10
                        color: currentTheme.cardColor
                        border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                        border.width: 1
                        implicitHeight: supportAssistantBundleColumn.implicitHeight + 28

                        ColumnLayout {
                            id: supportAssistantBundleColumn
                            anchors.fill: parent
                            anchors.margins: 14
                            spacing: 10

                            Text {
                                text: tr("Qué incluirá la incidencia")
                                color: currentTheme.textColor
                                font.bold: true
                                Layout.fillWidth: true
                            }

                            Text {
                                text: tr("Antes de guardar o enviar nada, este es el paquete de soporte que se preparará para ti.")
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                text: tr("Archivos del paquete")
                                color: currentTheme.textColor
                                font.bold: true
                                Layout.fillWidth: true
                            }

                            Repeater {
                                model: window.supportIncidentBundleFiles()
                                delegate: Text {
                                    Layout.fillWidth: true
                                    text: "\u2022 " + modelData
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WrapAnywhere
                                }
                            }

                            Text {
                                text: tr("Se incluirá")
                                color: currentTheme.textColor
                                font.bold: true
                                Layout.fillWidth: true
                            }

                            Repeater {
                                model: window.supportIncidentIncludedData()
                                delegate: Text {
                                    Layout.fillWidth: true
                                    text: "\u2022 " + modelData
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                }
                            }

                            Text {
                                text: tr("Se ocultará")
                                color: currentTheme.textColor
                                font.bold: true
                                Layout.fillWidth: true
                            }

                            Repeater {
                                model: window.supportIncidentOmittedData()
                                delegate: Text {
                                    Layout.fillWidth: true
                                    text: "\u2022 " + modelData
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                }
                            }
                        }
                    }

                    Rectangle {
                        width: parent.width
                        visible: window.diagnosticTechnicalExpanded
                        radius: 10
                        color: currentTheme.cardColor
                        border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                        border.width: 1
                        implicitHeight: supportAssistantStateColumn.implicitHeight + 28

                        ColumnLayout {
                            id: supportAssistantStateColumn
                            anchors.fill: parent
                            anchors.margins: 14
                            spacing: 8

                            Text {
                                text: tr("Estado actual")
                                color: currentTheme.textColor
                                font.bold: true
                            }

                            Text {
                                text: tr("Paso actual: %1").arg(window.supportAssistantCurrentStep())
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                text: tr("Operación o fichero: %1").arg(window.supportAssistantOperationLabel())
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                text: tr("Último estado visible: %1").arg(window.statusMessage !== "" ? localizeVisibleDiagnosticText(window.statusMessage) : tr("Sin mensaje todavía"))
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }
                        }
                    }

                    Rectangle {
                        width: parent.width
                        radius: 10
                        color: currentTheme.cardColor
                        border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                        border.width: 1
                        implicitHeight: supportAssistantStepsColumn.implicitHeight + 28

                        ColumnLayout {
                            id: supportAssistantStepsColumn
                            anchors.fill: parent
                            anchors.margins: 14
                            spacing: 12

                            Text {
                                text: tr("Siguientes pasos")
                                color: currentTheme.textColor
                                font.bold: true
                                Layout.fillWidth: true
                            }

                            Repeater {
                                model: window.supportAssistantSteps()
                                delegate: Rectangle {
                                    Layout.fillWidth: true
                                    radius: 8
                                    color: currentTheme.cardColor
                                    border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                                    border.width: 1
                                    implicitHeight: supportAssistantStepRow.implicitHeight + 18

                                    RowLayout {
                                        id: supportAssistantStepRow
                                        anchors.fill: parent
                                        anchors.margins: 9
                                        spacing: 10

                                        Rectangle {
                                            Layout.alignment: Qt.AlignTop
                                            width: 24
                                            height: 24
                                            radius: 12
                                            color: currentTheme.textColor

                                            Text {
                                                anchors.centerIn: parent
                                                text: String(index + 1)
                                                color: currentTheme.cardColor
                                                font.bold: true
                                            }
                                        }

                                        Text {
                                            text: modelData
                                            color: currentTheme.secondaryTextColor
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                        }
                                    }
                                }
                            }
                        }
                    }

                    Rectangle {
                        width: parent.width
                        radius: 8
                        color: currentTheme.cardColor
                        border.color: currentTheme.secondaryTextColor
                        border.width: 1
                        visible: window.diagnosticTechnicalExpanded
                        implicitHeight: supportAssistantExpertColumn.implicitHeight + 20

                        ColumnLayout {
                            id: supportAssistantExpertColumn
                            anchors.fill: parent
                            anchors.margins: 10
                            spacing: 6

                            Text {
                                text: tr("Vista experta")
                                color: currentTheme.textColor
                                font.bold: true
                            }

                            Text {
                                text: tr("Certificados cargados: %1").arg(window.certificates.length)
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                            }

                            Text {
                                text: tr("Lote activo: %1").arg(window.currentBatchPaths.length > 0 ? tr("Sí") : tr("No"))
                                color: currentTheme.secondaryTextColor
                            }

                            Text {
                                text: tr("Detalles de validación disponibles: %1").arg(verifyTab.verifyDetails !== null ? tr("Sí") : tr("No"))
                                color: currentTheme.secondaryTextColor
                            }

                            Text {
                                text: tr("Razón técnica visible: %1").arg(verifyTab.verifyDetails && verifyTab.verifyDetails.reason ? localizeVisibleDiagnosticText(verifyTab.verifyDetails.reason) : tr("No disponible"))
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                visible: window.currentFailureDiagnostic()
                                         && window.currentFailureDiagnostic().failureCode
                                text: tr("Código estable: %1").arg(
                                          window.currentFailureDiagnostic()
                                          && window.currentFailureDiagnostic().failureCode
                                          ? window.currentFailureDiagnostic().failureCode
                                          : tr("No disponible"))
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                visible: window.currentFailureRequestId() !== ""
                                text: tr("RequestId: %1").arg(
                                          window.currentFailureRequestId() !== ""
                                          ? window.currentFailureRequestId()
                                          : tr("No disponible"))
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                visible: window.currentFailureTraceId() !== ""
                                text: tr("TraceId: %1").arg(
                                          window.currentFailureTraceId() !== ""
                                          ? window.currentFailureTraceId()
                                          : tr("No disponible"))
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                text: tr("Responsable probable: %1").arg(
                                          window.currentFailureDiagnostic()
                                          && window.currentFailureDiagnostic().likelyOwner
                                          ? window.diagnosticOwnerText(
                                                window.currentFailureDiagnostic().likelyOwner)
                                          : tr("No disponible"))
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }

                            Text {
                                text: tr("Detalle experto: %1").arg(
                                          window.currentFailureDiagnostic()
                                          && window.currentFailureDiagnostic().expertMessage
                                          ? localizeVisibleDiagnosticText(
                                                window.currentFailureDiagnostic().expertMessage)
                                          : tr("No disponible"))
                                textFormat: Text.PlainText
                                color: currentTheme.secondaryTextColor
                                wrapMode: Text.WordWrap
                                Layout.fillWidth: true
                            }
                        }
                    }
                }
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 8

                ThemedButton {
                    text: supportAssistantGoal === "support" ? tr("Preparar incidencia") : tr("Generar diagnóstico")
                    ToolTip.visible: hovered
                    ToolTip.text: tr("Genera un diagnóstico para soporte. Según el backend activo, se dejará en el log y puede copiarse también al portapapeles.")
                    onClicked: {
                        copyTextToClipboard(window.supportAssistantExportText())
                        window.statusMessage = tr("Resumen de incidencia copiado al portapapeles.")
                        backend.exportDiagnosticReport()
                    }
                }

                ThemedButton {
                    text: tr("Guardar resumen")
                    onClicked: supportIncidentSaveDialog.open()
                }

                ThemedButton {
                    text: tr("Enviar incidencia")
                    visible: window.activeFailureContext !== null
                             && String(window.activeFailureContext.incidentPath || "") !== ""
                    enabled: visible
                    onClicked: {
                        supportIncidentSendDialog.consentAccepted = false
                        supportIncidentSendDialog.sourceIncidentPath =
                                String(window.activeFailureContext.incidentPath || "")
                        supportIncidentEndpointField.text = appSettings.supportIncidentEndpoint
                        supportIncidentSendDialog.open()
                    }
                }

                ThemedButton {
                    text: tr("Ir a experto")
                    visible: supportAssistantMode === "experto"
                    onClicked: {
                        activeTab = "experto"
                        supportAssistantDialog.close()
                    }
                }

                Item { Layout.fillWidth: true }

                ThemedButton {
                    text: tr("Cerrar")
                    onClicked: supportAssistantDialog.close()
                }
            }
        }
    }

    ThemedDialog {
        theme: currentTheme
        id: unsavedSettingsDialog
        title: tr("Cambios sin guardar")
        modal: true
        anchors.centerIn: parent

        ColumnLayout {
            width: 420
            spacing: 12

            Text {
                text: tr("Hay cambios en las preferencias de la app que todavía no se han guardado.")
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            Text {
                text: tr("¿Quieres guardar los cambios antes de cerrar?")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }

            RowLayout {
                Layout.alignment: Qt.AlignRight
                spacing: 8

                ThemedButton {
                    text: tr("Cancelar")
                    onClicked: unsavedSettingsDialog.close()
                }
                ThemedButton {
                    text: tr("Descartar cambios")
                    onClicked: {
                        unsavedSettingsDialog.close()
                        window.discardBackendSettingsChanges(true)
                    }
                }
                ThemedButton {
                    text: tr("Guardar y cerrar")
                    enabled: !window.settingsSaveInFlight
                    onClicked: {
                        unsavedSettingsDialog.close()
                        window.saveBackendSettingsAndClose()
                    }
                }
            }
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: p12FileDialog
        title: tr("Importar certificado al perfil local (.p12, .pfx)")
        nameFilters: [tr("Certificados (*.p12 *.pfx)"), tr("Todos los archivos (*)")]
        onAccepted: {
            importPasswordDialog.open()
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: temporaryCertificateFileDialog
        title: tr("Usar certificado solo durante esta sesión")
        nameFilters: [
            tr("Credenciales (*.p12 *.pfx *.pem)"),
            tr("Todos los archivos (*)")
        ]
        onAccepted: temporaryCertificatePasswordDialog.open()
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: guidedImportCertificateFileDialog
        title: tr("Importar certificado en el almacén seleccionado")
        nameFilters: [tr("Certificados (*.p12 *.pfx)"), tr("Todos los archivos (*)")]
        onAccepted: guidedImportPasswordDialog.open()
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: sealImageFileDialog
        title: tr("Seleccionar imagen de firma")
        nameFilters: [tr("Imágenes (*.png *.jpg *.jpeg)"), tr("Todos los archivos (*)")]
        onAccepted: {
            sealStyle = "image"
            signSealImagePath = selectedFile.toString()
        }
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: recipientPublicFileDialog
        title: tr("Importar certificado público del destinatario")
        nameFilters: [tr("Certificados públicos (*.cer *.crt *.pem *.der)"), tr("Todos los archivos (*)")]
        onAccepted: backend.importProtectionRecipient(localPathFromUrl(selectedFile))
    }

    FileDialog {
        acceptLabel: fileMode === FileDialog.SaveFile ? tr("Guardar") : tr("Abrir")
        rejectLabel: tr("Cancelar")
        id: publicCertificateSaveDialog
        title: tr("Exportar certificado público")
        fileMode: FileDialog.SaveFile
        nameFilters: [tr("Certificado público DER (*.cer)"), tr("Certificado público PEM (*.pem)")]
        onAccepted: {
            let path = localPathFromUrl(selectedFile)
            if (!path.toLowerCase().endsWith(".cer") && !path.toLowerCase().endsWith(".pem"))
                path += selectedNameFilter.index === 1 ? ".pem" : ".cer"
            const format = path.toLowerCase().endsWith(".pem") ? "pem" : "der"
            backend.exportPublicCertificate(window.publicCertificateExportId, path, format)
        }
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: publicCertificateSelectDialog
        title: tr("Elegir mi certificado")
        standardButtons: Dialog.Ok | Dialog.Cancel
        anchors.centerIn: parent
        width: Math.min(480, window.width - 40)
        onAccepted: {
            const own = (window.certificates || []).filter(function(cert) { return cert.canSign || cert.needsUnlock })
            const cert = own[publicCertificateCombo.currentIndex]
            if (cert) {
                window.publicCertificateExportId = window.certificateId(cert)
                publicCertificateSaveDialog.open()
            }
        }
        contentItem: ComboBox {
            id: publicCertificateCombo
            model: (window.certificates || []).filter(function(cert) { return cert.canSign || cert.needsUnlock })
            textRole: "subjectName"
            Accessible.name: tr("Mi certificado público")
        }
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: publicCertificateResultDialog
        standardButtons: Dialog.Ok
        anchors.centerIn: parent
        width: Math.min(520, window.width - 40)
        contentItem: Text {
            id: publicCertificateResultText
            wrapMode: Text.WordWrap
            color: currentTheme.textColor
        }
    }

    TextArea {
        id: clipboardProxy
        x: -10000
        y: -10000
        width: 1
        height: 1
        opacity: 0
        wrapMode: TextEdit.NoWrap
        textFormat: TextEdit.PlainText
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: cscRemoteDialog
        title: tr("csc.gui.titulo")
        accessibleDescription: tr("csc.gui.descripcion")
        modal: true
        anchors.centerIn: parent
        width: Math.min(560, window.width - 40)
        standardButtons: Dialog.Close

        ColumnLayout {
            width: parent ? parent.width : 520
            spacing: 10
            Text {
                Layout.fillWidth: true
                text: tr("csc.gui.descripcion")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
            }
            Text {
                Layout.fillWidth: true
                visible: window.cscProhibited
                text: tr("csc.error.prohibida")
                color: currentTheme.textColor
                font.bold: true
                wrapMode: Text.WordWrap
                Accessible.role: Accessible.AlertMessage
                Accessible.name: text
            }
            Label {
                visible: window.cscAllowed
                text: tr("csc.gui.url")
                color: currentTheme.textColor
                font.bold: true
            }
            ThemedTextField {
                id: cscServiceUrlField
                visible: window.cscAllowed
                Layout.fillWidth: true
                enabled: !window.cscBusy && window.cscState.connected !== true
                inputMethodHints: Qt.ImhUrlCharactersOnly | Qt.ImhNoAutoUppercase
                Accessible.name: tr("csc.gui.url")
                Accessible.description: tr("csc.gui.url_ayuda")
                onTextEdited: window.cscDiscovery = null
            }
            Text {
                Layout.fillWidth: true
                visible: window.cscAllowed
                text: tr("csc.gui.url_ayuda")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
            }
            Label {
                visible: window.cscAllowed
                text: tr("csc.gui.client_id")
                color: currentTheme.textColor
                font.bold: true
            }
            ThemedTextField {
                id: cscClientIdField
                visible: window.cscAllowed
                Layout.fillWidth: true
                enabled: !window.cscBusy && window.cscState.connected !== true
                inputMethodHints: Qt.ImhNoAutoUppercase | Qt.ImhNoPredictiveText
                Accessible.name: tr("csc.gui.client_id")
                Accessible.description: tr("csc.gui.client_id_ayuda")
                onTextEdited: window.cscDiscovery = null
            }
            Label {
                visible: window.cscAllowed
                text: tr("csc.gui.client_id_ayuda")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            ThemedButton {
                text: window.cscBusy && window.cscDiscovery === null ? tr("csc.gui.comprobando") : tr("csc.gui.comprobar")
                visible: window.cscAllowed && window.cscState.connected !== true
                enabled: !window.cscBusy && cscServiceUrlField.text.trim() !== "" && cscClientIdField.text.trim() !== ""
                onClicked: {
                    window.cscBusy = true
                    window.cscMessage = ""
                    window.cscDiscovery = null
                    backend.cscConfigure(cscServiceUrlField.text, cscClientIdField.text)
                }
            }
            GridLayout {
                visible: window.cscDiscovery !== null
                Layout.fillWidth: true
                columns: 2
                columnSpacing: 8
                Label { text: tr("csc.gui.host_servicio"); color: currentTheme.secondaryTextColor }
                Label {
                    Layout.fillWidth: true
                    text: window.cscDiscovery ? String(window.cscDiscovery.serviceHost || "") : ""
                    textFormat: Text.PlainText
                    color: currentTheme.textColor
                    font.bold: true
                    wrapMode: Text.WrapAnywhere
                }
                Label { text: tr("csc.gui.host_oauth"); color: currentTheme.secondaryTextColor }
                Label {
                    Layout.fillWidth: true
                    text: window.cscDiscovery ? String(window.cscDiscovery.oauthHost || "") : ""
                    textFormat: Text.PlainText
                    color: currentTheme.textColor
                    font.bold: true
                    wrapMode: Text.WrapAnywhere
                }
            }
            Text {
                Layout.fillWidth: true
                visible: window.cscDiscovery !== null && window.cscState.connected !== true
                text: tr("csc.gui.aviso_navegador")
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
            }
            RowLayout {
                Layout.fillWidth: true
                ThemedButton {
                    text: tr("csc.gui.conectar")
                    visible: window.cscDiscovery !== null && window.cscState.connected !== true
                    enabled: !window.cscBusy
                    onClicked: {
                        window.cscBusy = true
                        window.cscMessage = tr("csc.gui.conectando")
                        backend.cscConnect()
                    }
                }
                ThemedButton {
                    text: tr("csc.gui.desconectar")
                    visible: window.cscState.connected === true
                    enabled: !window.cscBusy
                    onClicked: {
                        window.cscBusy = true
                        backend.cscDisconnect()
                    }
                }
            }
            Text {
                Layout.fillWidth: true
                visible: window.cscMessage !== ""
                text: window.cscMessage
                textFormat: Text.PlainText
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
                Accessible.role: Accessible.AlertMessage
                Accessible.name: window.cscMessage
            }
        }
    }

    ThemedDialog {
        theme: currentTheme
        id: cscRemoteSecretDialog
        title: tr("csc.gui.dialogo_titulo")
        accessibleDescription: tr("csc.gui.dialogo_texto")
        modal: true
        anchors.centerIn: parent
        width: Math.min(460, window.width - 40)
        standardButtons: Dialog.NoButton
        closePolicy: Popup.CloseOnEscape

        function submitSecrets() {
            const cert = window.cscSecretCertificate
            const needPin = !!cert && cert.remotePin === true
            const needOtp = !!cert && cert.remoteOtp === true
            if ((needPin && cscPinField.text === "") || (needOtp && cscOtpField.text === "")) {
                cscSecretStatus.text = tr("csc.gui.falta_dato")
                return
            }
            window.cscPendingSecrets = {
                certificateId: String(cert.id || ""),
                pin: needPin ? cscPinField.text : "",
                otp: needOtp ? cscOtpField.text : ""
            }
            window.clearRemoteSecretFields()
            const index = window.cscSecretCertIndex
            const purpose = window.cscSecretPurpose
            cscRemoteSecretDialog.close()
            if (purpose === "protect")
                window.executeProtectSignRequest()
            else
                window.executeSignRequest(index)
        }

        ColumnLayout {
            width: parent ? parent.width : 420
            spacing: 10
            Text {
                Layout.fillWidth: true
                text: tr("csc.gui.dialogo_texto")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
            }
            Label {
                visible: !!window.cscSecretCertificate && window.cscSecretCertificate.remotePin === true
                text: tr("csc.gui.pin")
                color: currentTheme.textColor
                font.bold: true
            }
            ThemedTextField {
                id: cscPinField
                visible: !!window.cscSecretCertificate && window.cscSecretCertificate.remotePin === true
                Layout.fillWidth: true
                echoMode: TextInput.Password
                inputMethodHints: Qt.ImhSensitiveData | Qt.ImhNoPredictiveText | Qt.ImhHiddenText
                Accessible.name: tr("csc.gui.pin")
                onAccepted: cscRemoteSecretDialog.submitSecrets()
            }
            Label {
                visible: !!window.cscSecretCertificate && window.cscSecretCertificate.remoteOtp === true
                text: tr("csc.gui.otp")
                color: currentTheme.textColor
                font.bold: true
            }
            RowLayout {
                Layout.fillWidth: true
                visible: !!window.cscSecretCertificate && window.cscSecretCertificate.remoteOtp === true
                ThemedTextField {
                    id: cscOtpField
                    Layout.fillWidth: true
                    echoMode: TextInput.Password
                    inputMethodHints: Qt.ImhSensitiveData | Qt.ImhNoPredictiveText | Qt.ImhHiddenText
                    Accessible.name: tr("csc.gui.otp")
                    onAccepted: cscRemoteSecretDialog.submitSecrets()
                }
                ThemedButton {
                    visible: !!window.cscSecretCertificate && window.cscSecretCertificate.remoteOtpOnline === true
                    text: tr("csc.gui.enviar_codigo")
                    onClicked: {
                        cscSecretStatus.text = ""
                        backend.cscSendOtp(String(window.cscSecretCertificate.id || ""))
                    }
                }
            }
            Text {
                id: cscSecretStatus
                Layout.fillWidth: true
                visible: text !== ""
                textFormat: Text.PlainText
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
                Accessible.role: Accessible.AlertMessage
                Accessible.name: text
            }
            RowLayout {
                Layout.fillWidth: true
                Item { Layout.fillWidth: true }
                ThemedButton {
                    text: tr("csc.gui.cancelar")
                    onClicked: cscRemoteSecretDialog.close()
                }
                ThemedButton {
                    text: tr("csc.gui.firmar")
                    highlighted: true
                    onClicked: cscRemoteSecretDialog.submitSecrets()
                }
            }
        }
        onOpened: {
            if (cscPinField.visible) cscPinField.forceActiveFocus()
            else cscOtpField.forceActiveFocus()
        }
        onClosed: {
            window.clearRemoteSecretFields()
            window.cscSecretCertificate = null
        }
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: importPasswordDialog
        title: tr("Contraseña del Certificado")
        standardButtons: Dialog.Ok | Dialog.Cancel
        anchors.centerIn: parent
        modal: true
        
        ColumnLayout {
            spacing: 15; width: 350
            Text { text: tr("🔑 Contraseña Requerida"); color: currentTheme.textColor; font.bold: true; font.pixelSize: 18 }
            Text { text: tr("Introduzca la contraseña para importar el archivo P12/PFX."); color: currentTheme.secondaryTextColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
            ThemedTextField {
                id: importPasswordField
                echoMode: TextInput.Password
                placeholderText: tr("Contraseña...")
                Layout.fillWidth: true
                focus: true
                onAccepted: importPasswordDialog.accept()
            }
        }
        
        onAccepted: {
            let path = p12FileDialog.selectedFile.toString()
            backend.importCertificate(path, importPasswordField.text)
            importPasswordField.text = ""
        }
        onRejected: importPasswordField.text = ""
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: temporaryCertificatePasswordDialog
        title: tr("Usar certificado sin instalar")
        standardButtons: Dialog.Ok | Dialog.Cancel
        anchors.centerIn: parent
        modal: true

        ColumnLayout {
            width: 390
            spacing: 12
            Text {
                text: tr("La credencial se mantendrá solo en memoria mientras esta aplicación esté abierta. No se copiará al navegador ni al sistema.")
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            ThemedTextField {
                id: temporaryCertificatePasswordField
                echoMode: TextInput.Password
                placeholderText: tr("Contraseña (vacía para PEM sin cifrar)")
                Layout.fillWidth: true
                focus: true
                onAccepted: temporaryCertificatePasswordDialog.accept()
            }
        }
        onAccepted: {
            const path = temporaryCertificateFileDialog.selectedFile.toString()
            backend.useTemporaryCertificate(path, temporaryCertificatePasswordField.text)
            temporaryCertificatePasswordField.text = ""
        }
        onRejected: temporaryCertificatePasswordField.text = ""
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: guidedImportPasswordDialog
        title: tr("Importar en navegador o sistema")
        standardButtons: Dialog.Ok | Dialog.Cancel
        anchors.centerIn: parent
        modal: true

        ColumnLayout {
            width: 390
            spacing: 12
            Text {
                text: tr("Esta acción instalará el certificado de forma persistente en el almacén seleccionado.")
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            ThemedTextField {
                id: guidedImportPasswordField
                echoMode: TextInput.Password
                placeholderText: tr("Contraseña del P12/PFX")
                Layout.fillWidth: true
                focus: true
                onAccepted: guidedImportPasswordDialog.accept()
            }
        }
        onAccepted: {
            const target = certificateImportTargetCombo.currentValue
            const path = guidedImportCertificateFileDialog.selectedFile.toString()
            backend.importCertificateToStore(path, guidedImportPasswordField.text, target)
            guidedImportPasswordField.text = ""
        }
        onRejected: guidedImportPasswordField.text = ""
    }

    ThemedDialog {
        translate: function(key) { return window.tr(key) }
        theme: currentTheme
        id: certificateAccessDialog
        title: tr("Certificados del navegador o del sistema")
        modal: true
        anchors.centerIn: parent
        standardButtons: Dialog.Close

        ColumnLayout {
            width: 520
            spacing: 14

            Text {
                Layout.fillWidth: true
                text: tr("El uso temporal no instala nada. Las acciones de este cuadro sí abren o modifican un almacén persistente de forma explícita.")
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
            }
            Text {
                Layout.fillWidth: true
                text: certificateAccessOptions.detectedBrowser
                      ? tr("Navegador detectado: %1").arg(certificateAccessOptions.detectedBrowser)
                      : tr("No se pudo detectar un navegador preferido.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
            }

            Text {
                text: tr("Gestor que se va a abrir")
                color: currentTheme.textColor
                font.bold: true
            }
            ComboBox {
                id: certificateManagerCombo
                Layout.fillWidth: true
                model: certificateAccessOptions.managers || []
                textRole: "label"
                valueRole: "id"
            }
            ThemedButton {
                text: tr("Abrir gestor de certificados")
                enabled: certificateManagerCombo.currentValue !== undefined
                         && String(certificateManagerCombo.currentValue) !== ""
                onClicked: backend.openCertificateManager(certificateManagerCombo.currentValue)
            }

            Rectangle {
                Layout.fillWidth: true
                height: 1
                color: Qt.rgba(1, 1, 1, 0.15)
            }

            Text {
                text: tr("Almacén persistente de destino")
                color: currentTheme.textColor
                font.bold: true
            }
            ComboBox {
                id: certificateImportTargetCombo
                Layout.fillWidth: true
                model: certificateAccessOptions.importTargets || []
                textRole: "label"
                valueRole: "id"
            }
            ThemedButton {
                text: tr("Importar P12/PFX en este almacén")
                enabled: certificateImportTargetCombo.currentValue !== undefined
                         && String(certificateImportTargetCombo.currentValue) !== ""
                onClicked: guidedImportCertificateFileDialog.open()
            }
            Text {
                visible: (certificateAccessOptions.importTargets || []).length === 0
                Layout.fillWidth: true
                text: tr("No se encontró un almacén compatible. Puedes abrir el gestor e importar el P12/PFX manualmente.")
                color: currentTheme.secondaryTextColor
                wrapMode: Text.WordWrap
            }
        }
    }

    Component {
        id: noCertificatesPanelComponent
        Rectangle {
            color: Qt.rgba(1, 1, 1, 0.06)
            radius: 12
            border.color: Qt.rgba(1, 1, 1, 0.16)
            border.width: 1

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 14
                spacing: 9
                Text {
                    Layout.fillWidth: true
                    text: window.certificates.length === 0
                          ? tr("No hay certificados disponibles")
                          : tr("No hay certificados utilizables con los filtros actuales")
                    color: currentTheme.textColor
                    font.bold: true
                    font.pixelSize: 17
                    wrapMode: Text.WordWrap
                }
                Text {
                    Layout.fillWidth: true
                    text: window.certificates.length === 0
                          ? tr("Puedes usar un P12/PFX o un bundle PEM solo durante esta sesión, importarlo de forma explícita o abrir el gestor del navegador o del sistema.")
                          : tr("Los certificados encontrados están caducados, no son aptos para firma o están ocultos por los filtros. Puedes cambiar los filtros o añadir otro certificado.")
                    color: currentTheme.secondaryTextColor
                    wrapMode: Text.WordWrap
                }
                ThemedButton {
                    Layout.fillWidth: true
                    text: tr("Usar certificado sin instalar")
                    enabled: window.guidedCertificateAccessAvailable()
                    onClicked: temporaryCertificateFileDialog.open()
                }
                ThemedButton {
                    Layout.fillWidth: true
                    text: tr("Importar o abrir gestor")
                    onClicked: window.openGuidedCertificateAccess()
                }
                ThemedButton {
                    Layout.fillWidth: true
                    text: tr("Actualizar certificados")
                    onClicked: backend.refreshCertificates()
                }
                ThemedButton {
                    Layout.fillWidth: true
                    text: tr("Cancelar")
                    flat: true
                    onClicked: window.showNoCertificateHelp = false
                }
            }
        }
    }

    // --- LOGICA DE BACKEND ---
    Connections {
        target: backend
        function onCertificatesLoaded(certs) {
            if (portalSealMode) {
                window.requestPdfPreview()
                return
            }
            console.log(tr("QML: Certificados recibidos:"), certs.length)
            backend.cscStatus()
            window.certificates = certs
            window.syncCertificateSelection(certs)
            if (certs.length > 0) {
                window.showNoCertificateHelp = true
            }
            if (window.pendingTemporaryCertificateId !== "") {
                const temporaryIndex = window.findCertificateIndexById(window.pendingTemporaryCertificateId, certs)
                if (temporaryIndex !== -1) {
                    window.selectCertificateIndex(temporaryIndex, false)
                    window.pendingTemporaryCertificateId = ""
                }
            }
        }
        function onStatusChanged() {
            window.statusMessage = backend.status
        }
        function onIncidentReportSent(ok, message, localPath) {
            window.statusMessage = (ok ? "✅ " : "❌ ") + message
            if (localPath && localPath !== "") {
                window.lastIncidentReportPath = localPath
            }
        }
        function onActiveDiagnosticsFinished(completed, result) {
            window.activeDiagnosticInProgress = false
            if (window.activeFailureContext === null)
                return
            if (!completed) {
                window.statusMessage = tr("No se pudo iniciar el diagnóstico activo.")
                return
            }
            window.activeDiagnosticResult = result || ({})
            if (supportAssistantDialog.visible) {
                Qt.callLater(function() {
                    activeDiagnosticResultHeading.forceActiveFocus()
                })
            }
            const path = window.persistIncidentReport(
                           window.activeFailureContext.kind || "operation-failure",
                           window.activeFailureContext.message || "",
                           {
                               source: "active-diagnostics",
                               category: result && result.probableCause
                                         ? result.probableCause : "unknown",
                               outcome: result && result.confidence
                                        ? result.confidence : "limited"
                           })
            window.statusMessage = path && path !== ""
                    ? tr("Diagnóstico activo completado e integrado en una incidencia nueva.")
                    : tr("Diagnóstico activo completado; no se pudo guardar la incidencia.")
        }
        function onExpertModeChanged() {
            // Updated via explicit Connections block above to ensure settingsLoaded respects lifecycle
        }
        function onSigningFinished(success, message, outPath) {
            window.signingInProgress = false
            window.cscPendingSecrets = null
            window.statusMessage = (success ? "✅ " : "❌ ") + message
            window.signResultGeneration++
            const resultGeneration = window.signResultGeneration
            window.signResultKind = success ? "success" : "error"
            window.signResultPath = success ? String(outPath || "") : ""
            window.signResultCause = success ? "" : window.localizeVisibleDiagnosticText(message)
            window.signResultVerificationPending = success && !!outPath
            if (success)
                window.clearOperationFailure()
            if (success && outPath && outPath !== "") {
                window.rememberSessionDocumentPath(outPath)
                window.currentOutputPath = outPath
                window.currentOutputVerificationMessage = ""
                window.currentOutputVerificationDetails = null
                window.enqueueAutoVerification({ kind: "single", path: outPath })
            } else if (!success) {
                window.clearOperationFailure()
                const incidentPath = window.persistIncidentReport("sign-failure", message, {
                    source: "signingFinished"
                })
                window.recordOperationFailure("sign-failure", message, incidentPath)
            }
            signMainOuterScroll.contentItem.contentY = 0
            Qt.callLater(function() {
                if (resultGeneration !== window.signResultGeneration) return
                signResultHeading.Accessible.announce(window.signResultAnnouncement())
                if (window.signResultKind === "success" && window.signResultPath !== "")
                    openSignedResultButton.forceActiveFocus()
                else if (window.signResultKind === "error")
                    retrySignedResultButton.forceActiveFocus()
            })
        }
        function onBatchSigningFinished(success, message, results) {
            window.signingInProgress = false
            window.cscPendingSecrets = null
            window.currentBatchResults = results || []
            const firstBatchError = window.firstBatchFailureMessage(window.currentBatchResults)
            window.statusMessage = (!success && firstBatchError !== "")
                ? (tr("❌ Lote fallido: %1").arg(localizeVisibleDiagnosticText(firstBatchError)))
                : ((success ? "✅ " : "⚠️ ") + message)
            let queued = 0
            for (let i = 0; i < window.currentBatchResults.length; i++) {
                const item = window.currentBatchResults[i]
                if (item && item.ok && item.outputPath) {
                    window.enqueueAutoVerification({ kind: "batch", index: i, path: item.outputPath })
                    queued++
                }
            }
            if (!success) {
                window.clearOperationFailure()
                const failureMessage = firstBatchError !== "" ? firstBatchError : message
                const incidentPath = window.persistIncidentReport("batch-sign-failure", failureMessage, {
                    source: "batchSigningFinished"
                })
                window.recordOperationFailure("batch-sign-failure", failureMessage, incidentPath)
            } else {
                window.clearOperationFailure()
            }
            if (success && window.autoClose && queued === 0) {
                if (window.closeBehavior === "resident") {
                    window.keepResidentAfterClose()
                } else {
                    window.flushSettingsNow()
                    Qt.quit()
                }
            }
        }
        function onVerificationFinished(success, message, details) {
            window.verificationPendingCount = Math.max(0, window.verificationPendingCount - 1)
            if (window.autoVerificationContext !== null) {
                window.handleAutoVerificationFinished(success, message, details)
                return
            }
            if (window.pendingSignedDocumentWarningContext !== null) {
                const warnCtx = window.pendingSignedDocumentWarningContext
                if (success) {
                    window.pendingSignedDocumentWarningContext = {
                        selectedCertIndex: warnCtx.selectedCertIndex,
                        path: warnCtx.path
                    }
                    window.signedDocumentWarningSummary = window.buildSignedDocumentWarningSummary(details)
                    signedDocumentWarningDialog.open()
                } else {
                    window.pendingSignedDocumentWarningContext = null
                    window.skipSignedDocumentWarningOnce = true
                    Qt.callLater(function() {
                        window.executeSignRequest(warnCtx.selectedCertIndex)
                    })
                }
                return
            }
            if (window.pendingSealSignerSummaryContext !== null) {
                const ctx = window.pendingSealSignerSummaryContext
                const payload = ctx.payload || {}
                const previousSigners = success ? window.verificationSignerNames(details) : []
                const signerSummary = window.currentCoSignSealSummary(previousSigners)
                if (signerSummary.trim() !== "") {
                    const extraOptions = payload.extraOptions ? Object.assign({}, payload.extraOptions) : {}
                    extraOptions.visibleSealSignerSummary = signerSummary.trim()
                    payload.extraOptions = extraOptions
                }
                window.pendingSealSignerSummaryContext = {
                    selectedCertIndex: ctx.selectedCertIndex,
                    payload: payload,
                    path: ctx.path
                }
                Qt.callLater(function() {
                    window.executeSignRequest(ctx.selectedCertIndex)
                })
                return
            }
            verifyTab.verifyDetails = window.verificationPayload(success, message, details)
            const verificationOutcome = window.verificationOutcomeKind(verifyTab.verifyDetails)
            window.statusMessage = verificationOutcome === "untrusted"
                ? tr("Integridad válida; confianza no evaluada.")
                : (verificationOutcome === "incomplete"
                   ? tr("Verificación incompleta")
                   : message)
            if (!success) {
                window.clearOperationFailure()
                const incidentPath = window.persistIncidentReport("verify-failure", message, {
                    source: "verificationFinished"
                })
                window.recordOperationFailure("verify-failure", message, incidentPath)
            }
            if (success) {
                window.clearOperationFailure()
                activeTab = "verificar"
            }
        }
        function onCertificateOnlineCheckFinished(success, message, details) {
            certificateValidationDialog.onlineCheckInProgress = false
            if (!certificateValidationDialog.visible)
                return
            const currentId = certificateValidationDialog.requestedCertificateId
            if (currentId === "")
                return
            if (window.selectedCertData && window.certificateId(window.selectedCertData) !== currentId)
                return
            if (success) {
                certificateValidationDialog.onlineCheckResult = details || ({})
            } else {
                certificateValidationDialog.onlineCheckResult = {
                    status: "unavailable",
                    userMessage: message,
                    reason: message
                }
            }
            const merged = Object.assign({}, certificateValidationDialog.validationDetails || {})
            merged.onlineRevocation = certificateValidationDialog.onlineCheckResult
            certificateValidationDialog.validationDetails = merged
            window.updateCertificateValidationReport()
            window.statusMessage = message
        }
        function onCertificatePublicExportFinished(ok, message) {
            window.statusMessage = message
            publicCertificateResultDialog.title = ok ? tr("Certificado público exportado") : tr("No se pudo exportar el certificado")
            publicCertificateResultText.text = message
            publicCertificateResultDialog.open()
        }
        function onProtectionRecipientsLoaded(recipients) {
            window.protectionRecipients = recipients || []
            window.pruneProtectionSelection()
            window.statusMessage = tr("Destinatarios de protección cargados")
        }
        function onProtectionFinished(success, message, result) {
            window.protectionInProgress = false
            window.cscPendingSecrets = null
            window.clearTransientProtectionSecrets()
            window.statusMessage = message
            window.protectResult = success ? result : { error: message }
            if (success && result && result.outputPath) {
                window.rememberSessionDocumentPath(result.outputPath)
            } else if (!success) {
                window.clearOperationFailure()
                const incidentPath = window.persistIncidentReport("protect-failure", message, {
                    source: "protectionFinished"
                })
                window.recordOperationFailure("protect-failure", message, incidentPath)
            }
            if (success) {
                window.clearOperationFailure()
                activeTab = "cifrar"
            }
        }
        function onUnprotectionFinished(success, message, result) {
            window.unprotectionInProgress = false
            window.clearTransientProtectionSecrets()
            window.statusMessage = message
            window.unprotectResult = success ? result : { error: message }
            if (success && result && result.outputPath) {
                window.rememberSessionDocumentPath(result.outputPath)
            } else if (!success) {
                window.clearOperationFailure()
                const incidentPath = window.persistIncidentReport("unprotect-failure", message, {
                    source: "unprotectionFinished"
                })
                window.recordOperationFailure("unprotect-failure", message, incidentPath)
            }
            if (success) {
                window.clearOperationFailure()
                activeTab = "cifrar"
            }
        }
        function onSettingsLoaded(s) {
            console.log(tr("QML: Ajustes cargados desde el backend"))
            window.applyingLoadedSettings = true
            if (s.idioma !== undefined && s.idioma !== "") window.appLanguage = s.idioma
            if (s.expertMode !== undefined) backend.expertMode = s.expertMode
            if (s.themeIndex !== undefined) window.currentThemeIndex = s.themeIndex
            if (s.autoClose !== undefined) window.autoClose = s.autoClose
            if (s.closeBehavior !== undefined) window.closeBehavior = s.closeBehavior !== "" ? s.closeBehavior : "resident"
            if (s.webCompatibilityDurationMinutes !== undefined) {
                const duration = Number(s.webCompatibilityDurationMinutes)
                window.webCompatibilityDurationMinutes =
                    Number.isInteger(duration) && duration >= 5 && duration <= 240
                        ? duration
                        : 30
            }
            if (s.facturaeToolsEnabled !== undefined) {
                window.facturaeToolsEnabled = s.facturaeToolsEnabled
            }
            if (s.checkForUpdates !== undefined) {
                window.checkForUpdates = s.checkForUpdates
                appSettings.checkForUpdatesCached = s.checkForUpdates
            }
            if (s.stickySigner !== undefined) window.stickySigner = s.stickySigner
            if (s.autoSelectSingleCertificate !== undefined) window.autoSelectSingleCertificate = s.autoSelectSingleCertificate
            if (s.preferDefaultCertificate !== undefined) window.preferDefaultCertificate = s.preferDefaultCertificate
            if (s.showDefaultCertificateFirst !== undefined) window.showDefaultCertificateFirst = s.showDefaultCertificateFirst
            if (s.showUsableCertificatesFirst !== undefined) window.showUsableCertificatesFirst = s.showUsableCertificatesFirst
            if (s.showValidCertificatesFirst !== undefined) window.showValidCertificatesFirst = s.showValidCertificatesFirst
            if (s.certsExpiredShow !== undefined) window.certsExpiredShow = s.certsExpiredShow
            if (s.certsInvalidShow !== undefined) window.certsInvalidShow = s.certsInvalidShow
            if (s.useOnlySignatureCertificates !== undefined) window.useOnlySignatureCertificates = s.useOnlySignatureCertificates
            if (s.certificateTypeFilter !== undefined) window.certificateTypeFilter = normalizeCertificateTypeFilter(s.certificateTypeFilter)
            if (s.certificateRequireNIF !== undefined) window.certificateRequireNIF = s.certificateRequireNIF
            if (s.certificateRequireOrganization !== undefined) window.certificateRequireOrganization = s.certificateRequireOrganization
            if (s.tsaEnabled !== undefined) window.tsaEnabled = s.tsaEnabled
            if (s.tsaUrl !== undefined) window.tsaUrl = s.tsaUrl
            if (s.proxyEnabled !== undefined) window.proxyEnabled = s.proxyEnabled
            if (s.proxyType !== undefined) window.proxyType = s.proxyType !== "" ? s.proxyType : "none"
            if (s.proxyHost !== undefined) window.proxyHost = s.proxyHost
            if (s.proxyPort !== undefined) window.proxyPort = s.proxyPort
            if (s.proxyExcludedUrls !== undefined) {
                if (Array.isArray(s.proxyExcludedUrls)) {
                    window.proxyExcludedUrls = s.proxyExcludedUrls
                } else {
                    window.proxyExcludedUrls = parseProxyExcludedUrlsText(s.proxyExcludedUrls)
                }
            }
            window.proxyCredentialsConfigured =
                    s.proxySecretId !== undefined
                    && String(s.proxySecretId || "").trim() !== ""
            if (s.proxyRealm !== undefined) {
                window.proxyCredentialRealm = String(s.proxyRealm || "")
            }
            if (s.defaultHashAlgorithm !== undefined) window.defaultHashAlgorithm = s.defaultHashAlgorithm !== "" ? s.defaultHashAlgorithm : "SHA-256"
            if (s.defaultHashCopyToClipboard !== undefined) window.defaultHashCopyToClipboard = s.defaultHashCopyToClipboard
            if (s.defaultHashFormatFile !== undefined) window.defaultHashFormatFile = s.defaultHashFormatFile !== "" ? s.defaultHashFormatFile : "hex"
            if (s.defaultHashFormatDirectory !== undefined) window.defaultHashFormatDirectory = s.defaultHashFormatDirectory !== "" ? s.defaultHashFormatDirectory : "xml"
            if (s.defaultHashRecursive !== undefined) window.defaultHashRecursive = s.defaultHashRecursive
            if (s.defaultHashSaveReport !== undefined) window.defaultHashSaveReport = s.defaultHashSaveReport
            if (s.confirmToSign !== undefined) window.confirmToSign = s.confirmToSign
            if (s.omitAskOnClose !== undefined) window.omitAskOnClose = s.omitAskOnClose
            if (s.preferredCertificateId !== undefined) window.preferredCertificateId = s.preferredCertificateId
            if (s.defaultCertificateId !== undefined) window.defaultCertificateId = s.defaultCertificateId
            if (s.rememberCertificateFilter !== undefined) window.rememberCertificateFilter = s.rememberCertificateFilter
            if (window.rememberCertificateFilter && s.certificateFilterText !== undefined) {
                window.certificateFilterText = s.certificateFilterText
            } else {
                window.certificateFilterText = ""
            }
            verifyTab.hashAlgorithm = window.defaultHashAlgorithm
            verifyTab.hashFormatFile = window.defaultHashFormatFile
            verifyTab.hashFormatDirectory = window.defaultHashFormatDirectory
            verifyTab.hashRecursive = window.defaultHashRecursive
            verifyTab.hashSaveReport = window.defaultHashSaveReport
            if (s.signAction !== undefined) window.signAction = normalizedEnumSetting(s.signAction, "sign", ["sign", "cosign", "countersign"])
            if (s.signFormat !== undefined) window.signFormat = normalizedEnumSetting(s.signFormat, "", ["", "pades", "cades", "xades", "xmldsig", "odf", "ooxml", "facturae", "asic-xades"])
            if (s.signProfile !== undefined) window.signProfile = normalizedEnumSetting(s.signProfile, "baseline", ["baseline", "t", "lt", "lta"])
            if (s.signStrictCompat !== undefined) window.signStrictCompat = s.signStrictCompat
            if (s.signOverwrite !== undefined) window.signOverwrite = normalizedEnumSetting(s.signOverwrite, "rename", ["rename", "fail", "force"])
            if (s.autoFormatPdf !== undefined) window.autoFormatPdf = normalizedEnumSetting(s.autoFormatPdf, "pades", ["pades", "cades"])
            if (s.autoFormatOoxml !== undefined) window.autoFormatOoxml = normalizedEnumSetting(s.autoFormatOoxml, "ooxml", ["ooxml", "cades"])
            if (s.autoFormatFacturae !== undefined) window.autoFormatFacturae = normalizedEnumSetting(s.autoFormatFacturae, "facturae", ["facturae", "xades"])
            if (s.autoFormatOdf !== undefined) window.autoFormatOdf = normalizedEnumSetting(s.autoFormatOdf, "odf", ["odf", "cades"])
            if (s.autoFormatXml !== undefined) window.autoFormatXml = normalizedEnumSetting(s.autoFormatXml, "xades", ["xades", "xmldsig"])
            if (s.autoFormatBinary !== undefined) window.autoFormatBinary = normalizedEnumSetting(s.autoFormatBinary, "cades", ["cades", "asic-xades"])
            if (s.multiCosignEnabled !== undefined) window.multiCosignEnabled = s.multiCosignEnabled
            if (s.multiCosignPrimaryCertificateId !== undefined) window.multiCosignPrimaryCertificateId = s.multiCosignPrimaryCertificateId
            if (s.multiCosignCertificateIds !== undefined) window.multiCosignCertificateIds = s.multiCosignCertificateIds
            if (s.signVisibleSeal !== undefined) window.signVisibleSeal = s.signVisibleSeal
            if (s.signSealPages !== undefined) window.signSealPages = s.signSealPages !== "" ? s.signSealPages : "1"
            if (s.signSealAllPages !== undefined) window.signSealAllPages = s.signSealAllPages
            if (s.signSealX !== undefined) window.signSealX = Number(s.signSealX)
            if (s.signSealY !== undefined) window.signSealY = Number(s.signSealY)
            if (s.signSealW !== undefined) window.signSealW = Number(s.signSealW)
            if (s.signSealH !== undefined) window.signSealH = Number(s.signSealH)
            if (s.signSealRotation !== undefined) window.signSealRotation = Number(s.signSealRotation)
            if (s.signSealPlacements && typeof s.signSealPlacements === "object" && !Array.isArray(s.signSealPlacements)) window.signSealPlacements = s.signSealPlacements
            if (s.signSealPerPage !== undefined) window.signSealPerPage = !!s.signSealPerPage
            if (s.signSealKeepText !== undefined) window.signSealKeepText = s.signSealKeepText
            if (s.signSealImagePath !== undefined) window.signSealImagePath = normalizeSealImagePath(s.signSealImagePath)
            if (s.signSealLogoOpacityPercent !== undefined) window.signSealLogoOpacityPercent = Number(s.signSealLogoOpacityPercent)
            window.signSealLanguage = normalizedSealLanguage(s.signSealLanguage)
            if (s.signSealStyle !== undefined) window.sealStyle = normalizedEnumSetting(s.signSealStyle, "institutional", ["institutional", "text", "image"])
            else if (window.signSealImagePath !== "") window.sealStyle = "image"
            if (s.signQRContent !== undefined) window.signQRContent = s.signQRContent
            window.signQREnabled = s.signQREnabled === undefined
                ? window.signQRContent.trim() !== "" : !!s.signQREnabled
            if (s.signReason !== undefined) window.signReason = s.signReason
            if (s.signLocation !== undefined) window.signLocation = s.signLocation
            if (s.signContactInfo !== undefined) window.signContactInfo = s.signContactInfo
            if (s.padesSubFilter !== undefined) window.padesSubFilter = normalizedEnumSetting(s.padesSubFilter, "etsi", ["etsi", "adobe"])
            if (s.facturaePolicyVersion !== undefined) window.facturaePolicyVersion = s.facturaePolicyVersion !== "" ? s.facturaePolicyVersion : "3.1"
            if (s.policyIdentifier !== undefined) window.facturaePolicyIdentifier = s.policyIdentifier
            if (s.policyIdentifierHash !== undefined) window.facturaePolicyIdentifierHash = s.policyIdentifierHash
            if (s.policyQualifier !== undefined) window.facturaePolicyQualifier = s.policyQualifier
            if (s.signerClaimedRole !== undefined) window.facturaeSignerRole = s.signerClaimedRole !== "" ? s.signerClaimedRole : "emisor"
            if (s.signatureProductionCity !== undefined) window.facturaeSignatureCity = s.signatureProductionCity
            if (s.signatureProductionProvince !== undefined) window.facturaeSignatureProvince = s.signatureProductionProvince
            if (s.signatureProductionPostalCode !== undefined) window.facturaeSignaturePostalCode = s.signatureProductionPostalCode
            if (s.signatureProductionCountry !== undefined) window.facturaeSignatureCountry = s.signatureProductionCountry
            applyPageSelectionValidity()
            window.syncSealVisibilityControls()
            window.syncCertificateSelection(window.certificates)
            window.sanitizeMultiCosignCertificates()
            window.applyingLoadedSettings = false
            window.settingsSaveInFlight = false
            settingsRoundtripTimeoutTimer.stop()
            window.backendSettingsDirty = false
            if (window.signVisibleSeal && window.supportsVisibleSeal()) {
                window.requestPdfPreview()
            }
            if (window.pendingDiscardAndClose) {
                window.pendingDiscardAndClose = false
                window.statusMessage = tr("Cambios descartados.")
                window.continueWindowClose(true)
            }
            if ((!window.certificates || window.certificates.length === 0) && backend && backend.refreshCertificates) {
                console.log("QML: relanzando carga inicial de certificados tras onSettingsLoaded")
                Qt.callLater(function() {
                    if (!window.certificates || window.certificates.length === 0) {
                        backend.refreshCertificates()
                    }
                })
            }
        }
        function onUpdateCheckFinished(ok, message, result) {
            if (!window.updateCheckInProgress || window.updateUsingDirect) return
            updateEngineBudgetTimer.stop()
            if (!ok) {
                if (window.updateManualRequest) window.updateStatusMessage = message
                window.startDirectUpdateCheck()
                return
            }
            window.updateCheckInProgress = false
            if (result.estado === "sin_publicaciones") {
                if (window.updateManualRequest)
                    window.updateStatusMessage = result.mensaje || tr("Todavía no hay versiones publicadas en el canal oficial.")
                return
            }
            if (result.comparable === false) {
                if (window.updateManualRequest)
                    window.updateStatusMessage = tr("La última versión publicada es %1, pero este build de desarrollo no se puede comparar automáticamente.")
                        .arg(String(result.ultima_version || ""))
                return
            }
            window.acceptUpdateResult(result)
        }
        function onProxySecretStoreStatusReceived(available, platform, backendName, reason, runtimeMode) {
            window.proxySecretStoreAvailable = available
            window.proxySecretStorePlatform = platform || ""
            window.proxySecretStoreBackend = backendName || "none"
            window.proxySecretStoreReason = reason || ""
            window.proxyRuntimeMode = runtimeMode || ""
        }
        function onProxyCredentialsFinished(ok, message, configured, realm, username) {
            window.proxyCredentialBusy = false
            if (ok) {
                window.proxyCredentialsConfigured = configured
                window.proxyCredentialRealm = realm || ""
                window.proxyCredentialUsername = username || ""
                proxyPasswordField.clear()
                if (backend && backend.getProxySecretStoreStatus) {
                    backend.getProxySecretStoreStatus()
                }
            }
            window.statusMessage = message
        }
        function onSettingsSaved(ok, message) {
            console.log("QML: Resultado save_settings", ok, message)
            window.settingsSaveInFlight = false
            settingsRoundtripTimeoutTimer.stop()
            if (ok) {
                window.backendSettingsDirty = false
            }
            window.statusMessage = message
            if (window.pendingCloseAfterSettingsSave) {
                window.pendingCloseAfterSettingsSave = false
                if (ok) {
                    window.continueWindowClose(true)
                } else {
                    unsavedSettingsDialog.open()
                }
            }
        }
        function onCertificateImportFinished(ok, message) {
            backend.updateStatus(ok ? ("✅ " + message) : ("❌ " + message))
            if (ok) {
                backend.backendLogReceived(tr("Firma: Certificado importado con éxito."))
            } else {
                backend.backendLogReceived(tr("Error importando certificado: ") + message)
            }
        }
        function onCertificateAccessOptionsLoaded(ok, options, message) {
            if (ok) {
                window.certificateAccessOptions = options || ({ managers: [], importTargets: [] })
                Qt.callLater(function() {
                    certificateManagerCombo.currentIndex = window.optionIndexById(
                        window.certificateAccessOptions.managers,
                        window.certificateAccessOptions.preferredManager)
                    certificateImportTargetCombo.currentIndex = window.optionIndexById(
                        window.certificateAccessOptions.importTargets,
                        window.certificateAccessOptions.preferredTarget)
                })
            } else {
                window.certificateAccessOptions = ({ managers: [], importTargets: [] })
                window.statusMessage = "❌ " + message
            }
        }
        function onTemporaryCertificateFinished(ok, message, certificate) {
            window.statusMessage = (ok ? "✅ " : "❌ ") + message
            if (!ok) {
                window.pendingTemporaryRemovalId = ""
                backend.backendLogReceived(tr("Error cargando certificado temporal: ") + message)
                return
            }
            const id = certificate && certificate.id ? String(certificate.id) : ""
            if (id !== "") {
                window.rememberTemporaryCertificate(id)
                window.pendingTemporaryCertificateId = id
                backend.backendLogReceived(tr("Certificado temporal cargado solo en memoria para esta sesión."))
            } else if (window.pendingTemporaryRemovalId !== "") {
                window.forgetTemporaryCertificate(window.pendingTemporaryRemovalId)
                window.pendingTemporaryRemovalId = ""
            }
        }
        function onTemporaryCertificatesCleared(ok, message) {
            window.residentCredentialPurgePending = false
            if (ok) {
                window.temporaryCertificateIds = []
                window.pendingTemporaryCertificateId = ""
                window.pendingTemporaryRemovalId = ""
                return
            }
            // Fallo cerrado: la ventana vuelve a ser visible y no queda
            // residente mientras una identidad temporal pueda seguir viva.
            if (typeof residentAgent !== "undefined" && residentAgent) {
                residentAgent.showMainWindow()
            } else {
                window.visibility = Window.Windowed
            }
            window.closeBehavior = "exit"
            window.statusMessage = tr("Error")
        }
        function onPublicRootsInstallationFinished(ok, message) {
            backend.updateStatus(ok ? ("✅ " + message) : ("❌ " + message))
            if (ok) {
                backend.backendLogReceived(tr("Confianza: Raíces de AAPP instaladas correctamente."))
            } else {
                backend.backendLogReceived(tr("Error instalando raíces de confianza: ") + message)
            }
        }
    }

    onClosing: function(close) {
        if (portalSealMode) {
            if (!portalSealDecision) portalSealSubmit("cancel")
            close.accepted = true
            return
        }
        window.clearTransientProtectionSecrets()
        console.log("QML: onClosing", window.backendSettingsDirty ? "con cambios" : "sin cambios")
        window.flushSettingsNow()
        if (window.bypassUnsavedClosePrompt) {
            window.bypassUnsavedClosePrompt = false
            close.accepted = true
            return
        }
        if (window.backendSettingsDirty) {
            if (window.omitAskOnClose) {
                window.saveBackendSettingsAndClose()
                close.accepted = false
                return
            }
            close.accepted = false
            unsavedSettingsDialog.open()
            return
        }
        if (window.closeBehavior === "resident") {
            close.accepted = false
            window.keepResidentAfterClose()
        }
    }

    Timer {
        id: portalSealStartupTimer
        interval: 15000
        repeat: false
        onTriggered: {
            if (portalSealMode && previewGeometryPath !== portalSeal.documentPath) {
                portalSealDecision = true
                Qt.quit()
            }
        }
    }

    // Si la vista no llega a tiempo se sustituye «Cargando…» por una salida.
    property bool portalSealPreviewTimedOut: false
    Timer {
        id: portalSealPreviewTimeout
        interval: 10000
        onTriggered: window.portalSealPreviewTimedOut = true
    }

    Timer {
        id: portalSealPreviewRetryTimer
        interval: 1000
        repeat: true
        onTriggered: {
            if (previewGeometryPath === portalSeal.documentPath) stop()
            else requestPdfPreview()
        }
    }

    Rectangle {
        visible: portalSealMode
        anchors.fill: parent
        z: 1000
        color: currentTheme.backgroundColor
        ColumnLayout {
            anchors.fill: parent
            anchors.margins: 20
            spacing: 12
            Text { text: tr("portal.seal.title"); font.pixelSize: 22; font.bold: true; color: currentTheme.textColor }
            Text { text: tr("portal.seal.instructions"); wrapMode: Text.WordWrap; Layout.fillWidth: true; Layout.maximumWidth: 900; color: currentTheme.textColor }
            RowLayout {
                Layout.fillWidth: true
                ThemedButton { text: tr("portal.seal.previous_page"); enabled: previewCurrentPage > 1; onClicked: goToPreviewPage(previewCurrentPage - 1) }
                Text { text: tr("portal.seal.page").replace("%1", previewCurrentPage).replace("%2", previewTotalPages); color: currentTheme.textColor }
                ThemedButton { text: tr("portal.seal.next_page"); enabled: previewCurrentPage < previewTotalPages; onClicked: goToPreviewPage(previewCurrentPage + 1) }
                Item { Layout.fillWidth: true }
                ComboBox {
                    model: [tr("portal.seal.logo"), tr("portal.seal.text")]
                    currentIndex: sealStyle === "institutional" ? 0 : 1
                    onActivated: sealStyle = currentIndex === 0 ? "institutional" : "text"
                    Accessible.name: tr("portal.seal.style")
                }
                Text { text: tr("sign.seal.opacity"); color: currentTheme.textColor }
                Slider { from: 0; to: 100; stepSize: 1; value: signSealLogoOpacityPercent; focusPolicy: Qt.StrongFocus; onMoved: signSealLogoOpacityPercent = Math.round(value); Accessible.name: tr("sign.seal.opacity"); Accessible.description: tr("sign.seal.opacity_help") }
            }
            GridLayout {
                Layout.fillWidth: true
                columns: window.width < 980 ? 2 : 5
                ColumnLayout {
                    Text { text: tr("portal.seal.x"); color: currentTheme.textColor }
                    SpinBox { from: 0; to: 99; value: Math.round(signSealX * 100); onValueModified: signSealX = Math.min(value / 100, 1 - signSealW); Accessible.name: tr("portal.seal.x") }
                }
                ColumnLayout {
                    Text { text: tr("portal.seal.y"); color: currentTheme.textColor }
                    SpinBox { from: 0; to: 99; value: Math.round(signSealY * 100); onValueModified: signSealY = Math.min(value / 100, 1 - signSealH); Accessible.name: tr("portal.seal.y") }
                }
                ColumnLayout {
                    Text { text: tr("portal.seal.width"); color: currentTheme.textColor }
                    SpinBox { from: 1; to: 100; value: Math.round(signSealW * 100); onValueModified: signSealW = Math.min(value / 100, 1 - signSealX); Accessible.name: tr("portal.seal.width") }
                }
                ColumnLayout {
                    Text { text: tr("portal.seal.height"); color: currentTheme.textColor }
                    SpinBox { from: 1; to: 100; value: Math.round(signSealH * 100); onValueModified: signSealH = Math.min(value / 100, 1 - signSealY); Accessible.name: tr("portal.seal.height") }
                }
                ColumnLayout {
                    Text { text: tr("portal.seal.rotation"); color: currentTheme.textColor }
                    SpinBox { from: 0; to: 359; value: signSealRotation; onValueModified: applySealRotation(value); Accessible.name: tr("portal.seal.rotation") }
                }
            }
            Item { id: portalSealCanvas; Layout.fillWidth: true; Layout.fillHeight: true; clip: true }
            Text {
                text: window.portalSealPreviewTimedOut ? tr("portal.seal.preview_error") : tr("portal.seal.preview_loading")
                visible: previewGeometryPath !== portalSeal.documentPath
                color: currentTheme.textColor
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
                Accessible.role: Accessible.AlertMessage
                Accessible.name: text
            }
            RowLayout {
                Layout.fillWidth: true
                ThemedButton { text: tr("portal.seal.sign_here"); enabled: previewGeometryPath === portalSeal.documentPath && previewGeometryPage === previewCurrentPage; onClicked: portalSealSubmit("place") }
                ThemedButton { text: tr("portal.seal.sign_without"); onClicked: portalSealSubmit("without") }
                Item { Layout.fillWidth: true }
                ThemedButton { text: tr("portal.seal.cancel"); onClicked: portalSealSubmit("cancel") }
            }
        }
    }

    Rectangle {
        id: updateBanner
        visible: !portalSealMode && updateAvailable &&
                 !updateNoticeDismissed && updateNoticeMessage !== ""
        z: 200
        anchors.top: parent.top
        width: parent.width
        height: updateBannerContent.implicitHeight + 24
        color: currentTheme.backgroundColor
        border.color: currentTheme.textColor
        border.width: 1
        Accessible.role: Accessible.AlertMessage
        Accessible.name: updateNoticeMessage
        ColumnLayout {
            id: updateBannerContent
            anchors.left: parent.left
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: 12
            spacing: 8
            Text {
                text: updateNoticeMessage
                color: currentTheme.textColor
                font.bold: true
                wrapMode: Text.WordWrap
                Layout.fillWidth: true
            }
            Flow {
                Layout.fillWidth: true
                spacing: 8
                ThemedButton {
                    text: tr("Descargar e instalar")
                    highlighted: true
                    onClicked: window.openOfficialUpdateRelease()
                    Accessible.description: tr("Abre la página oficial de la versión; la aplicación no descarga ni ejecuta archivos.")
                }
                ThemedButton { text: tr("Ver novedades"); onClicked: window.openOfficialUpdateRelease() }
                ThemedButton {
                    text: tr("Ahora no")
                    onClicked: window.updateDismissedVersion = window.updateLatestVersion
                }
            }
        }
    }

    RowLayout {
        visible: !portalSealMode
        anchors.fill: parent
        anchors.topMargin: updateBanner.visible ? updateBanner.height : 0
        // El contenido termina encima de la barra de estado, sin quedar debajo de ella.
        anchors.bottomMargin: statusBar.height
        spacing: 0

        // SIDEBAR
        Rectangle {
            id: leftSidebar
            Layout.fillHeight: true
            Layout.preferredWidth: window.sidebarPreferredWidth
            Layout.minimumWidth: window.sidebarPreferredWidth
            Layout.maximumWidth: window.sidebarPreferredWidth
            width: window.sidebarPreferredWidth
            color: currentTheme.sidebarColor

            Behavior on width {
                NumberAnimation { duration: 180; easing.type: Easing.InOutQuad }
            }

            ColumnLayout {
                anchors.fill: parent
                anchors.margins: window.sidebarCollapsed ? 10 : 20
                spacing: window.sidebarCollapsed ? 16 : 24

                RowLayout {
                    Layout.fillWidth: true

                    Text {
                        visible: !window.sidebarCollapsed
                        text: tr("NAVEGACIÓN")
                        color: currentTheme.secondaryTextColor
                        font.pixelSize: 10
                        font.bold: true
                    }

                    Item { Layout.fillWidth: true }

                    ToolButton {
                        text: window.sidebarCollapsed ? "»" : "«"
                        onClicked: window.sidebarCollapsed = !window.sidebarCollapsed
                        Accessible.name: window.sidebarCollapsed ? tr("Expandir menú") : tr("Colapsar menú")
                        ToolTip.visible: hovered
                        ToolTip.delay: 400
                        ToolTip.text: window.sidebarCollapsed ? tr("Expandir menú") : tr("Colapsar menú")
                    }
                }

                // Logo Container - Maximized
                Item {
                    Layout.fillWidth: true
                    Layout.preferredHeight: window.sidebarCollapsed ? 64 : 220
                    Image {
                        source: "../assets/Logo-Horizontal-Color.png"
                        visible: !window.sidebarCollapsed
                        anchors.fill: parent
                        fillMode: Image.PreserveAspectFit
                        anchors.margins: 8
                    }
                    Rectangle {
                        visible: window.sidebarCollapsed
                        anchors.centerIn: parent
                        width: 48
                        height: 48
                        radius: 12
                        color: Qt.rgba(1, 1, 1, 0.92)
                        border.color: Qt.rgba(0, 0, 0, currentTheme.borderOpacity + 0.12)
                        border.width: 1
                        Image {
                            anchors.centerIn: parent
                            width: parent.width - 10
                            height: parent.height - 10
                            source: "../assets/logo-dipgra.png"
                            fillMode: Image.PreserveAspectFit
                            smooth: true
                        }
                    }
                }

                // Navegación
                ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 12
                    
                    NavButton { 
                        text: tr("FIRMAR")
                        iconTxt: "✍"
                        active: activeTab === "firmar"
                        onClicked: activeTab = "firmar"
                    }
                    NavButton { 
                        text: tr("VERIFICAR")
                        iconTxt: "✓"
                        active: activeTab === "verificar"
                        onClicked: activeTab = "verificar"
                    }
                    NavButton {
                        text: tr("CIFRAR")
                        iconTxt: "🔐"
                        active: activeTab === "cifrar"
                        onClicked: {
                            activeTab = "cifrar"
                            if (!window.protectionRecipients || window.protectionRecipients.length === 0) {
                                backend.loadProtectionRecipients()
                            }
                        }
                    }
                    NavButton {
                        text: tr("facturae.nav")
                        iconTxt: "€"
                        active: activeTab === "facturae"
                        visible: window.facturaeToolsEnabled && isIpcMode
                        onClicked: activeTab = "facturae"
                    }
                    NavButton { text: tr("paridad.lote3.eni.nav").toUpperCase(); iconTxt: "▣"; active: activeTab === "eni"; visible: isIpcMode; onClicked: activeTab = "eni" }
                    NavButton { 
                        text: tr("CONFIGURACIÓN")
                        iconTxt: "⚙"
                        active: activeTab === "config"
                        onClicked: activeTab = "config"
                    }
                    NavButton {
                        text: tr("ACERCA DE")
                        iconTxt: "ⓘ"
                        active: aboutDialog.visible
                        onClicked: aboutDialog.open()
                    }
                    NavButton { 
                        text: tr("EXPERTO")
                        iconTxt: "☣"
                        active: activeTab === "experto"
                        visible: backend.expertMode
                        onClicked: activeTab = "experto"
                    }
                }

                Item { Layout.fillHeight: true }
            }
        }

        // --- CONTENIDO ---
        StackLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            currentIndex: activeTab === "firmar" ? 0 : 
                          (activeTab === "verificar" ? 1 : 
                          (activeTab === "cifrar" ? 2 :
                          (activeTab === "config" ? 3 :
                          (activeTab === "experto" ? 4 :
                          (activeTab === "seguridad" ? 5 :
                          (activeTab === "facturae" && window.facturaeToolsEnabled && isIpcMode ? 6 : (activeTab === "eni" && isIpcMode ? 7 : 0)))))))

            // TAB: FIRMAR (0)
            Item {
                RowLayout {
                    anchors.fill: parent
                    anchors.margins: window.narrowWindow ? 16 : 40
                    spacing: window.narrowWindow ? 16 : 40

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        radius: 16
                        color: currentTheme.cardColor
                        border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                        border.width: 1

                        ScrollView {
                        id: signMainOuterScroll
                        anchors.fill: parent
                        leftPadding: 16
                        rightPadding: 26
                        topPadding: 16
                        bottomPadding: 26
                        // Solo desplazamiento vertical: el contenido se ajusta al ancho.
                        contentWidth: availableWidth
                        clip: true
                        ScrollBar.vertical.policy: ScrollBar.AsNeeded
                        ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
                        ScrollBar.vertical.width: 18

                        ColumnLayout {
                            id: signMainOuterContent
                            width: signMainOuterScroll.availableWidth
                            spacing: 20

                            // Título y acciones: en ventanas estrechas las acciones bajan de línea.
                            GridLayout {
                                Layout.fillWidth: true
                                columns: signMainOuterScroll.availableWidth < 560 ? 1 : 2
                                columnSpacing: 12
                                rowSpacing: 8

                                Text {
                                    text: tr("Firma Digital")
                                    font.pixelSize: 32
                                    font.bold: true
                                    color: currentTheme.textColor
                                    Layout.fillWidth: true
                                    Layout.preferredWidth: 1
                                    wrapMode: Text.WordWrap
                                }

                                AdaptiveRow {
                                    Layout.preferredWidth: Math.min(parent.width, naturalWidth)
                                    Layout.fillWidth: false
                                    Layout.alignment: Qt.AlignRight | Qt.AlignVCenter
                                    spacing: 12

                                    Text {
                                        height: 36
                                        verticalAlignment: Text.AlignVCenter
                                        visible: window.backendSettingsDirty
                                        text: tr("Cambios sin guardar")
                                        color: currentTheme.secondaryTextColor
                                        font.pixelSize: 12
                                    }

                                    ThemedButton {
                                        text: tr("Guardar preferencias")
                                        enabled: window.backendSettingsDirty
                                        onClicked: saveBackendSettings()
                                    }
                                    ThemedButton {
                                        text: tr("Descartar cambios")
                                        visible: window.backendSettingsDirty
                                        onClicked: discardBackendSettingsChanges(false)
                                    }
                                }
                            }

                            Rectangle {
                                id: signResultPanel
                                visible: window.signResultKind !== ""
                                Layout.fillWidth: true
                                implicitHeight: signResultContent.implicitHeight + 36
                                radius: 12
                                color: window.signResultKind === "success" ? "#123d2a" : "#4a1d1d"
                                border.color: window.signResultKind === "success" ? "#45d58a" : "#ff7777"
                                border.width: 1
                                Accessible.role: Accessible.Grouping
                                Accessible.name: window.signResultAnnouncement()

                                ColumnLayout {
                                    id: signResultContent
                                    anchors.fill: parent
                                    anchors.margins: 18
                                    spacing: 12

                                    RowLayout {
                                        Layout.fillWidth: true
                                        spacing: 12
                                        Canvas {
                                            id: signResultIcon
                                            Layout.preferredWidth: 36
                                            Layout.preferredHeight: 36
                                            Accessible.role: Accessible.Graphic
                                            Accessible.name: window.signResultKind === "success" ? tr("Firma completada correctamente") : tr("Error")
                                            onPaint: {
                                                const ctx = getContext("2d")
                                                ctx.clearRect(0, 0, width, height)
                                                ctx.fillStyle = window.signResultKind === "success" ? "#36b878" : "#de5c5c"
                                                ctx.beginPath()
                                                ctx.arc(18, 18, 17, 0, 2 * Math.PI)
                                                ctx.fill()
                                                ctx.strokeStyle = "#ffffff"
                                                ctx.lineWidth = 3.2
                                                ctx.lineCap = "round"
                                                ctx.lineJoin = "round"
                                                ctx.beginPath()
                                                if (window.signResultKind === "success") {
                                                    ctx.moveTo(10, 18)
                                                    ctx.lineTo(16, 24)
                                                    ctx.lineTo(27, 12)
                                                } else {
                                                    ctx.moveTo(12, 12)
                                                    ctx.lineTo(24, 24)
                                                    ctx.moveTo(24, 12)
                                                    ctx.lineTo(12, 24)
                                                }
                                                ctx.stroke()
                                            }
                                            Connections {
                                                target: window
                                                function onSignResultKindChanged() { signResultIcon.requestPaint() }
                                            }
                                        }
                                        Text {
                                            Layout.preferredWidth: 1
                                            id: signResultHeading
                                            Layout.fillWidth: true
                                            text: window.signResultKind === "success"
                                                  ? tr("Documento firmado correctamente")
                                                  : tr("No se pudo firmar el documento")
                                            color: "#ffffff"
                                            font.pixelSize: 22
                                            font.bold: true
                                            wrapMode: Text.WordWrap
                                            Accessible.role: Accessible.StaticText
                                            Accessible.name: window.signResultAnnouncement()
                                        }
                                    }

                                    Text {
                                        visible: window.signResultKind === "success"
                                        Layout.fillWidth: true
                                        text: window.basename(window.signResultPath)
                                        color: "#ffffff"
                                        font.pixelSize: 15
                                        font.bold: true
                                        wrapMode: Text.WrapAnywhere
                                    }
                                    Text {
                                        visible: window.signResultKind === "success" && window.signResultVerificationPending
                                        Layout.fillWidth: true
                                        text: tr("Comprobando la firma...")
                                        color: "#e4fff0"
                                        wrapMode: Text.WordWrap
                                    }
                                    Text {
                                        visible: window.signResultKind === "success" && !window.signResultVerificationPending
                                                 && window.currentOutputVerificationDetails !== null
                                                 && window.signResultPath === window.currentOutputPath
                                        Layout.fillWidth: true
                                        text: window.signVerificationResultText()
                                        color: window.verificationOutcomeKind(window.currentOutputVerificationDetails) === "trusted"
                                               ? "#a7f0c5" : "#ffdb9b"
                                        font.bold: true
                                        wrapMode: Text.WordWrap
                                    }
                                    Text {
                                        visible: window.signResultKind === "error"
                                        Layout.fillWidth: true
                                        text: window.signResultCause
                                        color: "#ffffff"
                                        font.pixelSize: 15
                                        wrapMode: Text.WordWrap
                                    }
                                    Text {
                                        visible: window.signResultKind === "error"
                                        Layout.fillWidth: true
                                        text: tr("Comprueba que el documento siga disponible y que el certificado permita firmar; después vuelve a intentarlo.")
                                        color: "#ffe0e0"
                                        wrapMode: Text.WordWrap
                                    }
                                    AdaptiveRow {
                                        Layout.fillWidth: true
                                        spacing: 10
                                        ThemedButton {
                                            id: openSignedResultButton
                                            visible: window.signResultKind === "success" && window.signResultPath !== ""
                                            text: tr("Ver documento firmado")
                                            font.bold: true
                                            Layout.preferredHeight: 44
                                            onClicked: backend.openSignedDocument(window.signResultPath)
                                            Accessible.name: text
                                        }
                                        ThemedButton {
                                            visible: window.signResultKind === "success" && window.signResultPath !== ""
                                            text: tr("Abrir carpeta")
                                            Layout.preferredHeight: 44
                                            onClicked: backend.openExternal(window.signedResultFolder(window.signResultPath))
                                            Accessible.name: text
                                        }
                                        ThemedButton {
                                            id: retrySignedResultButton
                                            visible: window.signResultKind === "error"
                                            text: tr("Reintentar firma")
                                            font.bold: true
                                            Layout.preferredHeight: 44
                                            onClicked: window.requestSignConfirmation(window.selectedCertIndex)
                                            Accessible.name: text
                                        }
                                    }
                                }
                            }

                            Rectangle {
                                Layout.fillWidth: true
                                implicitHeight: centralCertificatePicker.implicitHeight + 24
                                radius: 10
                                color: currentTheme.sidebarColor
                                border.color: currentTheme.primaryColor
                                border.width: 1
                                ColumnLayout {
                                    id: centralCertificatePicker
                                    anchors.fill: parent
                                    anchors.margins: 12
                                    spacing: 6
                                    Text {
                                        text: tr("Certificado de firma")
                                        color: currentTheme.textColor
                                        font.bold: true
                                    }
                                    RowLayout {
                                        Layout.fillWidth: true
                                        spacing: 8
                                        ComboBox {
                                            id: signingCertificateCombo
                                            Layout.fillWidth: true
                                            Layout.preferredHeight: 48
                                            model: window.signingCertificates()
                                            currentIndex: window.findCertificateIndexById(window.certificateId(window.selectedCertData), window.signingCertificates())
                                            Accessible.name: tr("Certificado de firma")
                                            Accessible.description: window.selectedCertData
                                                ? window.certificateStatusText(window.selectedCertData) + ". " + window.certificateStatusReason(window.selectedCertData)
                                                : tr("Seleccione un certificado")
                                            displayText: window.selectedCertData
                                                ? window.certificateDisplayName(window.selectedCertData)
                                                : tr("Seleccione un certificado")
                                            contentItem: Text {
                                                leftPadding: 12
                                                rightPadding: signingCertificateCombo.indicator.width + 12
                                                text: signingCertificateCombo.displayText
                                                color: currentTheme.textColor
                                                verticalAlignment: Text.AlignVCenter
                                                elide: Text.ElideRight
                                            }
                                            background: Rectangle {
                                                radius: 6
                                                color: currentTheme.cardColor
                                                border.color: signingCertificateCombo.activeFocus
                                                    ? currentTheme.primaryColor : currentTheme.textColor
                                            }
                                            onActivated: function(index) {
                                                const cert = window.signingCertificates()[index]
                                                window.selectCertificateIndex(window.findCertificateIndexById(window.certificateId(cert), window.certificates), true)
                                            }
                                            delegate: ItemDelegate {
                                                id: certificateOption
                                                required property int index
                                                required property var modelData
                                                // El color sigue al puntero o al teclado; el elegido se marca con la barra lateral.
                                                highlighted: signingCertificateCombo.highlightedIndex === index
                                                readonly property bool isCurrentCertificate: signingCertificateCombo.currentIndex === index
                                                width: signingCertificateCombo.width
                                                implicitHeight: Math.max(64, contentItem.implicitHeight + 12)
                                                Accessible.name: window.certificateDisplayName(modelData) + ", "
                                                    + window.certificateStatusText(modelData) + ". " + window.certificateStatusReason(modelData)
                                                background: Rectangle {
                                                    color: certificateOption.highlighted
                                                        ? window.certificateSelectionColor() : currentTheme.cardColor
                                                    Rectangle {
                                                        visible: certificateOption.isCurrentCertificate
                                                        anchors.left: parent.left
                                                        anchors.top: parent.top
                                                        anchors.bottom: parent.bottom
                                                        width: 4
                                                        color: currentTheme.primaryColor
                                                    }
                                                    Rectangle {
                                                        anchors.left: parent.left
                                                        anchors.right: parent.right
                                                        anchors.bottom: parent.bottom
                                                        height: 1
                                                        color: window.certificateDividerColor()
                                                    }
                                                }
                                                contentItem: ColumnLayout {
                                                    spacing: 2
                                                    Text {
                                                        wrapMode: Text.WordWrap
                                                        Layout.fillWidth: true
                                                        text: window.certificateDisplayName(modelData)
                                                        color: certificateOption.highlighted
                                                            ? window.certificateSelectionTextColor() : currentTheme.textColor
                                                        elide: Text.ElideRight
                                                    }
                                                    Text {
                                                        text: window.certificateStatusText(modelData)
                                                        color: window.certificateStatusColorForBackground(modelData,
                                                            certificateOption.highlighted
                                                                ? window.certificateSelectionColor() : currentTheme.cardColor)
                                                        font.bold: true
                                                    }
                                                    Text {
                                                        Layout.fillWidth: true
                                                        visible: window.certificateStatusReason(modelData) !== ""
                                                        text: window.certificateStatusReason(modelData)
                                                        color: certificateOption.highlighted
                                                            ? window.certificateSelectionTextColor() : currentTheme.textColor
                                                        font.pixelSize: 12
                                                        wrapMode: Text.WordWrap
                                                    }
                                                }
                                            }
                                        }
                                        ThemedButton {
                                            id: changeCertificateButton
                                            text: "⤢"
                                            Layout.preferredHeight: 48
                                            Layout.preferredWidth: 48
                                            onClicked: window.signCertificatePanelCollapsed = false
                                            Accessible.name: tr("Ver todos los certificados")
                                            ToolTip.visible: hovered
                                            ToolTip.text: tr("Ver todos los certificados")
                                        }
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        visible: window.selectedCertData !== null
                                        text: window.certificateStatusText(window.selectedCertData)
                                        color: window.certificateSummaryStatusColor(window.selectedCertData)
                                        font.pixelSize: 12
                                        font.bold: true
                                        wrapMode: Text.WordWrap
                                        Accessible.name: text
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        visible: window.selectedCertData !== null
                                            && window.certificateStatusReason(window.selectedCertData) !== ""
                                        text: window.certificateStatusReason(window.selectedCertData)
                                        color: currentTheme.textColor
                                        font.pixelSize: 11
                                        wrapMode: Text.WordWrap
                                        Accessible.name: text
                                    }
                                }
                            }

                        Rectangle {
                            Layout.fillWidth: true
                            Layout.preferredHeight: 400
                            radius: 15
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor
                            border.width: dropArea.containsDrag ? 3 : 1
                            
                            DropArea {
                                id: dropArea
                                anchors.fill: parent
                                property bool containsDrag: false
                                onEntered: containsDrag = true
                                onExited: containsDrag = false
                                onDropped: (drop) => {
                                    containsDrag = false
                                    if (drop.hasUrls) {
                                        let paths = localPathsFromUrls(drop.urls)
                                        if (paths.length > 1) {
                                            window.useMultiSelection(paths)
                                        } else if (paths.length === 1) {
                                            window.useSingleSelection(paths[0])
                                        }
                                    }
                                }
                            }

                            ColumnLayout {
                                anchors.centerIn: parent
                                spacing: 15
                                Text {
                                    text: selectedInputsSummary()
                                    color: currentTheme.textColor
                                    font.pixelSize: 18
                                    Layout.alignment: Qt.AlignCenter
                                    horizontalAlignment: Text.AlignHCenter
                                    wrapMode: Text.WordWrap
                                    Layout.maximumWidth: parent.width - 40
                                }
                                AdaptiveRow {
                                    Layout.alignment: Qt.AlignCenter
                                    spacing: 10
                                    ThemedButton {
                                        text: tr("Seleccionar archivo")
                                        onClicked: fileDialog.open()
                                    }
                                    ThemedButton {
                                        text: tr("Seleccionar varios")
                                        onClicked: multiFileDialog.open()
                                    }
                                    ThemedButton {
                                        text: tr("Seleccionar carpeta")
                                        onClicked: batchDirectoryDialog.open()
                                    }
                                    ThemedButton {
                                        text: tr("Ver Original")
                                        visible: window.currentFilePath !== "" && !window.isBatchMode()
                                        onClicked: backend.openExternal(window.currentFilePath)
                                    }
                                }
                            }
                        }

                        // NUEVO: Gestión de Rutas Visibles
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: pathCol.implicitHeight + 20
                            radius: 10
                            color: currentTheme.cardColor
                            border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                            
                            ColumnLayout {
                                id: pathCol
                                anchors.fill: parent
                                anchors.margins: 15
                                spacing: 10

                                Text {
                                    visible: !window.isBatchMode()
                                    text: tr("RUTA DE ENTRADA")
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 10; font.bold: true 
                                }
                                RowLayout {
                                    visible: !window.isBatchMode()
                                    Layout.fillWidth: true
                                    ThemedTextField {
                                        text: window.currentFilePath
                                        Layout.fillWidth: true
                                        placeholderText: tr("Seleccione un archivo...")
                                        onTextChanged: window.currentFilePath = text
                                    }
                                }

                                Text {
                                    visible: !window.isBatchMode()
                                    text: tr("RUTA DE SALIDA (PDF FIRMADO)")
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 10; font.bold: true 
                                }
                                // En estrecho la ruta ocupa la fila entera y los botones bajan debajo.
                                GridLayout {
                                    visible: !window.isBatchMode()
                                    Layout.fillWidth: true
                                    columns: signMainOuterScroll.availableWidth < 560 ? 3 : 4
                                    columnSpacing: 5
                                    rowSpacing: 5
                                    ThemedTextField {
                                        text: window.currentOutputPath
                                        Layout.fillWidth: true
                                        Layout.columnSpan: signMainOuterScroll.availableWidth < 560 ? 3 : 1
                                        Layout.preferredHeight: 44
                                        placeholderText: tr("Destino automático...")
                                        onTextChanged: window.currentOutputPath = text
                                    }
                                    ThemedButton {
                                        text: tr("Ver")
                                        icon.source: "../assets/eye_icon.png"
                                        icon.width: 22
                                        icon.height: 22
                                        icon.color: "white"
                                        display: AbstractButton.TextBesideIcon
                                        Layout.preferredWidth: 92
                                        Layout.preferredHeight: 44
                                        enabled: window.currentOutputPath !== ""
                                        onClicked: backend.openExternal(window.currentOutputPath)
                                        ToolTip.visible: hovered
                                        ToolTip.text: tr("Ver archivo (Abrir externamente)")
                                        ToolTip.delay: 500
                                    }
                                    ThemedButton {
                                        text: tr("Validar")
                                        icon.source: "../assets/search_icon.png"
                                        icon.width: 20
                                        icon.height: 20
                                        icon.color: "white"
                                        display: AbstractButton.TextBesideIcon
                                        Layout.preferredWidth: 110
                                        Layout.preferredHeight: 44
                                        enabled: window.currentOutputPath !== ""
                                        onClicked: jumpToVerify(window.currentOutputPath)
                                        ToolTip.visible: hovered
                                        ToolTip.text: tr("Validar firma del documento")
                                        ToolTip.delay: 500
                                    }
                                    ThemedButton {
                                        text: tr("Cambiar")
                                        icon.source: "../assets/folder_icon.png"
                                        icon.width: 22
                                        icon.height: 22
                                        icon.color: "white"
                                        display: AbstractButton.TextBesideIcon
                                        Layout.preferredWidth: 112
                                        Layout.preferredHeight: 44
                                        onClicked: saveFileDialog.open()
                                        ToolTip.visible: hovered
                                        ToolTip.text: tr("Cambiar ubicación del archivo de salida")
                                        ToolTip.delay: 500
                                    }
                                }

                                Text {
                                    visible: window.isBatchMode()
                                    text: tr("SELECCIÓN DE LOTE")
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 10; font.bold: true
                                }
                                ThemedTextArea {
                                    visible: window.isBatchMode()
                                    Layout.fillWidth: true
                                    readOnly: true
                                    wrapMode: TextEdit.WrapAnywhere
                                    textFormat: TextEdit.PlainText
                                    implicitHeight: 90
                                    text: window.currentBatchDirectory !== ""
                                          ? window.currentBatchDirectory
                                          : window.currentBatchPaths.join("\n")
                                    placeholderText: tr("Seleccione varios ficheros o una carpeta...")
                                }

                                Text {
                                    visible: window.isBatchMode()
                                    text: tr("CARPETA DE SALIDA DEL LOTE")
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 10; font.bold: true
                                }
                                RowLayout {
                                    visible: window.isBatchMode()
                                    Layout.fillWidth: true
                                    ThemedTextField {
                                        text: window.currentBatchOutputDir
                                        Layout.fillWidth: true
                                        Layout.preferredHeight: 44
                                        placeholderText: tr("Opcional. Si se deja vacío, cada fichero se guarda junto al original.")
                                        onTextChanged: window.currentBatchOutputDir = text
                                    }
                                    ThemedButton {
                                        text: tr("Cambiar")
                                        Layout.preferredHeight: 44
                                        onClicked: batchOutputDirectoryDialog.open()
                                    }
                                }
                            }
                        }

                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: optionsCol.implicitHeight + 24
                            radius: 10
                            color: currentTheme.cardColor
                            border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                            // Solo desplaza en horizontal con su barra. Sin interacción propia (y sin
                            // ScrollView, que filtra la rueda) la rueda llega a la página y se
                            // alcanza «Firmar ahora».
                            Flickable {
                                id: scrollOpts
                                anchors.fill: parent
                                anchors.margins: 12
                                clip: true
                                contentWidth: width
                                contentHeight: optionsCol.implicitHeight
                                interactive: false
                                boundsBehavior: Flickable.StopAtBounds

                                ColumnLayout {
                                    id: optionsCol
                                    width: scrollOpts.width
                                    spacing: 10

                                GridLayout {
                                    columns: signMainOuterScroll.availableWidth < 560 ? 1 : Math.min(3, Math.max(1, Math.floor(signMainOuterScroll.availableWidth / 160)))
                                    columnSpacing: 10
                                    rowSpacing: 10
                                    Layout.fillWidth: true
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Operación"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ComboBox {
                                            id: signActionCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Firmar"), valor: "sign" },
                                                { texto: tr("Cofirmar"), valor: "cosign" },
                                                { texto: tr("Contrafirmar"), valor: "countersign" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: signActionIndex()
                                            onActivated: function(index) { signAction = model[index].valor }
                                        }
                                        Binding { target: signActionCombo; property: "currentIndex"; value: signActionIndex() }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Formato"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ComboBox {
                                            id: signFormatCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Auto"), valor: "" },
                                                { texto: tr("PAdES"), valor: "pades" },
                                                { texto: tr("CAdES"), valor: "cades" },
                                                { texto: tr("XAdES"), valor: "xades" },
                                                { texto: tr("XMLdSig"), valor: "xmldsig" },
                                                { texto: tr("ODF"), valor: "odf" },
                                                { texto: tr("OOXML"), valor: "ooxml" },
                                                { texto: tr("FacturaE"), valor: "facturae" },
                                                { texto: tr("ASiC-XAdES"), valor: "asic-xades" }
                                            ].concat(window.verifactuInput ? [{ texto: tr("verifactu.profile_label"), valor: "verifactu" }] : [])
                                            textRole: "texto"
                                            currentIndex: signFormatIndex()
                                            onActivated: function(index) { signFormat = model[index].valor }
                                        }
                                        Binding { target: signFormatCombo; property: "currentIndex"; value: signFormatIndex() }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Sobrescritura"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ComboBox {
                                            id: signOverwriteCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Renombrar"), valor: "rename" },
                                                { texto: tr("Error si existe"), valor: "fail" },
                                                { texto: tr("Forzar"), valor: "force" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: signOverwriteIndex()
                                            onActivated: function(index) { signOverwrite = model[index].valor }
                                        }
                                        Binding { target: signOverwriteCombo; property: "currentIndex"; value: signOverwriteIndex() }
                                    }
                                }

                                Rectangle {
                                    Layout.fillWidth: true
                                    visible: signAction !== "countersign" && window.supportsGuidedMultiCosignFormat()
                                    color: Qt.rgba(1, 1, 1, 0.04)
                                    radius: 8
                                    border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                                    implicitHeight: multiCosignLayout.implicitHeight + 20

                                    ColumnLayout {
                                        id: multiCosignLayout
                                        anchors.fill: parent
                                        anchors.margins: 10
                                        spacing: 8

                                        AdaptiveRow {
                                            Layout.fillWidth: true
                                            ThemedCheckBox {
                                                id: multiCosignCheckBox
                                                text: tr("Cofirma múltiple guiada")
                                                enabled: window.supportsGuidedMultiCosignFormat()
                                                checked: window.multiCosignEnabled
                                                onToggled: window.multiCosignEnabled = checked
                                                ToolTip.visible: hovered
                                                ToolTip.delay: 500
                                                ToolTip.text: tr("Firma o cofirma el mismo documento con varios certificados en secuencia, generando una única salida final.")
                                            }
                                            Binding { target: multiCosignCheckBox; property: "checked"; value: window.multiCosignEnabled }
                                            ThemedButton {
                                                text: tr("Seleccionar certificados…")
                                                enabled: window.multiCosignEnabled && selectedCertIndex !== -1
                                                onClicked: multiCosignDialog.open()
                                            }
                                        }

                                        Text {
                                            Layout.fillWidth: true
                                            visible: window.multiCosignEnabled
                                            text: tr("Firmante principal: %1").arg(window.effectiveMultiCosignPrimaryName())
                                            color: currentTheme.textColor
                                            wrapMode: Text.WordWrap
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            visible: window.multiCosignEnabled
                                            text: window.multiCosignSummaryText()
                                            color: currentTheme.secondaryTextColor
                                            wrapMode: Text.WordWrap
                                        }
                                    }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    ThemedCheckBox {
                                        id: signVisibleSealCheckBox
                                        text: tr("Firma visible (PAdES)")
                                        onToggled: {
                                            if (window.applyingLoadedSettings) return
                                            window.signVisibleSeal = checked
                                            markBackendSettingsDirty()
                                            if (checked) requestPdfPreview()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Inserta un sello gráfico en el PDF indicando que ha sido firmado digitalmente.")
                                    }
                                    Binding { target: signVisibleSealCheckBox; property: "checked"; value: window.signVisibleSeal }
                                    Text {
                                        visible: window.firstSignFieldError !== "" && window.signVisibleSeal
                                        text: "⚠ " + tr("validacion.problemas").arg(Object.keys(window.signFieldErrors).length)
                                        color: currentTheme.errorColor
                                        Accessible.role: Accessible.StaticText
                                    }
                                    ThemedCheckBox {
                                        id: signStrictCompatCheckBox
                                        text: tr("Compatibilidad estricta")
                                        checked: signStrictCompat
                                        onToggled: signStrictCompat = checked
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Aplica perfiles de firma más restrictivos para maximizar la compatibilidad con administraciones públicas.")
                                    }
                                    Binding { target: signStrictCompatCheckBox; property: "checked"; value: window.signStrictCompat }
                                }

                                Rectangle {
                                    Layout.fillWidth: true
                                    visible: signVisibleSeal
                                    enabled: signVisibleSeal
                                    color: Qt.rgba(1, 1, 1, 0.04)
                                    radius: 8
                                    border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                                    implicitHeight: sealPresetsLayout.implicitHeight + 20

                                    ColumnLayout {
                                        id: sealPresetsLayout
                                        anchors.fill: parent
                                        anchors.margins: 10
                                        spacing: 8

                                        Text {
                                            text: tr("Presets del sello")
                                            color: currentTheme.textColor
                                            font.pixelSize: 13
                                            font.bold: true
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            text: tr("Aplica un estilo rápido al sello visible sin tener que recolocar todos los campos a mano.")
                                            color: currentTheme.secondaryTextColor
                                            font.pixelSize: 11
                                            wrapMode: Text.WordWrap
                                        }
                                        Flow {
                                            Layout.fillWidth: true
                                            width: parent.width
                                            spacing: 8

                                            ThemedButton { text: tr("Compacto"); onClicked: applySealPreset("compact") }
                                            ThemedButton { text: tr("Institucional"); onClicked: applySealPreset("institutional") }
                                            ThemedButton { text: tr("Solo texto"); onClicked: { sealStyle = "text"; signSealImagePath = ""; signSealKeepText = true } }
                                            ThemedButton { text: tr("Imagen propia"); onClicked: { sealStyle = "image"; sealImageFileDialog.open() } }
                                            ThemedButton { text: tr("Solo logo"); onClicked: applySealPreset("logo") }
                                            ThemedButton { text: tr("Logo + datos + QR"); onClicked: applySealPreset("logoqr") }
                                            ThemedButton { text: tr("Restaurar sello"); onClicked: restoreDefaultSealSettings() }
                                        }
                                        RowLayout {
                                            Layout.fillWidth: true
                                            spacing: 10
                                            Text { text: tr("sign.seal.opacity"); color: currentTheme.textColor; font.pixelSize: 12 }
                                            Slider {
                                                id: signSealLogoOpacitySlider
                                                Layout.fillWidth: true
                                                from: 0
                                                to: 100
                                                stepSize: 1
                                                value: window.signSealLogoOpacityPercent
                                                focusPolicy: Qt.StrongFocus
                                                Accessible.name: tr("sign.seal.opacity")
                                                Accessible.description: tr("sign.seal.opacity_help")
                                                ToolTip.visible: hovered
                                                ToolTip.text: tr("sign.seal.opacity_help")
                                                onValueChanged: {
                                                    if (!window.applyingLoadedSettings && Math.round(value) !== window.signSealLogoOpacityPercent)
                                                        window.signSealLogoOpacityPercent = Math.round(value)
                                                }
                                            }
                                            Text { text: window.signSealLogoOpacityPercent + " %"; color: currentTheme.textColor; font.pixelSize: 12 }
                                        }
                                        ThemedCheckBox {
                                            Layout.fillWidth: true
                                            id: signQREnabledCheckBox
                                            text: tr("sign.seal.include_verification_qr")
                                            Accessible.name: text
                                            checked: window.signQREnabled
                                            onToggled: window.signQREnabled = checked
                                        }
                                        Binding { target: signQREnabledCheckBox; property: "checked"; value: window.signQREnabled }
                                        ThemedTextField {
                                            id: signQRContentField
                                            Layout.fillWidth: true
                                            visible: window.signQREnabled
                                            enabled: window.signQREnabled
                                            text: signQRContent
                                            placeholderText: tr("https://verifica.ejemplo/")
                                            Accessible.name: tr("QR del sello")
                                            Accessible.description: window.signFieldError("qr") ? tr(window.signFieldError("qr")) : ""
                                            hasError: window.signFieldError("qr") !== ""
                                            errorColor: currentTheme.errorColor
                                            onTextChanged: {
                                                signQRContent = text
                                                if (window.signFieldError("qr")) window.validateSignField("qr")
                                            }
                                            onEditingFinished: window.validateSignField("qr")
                                        }
                                        Binding { target: signQRContentField; property: "text"; value: window.signQRContent; when: !signQRContentField.activeFocus }
                                        Text { Layout.fillWidth: true; visible: window.signFieldError("qr") !== ""; text: "⚠ " + tr(window.signFieldError("qr")); color: currentTheme.errorColor; wrapMode: Text.WordWrap; Accessible.role: Accessible.StaticText }
                                        ThemedCheckBox { Layout.fillWidth: true; id: csvEnabledCheck; text: tr("paridad.lote3.csv.enable"); checked: window.signCSVEnabled; Accessible.name: text; onToggled: window.signCSVEnabled = checked }
                                        Binding { target: csvEnabledCheck; property: "checked"; value: window.signCSVEnabled }
                                        Label { text: tr("paridad.lote3.csv.notice"); visible: window.signCSVEnabled; color: currentTheme.secondaryTextColor; wrapMode: Text.WordWrap; Layout.fillWidth: true }
                                        ThemedTextField { id: csvCodeField; visible: window.signCSVEnabled; enabled: window.signCSVEnabled; Layout.fillWidth: true; placeholderText: tr("paridad.lote3.csv.code"); Accessible.name: placeholderText; Accessible.description: window.signFieldError("csvCode") ? tr(window.signFieldError("csvCode")) : ""; maximumLength: 128; text: window.signCSVCode; hasError: window.signFieldError("csvCode") !== ""; errorColor: currentTheme.errorColor
                                            onTextChanged: { window.signCSVCode = text; if (window.signFieldError("csvCode")) window.validateSignField("csvCode") }
                                            onEditingFinished: window.validateSignField("csvCode") }
                                        Text { Layout.fillWidth: true; visible: window.signFieldError("csvCode") !== ""; text: "⚠ " + tr(window.signFieldError("csvCode")); color: currentTheme.errorColor; wrapMode: Text.WordWrap }
                                        ThemedTextField { id: csvUrlField; visible: window.signCSVEnabled; enabled: window.signCSVEnabled; Layout.fillWidth: true; placeholderText: tr("paridad.lote3.csv.url"); Accessible.name: placeholderText; Accessible.description: window.signFieldError("csvUrl") ? tr(window.signFieldError("csvUrl")) : ""; maximumLength: 2048; text: window.signCSVUrl; hasError: window.signFieldError("csvUrl") !== ""; errorColor: currentTheme.errorColor
                                            onTextChanged: { window.signCSVUrl = text; if (window.signFieldError("csvUrl")) window.validateSignField("csvUrl") }
                                            onEditingFinished: window.validateSignField("csvUrl") }
                                        Text { Layout.fillWidth: true; visible: window.signFieldError("csvUrl") !== ""; text: "⚠ " + tr(window.signFieldError("csvUrl")); color: currentTheme.errorColor; wrapMode: Text.WordWrap }
                                        ThemedTextField { id: csvTextField; visible: window.signCSVEnabled; enabled: window.signCSVEnabled; Layout.fillWidth: true; placeholderText: tr("paridad.lote3.csv.text_optional"); Accessible.name: placeholderText; Accessible.description: window.signFieldError("csvText") ? tr(window.signFieldError("csvText")) : ""; maximumLength: 512; text: window.signCSVText; hasError: window.signFieldError("csvText") !== ""; errorColor: currentTheme.errorColor
                                            onTextChanged: { window.signCSVText = text; if (window.signFieldError("csvText")) window.validateSignField("csvText") }
                                            onEditingFinished: window.validateSignField("csvText") }
                                        Text { Layout.fillWidth: true; visible: window.signFieldError("csvText") !== ""; text: "⚠ " + tr(window.signFieldError("csvText")); color: currentTheme.errorColor; wrapMode: Text.WordWrap }
                                        ThemedCheckBox { Layout.fillWidth: true; id: csvQrCheck; visible: window.signCSVEnabled; text: tr("paridad.lote3.csv.qr"); checked: window.signCSVQR; Accessible.name: text; onToggled: window.signCSVQR = checked }
                                        Text {
                                            Layout.fillWidth: true
                                            visible: window.signQREnabled
                                            text: signQRContent.trim() !== "" && window.normalizedQrUrl(signQRContent) !== ""
                                                ? tr("El QR se incorporará dentro del sello visible.")
                                                : "⚠ " + tr("sign.seal.qr_https_error")
                                            color: signQRContent.trim() !== "" && window.normalizedQrUrl(signQRContent) !== ""
                                                ? currentTheme.secondaryTextColor : currentTheme.errorColor
                                            font.pixelSize: 11
                                            wrapMode: Text.WordWrap
                                        }
                                    }
                                }

                                GridLayout {
                                    columns: signMainOuterScroll.availableWidth < 560 ? 1 : Math.min(7, Math.max(1, Math.floor(signMainOuterScroll.availableWidth / 160)))
                                    columnSpacing: 10
                                    rowSpacing: 10
                                    Layout.fillWidth: true
                                    visible: signVisibleSeal
                                    enabled: signVisibleSeal
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Página(s)"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: signSealPagesField
                                            Accessible.name: tr("Página(s)")
                                            Accessible.description: window.signFieldError("pages") ? tr(window.signFieldError("pages")) : ""
                                            enabled: !signSealAllPages
                                            text: signSealPages
                                            placeholderText: tr("1 o 1,3-5")
                                            hasError: window.signFieldError("pages") !== ""
                                            errorColor: currentTheme.errorColor
                                            onTextChanged: {
                                                if (window.signFieldError("pages")) {
                                                    signSealPages = text
                                                    window.validateSignField("pages")
                                                }
                                            }
                                            onEditingFinished: {
                                                signSealPages = text
                                                window.validateSignField("pages")
                                                if (!applyPageSelectionValidity()) return
                                                text = signSealPages
                                                requestPdfPreview()
                                            }
                                        }
                                        Text { Layout.fillWidth: true; visible: window.signFieldError("pages") !== ""; text: "⚠ " + tr(window.signFieldError("pages")); color: currentTheme.errorColor; wrapMode: Text.WordWrap }
                                        AdaptiveRow {
                                            ThemedButton { text: tr("Primera"); onClicked: { signSealAllPages = false; signSealPages = "1"; applyPageSelectionValidity(); requestPdfPreview() } }
                                            ThemedButton { text: tr("Última"); enabled: previewTotalPages > 0; onClicked: { signSealAllPages = false; signSealPages = String(previewTotalPages); applyPageSelectionValidity(); requestPdfPreview() } }
                                            ThemedButton { text: tr("Todas"); onClicked: { signSealAllPages = true; applyPageSelectionValidity(); requestPdfPreview() } }
                                        }
                                        Binding { target: signSealPagesField; property: "text"; value: signSealPages; when: !signSealPagesField.activeFocus }
                                        Text {
                                            visible: signSealPagesError !== "" && !signSealAllPages && window.signFieldError("pages") === ""
                                            text: signSealPagesError
                                            color: "#d62828"
                                            font.pixelSize: 11
                                            wrapMode: Text.WordWrap
                                        }
                                        ThemedCheckBox {
                                            Layout.fillWidth: true
                                            id: signSealAllPagesCheckBox
                                            text: tr("Todas las páginas")
                                            checked: signSealAllPages
                                            onToggled: {
                                                signSealAllPages = checked
                                                applyPageSelectionValidity()
                                                requestPdfPreview()
                                            }
                                        }
                                        Binding { target: signSealAllPagesCheckBox; property: "checked"; value: window.signSealAllPages }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        ThemedCheckBox {
                                            Layout.fillWidth: true
                                            id: sealPerPageCheckBox
                                            text: tr("sign.seal.one_by_one")
                                            Accessible.name: text
                                            focusPolicy: Qt.StrongFocus
                                            checked: window.signSealPerPage
                                            onClicked: {
                                                if (checked) window.enablePerPageSeal()
                                                else window.signSealPerPage = false
                                            }
                                        }
                                        Binding { target: sealPerPageCheckBox; property: "checked"; value: window.signSealPerPage }
                                        AdaptiveRow {
                                            visible: window.signSealPerPage
                                            ThemedButton {
                                                text: tr("sign.seal.apply_all_pages")
                                                Accessible.name: text
                                                focusPolicy: Qt.StrongFocus
                                                onClicked: window.applySealToAllPages()
                                            }
                                            ThemedButton {
                                                text: window.signSealPlacements[String(window.previewCurrentPage)]
                                                    ? tr("sign.seal.remove_this_page") : tr("sign.seal.add_this_page")
                                                Accessible.name: text
                                                focusPolicy: Qt.StrongFocus
                                                onClicked: {
                                                    if (window.signSealPlacements[String(window.previewCurrentPage)]) window.removeSealFromPage()
                                                    else window.addSealToPage()
                                                }
                                            }
                                        }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Giros rápidos"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ComboBox {
                                            id: signSealRotationCombo
                                            Accessible.name: tr("Giros rápidos")
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("0°"), valor: 0 },
                                                { texto: tr("90°"), valor: 90 },
                                                { texto: tr("180°"), valor: 180 },
                                                { texto: tr("270°"), valor: 270 }
                                            ]
                                            textRole: "texto"
                                            currentIndex: rotationIndexForSeal()
                                            onActivated: function(index) {
                                                applySealRotation(model[index].valor)
                                            }
                                        }
                                        Binding { target: signSealRotationCombo; property: "currentIndex"; value: rotationIndexForSeal() }
                                        SpinBox {
                                            Layout.fillWidth: true
                                            from: 0
                                            to: 359
                                            value: signSealRotation
                                            editable: true
                                            onValueModified: applySealRotation(value)
                                            Accessible.name: tr("Giro horario del sello, de 0 a 359 grados")
                                        }
                                        Text { text: tr("Grados en sentido horario"); color: currentTheme.secondaryTextColor; font.pixelSize: 11 }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("X (0..1)"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: signSealXField
                                            Accessible.name: tr("X (0..1)")
                                            text: Number(signSealX).toFixed(4)
                                            onEditingFinished: {
                                                signSealX = clamp01(Number(text))
                                                text = Number(signSealX).toFixed(4)
                                                syncPreviewFromSeal()
                                            }
                                        }
                                        Binding { target: signSealXField; property: "text"; value: Number(signSealX).toFixed(4); when: !signSealXField.activeFocus }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Y (0..1)"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: signSealYField
                                            Accessible.name: tr("Y (0..1)")
                                            text: Number(signSealY).toFixed(4)
                                            onEditingFinished: {
                                                signSealY = clamp01(Number(text))
                                                text = Number(signSealY).toFixed(4)
                                                syncPreviewFromSeal()
                                            }
                                        }
                                        Binding { target: signSealYField; property: "text"; value: Number(signSealY).toFixed(4); when: !signSealYField.activeFocus }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Ancho (0..1)"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: signSealWField
                                            Accessible.name: tr("Ancho (0..1)")
                                            text: Number(signSealW).toFixed(4)
                                            onEditingFinished: {
                                                signSealW = clamp01(Number(text))
                                                text = Number(signSealW).toFixed(4)
                                                syncPreviewFromSeal()
                                            }
                                        }
                                        Binding { target: signSealWField; property: "text"; value: Number(signSealW).toFixed(4); when: !signSealWField.activeFocus }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Alto (0..1)"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: signSealHField
                                            Accessible.name: tr("Alto (0..1)")
                                            text: Number(signSealH).toFixed(4)
                                            onEditingFinished: {
                                                signSealH = clamp01(Number(text))
                                                text = Number(signSealH).toFixed(4)
                                                syncPreviewFromSeal()
                                            }
                                        }
                                        Binding { target: signSealHField; property: "text"; value: Number(signSealH).toFixed(4); when: !signSealHField.activeFocus }
                                    }
                                }

                                Rectangle {
                                    Layout.fillWidth: true
                                    visible: effectiveSignFormat() === "pades"
                                    color: Qt.rgba(1, 1, 1, 0.04)
                                    radius: 8
                                    border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                                    implicitHeight: padesMetaLayout.implicitHeight + 20

                                    ColumnLayout {
                                        id: padesMetaLayout
                                        anchors.fill: parent
                                        anchors.margins: 10
                                        spacing: 8

                                        Text {
                                            text: tr("Metadatos de la firma")
                                            color: currentTheme.textColor
                                            font.pixelSize: 13
                                            font.bold: true
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            text: tr("Opcional. Estos datos se incrustan en la firma PAdES y no sustituyen al certificado ni al sello visible.")
                                            color: currentTheme.secondaryTextColor
                                            font.pixelSize: 11
                                            wrapMode: Text.WordWrap
                                        }

                                        ColumnLayout {
                                            Layout.fillWidth: true
                                            // La imagen solo se dibuja en el sello: sin firma visible no tiene sentido.
                                            visible: window.signVisibleSeal
                                            Text { text: tr("Imagen de firma (opcional)"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                            AdaptiveRow {
                                                Layout.fillWidth: true
                                                ThemedButton {
                                                    text: tr("Usar logo GrxFirma")
                                                    onClicked: { sealStyle = "image"; signSealImagePath = bundledSealLogoPath }
                                                }
                                                ThemedButton {
                                                    id: signSealImageButton
                                                    text: tr("Elegir imagen…")
                                                    Accessible.description: window.signFieldError("image") ? tr(window.signFieldError("image")) : ""
                                                    alertColor: window.signFieldError("image") ? currentTheme.errorColor : "transparent"
                                                    onClicked: sealImageFileDialog.open()
                                                }
                                                ThemedButton {
                                                    text: tr("Quitar imagen")
                                                    enabled: signSealImagePath !== ""
                                                    onClicked: { sealStyle = "text"; signSealImagePath = "" }
                                                }
                                                ThemedCheckBox {
                                                    id: signSealKeepTextCheckBox
                                                    text: tr("Mantener texto sobre la imagen")
                                                    checked: signSealKeepText
                                                    onToggled: signSealKeepText = checked
                                                }
                                                Binding { target: signSealKeepTextCheckBox; property: "checked"; value: window.signSealKeepText }
                                            }
                                            Text { Layout.fillWidth: true; visible: window.signFieldError("image") !== ""; text: "⚠ " + tr(window.signFieldError("image")); color: currentTheme.errorColor; wrapMode: Text.WordWrap }
                                            Text {
                                                Layout.fillWidth: true
                                                visible: signSealImagePath !== ""
                                                text: tr("Imagen seleccionada: %1").arg(basename(localPathFromUrl(signSealImagePath)))
                                                color: currentTheme.textColor
                                                font.pixelSize: 11
                                                wrapMode: Text.WrapAnywhere
                                            }
                                            Text {
                                                Layout.fillWidth: true
                                                text: tr("Puedes usar un PNG/JPG como fondo del sello. Si mantienes el texto, se dibujará encima como marca de agua.")
                                                color: currentTheme.secondaryTextColor
                                                font.pixelSize: 11
                                                wrapMode: Text.WordWrap
                                            }
                                        }

                                        GridLayout {
                                            columns: signMainOuterScroll.availableWidth < 560 ? 1 : 2
                                            columnSpacing: 10
                                            rowSpacing: 10
                                            Layout.fillWidth: true
                                            ColumnLayout {
                                                Layout.fillWidth: true
                                                Text { text: tr("Motivo"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                                ThemedTextField {
                                                    id: signReasonField
                                                    Layout.fillWidth: true
                                                    text: signReason
                                                    placeholderText: tr("Firma electrónica avanzada")
                                                    onTextChanged: signReason = text
                                                }
                                                Binding { target: signReasonField; property: "text"; value: window.signReason; when: !signReasonField.activeFocus }
                                            }
                                            ColumnLayout {
                                                Layout.fillWidth: true
                                                Text { text: tr("Ubicación"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                                ThemedTextField {
                                                    id: signLocationField
                                                    Layout.fillWidth: true
                                                    text: signLocation
                                                    placeholderText: tr("Granada")
                                                    onTextChanged: signLocation = text
                                                }
                                                Binding { target: signLocationField; property: "text"; value: window.signLocation; when: !signLocationField.activeFocus }
                                            }
                                        }

                                        ColumnLayout {
                                            Layout.fillWidth: true
                                            Text { text: tr("Contacto"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                            ThemedTextField {
                                                id: signContactField
                                                Layout.fillWidth: true
                                                text: signContactInfo
                                                placeholderText: tr("correo@ejemplo.es")
                                                onTextChanged: signContactInfo = text
                                            }
                                            Binding { target: signContactField; property: "text"; value: window.signContactInfo; when: !signContactField.activeFocus }
                                        }
                                    }
                                }

                                AdaptiveRow {
                                    visible: signVisibleSeal && supportsVisibleSeal()
                                    Layout.alignment: Qt.AlignRight
                                    spacing: 10
                                    ThemedButton {
                                        text: tr("↻ Rotar Documento")
                                        font.pixelSize: 12
                                        onClicked: {
                                            pagePreview.pageRatio = 1.0 / pagePreview.pageRatio
                                        }
                                    }
                                     ThemedButton {
                                         text: tr("↶ Rotar Firma")
                                         font.pixelSize: 12
                                         onClicked: {
                                             applySealRotation(window.signSealRotation + 90)
                                         }
                                     }
                                }

                                RowLayout {
                                    visible: signVisibleSeal && supportsVisibleSeal() && previewInputPath() !== ""
                                    Layout.fillWidth: true
                                    spacing: 10

                                    RowLayout {
                                        visible: currentBatchPaths.length > 1
                                        spacing: 6
                                        ThemedButton { text: "|<"; Accessible.name: tr("Primer documento"); enabled: previewDocumentIndex > 0; onClicked: goToPreviewDocument(0) }
                                        ThemedButton { text: "<"; Accessible.name: tr("Documento anterior"); enabled: previewDocumentIndex > 0; onClicked: goToPreviewDocument(previewDocumentIndex - 1) }
                                        Text {
                                            text: tr("PDF %1/%2: %3").arg(previewDocumentIndex + 1).arg(currentBatchPaths.length).arg(previewDocumentLabel())
                                            color: currentTheme.textColor
                                            font.pixelSize: 12
                                            elide: Text.ElideRight
                                            Layout.fillWidth: true; Layout.preferredWidth: 1; Layout.maximumWidth: 320
                                        }
                                        ThemedButton { text: ">"; Accessible.name: tr("Documento siguiente"); enabled: previewDocumentIndex < currentBatchPaths.length - 1; onClicked: goToPreviewDocument(previewDocumentIndex + 1) }
                                        ThemedButton { text: ">|"; Accessible.name: tr("Último documento"); enabled: previewDocumentIndex < currentBatchPaths.length - 1; onClicked: goToPreviewDocument(currentBatchPaths.length - 1) }
                                    }

                                    Item { Layout.fillWidth: true }

                                    RowLayout {
                                        spacing: 6
                                        ThemedButton { text: "|<"; Accessible.name: tr("Primera página"); enabled: previewCurrentPage > 1; onClicked: goToPreviewPage(1) }
                                        ThemedButton { text: "<"; Accessible.name: tr("Página anterior"); enabled: previewCurrentPage > 1; onClicked: goToPreviewPage(previewCurrentPage - 1) }
                                        Text {
                                            text: tr("Página %1/%2").arg(previewCurrentPage).arg(Math.max(1, previewTotalPages))
                                            color: currentTheme.textColor
                                            font.pixelSize: 12
                                        }
                                        ThemedButton { text: ">"; Accessible.name: tr("Página siguiente"); enabled: previewCurrentPage < Math.max(1, previewTotalPages); onClicked: goToPreviewPage(previewCurrentPage + 1) }
                                        ThemedButton { text: ">|"; Accessible.name: tr("Última página"); enabled: previewCurrentPage < Math.max(1, previewTotalPages); onClicked: goToPreviewPage(previewTotalPages) }
                                    }
                                }

                                Text {
                                    visible: signVisibleSeal && supportsVisibleSeal() && previewInputPath() !== ""
                                    Layout.fillWidth: true
                                    text: tr("Arrastra el recuadro sobre el PDF para colocar el sello. Usa la esquina para cambiar su tamaño; la vista se actualiza automáticamente.")
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                    font.pixelSize: 11
                                }

                                ColumnLayout {
                                    visible: currentBatchPaths.length > 1 && signVisibleSeal && supportsVisibleSeal()
                                    Layout.fillWidth: true
                                    ThemedCheckBox {
                                        Layout.fillWidth: true
                                        text: tr("Personalizar el sello para este PDF")
                                        checked: window.batchSealOverrideEnabled
                                        Accessible.name: text
                                        onClicked: window.setBatchSealOverrideEnabled(checked)
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        text: window.batchSealOverrideEnabled
                                            ? tr("Este PDF usa su propia posición y rango de páginas.")
                                            : tr("Este PDF usa la plantilla global del lote.")
                                        color: currentTheme.secondaryTextColor
                                        font.pixelSize: 11
                                        wrapMode: Text.WordWrap
                                    }
                                }

                                Rectangle {
                                    id: portalSealEditor
                                    Layout.fillWidth: true
                                    Layout.preferredHeight: signVisibleSeal ? pagePreview.height + sealDrawControls.implicitHeight + 30 : 0
                                    visible: signVisibleSeal && supportsVisibleSeal()
                                    // Colores del tema: la ayuda y los avisos usan textColor.
                                    color: currentTheme.cardColor
                                    border.color: currentTheme.secondaryTextColor
                                    radius: 8

                                    ColumnLayout {
                                        id: sealDrawControls
                                        anchors.top: parent.top
                                        anchors.left: parent.left
                                        anchors.right: parent.right
                                        anchors.margins: 10
                                        ThemedButton {
                                            id: sealDrawButton
                                            text: tr("sign.seal.draw_area")
                                            checkable: true
                                            enabled: sealDrawArea.ready
                                            Accessible.name: text
                                            Accessible.description: tr("sign.seal.draw_help")
                                            onToggled: sealDrawArea.drawMode = checked
                                            Keys.onPressed: (event) => sealDrawArea.handleKey(event)
                                        }
                                        Text {
                                            visible: sealDrawButton.checked
                                            text: tr("sign.seal.draw_help")
                                            color: currentTheme.textColor
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                        }
                                        Text {
                                            id: sealDrawNotice
                                            property string messageKey: ""
                                            text: messageKey ? tr(messageKey) : ""
                                            visible: text !== ""
                                            color: currentTheme.textColor
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                            Accessible.role: Accessible.StaticText
                                            Accessible.name: text
                                        }
                                    }

                                     Rectangle {
                                         anchors.centerIn: pagePreview
                                         anchors.horizontalCenterOffset: 6
                                         anchors.verticalCenterOffset: 8
                                         width: pagePreview.width
                                         height: pagePreview.height
                                         color: "#22000000"
                                         radius: 3
                                         z: 0
                                     }

                                     Rectangle {
                                         id: pagePreview
                                         anchors.top: sealDrawControls.bottom
                                         anchors.topMargin: 10
                                         anchors.horizontalCenter: parent.horizontalCenter
                                         property real pageRatio: 1.0
                                         z: 1

                                         Image {
                                             id: pdfPageImage
                                             anchors.fill: parent
                                             anchors.margins: 3
                                             fillMode: Image.PreserveAspectFit
                                             source: ""
                                             visible: source !== ""
                                         }
                                        width: Math.max(1, Math.min(parent.width - 20, 500 / pageRatio,
                                            window.portalSealMode ? Math.max(1, parent.height - sealDrawControls.height - 30) / pageRatio : 500 / pageRatio))
                                        height: width * pageRatio
                                        color: "#ffffff"
                                        border.color: "#7f8fa4"
                                        border.width: 2

                                        Rectangle {
                                            anchors.fill: parent
                                            anchors.margins: 1
                                            color: "transparent"
                                            border.color: "#d9e0ea"
                                            border.width: 1
                                        }
                                        onWidthChanged: syncPreviewFromSeal()
                                        onHeightChanged: syncPreviewFromSeal()

                                        SealDrawArea {
                                            id: sealDrawArea
                                            anchors.fill: parent
                                            z: 100
                                            ready: pdfPageImage.status === Image.Ready &&
                                                previewGeometryPath === previewInputPath() &&
                                                previewGeometryPage === previewCurrentPage
                                            initialRect: ({x: signSealX, y: signSealY, w: signSealW, h: signSealH})
                                            accessibleName: tr("sign.seal.draw_area")
                                            accessibleHelp: tr("sign.seal.draw_help")
                                            onCommitted: (rect) => {
                                                // Un único guardado por página, incluso si antes no había sello.
                                                window.loadingPageSeal = true
                                                window.signSealX = rect.x
                                                window.signSealY = rect.y
                                                window.signSealW = rect.w
                                                window.signSealH = rect.h
                                                window.loadingPageSeal = false
                                                if (window.signSealPerPage) window.addSealToPage()
                                                window.syncPreviewFromSeal()
                                                window.savePageSeal()
                                                window.scheduleSealPreview()
                                            }
                                            onFeedback: (key) => {
                                                sealDrawNotice.messageKey = key
                                                Qt.callLater(function() { backend.announceAccessible(sealDrawNotice, tr(key)) })
                                            }
                                            onKeyboardMoved: (rect) => {
                                                sealDrawAnnounceTimer.rect = rect
                                                sealDrawAnnounceTimer.restart()
                                            }
                                            // Intro aplica y Esc descarta; en ambos casos se sale del modo.
                                            onFinished: {
                                                sealDrawButton.checked = false
                                                sealDrawArea.drawMode = false
                                                sealDrawButton.forceActiveFocus()
                                            }
                                            Timer {
                                                id: sealDrawAnnounceTimer
                                                property var rect: null
                                                interval: 350
                                                onTriggered: {
                                                    if (!rect) return
                                                    const percent = (value) => Math.round(value * 100)
                                                    backend.announceAccessible(sealDrawNotice, tr("sign.seal.draw_position")
                                                        .replace("%1", percent(rect.x))
                                                        .replace("%2", percent(1 - rect.y - rect.h))
                                                        .replace("%3", percent(rect.w))
                                                        .replace("%4", percent(rect.h)))
                                                }
                                            }
                                        }

                                        Rectangle {
                                            id: sealRect
                                            x: Math.max(0, Math.min(parent.width - width, signSealX * parent.width))
                                            y: Math.max(0, Math.min(parent.height - height, (1.0 - signSealY - signSealH) * parent.height))
                                            width: Math.max(30, signSealW * parent.width)
                                            height: Math.max(20, signSealH * parent.height)
                                            rotation: window.signSealRotation
                                            color: "transparent"
                                            border.color: "#2980b9"
                                            border.width: 2
                                            visible: signVisibleSeal && supportsVisibleSeal()
                                                && (!signSealPerPage || !!signSealPlacements[String(previewCurrentPage)])

                                            // Contenido del sello (lo que se verá en el PDF)
                                            Item {
                                                id: sealContentPreview
                                                visible: false
                                                anchors.centerIn: parent
                                                property real previewAspectRatio: 3.9
                                                property real availableWidth: (window.signSealRotation % 180 === 0) ? (parent.width - 10) : (parent.height - 10)
                                                property real availableHeight: (window.signSealRotation % 180 === 0) ? (parent.height - 10) : (parent.width - 10)
                                                width: Math.max(24, Math.min(availableWidth, availableHeight * previewAspectRatio))
                                                height: Math.max(16, Math.min(availableHeight, availableWidth / previewAspectRatio))
                                                rotation: window.signSealRotation
                                                clip: true

                                                Image {
                                                    anchors.fill: parent
                                                    visible: signSealImagePath !== ""
                                                    source: signSealImagePath
                                                    fillMode: Image.PreserveAspectFit
                                                    smooth: true
                                                    opacity: signSealKeepText ? 0.58 : 0.96
                                                }

                                                Row {
                                                    anchors.centerIn: parent
                                                    width: parent.width
                                                    spacing: 6
                                                    visible: signSealImagePath === "" || signSealKeepText || signQRContent.trim() !== ""

                                                    Column {
                                                        visible: signSealImagePath === "" || signSealKeepText
                                                        width: visible ? (previewQRBox.visible ? (parent.width - previewQRBox.width - parent.spacing) : parent.width) : 0
                                                        spacing: 2
                                                        clip: true

                                                        Text {
                                                            text: tr("✍ FIRMA DIGITAL")
                                                            font.bold: true
                                                            font.pixelSize: Math.max(8, Math.min(14, sealContentPreview.height * 0.2))
                                                            color: "#2980b9"
                                                            anchors.horizontalCenter: parent.horizontalCenter
                                                        }
                                                        Text {
                                                            text: (selectedCertIndex !== -1 && window.selectedCertData)
                                                                  ? (window.selectedCertData.subjectName || (window.selectedCertData.subject && window.selectedCertData.subject.CN) || tr("Firmante"))
                                                                  : tr("Muestra de Firma")
                                                            font.pixelSize: Math.max(7, Math.min(12, sealContentPreview.height * 0.15))
                                                            color: "#34495e"
                                                            width: parent.width
                                                            wrapMode: Text.Wrap
                                                            horizontalAlignment: Text.AlignHCenter
                                                            elide: Text.ElideRight
                                                            maximumLineCount: 2
                                                            anchors.horizontalCenter: parent.horizontalCenter
                                                        }
                                                    }

                                                    Rectangle {
                                                        id: previewQRBox
                                                        visible: signQREnabled && signQRContent.trim() !== ""
                                                        width: Math.max(28, Math.min(parent.width * 0.26, parent.height - 4))
                                                        height: width
                                                        radius: 6
                                                        color: "#ffffff"
                                                        border.color: "#2980b9"
                                                        border.width: 1

                                                        Rectangle {
                                                            anchors.fill: parent
                                                            anchors.margins: 4
                                                            radius: 4
                                                            color: "#e8f1fb"
                                                            border.color: "#7fb0da"
                                                            border.width: 1

                                                            Text {
                                                                anchors.centerIn: parent
                                                                text: tr("QR")
                                                                color: "#2980b9"
                                                                font.bold: true
                                                                font.pixelSize: Math.max(8, Math.min(12, parent.height * 0.22))
                                                            }
                                                        }
                                                    }
                                                }
                                            }

                                            Image {
                                                anchors.fill: parent
                                                anchors.margins: 2
                                                source: window.sealPreviewImage
                                                // El motor entrega aquí la tarjeta sin girar; sealRect la gira completa.
                                                fillMode: Image.Stretch
                                                visible: window.sealPreviewImage !== ""
                                                Accessible.role: Accessible.Graphic
                                                Accessible.name: tr("Vista real del sello generado por el motor")
                                                onStatusChanged: {
                                                    if (status === Image.Error && window.sealPreviewImage !== "") {
                                                        window.sealPreviewMessage = tr("No se pudo mostrar la imagen del sello.")
                                                        window.sealPreviewImage = ""
                                                    }
                                                }
                                            }
                                            Text {
                                                anchors.centerIn: parent
                                                width: parent.width - 8
                                                visible: window.sealPreviewImage === ""
                                                text: window.sealPreviewMessage || tr("Preparando vista real…")
                                                wrapMode: Text.WordWrap
                                                horizontalAlignment: Text.AlignHCenter
                                                color: "#263849"
                                                font.pixelSize: 10
                                            }

                                            MouseArea {
                                                id: dragArea
                                                anchors.fill: parent
                                                drag.target: parent
                                                drag.minimumX: 0
                                                drag.minimumY: 0
                                                drag.maximumX: pagePreview.width - sealRect.width
                                                drag.maximumY: pagePreview.height - sealRect.height
                                                cursorShape: Qt.OpenHandCursor
                                                preventStealing: true
                                                enabled: !resizeArea.pressed && !rotateArea.pressed
                                                onPressed: cursorShape = Qt.ClosedHandCursor
                                                onReleased: cursorShape = Qt.OpenHandCursor
                                                onPositionChanged: {
                                                    syncSealFromPreview()
                                                }
                                            }

                                            Rectangle {
                                                width: 44
                                                height: 44
                                                color: "transparent"
                                                border.color: rotateArea.activeFocus ? "#f4b400" : "transparent"
                                                border.width: 2
                                                radius: 4
                                                anchors.right: parent.right
                                                anchors.top: parent.top
                                                z: 11
                                                Rectangle {
                                                    width: 22
                                                    height: 22
                                                    radius: 11
                                                    color: "#2980b9"
                                                    border.color: "white"
                                                    border.width: 1
                                                    anchors.right: parent.right
                                                    anchors.top: parent.top
                                                    Text {
                                                        anchors.centerIn: parent
                                                        text: "↻"
                                                        color: "white"
                                                        font.pixelSize: 18
                                                    }
                                                }
                                                MouseArea {
                                                    id: rotateArea
                                                    anchors.fill: parent
                                                    cursorShape: Qt.CrossCursor
                                                    preventStealing: true
                                                    activeFocusOnTab: true
                                                    Accessible.name: tr("Girar sello")
                                                    Accessible.description: tr("sign.seal.rotate_help")
                                                    Accessible.role: Accessible.Button
                                                    onPressed: {
                                                        forceActiveFocus()
                                                        window.sealRotationDragging = true
                                                        sealPreviewDelay.stop()
                                                    }
                                                    onPositionChanged: (mouse) => {
                                                        if (!pressed) return
                                                        const point = mapToItem(pagePreview, mouse.x, mouse.y)
                                                        window.applySealRotation(window.rotationFromSealPointer(point, mouse.modifiers))
                                                    }
                                                    onReleased: {
                                                        window.sealRotationDragging = false
                                                        window.scheduleSealPreview()
                                                    }
                                                    onCanceled: {
                                                        window.sealRotationDragging = false
                                                        window.scheduleSealPreview()
                                                    }
                                                    Keys.onPressed: (event) => {
                                                        if (event.key === Qt.Key_Left || event.key === Qt.Key_Down ||
                                                                event.key === Qt.Key_Right || event.key === Qt.Key_Up) {
                                                            const direction = (event.key === Qt.Key_Left || event.key === Qt.Key_Down) ? -1 : 1
                                                            window.applySealRotation(window.signSealRotation + direction *
                                                                                     ((event.modifiers & Qt.ShiftModifier) ? 15 : 1))
                                                            event.accepted = true
                                                        }
                                                    }
                                                }
                                            }

                                            // Manejador de redimensionado (esquina inferior derecha)
                                            Rectangle {
                                                width: 16
                                                height: 16
                                                color: "#2980b9"
                                                radius: 8
                                                anchors.right: parent.right
                                                anchors.bottom: parent.bottom
                                                anchors.margins: -8
                                                z: 10
                                                border.color: "white"
                                                border.width: 1

                                                MouseArea {
                                                    id: resizeArea
                                                    anchors.centerIn: parent
                                                    width: 44
                                                    height: 44
                                                    cursorShape: Qt.SizeFDiagCursor
                                                    // Sin esto, el área desplazable de la página roba el arrastre y lo corta.
                                                    preventStealing: true
                                                    onPositionChanged: (mouse) => {
                                                        if (pressed) {
                                                            let p = mapToItem(sealRect, mouse.x, mouse.y)
                                                            let newW = Math.max(40, Math.min(pagePreview.width - sealRect.x, p.x))
                                                            let newH = Math.max(25, Math.min(pagePreview.height - sealRect.y, p.y))
                                                            sealRect.width = newW
                                                            sealRect.height = newH
                                                            syncSealFromPreview()
                                                        }
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }

                                Text {
                                    visible: signVisibleSeal && !supportsVisibleSeal()
                                    color: currentTheme.secondaryTextColor
                                    text: window.currentBatchDirectory !== ""
                                          ? tr("Para previsualizar el sello en un lote por carpeta, seleccione uno o varios PDF concretos. La firma visible solo se aplica a PAdES (PDF).")
                                          : tr("La firma visible solo se aplica a PAdES (PDF).")
                                }
                                }
                            }
                        }

                        AdaptiveRow {
                            spacing: 10
                                ThemedButton {
                                    text: window.signingInProgress ? tr("⌛ Firmando...") : (window.autoVerificationInProgress ? tr("⌛ Verificando...") : tr("Firmar ahora"))
                                    font.bold: true
                                    palette.button: (window.signingInProgress || window.autoVerificationInProgress) ? currentTheme.secondaryTextColor : currentTheme.primaryColor
                                    palette.buttonText: "white"
                                    enabled: !window.signingInProgress && !window.autoVerificationInProgress
                                             && !window.signWaitingForPreview
                                             && window.selectedCertificateUsable
                                    ToolTip.visible: hovered && !window.selectedCertificateUsable
                                    ToolTip.text: window.certificateStatusReason(window.selectedCertData)
                                    onClicked: {
                                        if (!window.hasSigningSelection() && selectedCertIndex === -1) {
                                            signValidationErrorDialog.errorMessage = tr("Debe cargar uno o varios documentos y seleccionar un certificado para poder firmar.")
                                            signValidationErrorDialog.open()
                                        } else if (!window.hasSigningSelection()) {
                                            signValidationErrorDialog.errorMessage = tr("Debe cargar uno o varios documentos antes de realizar la firma.")
                                            signValidationErrorDialog.open()
                                        } else if (selectedCertIndex === -1) {
                                            signValidationErrorDialog.errorMessage = tr("Debe seleccionar un certificado de la lista para poder firmar el documento.")
                                            signValidationErrorDialog.open()
                                        } else if (window.multiCosignEnabled && signAction === "countersign") {
                                            signValidationErrorDialog.errorMessage = tr("La cofirma múltiple guiada solo está disponible para firmar o cofirmar, no para contrafirmar.")
                                            signValidationErrorDialog.open()
                                        } else if (window.multiCosignEnabled && window.multiCosignCertificateIds.length === 0) {
                                            signValidationErrorDialog.errorMessage = tr("Debe seleccionar al menos un certificado adicional para la cofirma múltiple guiada.")
                                            signValidationErrorDialog.open()
                                        } else {
                                            window.requestSignConfirmation(selectedCertIndex)
                                        }
                                    }
                                }
                                ThemedButton {
                                    text: tr("Ver firmado")
                                    visible: !window.isBatchMode() && window.currentOutputPath !== "" && !window.signingInProgress
                                    onClicked: backend.openExternal(window.currentOutputPath)
                                }
                                ThemedButton {
                                    text: tr("Validar")
                                    visible: !window.isBatchMode() && window.currentOutputPath !== "" && !window.signingInProgress
                                    onClicked: jumpToVerify(window.currentOutputPath)
                                }
                            ThemedButton {
                                text: tr("Limpiar")
                                onClicked: {
                                    window.clearSignResult()
                                    window.currentFilePath = ""
                                    window.clearBatchSelection()
                                    window.currentOutputPath = ""
                                    window.currentOutputVerificationMessage = ""
                                    window.currentOutputVerificationDetails = null
                                    signVisibleSeal = false
                                }
                            }
                        }

                        // Espera de la vista del PDF antes de firmar con sello visible.
                        Text {
                            Layout.fillWidth: true
                            visible: window.signWaitingForPreview
                            text: tr("sign.seal.preview_loading_before_sign")
                            color: currentTheme.textColor
                            wrapMode: Text.WordWrap
                            Accessible.role: Accessible.AlertMessage
                            Accessible.name: text
                        }

                        // Motivo visible cuando «Firmar ahora» está desactivado por el certificado.
                        Text {
                            Layout.fillWidth: true
                            visible: !window.signingInProgress && !window.autoVerificationInProgress && !window.selectedCertificateUsable
                            text: window.selectedCertData ? window.certificateStatusReason(window.selectedCertData) : tr("sign.need_certificate")
                            color: currentTheme.textColor
                            wrapMode: Text.WordWrap
                            Accessible.role: Accessible.StaticText
                            Accessible.name: text
                        }

                        Text {
                            visible: !window.isBatchMode() && window.currentOutputVerificationMessage !== ""
                            text: window.currentOutputVerificationMessage
                            color: verificationOutcomeColor(window.currentOutputVerificationDetails)
                            font.pixelSize: 12
                            wrapMode: Text.WordWrap
                            Layout.fillWidth: true
                        }

                        Rectangle {
                            visible: !window.isBatchMode() && window.currentOutputVerificationDetails !== null
                            Layout.fillWidth: true
                            implicitHeight: singleVerifySummaryColumn.implicitHeight + 18
                            radius: 10
                            color: currentTheme.cardColor
                            border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)

                            ColumnLayout {
                                id: singleVerifySummaryColumn
                                anchors.fill: parent
                                anchors.margins: 10
                                spacing: 8

                                Text {
                                    text: tr("Resumen de verificación")
                                    color: currentTheme.primaryColor
                                    font.pixelSize: 14
                                    font.bold: true
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 8

                                    Rectangle {
                                        Layout.fillWidth: true
                                        implicitHeight: singleVerifyBasicCol.implicitHeight + 16
                                        radius: 8
                                        color: "#223244"

                                        Column {
                                            id: singleVerifyBasicCol
                                            anchors.fill: parent
                                            anchors.margins: 8
                                            spacing: 4

                                            Text {
                                                text: tr("Formato: ") + (window.currentOutputVerificationDetails && window.currentOutputVerificationDetails.format ? window.currentOutputVerificationDetails.format : tr("No disponible"))
                                                color: "white"
                                                font.pixelSize: 12
                                                width: parent.width
                                                wrapMode: Text.Wrap
                                            }
                                            Text {
                                                text: tr("Cobertura: ") + (window.currentOutputVerificationDetails && window.currentOutputVerificationDetails.coverage ? verificationCoverageText(window.currentOutputVerificationDetails.coverage) : tr("No disponible"))
                                                color: "white"
                                                font.pixelSize: 12
                                                width: parent.width
                                                wrapMode: Text.Wrap
                                            }
                                            Text {
                                                text: tr("Firmantes: ") + verificationSignerShortText(window.currentOutputVerificationDetails)
                                                color: "white"
                                                opacity: 0.9
                                                font.pixelSize: 12
                                                width: parent.width
                                                wrapMode: Text.Wrap
                                            }
                                        }
                                    }

                                    Rectangle {
                                        Layout.fillWidth: true
                                        implicitHeight: singleVerifyWarningsCol.implicitHeight + 16
                                        radius: 8
                                        color: "#223244"

                                        Column {
                                            id: singleVerifyWarningsCol
                                            anchors.fill: parent
                                            anchors.margins: 8
                                            spacing: 4

                                            Text {
                                                text: tr("Advertencias")
                                                color: "#f39c12"
                                                font.bold: true
                                                font.pixelSize: 12
                                                width: parent.width
                                                wrapMode: Text.Wrap
                                            }
                                            Text {
                                                text: verificationArrayText(window.currentOutputVerificationDetails ? window.currentOutputVerificationDetails.warnings : [])
                                                color: "white"
                                                opacity: 0.9
                                                font.pixelSize: 12
                                                width: parent.width
                                                wrapMode: Text.Wrap
                                            }
                                            Text {
                                                text: tr("Errores: ") + verificationArrayText(window.currentOutputVerificationDetails ? window.currentOutputVerificationDetails.errors : [])
                                                color: "#ffb3b3"
                                                opacity: 0.95
                                                font.pixelSize: 12
                                                width: parent.width
                                                wrapMode: Text.Wrap
                                            }
                                        }
                                    }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 8

                                        Repeater {
                                            model: [
                                                { label: tr("Integridad"), value: window.currentOutputVerificationDetails ? window.currentOutputVerificationDetails.integrity : null },
                                                { label: tr("Certificado"), value: window.currentOutputVerificationDetails ? window.currentOutputVerificationDetails.certificate : null },
                                                { label: tr("Confianza"), value: window.currentOutputVerificationDetails ? window.currentOutputVerificationDetails.trust : null }
                                            ]

                                        delegate: Rectangle {
                                            Layout.fillWidth: true
                                            property bool expanded: false
                                            implicitHeight: singleVerifyAspectCol.implicitHeight + 16
                                            radius: 8
                                            color: "#223244"
                                            visible: modelData.value !== null && modelData.value !== undefined

                                            Column {
                                                id: singleVerifyAspectCol
                                                anchors.fill: parent
                                                anchors.margins: 8
                                                spacing: 4

                                                RowLayout {
                                                    id: singleVerifyAspectHeader
                                                    width: parent.width
                                                    spacing: 8

                                                    Text {
                                                        Layout.preferredWidth: 1
                                                        text: modelData.label + ": " + verificationStatusText(modelData.value && modelData.value.status ? modelData.value.status : "")
                                                        color: verificationAspectColor(modelData.value && modelData.value.status ? modelData.value.status : "")
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                    ToolButton {
                                                        text: parent.parent.parent.expanded ? "▼" : "▶"
                                                        ToolTip.visible: hovered
                                                        ToolTip.delay: 250
                                                        ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                    }

                                                    MouseArea {
                                                        anchors.fill: parent
                                                        onClicked: parent.parent.parent.expanded = !parent.parent.parent.expanded
                                                        acceptedButtons: Qt.LeftButton
                                                        cursorShape: Qt.PointingHandCursor
                                                    }
                                                }
                                                Text {
                                                    text: tr("Razón: ") + ((modelData.value && modelData.value.reason) ? modelData.value.reason : tr("No disponible"))
                                                    color: "white"
                                                    opacity: 0.9
                                                    font.pixelSize: 12
                                                    width: parent.width
                                                    wrapMode: Text.Wrap
                                                    visible: parent.parent.expanded
                                                }
                                                Text {
                                                    text: tr("Detalles: ") + verificationAspectDetailsText(modelData.value)
                                                    color: "white"
                                                    opacity: 0.8
                                                    font.pixelSize: 12
                                                    width: parent.width
                                                    wrapMode: Text.Wrap
                                                    visible: parent.parent.expanded
                                                }
                                            }

                                        }
                                    }
                                }

                                Rectangle {
                                    Layout.fillWidth: true
                                    implicitHeight: singleVerifyEvidenceCol.implicitHeight + 16
                                    radius: 8
                                    color: "#223244"

                                    Column {
                                        id: singleVerifyEvidenceCol
                                        anchors.fill: parent
                                        anchors.margins: 8
                                        spacing: 4

                                        Text {
                                            text: tr("Evidencias")
                                            color: currentTheme.primaryColor
                                            font.bold: true
                                            font.pixelSize: 12
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        Text {
                                            text: verificationEvidenceText(window.currentOutputVerificationDetails)
                                            color: "white"
                                            opacity: 0.9
                                            font.pixelSize: 12
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                    }
                                }
                            }
                        }

                        Rectangle {
                            visible: window.currentBatchResults.length > 0
                            Layout.fillWidth: true
                            implicitHeight: batchResultsCol.implicitHeight + 24
                            radius: 10
                            color: currentTheme.cardColor
                            border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)

                            ColumnLayout {
                                id: batchResultsCol
                                anchors.fill: parent
                                anchors.margins: 12
                                spacing: 10

                                Text {
                                    text: tr("Resultados del lote")
                                    color: currentTheme.textColor
                                    font.pixelSize: 16
                                    font.bold: true
                                }

                                Repeater {
                                    model: window.currentBatchResults
                                    delegate: Rectangle {
                                        property bool expanded: false
                                        Layout.fillWidth: true
                                        implicitHeight: itemCol.implicitHeight + 16
                                        radius: 8
                                        color: Qt.rgba(1, 1, 1, 0.03)
                                        border.color: !modelData.ok
                                                      ? "#e74c3c"
                                                      : (modelData.verifyDone
                                                         ? verificationOutcomeColor(modelData.verifyDetails)
                                                         : "#f39c12")
                                        border.width: 1

                                        ColumnLayout {
                                            id: itemCol
                                            anchors.fill: parent
                                            anchors.margins: 8
                                            spacing: 6

                                            RowLayout {
                                                Layout.fillWidth: true
                                                Text {
                                                    Layout.preferredWidth: 1
                                                    Layout.fillWidth: true
                                                    text: window.basename(modelData.inputPath || "")
                                                    color: currentTheme.textColor
                                                    font.bold: true
                                                    wrapMode: Text.WordWrap
                                                }
                                                Rectangle {
                                                    radius: 10
                                                    color: !modelData.ok
                                                           ? "#e74c3c"
                                                           : (modelData.verifyDone
                                                              ? verificationOutcomeColor(modelData.verifyDetails)
                                                              : "#f39c12")
                                                    implicitWidth: stateLabel.implicitWidth + 14
                                                    implicitHeight: stateLabel.implicitHeight + 6

                                                    Text {
                                                        id: stateLabel
                                                        anchors.centerIn: parent
                                                        text: !modelData.ok
                                                              ? tr("Error")
                                                              : (modelData.verifyDone
                                                                 ? verificationOutcomeLabel(modelData.verifyDetails)
                                                                 : tr("Verificación incompleta"))
                                                        color: "white"
                                                        font.pixelSize: 11
                                                        font.bold: true
                                                    }
                                                }
                                            }

                                            Text {
                                                visible: !!modelData.error || (!!modelData.outputPath && !expanded)
                                                text: !modelData.ok && !!modelData.error
                                                      ? localizeVisibleDiagnosticText(modelData.error || "")
                                                      : (!!modelData.outputPath
                                                         ? tr("Documento procesado correctamente.")
                                                         : "")
                                                color: modelData.ok ? currentTheme.secondaryTextColor : "#ffb3b3"
                                                font.pixelSize: 12
                                                wrapMode: Text.WordWrap
                                            }

                                            AdaptiveRow {
                                                visible: !!modelData.error || !!modelData.outputPath || (modelData.ok && modelData.verifyDone && !!modelData.verifyDetails)
                                                Layout.fillWidth: true
                                                spacing: 8
                                                ThemedButton {
                                                    text: expanded ? tr("▼ Ocultar detalles") : tr("▶ Ver detalles")
                                                    onClicked: expanded = !expanded
                                                }
                                                ThemedButton {
                                                    visible: modelData.ok && !!modelData.outputPath
                                                    text: tr("Abrir")
                                                    onClicked: backend.openExternal(modelData.outputPath)
                                                }
                                                ThemedButton {
                                                    visible: modelData.ok && !!modelData.outputPath
                                                    text: tr("Validar")
                                                    onClicked: jumpToVerify(modelData.outputPath)
                                                }
                                            }

                                            Text {
                                                visible: expanded && modelData.ok && !!modelData.outputPath && !modelData.verifyDone
                                                text: tr("Verificando firma generada...")
                                                color: currentTheme.secondaryTextColor
                                                font.pixelSize: 12
                                                wrapMode: Text.WordWrap
                                            }
                                            Text {
                                                visible: expanded && modelData.ok && !!modelData.outputPath && modelData.verifyDone
                                                text: verificationOutcomeKind(modelData.verifyDetails) === "trusted"
                                                      ? tr("Firma verificada correctamente.")
                                                      : (verificationOutcomeKind(modelData.verifyDetails) === "untrusted"
                                                         ? tr("Integridad válida; confianza no evaluada.")
                                                         : (verificationOutcomeKind(modelData.verifyDetails) === "incomplete"
                                                            ? tr("Verificación incompleta")
                                                            : (modelData.verifyReason || tr("La verificación ha devuelto incidencias."))))
                                                color: verificationOutcomeColor(modelData.verifyDetails)
                                                font.pixelSize: 12
                                                wrapMode: Text.WordWrap
                                            }

                                            Rectangle {
                                                visible: expanded && !!modelData.outputPath
                                                Layout.fillWidth: true
                                                implicitHeight: batchOutputColumn.implicitHeight + 18
                                                radius: 8
                                                color: "#223244"
                                                border.color: Qt.rgba(1, 1, 1, 0.08)
                                                border.width: 1

                                                ColumnLayout {
                                                    id: batchOutputColumn
                                                    anchors.fill: parent
                                                    anchors.margins: 10
                                                    spacing: 6

                                                    Text {
                                                        text: tr("Ruta de salida")
                                                        color: currentTheme.primaryColor
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                    Text {
                                                        text: modelData.outputPath || tr("No disponible")
                                                        color: "white"
                                                        opacity: 0.9
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.WrapAnywhere
                                                    }
                                                }
                                            }

                                            Rectangle {
                                                visible: expanded && !modelData.ok && !!modelData.error
                                                Layout.fillWidth: true
                                                implicitHeight: batchErrorDetailColumn.implicitHeight + 18
                                                radius: 8
                                                color: "#223244"
                                                border.color: Qt.rgba(1, 1, 1, 0.08)
                                                border.width: 1

                                                ColumnLayout {
                                                    id: batchErrorDetailColumn
                                                    anchors.fill: parent
                                                    anchors.margins: 10
                                                    spacing: 6

                                                    Text {
                                                        text: tr("Detalle del error")
                                                        color: "#e74c3c"
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                    Text {
                                                        text: localizeVisibleDiagnosticText(modelData.error || "")
                                                        color: "white"
                                                        opacity: 0.9
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                }
                                            }

                                            Rectangle {
                                                visible: expanded && modelData.ok && !!modelData.outputPath && modelData.verifyDone && !!modelData.verifyDetails
                                                Layout.fillWidth: true
                                                implicitHeight: batchVerifySummaryColumn.implicitHeight + 18
                                                radius: 8
                                                color: "#223244"
                                                border.color: Qt.rgba(1, 1, 1, 0.08)
                                                border.width: 1

                                                ColumnLayout {
                                                    id: batchVerifySummaryColumn
                                                    anchors.fill: parent
                                                    anchors.margins: 10
                                                    spacing: 6

                                                    Text {
                                                        text: tr("Resumen de verificación")
                                                        color: currentTheme.primaryColor
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                    Text {
                                                        text: tr("Formato: ") + (modelData.verifyDetails && modelData.verifyDetails.format ? modelData.verifyDetails.format : tr("No disponible"))
                                                        color: "white"
                                                        opacity: 0.95
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                    Text {
                                                        text: tr("Cobertura: ") + (modelData.verifyDetails && modelData.verifyDetails.coverage ? verificationCoverageText(modelData.verifyDetails.coverage) : tr("No disponible"))
                                                        color: "white"
                                                        opacity: 0.95
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                }
                                            }

                                            ColumnLayout {
                                                visible: expanded && modelData.ok && !!modelData.outputPath && modelData.verifyDone && !!modelData.verifyDetails
                                                Layout.fillWidth: true
                                                spacing: 8

                                                Repeater {
                                                    model: [
                                                        { label: tr("Integridad"), value: modelData.verifyDetails ? modelData.verifyDetails.integrity : null },
                                                        { label: tr("Certificado"), value: modelData.verifyDetails ? modelData.verifyDetails.certificate : null },
                                                        { label: tr("Confianza"), value: modelData.verifyDetails ? modelData.verifyDetails.trust : null }
                                                    ]

                                                    delegate: Rectangle {
                                                        Layout.fillWidth: true
                                                        implicitHeight: batchAspectColumn.implicitHeight + 16
                                                        radius: 8
                                                        color: "#223244"
                                                        border.color: Qt.rgba(1, 1, 1, 0.08)
                                                        border.width: 1
                                                        visible: modelData.value !== null && modelData.value !== undefined

                                                        Column {
                                                            id: batchAspectColumn
                                                            anchors.fill: parent
                                                            anchors.margins: 8
                                                            spacing: 4

                                                            Text {
                                                                text: modelData.label + ": " + verificationStatusText(modelData.value && modelData.value.status ? modelData.value.status : "")
                                                                color: verificationAspectColor(modelData.value && modelData.value.status ? modelData.value.status : "")
                                                                font.bold: true
                                                                font.pixelSize: 12
                                                                width: parent.width
                                                                wrapMode: Text.Wrap
                                                            }
                                                            Text {
                                                                text: tr("Razón: ") + ((modelData.value && modelData.value.reason) ? localizeVisibleDiagnosticText(modelData.value.reason) : tr("No disponible"))
                                                                color: "white"
                                                                opacity: 0.9
                                                                font.pixelSize: 12
                                                                width: parent.width
                                                                wrapMode: Text.Wrap
                                                            }
                                                        }
                                                    }
                                                }
                                            }

                                            ColumnLayout {
                                                visible: expanded && modelData.ok && !!modelData.outputPath && modelData.verifyDone && !!modelData.verifyDetails
                                                Layout.fillWidth: true
                                                spacing: 8

                                                Rectangle {
                                                    Layout.fillWidth: true
                                                    implicitHeight: batchWarningsColumn.implicitHeight + 16
                                                    radius: 8
                                                    color: "#223244"
                                                    border.color: Qt.rgba(1, 1, 1, 0.08)
                                                    border.width: 1

                                                    Column {
                                                        id: batchWarningsColumn
                                                        anchors.fill: parent
                                                        anchors.margins: 8
                                                        spacing: 4

                                                        Text {
                                                            text: tr("Advertencias")
                                                            color: "#f39c12"
                                                            font.bold: true
                                                            font.pixelSize: 12
                                                            width: parent.width
                                                            wrapMode: Text.Wrap
                                                        }
                                                        Text {
                                                            text: verificationArrayText(modelData.verifyDetails ? modelData.verifyDetails.warnings : [])
                                                            color: "white"
                                                            opacity: 0.9
                                                            font.pixelSize: 12
                                                            width: parent.width
                                                            wrapMode: Text.Wrap
                                                        }
                                                    }
                                                }

                                                Rectangle {
                                                    Layout.fillWidth: true
                                                    implicitHeight: batchErrorsColumn.implicitHeight + 16
                                                    radius: 8
                                                    color: "#223244"
                                                    border.color: Qt.rgba(1, 1, 1, 0.08)
                                                    border.width: 1

                                                    Column {
                                                        id: batchErrorsColumn
                                                        anchors.fill: parent
                                                        anchors.margins: 8
                                                        spacing: 4

                                                        Text {
                                                            text: tr("Errores")
                                                            color: "#e74c3c"
                                                            font.bold: true
                                                            font.pixelSize: 12
                                                            width: parent.width
                                                            wrapMode: Text.Wrap
                                                        }
                                                        Text {
                                                            text: verificationArrayText(modelData.verifyDetails ? modelData.verifyDetails.errors : [])
                                                            color: "white"
                                                            opacity: 0.9
                                                            font.pixelSize: 12
                                                            width: parent.width
                                                            wrapMode: Text.Wrap
                                                        }
                                                    }
                                                }
                                            }

                                            Rectangle {
                                                visible: expanded && modelData.ok && !!modelData.outputPath && modelData.verifyDone && !!modelData.verifyDetails
                                                Layout.fillWidth: true
                                                implicitHeight: batchSignerSummaryColumn.implicitHeight + 18
                                                radius: 8
                                                color: "#223244"
                                                border.color: Qt.rgba(1, 1, 1, 0.08)
                                                border.width: 1

                                                ColumnLayout {
                                                    id: batchSignerSummaryColumn
                                                    anchors.fill: parent
                                                    anchors.margins: 10
                                                    spacing: 6

                                                    Text {
                                                        text: tr("Firmantes")
                                                        color: currentTheme.primaryColor
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                    Text {
                                                        text: verificationSignerSummariesText(modelData.verifyDetails)
                                                        color: "white"
                                                        opacity: 0.9
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                }
                                            }

                                            Rectangle {
                                                visible: modelData.ok && !!modelData.outputPath && modelData.verifyDone && !!modelData.verifyDetails
                                                Layout.fillWidth: true
                                                implicitHeight: batchEvidenceColumn.implicitHeight + 18
                                                radius: 8
                                                color: "#223244"
                                                border.color: Qt.rgba(1, 1, 1, 0.08)
                                                border.width: 1

                                                ColumnLayout {
                                                    id: batchEvidenceColumn
                                                    anchors.fill: parent
                                                    anchors.margins: 10
                                                    spacing: 6

                                                    Text {
                                                        text: tr("Evidencias")
                                                        color: currentTheme.primaryColor
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                    Text {
                                                        text: verificationEvidenceText(modelData.verifyDetails)
                                                        color: "white"
                                                        opacity: 0.9
                                                        font.pixelSize: 12
                                                        Layout.fillWidth: true
                                                        wrapMode: Text.Wrap
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                    }
                    }

                    // Panel certificados
                    Rectangle {
                        Layout.fillHeight: true
                        id: rightSidebar
                        visible: !window.signCertificatePanelCollapsed
                        Layout.preferredWidth: window.rightSidebarCollapsed ? window.rightSidebarCollapsedWidth : window.rightSidebarExpandedWidth
                        Layout.minimumWidth: window.rightSidebarCollapsed ? window.rightSidebarCollapsedWidth : window.rightSidebarExpandedWidth
                        Layout.maximumWidth: window.rightSidebarCollapsed ? window.rightSidebarCollapsedWidth : window.rightSidebarExpandedWidth
                        width: window.rightSidebarCollapsed ? window.rightSidebarCollapsedWidth : window.rightSidebarExpandedWidth
                        radius: 15
                        color: currentTheme.sidebarColor
                        Behavior on width {
                            NumberAnimation { duration: 180; easing.type: Easing.InOutQuad }
                        }
                        ColumnLayout {
                            anchors.fill: parent
                            anchors.margins: window.rightSidebarCollapsed ? 10 : 20
                            spacing: window.rightSidebarCollapsed ? 10 : 15
                            
                            AdaptiveRow {
                                Layout.fillWidth: true
                                ThemedButton {
                                    visible: !window.rightSidebarCollapsed
                                    text: tr("+ CERTIFICADOS")
                                    Layout.fillWidth: true
                                    onClicked: window.openGuidedCertificateAccess()
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Importar certificado")
                                }
                                ToolButton {
                                    visible: !window.rightSidebarCollapsed
                                    text: "↻"
                                    onClicked: backend.checkCertificates()
                                    Accessible.name: tr("Actualizar certificados")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Actualizar certificados")
                                }
                                ToolButton {
                                    visible: !window.rightSidebarCollapsed
                                    text: tr("Firefox/NSS")
                                    onClicked: {
                                        backend.backendLogReceived(tr("Buscando certificados en almacenes NSS/Firefox..."))
                                        backend.checkCertificates()
                                    }
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Buscar certificados en Firefox/NSS")
                                }
                                ToolButton {
                                    text: tr("Ocultar")
                                    onClicked: {
                                        window.signCertificatePanelCollapsed = true
                                        Qt.callLater(function() {
                                            if (window.activeTab === "firmar" && changeCertificateButton.visible)
                                                changeCertificateButton.forceActiveFocus()
                                        })
                                    }
                                    Accessible.name: tr("Ocultar")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Ocultar")
                                }
                            }

                            ThemedButton {
                                visible: (window.cscAllowed || window.cscProhibited) && !window.rightSidebarCollapsed
                                Layout.fillWidth: true
                                text: tr("csc.gui.titulo")
                                Accessible.name: tr("csc.gui.titulo")
                                Accessible.description: tr("csc.gui.descripcion")
                                onClicked: window.openRemoteSigningDialog()
                            }

                            ColumnLayout {
                                visible: window.rightSidebarCollapsed
                                Layout.fillWidth: true
                                Layout.fillHeight: true
                                spacing: 12

                                Text {
                                    Layout.fillWidth: true
                                    text: window.certificates.length > 0
                                          ? tr("%1 cert.").arg(window.certificates.length)
                                          : tr("Sin cert.")
                                    color: currentTheme.textColor
                                    font.pixelSize: 11
                                    horizontalAlignment: Text.AlignHCenter
                                    wrapMode: Text.WordWrap
                                }

                                ScrollView {
                                    Layout.fillWidth: true
                                    Layout.fillHeight: true
                                    clip: true
                                    ScrollBar.horizontal.policy: ScrollBar.AlwaysOff

                                    Column {
                                        width: parent.width
                                        spacing: 8

                                        Repeater {
                                            model: window.filteredCertificates()
                                            delegate: Button {
                                                required property int index
                                                required property var modelData

                                                width: parent.width
                                                height: 36
                                                text: window.compactCertificateBadge(modelData)

                                                background: Rectangle {
                                                    radius: 10
                                                    color: window.certificateId(window.selectedCertData) === window.certificateId(modelData)
                                                           ? currentTheme.primaryColor
                                                           : Qt.rgba(1, 1, 1, 0.08)
                                                    border.color: Qt.rgba(1, 1, 1, 0.18)
                                                    border.width: 1
                                                }

                                                ToolTip.visible: hovered
                                                ToolTip.delay: 300
                                                ToolTip.text: window.compactCertificateTooltip(modelData)

                                                onClicked: {
                                                    const realIndex = window.findCertificateIndexById(window.certificateId(modelData), window.certificates)
                                                    window.selectCertificateIndex(realIndex, true)
                                                }
                                            }
                                        }
                                    }
                                }

                                ToolButton {
                                    Layout.alignment: Qt.AlignHCenter
                                    text: "+"
                                    onClicked: window.openGuidedCertificateAccess()
                                    Accessible.name: tr("Importar certificado")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Importar certificado")
                                }
                                ToolButton {
                                    Layout.alignment: Qt.AlignHCenter
                                    text: "↻"
                                    onClicked: backend.checkCertificates()
                                    Accessible.name: tr("Actualizar certificados")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Actualizar certificados")
                                }
                                ToolButton {
                                    Layout.alignment: Qt.AlignHCenter
                                    text: "Fx"
                                    onClicked: {
                                        backend.backendLogReceived(tr("Buscando certificados en almacenes NSS/Firefox..."))
                                        backend.checkCertificates()
                                    }
                                    Accessible.name: tr("Buscar certificados en Firefox/NSS")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Buscar certificados en Firefox/NSS")
                                }
                            }

                            ColumnLayout {
                                visible: !window.rightSidebarCollapsed
                                Layout.fillWidth: true
                                spacing: 6

                                ThemedTextField {
                                    id: certificateFilterField
                                    Layout.fillWidth: true
                                    text: certificateFilterText
                                    placeholderText: tr("Buscar certificado")
                                    onTextChanged: certificateFilterText = text
                                }
                                Text {
                                    Layout.fillWidth: true
                                    text: tr("Mostrando %1 de %2").arg(window.filteredCertificates().length).arg(window.certificates.length)
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 11
                                    wrapMode: Text.WordWrap
                                }
                                ThemedButton {
                                    Layout.fillWidth: true
                                    text: tr("Usar DNIe o tarjeta")
                                    enabled: isIpcMode
                                    onClicked: backend.requestSmartcardStatus()
                                    Accessible.description: tr("Consulta el lector y actualiza los certificados disponibles.")
                                }
                                Text {
                                    Layout.fillWidth: true
                                    visible: window.smartcardMessage !== ""
                                    text: window.smartcardMessage
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                    font.pixelSize: 11
                                }
                            }

                            Loader {
                                visible: !window.rightSidebarCollapsed
                                         && window.filteredCertificates().length === 0
                                         && window.showNoCertificateHelp
                                Layout.fillWidth: true
                                Layout.fillHeight: true
                                sourceComponent: noCertificatesPanelComponent
                            }

                            ListView {
                                visible: !window.rightSidebarCollapsed
                                         && (window.filteredCertificates().length > 0
                                             || !window.showNoCertificateHelp)
                                Layout.fillWidth: true
                                Layout.fillHeight: true
                                model: window.filteredCertificates()
                                spacing: 8
                                clip: true
                                delegate: Rectangle {
                                    width: ListView.view.width
                                    height: Math.max(92, cardContents.implicitHeight + 24)
                                    radius: 12
                                    activeFocusOnTab: true
                                    property bool emphasized: activeFocus
                                        || window.certificateId(window.selectedCertData) === window.certificateId(modelData)
                                    Accessible.role: Accessible.Button
                                    Accessible.name: (modelData.subjectName || modelData.subject || tr("Certificado")) + ", " +
                                                     window.certificateStatusText(modelData) + ". " + window.certificateStatusReason(modelData)
                                    Accessible.description: tr("Emisor: %1. Vence: %2").arg(modelData.issuerName || modelData.issuer || tr("Desconocido")).arg(window.certificateExpiry(modelData))
                                    Accessible.onPressAction: {
                                        const realIndex = window.findCertificateIndexById(window.certificateId(modelData), window.certificates)
                                        window.selectCertificateIndex(realIndex, true)
                                    }
                                    color: emphasized
                                        ? window.certificateSelectionColor() : currentTheme.cardColor
                                    border.color: certificateId(window.selectedCertData) === certificateId(modelData)
                                        ? currentTheme.primaryColor : window.certificateStatusColor(modelData)
                                    border.width: activeFocus || certificateId(window.selectedCertData) === certificateId(modelData) ? 2 : 1

                                    Rectangle {
                                        anchors.left: parent.left
                                        anchors.right: parent.right
                                        anchors.bottom: parent.bottom
                                        anchors.margins: 8
                                        height: 1
                                        color: window.certificateDividerColor()
                                    }

                                    Keys.onPressed: function(event) {
                                        if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter || event.key === Qt.Key_Space) {
                                            const realIndex = window.findCertificateIndexById(window.certificateId(modelData), window.certificates)
                                            window.selectCertificateIndex(realIndex, true)
                                            event.accepted = true
                                        }
                                    }
                                    
                                    ColumnLayout {
                                        id: cardContents
                                        anchors.fill: parent
                                        anchors.margins: 12
                                        spacing: 2
                                        RowLayout {
                                            Layout.fillWidth: true
                                            Text {
                                                wrapMode: Text.WordWrap
                                                Layout.preferredWidth: 1
                                                text: modelData.subjectName || modelData.subject || tr("Certificado")
                                                color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : currentTheme.textColor
                                                font.bold: true
                                                Layout.fillWidth: true
                                                elide: Text.ElideRight
                                            }
                                            Text {
                                                visible: window.isDefaultCertificate(modelData)
                                                text: tr("Predeterminado")
                                                color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : "#f1c40f"
                                                font.pixelSize: 10
                                                font.bold: true
                                            }
                                            Text {
                                                visible: window.isTemporaryCertificate(modelData)
                                                text: tr("Solo esta sesión")
                                                color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : "#f39c12"
                                                font.pixelSize: 10
                                                font.bold: true
                                            }
                                            Text {
                                                text: window.certificateStatusText(modelData)
                                                color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : window.certificateStatusColor(modelData)
                                                font.pixelSize: 10
                                                font.bold: true
                                            }
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            visible: window.certificateStatusReason(modelData) !== ""
                                            text: window.certificateStatusReason(modelData)
                                            color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : currentTheme.textColor
                                            font.pixelSize: 12
                                            wrapMode: Text.WordWrap
                                            Accessible.name: text
                                        }
                                        Text {
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                            text: tr("Vence: %1").arg(window.certificateExpiry(modelData))
                                            color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : currentTheme.textColor
                                            font.pixelSize: 11
                                        }
                                        Text {
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                            text: tr("Emisor: %1").arg(modelData.issuerName || modelData.issuer || tr("Desconocido"))
                                            color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : currentTheme.textColor
                                            font.pixelSize: 10
                                            elide: Text.ElideRight
                                        }
                                    }
                                    MouseArea { 
                                        anchors.fill: parent;
                                        cursorShape: Qt.PointingHandCursor
                                        onClicked: { 
                                            const realIndex = window.findCertificateIndexById(window.certificateId(modelData), window.certificates)
                                            window.selectCertificateIndex(realIndex, true)
                                            console.log(tr("QML: Certificado seleccionado:"), (modelData.subjectName || modelData.subject || tr("Certificado")), tr("ID:"), modelData.id)
                                        } 
                                    }
                                }
                            }

                            // Subpanel con detalles del certificado seleccionado
                            Rectangle {
                                Layout.fillWidth: true
                                Layout.preferredHeight: 270
                                visible: !window.rightSidebarCollapsed && selectedCertIndex !== -1 && window.selectedCertData
                                color: currentTheme.cardColor
                                radius: 10
                                border.color: currentTheme.primaryColor
                                border.width: 1

                                ColumnLayout {
                                    anchors.fill: parent
                                    anchors.margins: 10
                                    spacing: 5
                                    
                                    Text {
                                        text: tr("Detalles del certificado")
                                        font.bold: true
                                        color: currentTheme.textColor
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        visible: !!window.selectedCertData && window.selectedCertData.canSign
                                                 && !window.selectedCertData.caducado
                                                 && !!window.selectedCertData.validTo
                                                 && Number(window.selectedCertData.diasCaducidad) >= 0
                                                 && Number(window.selectedCertData.diasCaducidad) <= 60
                                        text: tr("Este certificado caduca en %1 días. Comprueba si debes renovarlo antes de firmar.").arg(window.selectedCertData ? window.selectedCertData.diasCaducidad : 0)
                                        color: "#f2c66d"
                                        wrapMode: Text.WordWrap
                                        font.pixelSize: 11
                                    }
                                    ThemedButton {
                                        visible: window.canRenewFnmt(window.selectedCertData)
                                        text: tr("Renovar en la FNMT")
                                        onClicked: backend.openExternal("https://www.sede.fnmt.gob.es/certificados/persona-fisica/renovar")
                                        Accessible.description: tr("Abre la página oficial de renovación de certificado de persona física de la FNMT.")
                                    }

                                    RowLayout {
                                        Layout.fillWidth: true
                                        spacing: 8

                                        Text {
                                            visible: window.isDefaultCertificate(window.selectedCertData)
                                            text: tr("Predeterminado")
                                            color: "#f1c40f"
                                            font.bold: true
                                            font.pixelSize: 11
                                        }
                                        Item { Layout.fillWidth: true }
                                        ThemedButton {
                                            visible: window.isTemporaryCertificate(window.selectedCertData)
                                            text: tr("Retirar temporal")
                                            onClicked: {
                                                const id = window.certificateId(window.selectedCertData)
                                                window.pendingTemporaryRemovalId = id
                                                backend.removeTemporaryCertificate(id)
                                            }
                                        }
                                        ThemedButton {
                                            text: window.isDefaultCertificate(window.selectedCertData)
                                                  ? tr("Quitar predeterminado")
                                                  : tr("Usar como predeterminado")
                                            onClicked: {
                                                if (window.isDefaultCertificate(window.selectedCertData)) {
                                                    window.clearDefaultCertificate()
                                                } else {
                                                    window.setSelectedCertificateAsDefault()
                                                }
                                            }
                                        }
                                    }

                                    ScrollView {
                                        Layout.fillWidth: true
                                        Layout.fillHeight: true
                                        clip: true

                                        Column {
                                            width: parent.width
                                            spacing: 6
                                            
                                            Text { 
                                                textFormat: Text.PlainText
                                                text: tr("Titular: ") + (window.selectedCertData ? (window.selectedCertData.subjectName || window.selectedCertData.subject || "---") : "")
                                                color: currentTheme.textColor
                                                font.pixelSize: 11
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text { 
                                                textFormat: Text.PlainText
                                                text: tr("Emisor: ") + (window.selectedCertData ? (window.selectedCertData.issuerName || window.selectedCertData.issuer || "---") : "")
                                                color: currentTheme.textColor
                                                opacity: 0.8
                                                font.pixelSize: 11
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text { 
                                                textFormat: Text.PlainText
                                                text: tr("Nº Serie: ") + (window.selectedCertData ? (window.selectedCertData.serialNumber || window.selectedCertData.nif || "") : "")
                                                color: currentTheme.textColor
                                                opacity: 0.8
                                                font.pixelSize: 10
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text { 
                                                textFormat: Text.PlainText
                                                text: tr("Válido hasta: ") + (window.selectedCertData ? (window.selectedCertData.validTo || window.selectedCertData.notAfter || "") : "")
                                                color: currentTheme.textColor
                                                opacity: 0.8
                                                font.pixelSize: 10
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Tipo: ") + (window.selectedCertData ? (window.selectedCertData.tipo || tr("Desconocido")) : "")
                                                color: currentTheme.textColor
                                                font.pixelSize: 10
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Organización: ") + (window.selectedCertData ? (window.selectedCertData.organizacion || tr("No indicada")) : "")
                                                color: currentTheme.textColor
                                                font.pixelSize: 10
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Estado: ") + window.certificateStatusText(window.selectedCertData)
                                                color: currentTheme.textColor
                                                font.pixelSize: 10
                                                width: parent.width
                                            }
                                            Text { 
                                                textFormat: Text.PlainText
                                                text: tr("Huella: ") + (window.selectedCertData ? formatFingerprintForDisplay(window.selectedCertData.fingerprint || "") : "")
                                                color: currentTheme.textColor
                                                opacity: 0.6
                                                font.pixelSize: 9
                                                wrapMode: Text.WrapAnywhere
                                                width: parent.width
                                            }
                                        }
                                    }

                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("Verificar certificado")
                                        palette.button: "#16a085"
                                        palette.buttonText: "white"
                                        onClicked: window.openCertificateValidation(window.selectedCertData, false)
                                    }
                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("Exportar certificado público…")
                                        enabled: isIpcMode && !!window.selectedCertData
                                        onClicked: window.startPublicCertificateExport(false)
                                    }
                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("Refrescar validez online")
                                        enabled: !!window.selectedCertData
                                        onClicked: window.openCertificateValidation(window.selectedCertData, true)
                                    }
                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("Abrir VALIDE")
                                        flat: true
                                        onClicked: backend.openExternal("https://valide.redsara.es/valide/")
                                    }
                                }
                            }
                        }
                    }
                }
            }

            // TAB: VERIFICAR
            Item {
                id: verifyTab
                property string verifyFilePath: ""
                property string verifyOriginalPath: ""
                property var verifyDetails: null
                property bool verifyDetailsAllExpanded: false
                property string hashInputPath: ""
                property bool hashInputIsDirectory: false
                property string hashReferencePath: ""
                property string hashAlgorithm: window.defaultHashAlgorithm
                property string hashFormatFile: window.defaultHashFormatFile
                property string hashFormatDirectory: window.defaultHashFormatDirectory
                property bool hashRecursive: window.defaultHashRecursive
                property bool hashSaveReport: window.defaultHashSaveReport
                property var hashCreateResult: null
                property var hashCheckResult: null
                // Subpaneles de Verificar con colores del tema: fondo teñido con el color
                // principal y textos, títulos y estados ajustados a contraste AA (4,5:1).
                readonly property color subPanelColor: Qt.tint(currentTheme.sidebarColor, Qt.alpha(currentTheme.primaryColor, 0.12))
                readonly property color subPanelBorder: Qt.alpha(currentTheme.primaryColor, 0.45)
                readonly property color subPanelText: Contrast.readableOn(subPanelColor, currentTheme.textColor)
                readonly property color subPanelTitle: Contrast.accentOn(subPanelColor, currentTheme.primaryColor)
                readonly property color subPanelWarning: Contrast.accentOn(subPanelColor, "#f39c12")
                readonly property color subPanelError: Contrast.accentOn(subPanelColor, currentTheme.errorColor)

                function collectExpandablePanels(root, out) {
                    if (!root)
                        return
                    if (typeof root.expanded !== "undefined" && root.visible !== false)
                        out.push(root)
                    if (!root.children)
                        return
                    for (let i = 0; i < root.children.length; i++)
                        collectExpandablePanels(root.children[i], out)
                }

                function refreshVerifyDetailsExpandState() {
                    if (!verifyDetailsContentColumn) {
                        verifyDetailsAllExpanded = false
                        return
                    }
                    const panels = []
                    collectExpandablePanels(verifyDetailsContentColumn, panels)
                    let anyPanel = panels.length > 0
                    let allExpanded = true
                    for (let i = 0; i < panels.length; i++) {
                        if (!panels[i].expanded)
                            allExpanded = false
                    }
                    verifyDetailsAllExpanded = anyPanel && allExpanded
                }

                function setVerifyDetailsPanelsExpanded(expanded) {
                    if (!verifyDetailsContentColumn) {
                        verifyDetailsAllExpanded = expanded
                        return
                    }
                    const panels = []
                    collectExpandablePanels(verifyDetailsContentColumn, panels)
                    for (let i = 0; i < panels.length; i++) {
                        panels[i].expanded = expanded
                    }
                    verifyDetailsAllExpanded = expanded
                }

                function toggleVerifyDetailsPanels() {
                    setVerifyDetailsPanelsExpanded(!verifyDetailsAllExpanded)
                }

                function toggleVerifyDetailsPanel(panel) {
                    if (!panel || typeof panel.expanded === "undefined")
                        return
                    panel.expanded = !panel.expanded
                    refreshVerifyDetailsExpandState()
                }

                Connections {
                    target: backend
                    function onVerificationFinished(success, message, details) {
                        if (activeTab === "verificar") {
                            verifyTab.verifyDetails = success ? details : { valid: false, reason: message }
                            verifyTab.setVerifyDetailsPanelsExpanded(false)
                        }
                    }
                    function onHashCreateFinished(success, message, result) {
                        window.hashPendingCount = Math.max(0, window.hashPendingCount - 1)
                        if (activeTab === "verificar") {
                            verifyTab.hashCreateResult = success ? result : { ok: false, error: message }
                            if (success && window.defaultHashCopyToClipboard && result && result.hash) {
                                window.copyTextToClipboard(result.hash)
                                window.statusMessage = tr("Huella copiada al portapapeles.")
                            }
                        }
                        window.clearOperationFailure()
                        if (!success) {
                            const incidentPath = window.persistIncidentReport("hash-create-failure", message, {
                                source: "hashCreateFinished"
                            })
                            window.recordOperationFailure("hash-create-failure", message, incidentPath)
                        }
                    }
                    function onHashCheckFinished(success, message, result) {
                        window.hashPendingCount = Math.max(0, window.hashPendingCount - 1)
                        if (activeTab === "verificar") {
                            verifyTab.hashCheckResult = success ? result : { ok: false, valid: false, error: message }
                        }
                        window.clearOperationFailure()
                        if (!success) {
                            const incidentPath = window.persistIncidentReport("hash-check-failure", message, {
                                source: "hashCheckFinished"
                            })
                            window.recordOperationFailure("hash-check-failure", message, incidentPath)
                        }
                    }
                }

                RowLayout {
                    anchors.fill: parent
                    anchors.margins: window.narrowWindow ? 16 : 40
                    spacing: 40

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        radius: 16
                        color: currentTheme.cardColor
                        border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                        border.width: 1

                        ScrollView {
                            id: verifyMainOuterScroll
                            anchors.fill: parent
                            leftPadding: 16
                            rightPadding: 26
                            topPadding: 16
                            bottomPadding: 26
                            // Solo desplazamiento vertical: los paneles se ajustan al ancho.
                            contentWidth: availableWidth
                            contentHeight: verifyMainOuterContent.implicitHeight
                            clip: true
                            ScrollBar.vertical.policy: ScrollBar.AsNeeded
                            ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
                            ScrollBar.vertical.width: 18

                            ColumnLayout {
                                id: verifyMainOuterContent
                                width: verifyMainOuterScroll.availableWidth
                                spacing: 20

                                Text {
                                    text: tr("Verificación de Firma")
                                    font.pixelSize: 32
                                    font.bold: true
                                    color: currentTheme.textColor
                                    Layout.fillWidth: true
                                    wrapMode: Text.Wrap
                                }

                        Rectangle {
                            id: verifyInputPanel
                            Layout.fillWidth: true
                            Layout.preferredHeight: Math.max(300, verifyInputColumn.implicitHeight + 40)
                            radius: 15
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor
                            border.width: verifyDrop.containsDrag ? 3 : 1
                            
                            DropArea {
                                id: verifyDrop
                                anchors.fill: parent
                                property bool containsDrag: false
                                onEntered: containsDrag = true
                                onExited: containsDrag = false
                                onDropped: (drop) => {
                                    containsDrag = false
                                    if (drop.hasUrls) {
                                        verifyTab.verifyFilePath = window.localPathFromUrl(drop.urls[0].toString())
                                    }
                                }
                            }

                            ColumnLayout {
                                id: verifyInputColumn
                                anchors.centerIn: parent
                                width: Math.min(parent.width - 40, 640)
                                spacing: 15
                                Text {
                                    text: verifyTab.verifyFilePath === "" ? tr("Arrastra un archivo firmado para verificar") : verifyTab.verifyFilePath.split('/').pop()
                                    color: currentTheme.textColor
                                    font.pixelSize: 18
                                    Layout.alignment: Qt.AlignHCenter
                                    Layout.fillWidth: true
                                    horizontalAlignment: Text.AlignHCenter
                                    wrapMode: Text.Wrap
                                }
                                Text {
                                    text: verifyTab.verifyOriginalPath === "" ? tr("Original opcional no seleccionado") : (tr("Original: ") + verifyTab.verifyOriginalPath.split('/').pop())
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 13
                                    Layout.alignment: Qt.AlignHCenter
                                    Layout.fillWidth: true
                                    horizontalAlignment: Text.AlignHCenter
                                    wrapMode: Text.Wrap
                                }
                                AdaptiveRow {
                                    centered: true

                                    ThemedButton {
                                        text: tr("Seleccionar fichero...")
                                        onClicked: verifyFileDialog.open()
                                    }
                                    ThemedButton {
                                        text: tr("✅ Validar Documento")
                                        visible: verifyTab.verifyFilePath !== ""
                                        font.bold: true
                                        palette.button: currentTheme.primaryColor
                                        palette.buttonText: "#ffffff"
                                        onClicked: {
                                            console.log("QML: Validar documento", verifyTab.verifyFilePath, verifyTab.verifyOriginalPath)
                                            window.requestVerification(verifyTab.verifyFilePath, verifyTab.verifyOriginalPath)
                                        }
                                    }
                                }

                                AdaptiveRow {
                                    centered: true

                                    ThemedButton {
                                        text: tr("Seleccionar original...")
                                        onClicked: verifyOriginalFileDialog.open()
                                    }
                                    ThemedButton {
                                        text: tr("Quitar original")
                                        visible: verifyTab.verifyOriginalPath !== ""
                                        onClicked: {
                                            verifyTab.verifyOriginalPath = ""
                                            verifyTab.verifyDetails = null
                                        }
                                    }
                                }
                                AdaptiveRow {
                                    centered: true

                                    ThemedButton {
                                        text: tr("Asistente")
                                        visible: verifyTab.verifyFilePath !== "" || verifyTab.verifyDetails !== null
                                        onClicked: {
                                            const goal = verificationOutcomeKind(verifyTab.verifyDetails) === "invalid"
                                                         ? "verify-failure"
                                                         : "verify"
                                            window.openSupportAssistant(goal)
                                        }
                                    }
                                    ThemedButton {
                                        text: tr("Verificar certificado")
                                        visible: window.selectedCertData !== null
                                        onClicked: window.openCurrentCertificateValidation()
                                    }
                                }
                                AdaptiveRow {
                                    centered: true
                                    visible: verifyTab.verifyDetails !== null

                                    ThemedButton {
                                        text: tr("winui.parity.verify.export_html")
                                        visible: window.verificationHasHtmlReport(verifyTab.verifyDetails)
                                        font.bold: true
                                        onClicked: window.openSaveDialog(verifyHtmlReportSaveDialog, verifyTab.verifyFilePath,
                                                                         window.suggestVerificationHtmlReportName(verifyTab.verifyFilePath))
                                    }
                                    ThemedButton {
                                        text: tr("Copiar resumen")
                                        onClicked: {
                                            window.copyTextToClipboard(window.buildVerificationUserSummary(
                                                verifyTab.verifyDetails || {},
                                                verifyTab.verifyFilePath,
                                                verifyTab.verifyOriginalPath))
                                            window.statusMessage = tr("Resumen de validación copiado al portapapeles.")
                                        }
                                    }
                                    ThemedButton {
                                        text: tr("Guardar resumen")
                                        onClicked: window.openSaveDialog(verifySummarySaveDialog, verifyTab.verifyFilePath,
                                                                         window.suggestVerificationSummaryName(verifyTab.verifyFilePath))
                                    }
                                    ThemedButton {
                                        text: tr("Guardar informe JSON")
                                        onClicked: window.openSaveDialog(verifyReportSaveDialog, verifyTab.verifyFilePath,
                                                                         window.suggestVerificationReportName(verifyTab.verifyFilePath))
                                    }
                                }
                            }
                        }

                        Rectangle {
                            id: verifyHashPanel
                            property bool expanded: false
                            Layout.fillWidth: true
                            radius: 15
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor
                            border.width: 1
                            implicitHeight: verifyHashPanelColumn.implicitHeight + 40

                            ColumnLayout {
                                id: verifyHashPanelColumn
                                anchors.fill: parent
                                anchors.margins: 20
                                spacing: 12

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 8

                                    Text {
                                        Layout.preferredWidth: 1
                                        Layout.fillWidth: true
                                        text: tr("Huellas e integridad")
                                        font.pixelSize: 22
                                        font.bold: true
                                        color: currentTheme.textColor
                                        wrapMode: Text.Wrap
                                    }
                                    ToolButton {
                                        text: verifyHashPanel.expanded ? "▼" : "▶"
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 250
                                        ToolTip.text: verifyHashPanel.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                        onClicked: verifyHashPanel.expanded = !verifyHashPanel.expanded
                                    }
                                }

                                Text {
                                    text: tr("Crea o comprueba huellas de ficheros y manifiestos de directorio.")
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 13
                                    wrapMode: Text.WordWrap
                                    Layout.fillWidth: true
                                    visible: verifyHashPanel.expanded
                                }

                                Text {
                                    visible: !verifyHashPanel.expanded
                                    text: verifyTab.hashInputPath !== ""
                                          ? tr("Entrada preparada: %1").arg(verifyTab.hashInputIsDirectory ? tr("Directorio") : tr("Fichero"))
                                          : tr("Sin entrada seleccionada")
                                    color: currentTheme.textColor
                                    opacity: 0.9
                                    font.pixelSize: 12
                                    wrapMode: Text.WordWrap
                                    Layout.fillWidth: true
                                }

                                AdaptiveRow {
                                    visible: verifyHashPanel.expanded
                                    spacing: 10

                                    ThemedButton {
                                        text: tr("Seleccionar fichero")
                                        onClicked: hashInputFileDialog.open()
                                    }
                                    ThemedButton {
                                        text: tr("Seleccionar carpeta")
                                        onClicked: hashInputDirectoryDialog.open()
                                    }
                                    ThemedButton {
                                        text: tr("Seleccionar huella")
                                        onClicked: hashReferenceDialog.open()
                                    }
                                }

                                Rectangle {
                                    visible: verifyHashPanel.expanded
                                    Layout.fillWidth: true
                                    color: verifyTab.subPanelColor
                                    border.color: verifyTab.subPanelBorder
                                    border.width: 1
                                    radius: 8
                                    implicitHeight: hashSelectionColumn.implicitHeight + 18

                                    Column {
                                        id: hashSelectionColumn
                                        anchors.fill: parent
                                        anchors.margins: 9
                                        spacing: 5

                                        Text {
                                            text: tr("Entrada: ") + (verifyTab.hashInputPath !== "" ? verifyTab.hashInputPath : tr("No seleccionada"))
                                            color: verifyTab.subPanelText
                                            width: parent.width
                                            wrapMode: Text.WrapAnywhere
                                        }
                                        Text {
                                            text: tr("Tipo: ") + (verifyTab.hashInputIsDirectory ? tr("Directorio") : tr("Fichero"))
                                            color: verifyTab.subPanelText
                                            opacity: 0.85
                                            visible: verifyTab.hashInputPath !== ""
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        Text {
                                            text: tr("Huella o manifiesto: ") + (verifyTab.hashReferencePath !== "" ? verifyTab.hashReferencePath : tr("No seleccionado"))
                                            color: verifyTab.subPanelText
                                            opacity: 0.85
                                            width: parent.width
                                            wrapMode: Text.WrapAnywhere
                                        }
                                    }
                                }

                                GridLayout {
                                    visible: verifyHashPanel.expanded
                                    Layout.fillWidth: true
                                    columns: verifyMainOuterScroll.availableWidth < 560 ? 1 : 2
                                    columnSpacing: 12
                                    rowSpacing: 12

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 6

                                        Text {
                                            text: tr("Algoritmo")
                                            color: currentTheme.secondaryTextColor
                                            font.pixelSize: 12
                                        }
                                        ComboBox {
                                            Layout.fillWidth: true
                                            model: hashAlgorithmOptions()
                                            textRole: "texto"
                                            currentIndex: optionIndexByValue(model, verifyTab.hashAlgorithm)
                                            onActivated: function(index) { verifyTab.hashAlgorithm = model[index].valor }
                                        }
                                    }

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 6

                                        Text {
                                            text: tr("Formato")
                                            color: currentTheme.secondaryTextColor
                                            font.pixelSize: 12
                                        }
                                        ComboBox {
                                            Layout.fillWidth: true
                                            model: verifyTab.hashInputIsDirectory ? ["xml", "txt", "csv"] : ["hex", "base64", "bin"]
                                            currentIndex: {
                                                const value = verifyTab.hashInputIsDirectory ? verifyTab.hashFormatDirectory : verifyTab.hashFormatFile
                                                const idx = model.indexOf(value)
                                                return idx >= 0 ? idx : 0
                                            }
                                            onActivated: {
                                                if (verifyTab.hashInputIsDirectory) verifyTab.hashFormatDirectory = model[index]
                                                else verifyTab.hashFormatFile = model[index]
                                            }
                                        }
                                    }
                                }

                                AdaptiveRow {
                                    visible: verifyHashPanel.expanded
                                    spacing: 12

                                    ThemedCheckBox {
                                        text: tr("Recursivo")
                                        checked: verifyTab.hashRecursive
                                        onToggled: verifyTab.hashRecursive = checked
                                    }

                                    ThemedCheckBox {
                                        text: tr("Guardar informe de directorio")
                                        checked: verifyTab.hashSaveReport
                                        visible: verifyTab.hashInputIsDirectory
                                        onToggled: verifyTab.hashSaveReport = checked
                                    }
                                }

                                AdaptiveRow {
                                    visible: verifyHashPanel.expanded
                                    spacing: 10

                                    ThemedButton {
                                        text: tr("Crear huella")
                                        enabled: verifyTab.hashInputPath !== ""
                                        onClicked: {
                                            window.hashPendingCount += 1
                                            backend.createHash(
                                            verifyTab.hashInputPath,
                                            "",
                                            verifyTab.hashAlgorithm,
                                            verifyTab.hashInputIsDirectory ? verifyTab.hashFormatDirectory : verifyTab.hashFormatFile,
                                            verifyTab.hashInputIsDirectory ? verifyTab.hashRecursive : false)
                                        }
                                    }
                                    ThemedButton {
                                        text: tr("Comprobar huella")
                                        enabled: verifyTab.hashInputPath !== "" && verifyTab.hashReferencePath !== ""
                                        onClicked: {
                                            window.hashPendingCount += 1
                                            backend.checkHash(
                                            verifyTab.hashInputPath,
                                            verifyTab.hashReferencePath,
                                            "",
                                            verifyTab.hashAlgorithm,
                                            verifyTab.hashInputIsDirectory ? verifyTab.hashRecursive : false,
                                            verifyTab.hashInputIsDirectory ? verifyTab.hashSaveReport : false)
                                        }
                                    }
                                }

                                Rectangle {
                                    visible: verifyHashPanel.expanded && (verifyTab.hashCreateResult !== null || verifyTab.hashCheckResult !== null)
                                    Layout.fillWidth: true
                                    color: verifyTab.subPanelColor
                                    border.color: verifyTab.subPanelBorder
                                    border.width: 1
                                    radius: 8
                                    implicitHeight: hashResultColumn.implicitHeight + 18

                                    Column {
                                        id: hashResultColumn
                                        anchors.fill: parent
                                        anchors.margins: 9
                                        spacing: 6

                                        Text {
                                            text: tr("Resultado de huella")
                                            color: verifyTab.subPanelTitle
                                            font.bold: true
                                            font.pixelSize: 12
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        Text {
                                            text: verifyTab.hashCreateResult && verifyTab.hashCreateResult.hash ? (tr("Huella: ") + verifyTab.hashCreateResult.hash) : ""
                                            visible: text !== ""
                                            color: verifyTab.subPanelText
                                            width: parent.width
                                            wrapMode: Text.WrapAnywhere
                                        }
                                        Text {
                                            text: verifyTab.hashCreateResult && verifyTab.hashCreateResult.outputPath ? (tr("Salida: ") + verifyTab.hashCreateResult.outputPath) : ""
                                            visible: text !== ""
                                            color: verifyTab.subPanelText
                                            opacity: 0.88
                                            width: parent.width
                                            wrapMode: Text.WrapAnywhere
                                        }
                                        Text {
                                            text: verifyTab.hashCheckResult && verifyTab.hashCheckResult.valid !== undefined
                                                  ? (tr("Estado: ") + (verifyTab.hashCheckResult.valid ? tr("✅ VÁLIDA") : tr("❌ NO VÁLIDA")))
                                                  : ""
                                            visible: text !== ""
                                            color: verifyTab.subPanelText
                                            font.bold: true
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        Text {
                                            text: verifyTab.hashCheckResult && verifyTab.hashCheckResult.expectedHash ? (tr("Esperada: ") + verifyTab.hashCheckResult.expectedHash) : ""
                                            visible: text !== ""
                                            color: verifyTab.subPanelText
                                            opacity: 0.9
                                            width: parent.width
                                            wrapMode: Text.WrapAnywhere
                                        }
                                        Text {
                                            text: verifyTab.hashCheckResult && verifyTab.hashCheckResult.actualHash ? (tr("Actual: ") + verifyTab.hashCheckResult.actualHash) : ""
                                            visible: text !== ""
                                            color: verifyTab.subPanelText
                                            opacity: 0.9
                                            width: parent.width
                                            wrapMode: Text.WrapAnywhere
                                        }
                                        Text {
                                            text: verifyTab.hashCheckResult && verifyTab.hashCheckResult.reportOutputPath ? (tr("Informe: ") + verifyTab.hashCheckResult.reportOutputPath) : ""
                                            visible: text !== ""
                                            color: verifyTab.subPanelText
                                            opacity: 0.85
                                            width: parent.width
                                            wrapMode: Text.WrapAnywhere
                                        }
                                        Text {
                                            text: verifyTab.hashCheckResult && verifyTab.hashCheckResult.matching_hash
                                                  ? (tr("Coinciden: ") + verifyTab.hashCheckResult.matching_hash.length
                                                     + tr(" | No coinciden: ") + verifyTab.hashCheckResult.not_matching_hash.length
                                                     + tr(" | Sin fichero: ") + verifyTab.hashCheckResult.hash_without_file.length
                                                     + tr(" | Sin hash: ") + verifyTab.hashCheckResult.file_without_hash.length)
                                                  : ""
                                            visible: text !== ""
                                            color: verifyTab.subPanelText
                                            opacity: 0.85
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        Text {
                                            text: verifyTab.hashCreateResult && verifyTab.hashCreateResult.error ? verifyTab.hashCreateResult.error
                                                  : (verifyTab.hashCheckResult && verifyTab.hashCheckResult.error ? verifyTab.hashCheckResult.error : "")
                                            visible: text !== ""
                                            color: verifyTab.subPanelError
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                    }
                                }
                            }
                        }

                        // Resultado Detallado
                                Rectangle {
                                    id: verifyDetailsPanel
                                    Layout.fillWidth: true
                                    radius: 15
                                    color: currentTheme.sidebarColor
                                    visible: verifyTab.verifyDetails !== null
                                    implicitHeight: verifyDetailsPanelColumn.implicitHeight + 40
                            
                            ColumnLayout {
                                id: verifyDetailsPanelColumn
                                anchors.fill: parent
                                anchors.margins: 20
                                spacing: 10
                                Text {
                                    text: tr("DETALLES DE LA FIRMA")
                                    font.bold: true
                                    color: Contrast.accentOn(currentTheme.sidebarColor, currentTheme.primaryColor)
                                    Layout.fillWidth: true
                                    wrapMode: Text.Wrap
                                }
                                Column {
                                    id: verifyDetailsContentColumn
                                    Layout.fillWidth: true
                                    width: parent.width
                                    spacing: 10

                                        Text { 
                                            text: tr("Estado: ") + verificationOutcomeDisplay(verifyTab.verifyDetails)
                                            color: verificationOutcomeColorOn(verifyTab.verifyDetails, currentTheme.sidebarColor)
                                            font.pixelSize: 14
                                            font.bold: true
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        Text { 
                                            text: tr("Documento: ") + (verifyTab.verifyFilePath !== "" ? verifyTab.verifyFilePath.split('/').pop() : "")
                                            color: currentTheme.textColor
                                            opacity: 0.85
                                            font.pixelSize: 12
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                        }
                                        Text { 
                                            text: tr("Razón: ") + (verifyTab.verifyDetails && verifyTab.verifyDetails.reason ? localizeVisibleDiagnosticText(verifyTab.verifyDetails.reason) : "")
                                            color: verificationOutcomeColorOn(verifyTab.verifyDetails, currentTheme.sidebarColor)
                                            font.pixelSize: 12
                                            width: parent.width
                                            wrapMode: Text.Wrap
                                            visible: verifyTab.verifyDetails && verifyTab.verifyDetails.reason !== undefined
                                        }

                                        AdaptiveRow {
                                            width: parent.width

                                            ThemedButton {
                                                text: tr("Asistente")
                                                onClicked: {
                                                    const goal = verificationOutcomeKind(verifyTab.verifyDetails) === "invalid"
                                                                 ? "verify-failure"
                                                                 : "verify"
                                                    window.openSupportAssistant(goal)
                                                }
                                            }
                                            ThemedButton {
                                                text: tr("Preparar incidencia")
                                                visible: verificationOutcomeKind(verifyTab.verifyDetails) === "invalid"
                                                onClicked: window.openSupportAssistant("support")
                                            }
                                            ThemedButton {
                                                text: tr("Verificar certificado")
                                                enabled: window.selectedCertData !== null
                                                onClicked: window.openCurrentCertificateValidation()
                                            }
                                            ThemedButton {
                                                text: verifyTab.verifyDetailsAllExpanded ? tr("Contraer todo") : tr("Expandir todo")
                                                visible: verifyTab.verifyDetails !== null
                                                onClicked: verifyTab.toggleVerifyDetailsPanels()
                                            }
                                        }

                                        ColumnLayout {
                                            id: verifyDetailsSectionsColumn
                                            width: parent.width
                                            spacing: 10
                                            visible: verifyTab.verifyDetails && (
                                                (verifyTab.verifyDetails.format && verifyTab.verifyDetails.format !== "") ||
                                                (verifyTab.verifyDetails.coverage && verifyTab.verifyDetails.coverage !== "") ||
                                                verifyTab.verifyDetails.integrity ||
                                                verifyTab.verifyDetails.certificate ||
                                                verifyTab.verifyDetails.trust
                                            )

                                            Rectangle {
                                                id: verificationSummaryPanel
                                                property bool expanded: false
                                                Layout.fillWidth: true
                                                implicitHeight: Math.max(verificationSummaryColumn.implicitHeight + 20, 52)
                                                color: verifyTab.subPanelColor
                                                border.color: verifyTab.subPanelBorder
                                                border.width: 1
                                                radius: 8
                                                visible: verifyTab.verifyDetails && (
                                                    (verifyTab.verifyDetails.format && verifyTab.verifyDetails.format !== "") ||
                                                    (verifyTab.verifyDetails.coverage && verifyTab.verifyDetails.coverage !== "")
                                                )

                                                Column {
                                                    id: verificationSummaryColumn
                                                    anchors.fill: parent
                                                    anchors.margins: 10
                                                    spacing: 6

                                                    RowLayout {
                                                        width: parent.width
                                                        spacing: 8

                                                        Text {
                                                            Layout.preferredWidth: 1
                                                            Layout.fillWidth: true
                                                            text: tr("Resumen de verificación")
                                                            color: verifyTab.subPanelTitle
                                                            font.bold: true
                                                            font.pixelSize: 12
                                                            wrapMode: Text.Wrap
                                                        }
                                                        ToolButton {
                                                            text: parent.parent.parent.expanded ? "▼" : "▶"
                                                            ToolTip.visible: hovered
                                                            ToolTip.delay: 250
                                                            ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                            onClicked: verifyTab.toggleVerifyDetailsPanel(verificationSummaryPanel)
                                                        }
                                                    }
                                                    Text {
                                                        text: tr("Formato: ") + (verifyTab.verifyDetails && verifyTab.verifyDetails.format ? verifyTab.verifyDetails.format : tr("No disponible"))
                                                        color: verifyTab.subPanelText
                                                        opacity: 0.95
                                                        width: parent.width
                                                        wrapMode: Text.Wrap
                                                        font.pixelSize: 12
                                                        visible: !verificationSummaryColumn.parent.expanded
                                                    }
                                                    Text {
                                                        text: tr("Formato: ") + (verifyTab.verifyDetails && verifyTab.verifyDetails.format ? verifyTab.verifyDetails.format : tr("No disponible"))
                                                        color: verifyTab.subPanelText
                                                        opacity: 0.95
                                                        width: parent.width
                                                        wrapMode: Text.Wrap
                                                        font.pixelSize: 12
                                                        visible: verificationSummaryColumn.parent.expanded
                                                    }
                                                    Text {
                                                        text: tr("Cobertura: ") + (verifyTab.verifyDetails && verifyTab.verifyDetails.coverage ? verificationCoverageText(verifyTab.verifyDetails.coverage) : tr("No disponible"))
                                                        color: verifyTab.subPanelText
                                                        opacity: 0.95
                                                        width: parent.width
                                                        wrapMode: Text.Wrap
                                                        font.pixelSize: 12
                                                        visible: verificationSummaryColumn.parent.expanded
                                                    }
                                                }
                                            }

                                            Repeater {
                                                model: [
                                                    { label: tr("Integridad"), value: verifyTab.verifyDetails ? verifyTab.verifyDetails.integrity : null },
                                                    { label: tr("Certificado"), value: verifyTab.verifyDetails ? verifyTab.verifyDetails.certificate : null },
                                                    { label: tr("Confianza"), value: verifyTab.verifyDetails ? verifyTab.verifyDetails.trust : null }
                                                ]

                                                delegate: Rectangle {
                                                    Layout.fillWidth: true
                                                    property bool expanded: false
                                                    implicitHeight: Math.max(aspectColumn.implicitHeight + 20, 52)
                                                    color: verifyTab.subPanelColor
                                                    border.color: verifyTab.subPanelBorder
                                                    border.width: 1
                                                    radius: 8
                                                    visible: modelData.value !== null && modelData.value !== undefined

                                                    Column {
                                                        id: aspectColumn
                                                        anchors.fill: parent
                                                        anchors.margins: 10
                                                        spacing: 6

                                                        RowLayout {
                                                            id: verifyAspectHeader
                                                            width: parent.width
                                                            spacing: 8

                                                            Text {
                                                                Layout.preferredWidth: 1
                                                                text: modelData.label + ": " + verificationStatusText(modelData.value && modelData.value.status ? modelData.value.status : "")
                                                                color: Contrast.accentOn(verifyTab.subPanelColor, verificationAspectColor(modelData.value && modelData.value.status ? modelData.value.status : ""))
                                                                font.bold: true
                                                                font.pixelSize: 12
                                                                Layout.fillWidth: true
                                                                wrapMode: Text.Wrap
                                                            }
                                                            ToolButton {
                                                                text: parent.parent.parent.expanded ? "▼" : "▶"
                                                                ToolTip.visible: hovered
                                                                ToolTip.delay: 250
                                                                ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                            }

                                                            MouseArea {
                                                                anchors.fill: parent
                                                                onClicked: verifyTab.toggleVerifyDetailsPanel(parent.parent.parent)
                                                                acceptedButtons: Qt.LeftButton
                                                                cursorShape: Qt.PointingHandCursor
                                                            }
                                                        }
                                                        Text {
                                                            text: tr("Razón: ") + ((modelData.value && modelData.value.reason) ? localizeVisibleDiagnosticText(modelData.value.reason) : tr("No disponible"))
                                                            color: verifyTab.subPanelText
                                                            opacity: 0.92
                                                            font.pixelSize: 12
                                                            width: parent.width
                                                            wrapMode: Text.Wrap
                                                            visible: parent.parent.expanded
                                                        }
                                                        Text {
                                                            text: tr("Detalles: ") + verificationAspectDetailsText(modelData.value)
                                                            color: verifyTab.subPanelText
                                                            opacity: 0.82
                                                            font.pixelSize: 12
                                                            width: parent.width
                                                            wrapMode: Text.Wrap
                                                            visible: parent.parent.expanded
                                                        }
                                                    }

                                                }
                                            }
                                        }

                                        Rectangle {
                                            // Quién firmó y cuándo es lo primero que se busca: abierto.
                                            property bool expanded: true
                                            width: parent.width
                                            implicitHeight: Math.max(signersColumn.implicitHeight + 20, 52)
                                            color: verifyTab.subPanelColor
                                            border.color: verifyTab.subPanelBorder
                                            border.width: 1
                                            radius: 8
                                            visible: verifyTab.verifyDetails !== null && window.verificationSignerEntries(verifyTab.verifyDetails).length > 0

                                            Column {
                                                id: signersColumn
                                                anchors.fill: parent
                                                anchors.margins: 10
                                                spacing: 6

                                                RowLayout {
                                                    width: parent.width
                                                    spacing: 8

                                                    Text {
                                                        Layout.preferredWidth: 1
                                                        Layout.fillWidth: true
                                                        text: tr("Firmantes") + " (" + window.verificationSignerEntries(verifyTab.verifyDetails).length + ")"
                                                        color: verifyTab.subPanelTitle
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        wrapMode: Text.Wrap
                                                    }
                                                    ToolButton {
                                                        text: parent.parent.parent.expanded ? "▼" : "▶"
                                                        ToolTip.visible: hovered
                                                        ToolTip.delay: 250
                                                        ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                        onClicked: verifyTab.toggleVerifyDetailsPanel(signersColumn.parent)
                                                    }
                                                }

                                                Text {
                                                    text: window.verificationSignerSummariesText(verifyTab.verifyDetails)
                                                    color: verifyTab.subPanelText
                                                    font.pixelSize: 12
                                                    wrapMode: Text.Wrap
                                                    width: signersColumn.width
                                                    visible: signersColumn.parent.expanded
                                                }
                                            }
                                        }

                                        Rectangle {
                                            property bool expanded: false
                                            width: parent.width
                                            implicitHeight: Math.max(signerSummaryColumn.implicitHeight + 20, 52)
                                            color: verifyTab.subPanelColor
                                            border.color: verifyTab.subPanelBorder
                                            border.width: 1
                                            radius: 8
                                            visible: verifyTab.verifyDetails !== null && window.verificationSignerTechnicalText(verifyTab.verifyDetails) !== ""

                                            Column {
                                                id: signerSummaryColumn
                                                anchors.fill: parent
                                                anchors.margins: 10
                                                spacing: 6

                                                RowLayout {
                                                    width: parent.width
                                                    spacing: 8

                                                    Text {
                                                        Layout.preferredWidth: 1
                                                        Layout.fillWidth: true
                                                        text: tr("verificacion.firmantes_datos_tecnicos")
                                                        color: verifyTab.subPanelTitle
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        wrapMode: Text.Wrap
                                                    }
                                                    ToolButton {
                                                        text: parent.parent.parent.expanded ? "▼" : "▶"
                                                        ToolTip.visible: hovered
                                                        ToolTip.delay: 250
                                                        ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                        onClicked: verifyTab.toggleVerifyDetailsPanel(signerSummaryColumn.parent)
                                                    }
                                                }

                                                Text {
                                                    text: window.verificationSignerTechnicalText(verifyTab.verifyDetails)
                                                    color: verifyTab.subPanelText
                                                    opacity: 0.9
                                                    wrapMode: Text.Wrap
                                                    width: parent.width
                                                    font.pixelSize: 12
                                                    visible: signerSummaryColumn.parent.expanded
                                                }
                                            }
                                        }

                                        Rectangle {
                                            property bool expanded: false
                                            width: parent.width
                                            implicitHeight: Math.max(signatureDataColumn.implicitHeight + 20, 52)
                                            color: verifyTab.subPanelColor
                                            border.color: verifyTab.subPanelBorder
                                            border.width: 1
                                            radius: 8
                                            visible: verifyTab.verifyDetails && verifyTab.verifyDetails.details && verifyTab.verifyDetails.details.length > 0

                                            Column {
                                                id: signatureDataColumn
                                                anchors.fill: parent
                                                anchors.margins: 10
                                                spacing: 6

                                                RowLayout {
                                                    width: parent.width
                                                    spacing: 8

                                                    Text {
                                                        Layout.preferredWidth: 1
                                                        Layout.fillWidth: true
                                                        text: tr("verificacion.ver_detalles_tecnicos") + " (" + ((verifyTab.verifyDetails && verifyTab.verifyDetails.details) ? verifyTab.verifyDetails.details.length : 0) + ")"
                                                        color: verifyTab.subPanelTitle
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        wrapMode: Text.Wrap
                                                    }
                                                    ToolButton {
                                                        text: parent.parent.parent.expanded ? "▼" : "▶"
                                                        ToolTip.visible: hovered
                                                        ToolTip.delay: 250
                                                        ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                        onClicked: verifyTab.toggleVerifyDetailsPanel(signatureDataColumn.parent)
                                                    }
                                                }

                                                Repeater {
                                                    model: (verifyTab.verifyDetails && verifyTab.verifyDetails.details) ? verifyTab.verifyDetails.details : []
                                                    delegate: Text {
                                                        text: "• " + window.verificationDetailText(modelData)
                                                        color: verifyTab.subPanelText
                                                        opacity: 0.9
                                                        wrapMode: Text.Wrap
                                                        width: signatureDataColumn.width
                                                        font.pixelSize: 12
                                                        visible: signatureDataColumn.parent.expanded
                                                    }
                                                }
                                            }
                                        }

                                        Rectangle {
                                            property bool expanded: false
                                            width: parent.width
                                            implicitHeight: Math.max(warningsColumn.implicitHeight + 20, 52)
                                            color: verifyTab.subPanelColor
                                            border.color: verifyTab.subPanelBorder
                                            border.width: 1
                                            radius: 8
                                            visible: verifyTab.verifyDetails && verifyTab.verifyDetails.warnings && verifyTab.verifyDetails.warnings.length > 0

                                            Column {
                                                id: warningsColumn
                                                anchors.fill: parent
                                                anchors.margins: 10
                                                spacing: 6

                                                RowLayout {
                                                    width: parent.width
                                                    spacing: 8

                                                    Text {
                                                        Layout.preferredWidth: 1
                                                        Layout.fillWidth: true
                                                        text: tr("Advertencias") + " (" + ((verifyTab.verifyDetails && verifyTab.verifyDetails.warnings) ? verifyTab.verifyDetails.warnings.length : 0) + ")"
                                                        color: verifyTab.subPanelWarning
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        wrapMode: Text.Wrap
                                                    }
                                                    ToolButton {
                                                        text: parent.parent.parent.expanded ? "▼" : "▶"
                                                        ToolTip.visible: hovered
                                                        ToolTip.delay: 250
                                                        ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                        onClicked: verifyTab.toggleVerifyDetailsPanel(warningsColumn.parent)
                                                    }
                                                }

                                                Repeater {
                                                    model: (verifyTab.verifyDetails && verifyTab.verifyDetails.warnings) ? verifyTab.verifyDetails.warnings : []
                                                    delegate: Text {
                                                        text: "• " + window.localizeVisibleDiagnosticText(modelData)
                                                        color: verifyTab.subPanelText
                                                        opacity: 0.9
                                                        wrapMode: Text.Wrap
                                                        width: warningsColumn.width
                                                        font.pixelSize: 12
                                                        visible: warningsColumn.parent.expanded
                                                    }
                                                }
                                            }
                                        }

                                        Rectangle {
                                            property bool expanded: false
                                            width: parent.width
                                            implicitHeight: Math.max(errorsColumn.implicitHeight + 20, 52)
                                            color: verifyTab.subPanelColor
                                            border.color: verifyTab.subPanelBorder
                                            border.width: 1
                                            radius: 8
                                            visible: verifyTab.verifyDetails && verifyTab.verifyDetails.errors && verifyTab.verifyDetails.errors.length > 0

                                            Column {
                                                id: errorsColumn
                                                anchors.fill: parent
                                                anchors.margins: 10
                                                spacing: 6

                                                RowLayout {
                                                    width: parent.width
                                                    spacing: 8

                                                    Text {
                                                        Layout.preferredWidth: 1
                                                        Layout.fillWidth: true
                                                        text: tr("Errores") + " (" + ((verifyTab.verifyDetails && verifyTab.verifyDetails.errors) ? verifyTab.verifyDetails.errors.length : 0) + ")"
                                                        color: verifyTab.subPanelError
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        wrapMode: Text.Wrap
                                                    }
                                                    ToolButton {
                                                        text: parent.parent.parent.expanded ? "▼" : "▶"
                                                        ToolTip.visible: hovered
                                                        ToolTip.delay: 250
                                                        ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                        onClicked: verifyTab.toggleVerifyDetailsPanel(errorsColumn.parent)
                                                    }
                                                }

                                                Repeater {
                                                    model: (verifyTab.verifyDetails && verifyTab.verifyDetails.errors) ? verifyTab.verifyDetails.errors : []
                                                    delegate: Text {
                                                        text: "• " + window.localizeVisibleDiagnosticText(modelData)
                                                        color: verifyTab.subPanelText
                                                        opacity: 0.9
                                                        wrapMode: Text.Wrap
                                                        width: errorsColumn.width
                                                        font.pixelSize: 12
                                                        visible: errorsColumn.parent.expanded
                                                    }
                                                }
                                            }
                                        }

                                        Rectangle {
                                            property bool expanded: false
                                            width: parent.width
                                            implicitHeight: Math.max(evidenceColumn.implicitHeight + 20, 52)
                                            color: verifyTab.subPanelColor
                                            border.color: verifyTab.subPanelBorder
                                            border.width: 1
                                            radius: 8
                                            visible: verifyTab.verifyDetails && verifyTab.verifyDetails.evidence && verifyTab.verifyDetails.evidence.length > 0

                                            Column {
                                                id: evidenceColumn
                                                anchors.fill: parent
                                                anchors.margins: 10
                                                spacing: 6

                                                RowLayout {
                                                    width: parent.width
                                                    spacing: 8

                                                    Text {
                                                        Layout.preferredWidth: 1
                                                        Layout.fillWidth: true
                                                        text: tr("Evidencias") + " (" + ((verifyTab.verifyDetails && verifyTab.verifyDetails.evidence) ? verifyTab.verifyDetails.evidence.length : 0) + ")"
                                                        color: verifyTab.subPanelTitle
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        wrapMode: Text.Wrap
                                                    }
                                                    ToolButton {
                                                        text: parent.parent.parent.expanded ? "▼" : "▶"
                                                        ToolTip.visible: hovered
                                                        ToolTip.delay: 250
                                                        ToolTip.text: parent.parent.parent.expanded ? tr("Ocultar detalles") : tr("Mostrar detalles")
                                                        onClicked: verifyTab.toggleVerifyDetailsPanel(evidenceColumn.parent)
                                                    }
                                                }

                                                Text {
                                                    text: verificationEvidenceText(verifyTab.verifyDetails)
                                                    color: verifyTab.subPanelText
                                                    opacity: 0.9
                                                    wrapMode: Text.Wrap
                                                    width: parent.width
                                                    font.pixelSize: 12
                                                    visible: evidenceColumn.parent.expanded
                                                }
                                            }
                                        }

                                        // Alerta de confianza
                                        Rectangle {
                                            width: parent.width
                                            implicitHeight: trustAlertRow.implicitHeight + 20
                                            visible: verifyTab.verifyDetails && verifyTab.verifyDetails.reason && verifyTab.verifyDetails.reason.indexOf(tr("emisor no confiable")) !== -1
                                            color: verifyTab.subPanelColor
                                            radius: 8
                                            border.color: verifyTab.subPanelWarning
                                            border.width: 1

                                            GridLayout {
                                                id: trustAlertRow
                                                anchors.fill: parent
                                                anchors.margins: 10
                                                columns: width < 520 ? 2 : 3
                                                columnSpacing: 10
                                                rowSpacing: 10
                                                Text {
                                                    text: tr("⚠️")
                                                    font.pixelSize: 24
                                                }
                                                ColumnLayout {
                                                    Layout.fillWidth: true
                                                    Text {
                                                        text: tr("Confianza TLS local")
                                                        color: verifyTab.subPanelText
                                                        font.bold: true
                                                        font.pixelSize: 12
                                                        wrapMode: Text.WordWrap
                                                        Layout.fillWidth: true
                                                    }
                                                    Text {
                                                        text: tr("Para abrir la API REST local sin avisos del navegador, instale la confianza del certificado TLS local.")
                                                        color: verifyTab.subPanelText
                                                        font.pixelSize: 12
                                                        wrapMode: Text.WordWrap
                                                        Layout.fillWidth: true
                                                    }
                                                }
                                                ThemedButton {
                                                    text: tr("Confiar en la API local")
                                                    Layout.columnSpan: trustAlertRow.columns === 2 ? 2 : 1
                                                    onClicked: backend.installPublicRoots()
                                                    palette.button: "#2ecc71"
                                                    palette.buttonText: "white"
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                        }
                    }
                }
            }
            Item {
                id: protectTab
                RowLayout {
                    anchors.fill: parent
                    anchors.margins: window.narrowWindow ? 16 : 40
                    spacing: window.narrowWindow ? 16 : 40

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        radius: 16
                        color: currentTheme.cardColor
                        border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                        border.width: 1

                        ScrollView {
                            id: protectMainOuterScroll
                            anchors.fill: parent
                            leftPadding: 16
                            rightPadding: 26
                            topPadding: 16
                            bottomPadding: 26
                            // Solo desplazamiento vertical: las tarjetas se ajustan al ancho.
                            contentWidth: availableWidth
                            clip: true
                            ScrollBar.vertical.policy: ScrollBar.AsNeeded
                            ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
                            ScrollBar.vertical.width: 18

                            ColumnLayout {
                                id: protectMainOuterContent
                                width: protectMainOuterScroll.availableWidth
                                spacing: 24

                                Rectangle {
                                    id: protectionCard
                                    Layout.fillWidth: true
                                    radius: 16
                                    color: currentTheme.cardColor
                                    border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                                    border.width: 1
                                    implicitHeight: protectionCardColumn.implicitHeight + 32

                                    ColumnLayout {
                                        id: protectionCardColumn
                                        anchors.fill: parent
                                        anchors.margins: 16
                                        spacing: 14

                                    Text {
                                        text: tr("Cifrar / Proteger")
                                        color: currentTheme.textColor
                                        font.pixelSize: 22
                                        font.bold: true
                                    }

                                    Text {
                                        text: tr("Protege un fichero para uno o varios destinatarios. También puedes protegerlo y firmarlo con el certificado seleccionado.")
                                        color: currentTheme.secondaryTextColor
                                        wrapMode: Text.WordWrap
                                        Layout.fillWidth: true
                                    }

                                    AdaptiveRow {
                                        spacing: 12

                                        ThemedButton {
                                            text: tr("Seleccionar fichero...")
                                            onClicked: protectFileDialog.open()
                                        }
                                        ThemedButton {
                                            text: tr("Cargar destinatarios")
                                            onClicked: backend.loadProtectionRecipients()
                                        }
                                    }
                                    Text {
                                        text: tr("Certificado para proteger firmando: %1").arg(window.selectedCertData ? window.certificateDisplayName(window.selectedCertData) : tr("No seleccionado"))
                                        color: currentTheme.secondaryTextColor
                                        wrapMode: Text.WordWrap
                                        Layout.fillWidth: true
                                    }

                                    Text {
                                        text: tr("Fichero: %1").arg(window.protectInputPath !== "" ? window.protectInputPath : tr("No seleccionado"))
                                        color: currentTheme.textColor
                                        wrapMode: Text.WordWrap
                                        Layout.fillWidth: true
                                    }

                                    GridLayout {
                                        columns: protectMainOuterScroll.availableWidth < 560 ? 1 : Math.min(3, Math.max(1, Math.floor(protectMainOuterScroll.availableWidth / 160)))
                                        columnSpacing: 12
                                        rowSpacing: 10
                                        Layout.fillWidth: true

                                        ColumnLayout {
                                            Layout.fillWidth: true
                                            spacing: 6
                                            Text { text: tr("Ruta de salida opcional"); color: currentTheme.secondaryTextColor }
                                            ThemedTextField {
                                                Layout.fillWidth: true
                                                text: window.protectOutputPath
                                                placeholderText: tr("/ruta/de/salida.opcional")
                                                onTextChanged: window.protectOutputPath = text
                                            }
                                        }

                                        ColumnLayout {
                                            spacing: 6
                                            Text { text: tr("Perfil"); color: currentTheme.secondaryTextColor }
                                            ComboBox {
                                                model: [
                                                    { texto: tr("Compat"), valor: "compat" },
                                                    { texto: tr("Alto"), valor: "alto" }
                                                ]
                                                textRole: "texto"
                                                currentIndex: optionIndexByValue(model, window.protectProfile)
                                                onActivated: {
                                                    window.clearTransientProtectionSecrets()
                                                    window.protectProfile = model[index].valor
                                                    window.pruneProtectionSelection()
                                                }
                                            }
                                        }

                                        ColumnLayout {
                                            spacing: 6
                                            Text { text: tr("Contenedor"); color: currentTheme.secondaryTextColor }
                                            ComboBox {
                                                enabled: window.protectProfile !== "alto"
                                                model: [
                                                    { texto: tr("JSON (.afp)"), valor: "json" },
                                                    { texto: tr("CMS (.enveloped)"), valor: "cms" },
                                                    { texto: tr("CMS EncryptedData (.encrypted.p7m)"), valor: "cms-encrypted" },
                                                    { texto: tr("CMS AuthEnvelopedData (.authenveloped.p7m)"), valor: "authenvelopeddata" }
                                                ]
                                                textRole: "texto"
                                                currentIndex: optionIndexByValue(model, window.protectContainer)
                                                onActivated: {
                                                    window.clearTransientProtectionSecrets()
                                                    window.protectContainer = model[index].valor
                                                    window.pruneProtectionSelection()
                                                }
                                            }
                                        }
                                    }

                                    Text {
                                        Layout.fillWidth: true
                                        wrapMode: Text.WordWrap
                                        color: currentTheme.secondaryTextColor
                                        text: window.protectContainer === "authenvelopeddata" && window.protectProfile === "compat"
                                              ? tr("AuthEnvelopedData usa AES-256-GCM y RSA-OAEP-SHA256/MGF1-SHA256. Requiere un destinatario compatible con certificado X.509 RSA válido y aporta integridad autenticada, no firma electrónica.")
                                              : (window.isEncryptedDataProtection()
                                                 ? tr("EncryptedData usa una clave AES-256 transitoria en Base64, no requiere destinatarios y no constituye una firma electrónica.")
                                              : tr("Extensión protegida prevista: %1").arg(window.protectionContainerSuffix())
                                                )
                                    }

                                    Rectangle {
                                        Layout.fillWidth: true
                                        visible: window.isEncryptedDataProtection()
                                        radius: 10
                                        color: Qt.rgba(0, 0, 0, 0.10)
                                        implicitHeight: protectEncryptedSecretColumn.implicitHeight + 20

                                        ColumnLayout {
                                            id: protectEncryptedSecretColumn
                                            anchors.fill: parent
                                            anchors.margins: 10
                                            spacing: 8

                                            Text {
                                                wrapMode: Text.WordWrap
                                                Layout.fillWidth: true
                                                text: tr("Clave transitoria de EncryptedData")
                                                color: currentTheme.textColor
                                                font.bold: true
                                            }
                                            Text {
                                                Layout.fillWidth: true
                                                wrapMode: Text.WordWrap
                                                text: tr("Introduzca una clave AES-256 en Base64 canónico (44 caracteres) y repítala. La clave solo se mantiene durante esta operación y no se guarda; sin ella no podrá recuperar el documento.")
                                                color: currentTheme.secondaryTextColor
                                            }
                                            ThemedTextField {
                                                id: protectEncryptedSecretField
                                                Layout.fillWidth: true
                                                maximumLength: 44
                                                echoMode: TextInput.Password
                                                inputMethodHints: Qt.ImhSensitiveData | Qt.ImhHiddenText | Qt.ImhNoPredictiveText | Qt.ImhNoAutoUppercase
                                                placeholderText: tr("Clave AES-256 en Base64 (44 caracteres)")
                                                Accessible.name: tr("Clave transitoria de EncryptedData")
                                            }
                                            ThemedTextField {
                                                id: protectEncryptedSecretConfirmField
                                                Layout.fillWidth: true
                                                maximumLength: 44
                                                echoMode: TextInput.Password
                                                inputMethodHints: Qt.ImhSensitiveData | Qt.ImhHiddenText | Qt.ImhNoPredictiveText | Qt.ImhNoAutoUppercase
                                                placeholderText: tr("Repita la clave transitoria")
                                                Accessible.name: tr("Confirmación de la clave transitoria de EncryptedData")
                                            }
                                            Text {
                                                Layout.fillWidth: true
                                                visible: protectEncryptedSecretConfirmField.text.length > 0
                                                         && protectEncryptedSecretField.text !== protectEncryptedSecretConfirmField.text
                                                wrapMode: Text.WordWrap
                                                text: tr("Las claves transitorias no coinciden.")
                                                color: "#ff8a80"
                                            }
                                            ThemedButton {
                                                text: tr("Borrar clave")
                                                onClicked: {
                                                    protectEncryptedSecretField.clear()
                                                    protectEncryptedSecretConfirmField.clear()
                                                }
                                            }
                                        }
                                    }

                                    Rectangle {
                                        Layout.fillWidth: true
                                        visible: !window.isEncryptedDataProtection()
                                        radius: 10
                                        color: Qt.rgba(0, 0, 0, 0.10)
                                        implicitHeight: protectionRecipientsColumn.implicitHeight + 20

                                        ColumnLayout {
                                            id: protectionRecipientsColumn
                                            anchors.fill: parent
                                            anchors.margins: 10
                                            spacing: 8

                                            Text {
                                                text: tr("Destinatarios de protección")
                                                color: currentTheme.textColor
                                                font.bold: true
                                            }
                                            ThemedButton {
                                                text: tr("Añadir destinatario público…")
                                                enabled: isIpcMode
                                                onClicked: recipientPublicFileDialog.open()
                                                Accessible.description: tr("Importa solo un certificado X.509 público; no se importa ninguna clave privada.")
                                            }
                                            ThemedButton {
                                                text: tr("Compartir mi certificado…")
                                                enabled: isIpcMode
                                                onClicked: window.startPublicCertificateExport(true)
                                            }

                                            Text {
                                                text: visibleProtectionRecipients().length === 0
                                                      ? (window.protectContainer === "authenvelopeddata" && window.protectProfile === "compat"
                                                         ? tr("No hay destinatarios compatibles con AuthEnvelopedData. Importe o configure un certificado X.509 RSA válido para cifrado de clave.")
                                                           + " "
                                                           + tr("El perfil compat requiere una identidad RSA con clave privada descifrable, por ejemplo un P12/PFX autorizado. Los certificados opacos del almacén del sistema siguen disponibles para firmar; carga un P12/PFX apto o usa el perfil alto.")
                                                         : (window.protectProfile === "compat"
                                                            ? tr("El perfil compat requiere una identidad RSA con clave privada descifrable, por ejemplo un P12/PFX autorizado. Los certificados opacos del almacén del sistema siguen disponibles para firmar; carga un P12/PFX apto o usa el perfil alto.")
                                                            : tr("No hay destinatarios cargados para este perfil.")))
                                                      : tr("Selecciona uno o varios destinatarios.")
                                                color: currentTheme.secondaryTextColor
                                                wrapMode: Text.WordWrap
                                                Layout.fillWidth: true
                                            }

                                            Repeater {
                                                model: visibleProtectionRecipients()
                                                delegate: ColumnLayout {
                                                    Layout.fillWidth: true
                                                    ThemedCheckBox {
                                                        text: window.protectionRecipientLabel(modelData)
                                                        checked: window.protectSelectedRecipientIds.indexOf(String(modelData.id || modelData.ID || "")) !== -1
                                                        onToggled: {
                                                            const id = String(modelData.id || modelData.ID || "")
                                                            let next = window.protectSelectedRecipientIds.slice()
                                                            const idx = next.indexOf(id)
                                                            if (checked) {
                                                                if (idx === -1) next.push(id)
                                                            } else if (idx !== -1) {
                                                                next.splice(idx, 1)
                                                            }
                                                            window.protectSelectedRecipientIds = next
                                                        }
                                                    }
                                                    RowLayout {
                                                        Layout.fillWidth: true
                                                        Text {
                                                            wrapMode: Text.WordWrap
                                                            Layout.preferredWidth: 1
                                                            Layout.fillWidth: true
                                                            text: tr("Origen: %1").arg(String(modelData.origin || "propio") === "importado" ? tr("importado") : (String(modelData.origin || "propio") === "otras_personas" ? tr("otras personas") : tr("certificado propio")))
                                                            color: currentTheme.secondaryTextColor
                                                            font.pixelSize: 11
                                                        }
                                                        ThemedButton {
                                                            visible: String(modelData.origin || "") === "importado"
                                                            text: tr("Quitar")
                                                            onClicked: backend.removeProtectionRecipient(String(modelData.id || modelData.ID || ""))
                                                        }
                                                    }
                                                }
                                            }
                                        }
                                    }

                                    RowLayout {
                                        Layout.fillWidth: true
                                        spacing: 12

                                        ThemedButton {
                                            text: tr("Proteger")
                                            enabled: !window.protectionInProgress
                                                     && window.protectInputPath !== ""
                                                     && (window.isEncryptedDataProtection()
                                                         ? window.looksLikeCanonicalAES256Secret(protectEncryptedSecretField.text)
                                                           && protectEncryptedSecretField.text === protectEncryptedSecretConfirmField.text
                                                         : window.protectSelectedRecipientIds.length > 0)
                                            onClicked: {
                                                window.protectResult = null
                                                window.protectionInProgress = true
                                                if (window.isEncryptedDataProtection()) {
                                                    backend.protectEncryptedDataFile(
                                                        window.protectInputPath,
                                                        window.protectOutputPath,
                                                        protectEncryptedSecretField.text)
                                                    protectEncryptedSecretField.clear()
                                                    protectEncryptedSecretConfirmField.clear()
                                                } else {
                                                    backend.protectFileAdvanced(
                                                        window.protectInputPath,
                                                        window.protectOutputPath,
                                                        window.protectSelectedRecipientIds,
                                                        window.selectedCertIndex,
                                                        {
                                                            profile: window.protectProfile,
                                                            overwrite: "rename",
                                                            saveToDisk: true,
                                                            signToo: false,
                                                            options: {
                                                                container: window.protectProfile === "alto" ? "json" : window.protectContainer
                                                            }
                                                        })
                                                }
                                            }
                                        }

                                        ThemedButton {
                                            text: tr("Proteger y firmar")
                                            enabled: !window.protectionInProgress && window.protectInputPath !== "" && window.protectSelectedRecipientIds.length > 0 && window.selectedCertIndex !== -1 && window.protectProfile === "compat" && window.protectContainer !== "authenvelopeddata" && window.protectContainer !== "cms-encrypted"
                                            onClicked: window.executeProtectSignRequest()
                                        }
                                        BusyIndicator {
                                            running: window.protectionInProgress
                                            visible: running
                                            implicitWidth: 28
                                            implicitHeight: 28
                                        }
                                    }

                                    Rectangle {
                                        Layout.fillWidth: true
                                        visible: window.protectResult !== null
                                        radius: 10
                                        color: window.protectResult && window.protectResult.error ? "#4a1f1f" : "#1f3a2c"
                                        implicitHeight: protectResultColumn.implicitHeight + 20

                                        ColumnLayout {
                                            id: protectResultColumn
                                            anchors.fill: parent
                                            anchors.margins: 10
                                            spacing: 6

                                            Text {
                                                text: window.protectResult && window.protectResult.error
                                                      ? tr("Error al proteger")
                                                      : tr("Resultado de protección")
                                                color: "white"
                                                font.bold: true
                                            }
                                            Text {
                                                text: window.protectResult && window.protectResult.error
                                                      ? window.protectResult.error
                                                      : tr("Salida: %1").arg(window.protectResult && window.protectResult.outputPath ? window.protectResult.outputPath : tr("No disponible"))
                                                color: "white"
                                                wrapMode: Text.WordWrap
                                                Layout.fillWidth: true
                                            }
                                            Text {
                                                visible: window.protectResult && !window.protectResult.error
                                                text: tr("Perfil: %1 | Destinatarios: %2").arg(window.protectResult && window.protectResult.profile ? window.protectResult.profile : "-").arg(window.protectResult && window.protectResult.recipientCount !== undefined ? window.protectResult.recipientCount : 0)
                                                color: "white"
                                                wrapMode: Text.WordWrap
                                            }
                                            ThemedButton {
                                                visible: window.protectResult && !window.protectResult.error && !!window.protectResult.outputPath
                                                text: tr("Abrir protegido")
                                                onClicked: backend.openExternal(window.protectResult.outputPath)
                                            }
                                        }
                                    }
                                    }
                                }

                                Rectangle {
                                    id: unprotectCard
                                    Layout.fillWidth: true
                                    radius: 16
                                    color: currentTheme.cardColor
                                    border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                                    border.width: 1
                                    implicitHeight: unprotectCardColumn.implicitHeight + 32

                                    ColumnLayout {
                                        id: unprotectCardColumn
                                        anchors.fill: parent
                                        anchors.margins: 16
                                        spacing: 14

                                Text {
                                    text: tr("Descifrar / Desproteger")
                                    color: currentTheme.textColor
                                    font.pixelSize: 22
                                    font.bold: true
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 12
                                    ThemedButton {
                                        text: tr("Seleccionar protegido...")
                                        onClicked: unprotectFileDialog.open()
                                    }
                                    Item { Layout.fillWidth: true }
                                }

                                Text {
                                    text: tr("Fichero protegido: %1").arg(window.unprotectInputPath !== "" ? window.unprotectInputPath : tr("No seleccionado"))
                                    color: currentTheme.textColor
                                    wrapMode: Text.WordWrap
                                    Layout.fillWidth: true
                                }

                                Text {
                                    visible: window.unprotectInputPath !== ""
                                    text: tr("Contenedor detectado: %1").arg(window.protectedContainerLabel(window.unprotectInputPath))
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                    Layout.fillWidth: true
                                }

                                ThemedCheckBox {
                                    id: unprotectEncryptedDataToggle
                                    Layout.fillWidth: true
                                    visible: window.isGenericCMSPath(window.unprotectInputPath)
                                    text: tr("Este CMS genérico es EncryptedData y usa una clave transitoria")
                                    onToggled: {
                                        if (!checked)
                                            unprotectEncryptedSecretField.clear()
                                    }
                                }

                                Rectangle {
                                    Layout.fillWidth: true
                                    visible: window.usesTransientUnprotectionSecret()
                                    radius: 10
                                    color: Qt.rgba(0, 0, 0, 0.10)
                                    implicitHeight: unprotectEncryptedSecretColumn.implicitHeight + 20

                                    ColumnLayout {
                                        id: unprotectEncryptedSecretColumn
                                        anchors.fill: parent
                                        anchors.margins: 10
                                        spacing: 8

                                        Text {
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                            text: tr("Clave transitoria de EncryptedData")
                                            color: currentTheme.textColor
                                            font.bold: true
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            wrapMode: Text.WordWrap
                                            text: tr("Introduzca la misma clave AES-256 en Base64 usada al proteger. Se enviará una sola vez al motor local y no se guardará.")
                                            color: currentTheme.secondaryTextColor
                                        }
                                        ThemedTextField {
                                            id: unprotectEncryptedSecretField
                                            Layout.fillWidth: true
                                            maximumLength: 44
                                            echoMode: TextInput.Password
                                            inputMethodHints: Qt.ImhSensitiveData | Qt.ImhHiddenText | Qt.ImhNoPredictiveText | Qt.ImhNoAutoUppercase
                                            placeholderText: tr("Clave AES-256 en Base64 (44 caracteres)")
                                            Accessible.name: tr("Clave transitoria de EncryptedData")
                                        }
                                        ThemedButton {
                                            text: tr("Borrar clave")
                                            onClicked: unprotectEncryptedSecretField.clear()
                                        }
                                    }
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 6
                                    Text { text: tr("Ruta de salida opcional"); color: currentTheme.secondaryTextColor }
                                    ThemedTextField {
                                        Layout.fillWidth: true
                                        text: window.unprotectOutputPath
                                        placeholderText: tr("/ruta/de/salida.opcional")
                                        onTextChanged: window.unprotectOutputPath = text
                                    }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 12
                                    ThemedButton {
                                        text: tr("Desproteger")
                                        enabled: !window.unprotectionInProgress
                                                 && window.unprotectInputPath !== ""
                                                 && (!window.usesTransientUnprotectionSecret()
                                                     || window.looksLikeCanonicalAES256Secret(unprotectEncryptedSecretField.text))
                                        onClicked: {
                                            window.unprotectResult = null
                                            window.unprotectionInProgress = true
                                            if (window.usesTransientUnprotectionSecret()) {
                                                backend.unprotectEncryptedDataFile(
                                                    window.unprotectInputPath,
                                                    window.unprotectOutputPath,
                                                    unprotectEncryptedSecretField.text)
                                                unprotectEncryptedSecretField.clear()
                                            } else {
                                                backend.unprotectFileAdvanced(
                                                    window.unprotectInputPath,
                                                    window.unprotectOutputPath,
                                                    {
                                                        overwrite: "rename",
                                                        saveToDisk: true
                                                    })
                                            }
                                        }
                                    }
                                    BusyIndicator {
                                        running: window.unprotectionInProgress
                                        visible: running
                                        implicitWidth: 28
                                        implicitHeight: 28
                                    }
                                }

                                Rectangle {
                                    Layout.fillWidth: true
                                    visible: window.unprotectResult !== null
                                    radius: 10
                                    color: window.unprotectResult && window.unprotectResult.error ? "#4a1f1f" : "#1f3a2c"
                                    implicitHeight: unprotectResultColumn.implicitHeight + 20

                                    ColumnLayout {
                                        id: unprotectResultColumn
                                        anchors.fill: parent
                                        anchors.margins: 10
                                        spacing: 6

                                        Text {
                                            text: window.unprotectResult && window.unprotectResult.error
                                                  ? tr("Error al desproteger")
                                                  : tr("Resultado de desprotección")
                                            color: "white"
                                            font.bold: true
                                        }
                                        Text {
                                            text: window.unprotectResult && window.unprotectResult.error
                                                  ? window.unprotectResult.error
                                                  : tr("Salida: %1").arg(window.unprotectResult && window.unprotectResult.outputPath ? window.unprotectResult.outputPath : tr("No disponible"))
                                            color: "white"
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                        }
                                        Text {
                                            visible: window.unprotectResult && !window.unprotectResult.error
                                            text: tr("Perfil: %1 | Destinatario: %2").arg(window.unprotectResult && window.unprotectResult.profile ? window.unprotectResult.profile : "-").arg(window.unprotectResult && window.unprotectResult.recipientId ? window.unprotectResult.recipientId : tr("No disponible"))
                                            color: "white"
                                            wrapMode: Text.WordWrap
                                        }
                                        ThemedButton {
                                            visible: window.unprotectResult && !window.unprotectResult.error && !!window.unprotectResult.outputPath
                                            text: tr("Abrir desprotegido")
                                            onClicked: backend.openExternal(window.unprotectResult.outputPath)
                                        }
                                    }
                                }
                                    }
                                }
                            }
                        }
                    }

                    Rectangle {
                        Layout.fillHeight: true
                        Layout.preferredWidth: window.rightSidebarCollapsed ? window.rightSidebarCollapsedWidth : window.rightSidebarExpandedWidth
                        Layout.minimumWidth: window.rightSidebarCollapsed ? window.rightSidebarCollapsedWidth : window.rightSidebarExpandedWidth
                        Layout.maximumWidth: window.rightSidebarCollapsed ? window.rightSidebarCollapsedWidth : window.rightSidebarExpandedWidth
                        width: window.rightSidebarCollapsed ? window.rightSidebarCollapsedWidth : window.rightSidebarExpandedWidth
                        radius: 15
                        color: currentTheme.sidebarColor
                        Behavior on width {
                            NumberAnimation { duration: 180; easing.type: Easing.InOutQuad }
                        }

                        ColumnLayout {
                            anchors.fill: parent
                            anchors.margins: window.rightSidebarCollapsed ? 10 : 20
                            spacing: window.rightSidebarCollapsed ? 10 : 15

                            AdaptiveRow {
                                Layout.fillWidth: true
                                ThemedButton {
                                    visible: !window.rightSidebarCollapsed
                                    text: tr("+ CERTIFICADOS")
                                    Layout.fillWidth: true
                                    onClicked: window.openGuidedCertificateAccess()
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Importar certificado")
                                }
                                ToolButton {
                                    visible: !window.rightSidebarCollapsed
                                    text: "↻"
                                    onClicked: backend.checkCertificates()
                                    Accessible.name: tr("Actualizar certificados")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Actualizar certificados")
                                }
                                ToolButton {
                                    visible: !window.rightSidebarCollapsed
                                    text: tr("Firefox/NSS")
                                    onClicked: {
                                        backend.backendLogReceived(tr("Buscando certificados en almacenes NSS/Firefox..."))
                                        backend.checkCertificates()
                                    }
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Buscar certificados en Firefox/NSS")
                                }
                                ToolButton {
                                    text: window.rightSidebarCollapsed ? "«" : "»"
                                    onClicked: {
                                        window.rightSidebarAutoCollapsed = false
                                        window.rightSidebarCollapsed = !window.rightSidebarCollapsed
                                    }
                                    Accessible.name: window.rightSidebarCollapsed ? tr("Expandir panel de certificados") : tr("Colapsar panel de certificados")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: window.rightSidebarCollapsed ? tr("Expandir panel de certificados") : tr("Colapsar panel de certificados")
                                }
                            }

                            ColumnLayout {
                                visible: window.rightSidebarCollapsed
                                Layout.fillWidth: true
                                Layout.fillHeight: true
                                spacing: 12

                                Text {
                                    Layout.fillWidth: true
                                    text: window.certificates.length > 0 ? tr("%1 cert.").arg(window.certificates.length) : tr("Sin cert.")
                                    color: currentTheme.textColor
                                    font.pixelSize: 11
                                    horizontalAlignment: Text.AlignHCenter
                                    wrapMode: Text.WordWrap
                                }

                                ScrollView {
                                    Layout.fillWidth: true
                                    Layout.fillHeight: true
                                    clip: true
                                    ScrollBar.horizontal.policy: ScrollBar.AlwaysOff

                                    Column {
                                        width: parent.width
                                        spacing: 8

                                        Repeater {
                                            model: window.filteredCertificates()
                                            delegate: Button {
                                                required property int index
                                                required property var modelData

                                                width: parent.width
                                                height: 36
                                                text: window.compactCertificateBadge(modelData)

                                                background: Rectangle {
                                                    radius: 10
                                                    color: window.certificateId(window.selectedCertData) === window.certificateId(modelData)
                                                           ? currentTheme.primaryColor
                                                           : Qt.rgba(1, 1, 1, 0.08)
                                                    border.color: Qt.rgba(1, 1, 1, 0.18)
                                                    border.width: 1
                                                }

                                                ToolTip.visible: hovered
                                                ToolTip.delay: 300
                                                ToolTip.text: window.compactCertificateTooltip(modelData)

                                                onClicked: {
                                                    const realIndex = window.findCertificateIndexById(window.certificateId(modelData), window.certificates)
                                                    window.selectCertificateIndex(realIndex, true)
                                                }
                                            }
                                        }
                                    }
                                }

                                ToolButton {
                                    Layout.alignment: Qt.AlignHCenter
                                    text: "+"
                                    onClicked: window.openGuidedCertificateAccess()
                                    Accessible.name: tr("Importar certificado")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Importar certificado")
                                }
                                ToolButton {
                                    Layout.alignment: Qt.AlignHCenter
                                    text: "↻"
                                    onClicked: backend.checkCertificates()
                                    Accessible.name: tr("Actualizar certificados")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Actualizar certificados")
                                }
                                ToolButton {
                                    Layout.alignment: Qt.AlignHCenter
                                    text: "Fx"
                                    onClicked: {
                                        backend.backendLogReceived(tr("Buscando certificados en almacenes NSS/Firefox..."))
                                        backend.checkCertificates()
                                    }
                                    Accessible.name: tr("Buscar certificados en Firefox/NSS")
                                    ToolTip.visible: hovered
                                    ToolTip.delay: 350
                                    ToolTip.text: tr("Buscar certificados en Firefox/NSS")
                                }
                            }

                            ColumnLayout {
                                visible: !window.rightSidebarCollapsed
                                Layout.fillWidth: true
                                spacing: 6

                                ThemedTextField {
                                    Layout.fillWidth: true
                                    text: certificateFilterText
                                    placeholderText: tr("Buscar certificado")
                                    onTextChanged: certificateFilterText = text
                                }
                                Text {
                                    Layout.fillWidth: true
                                    text: tr("Mostrando %1 de %2").arg(window.filteredCertificates().length).arg(window.certificates.length)
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 11
                                    wrapMode: Text.WordWrap
                                }
                                ThemedButton {
                                    Layout.fillWidth: true
                                    text: tr("Usar DNIe o tarjeta")
                                    enabled: isIpcMode
                                    onClicked: backend.requestSmartcardStatus()
                                    Accessible.description: tr("Consulta el lector y actualiza los certificados disponibles.")
                                }
                                Text {
                                    Layout.fillWidth: true
                                    visible: window.smartcardMessage !== ""
                                    text: window.smartcardMessage
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                    font.pixelSize: 11
                                }
                            }

                            Loader {
                                visible: !window.rightSidebarCollapsed
                                         && window.filteredCertificates().length === 0
                                         && window.showNoCertificateHelp
                                Layout.fillWidth: true
                                Layout.fillHeight: true
                                sourceComponent: noCertificatesPanelComponent
                            }

                            ListView {
                                visible: !window.rightSidebarCollapsed
                                         && (window.filteredCertificates().length > 0
                                             || !window.showNoCertificateHelp)
                                Layout.fillWidth: true
                                Layout.fillHeight: true
                                model: window.filteredCertificates()
                                spacing: 8
                                clip: true
                                delegate: Rectangle {
                                    width: ListView.view.width
                                    height: Math.max(92, cardContents.implicitHeight + 24)
                                    radius: 12
                                    activeFocusOnTab: true
                                    property bool emphasized: activeFocus
                                        || window.certificateId(window.selectedCertData) === window.certificateId(modelData)
                                    Accessible.role: Accessible.Button
                                    Accessible.name: (modelData.subjectName || modelData.subject || tr("Certificado")) + ", " +
                                                     window.certificateStatusText(modelData) + ". " + window.certificateStatusReason(modelData)
                                    Accessible.description: tr("Emisor: %1. Vence: %2").arg(modelData.issuerName || modelData.issuer || tr("Desconocido")).arg(window.certificateExpiry(modelData))
                                    Accessible.onPressAction: {
                                        const realIndex = window.findCertificateIndexById(window.certificateId(modelData), window.certificates)
                                        window.selectCertificateIndex(realIndex, true)
                                    }
                                    color: emphasized
                                        ? window.certificateSelectionColor() : currentTheme.cardColor
                                    border.color: certificateId(window.selectedCertData) === certificateId(modelData)
                                        ? currentTheme.primaryColor : window.certificateStatusColor(modelData)
                                    border.width: activeFocus || certificateId(window.selectedCertData) === certificateId(modelData) ? 2 : 1

                                    Rectangle {
                                        anchors.left: parent.left
                                        anchors.right: parent.right
                                        anchors.bottom: parent.bottom
                                        anchors.margins: 8
                                        height: 1
                                        color: window.certificateDividerColor()
                                    }

                                    Keys.onPressed: function(event) {
                                        if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter || event.key === Qt.Key_Space) {
                                            const realIndex = window.findCertificateIndexById(window.certificateId(modelData), window.certificates)
                                            window.selectCertificateIndex(realIndex, true)
                                            event.accepted = true
                                        }
                                    }

                                    ColumnLayout {
                                        id: cardContents
                                        anchors.fill: parent
                                        anchors.margins: 12
                                        spacing: 2
                                        RowLayout {
                                            Layout.fillWidth: true
                                            Text {
                                                wrapMode: Text.WordWrap
                                                Layout.preferredWidth: 1
                                                text: modelData.subjectName || modelData.subject || tr("Certificado")
                                                color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : currentTheme.textColor
                                                font.bold: true
                                                Layout.fillWidth: true
                                                elide: Text.ElideRight
                                            }
                                            Text {
                                                visible: window.isDefaultCertificate(modelData)
                                                text: tr("Predeterminado")
                                                color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : "#f1c40f"
                                                font.pixelSize: 10
                                                font.bold: true
                                            }
                                            Text {
                                                visible: window.isTemporaryCertificate(modelData)
                                                text: tr("Solo esta sesión")
                                                color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : "#f39c12"
                                                font.pixelSize: 10
                                                font.bold: true
                                            }
                                            Text {
                                                text: window.certificateStatusText(modelData)
                                                color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : window.certificateStatusColor(modelData)
                                                font.pixelSize: 10
                                                font.bold: true
                                            }
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            visible: window.certificateStatusReason(modelData) !== ""
                                            text: window.certificateStatusReason(modelData)
                                            color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : currentTheme.textColor
                                            font.pixelSize: 12
                                            wrapMode: Text.WordWrap
                                            Accessible.name: text
                                        }
                                        Text {
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                            text: tr("Vence: %1").arg(window.certificateExpiry(modelData))
                                            color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : currentTheme.textColor
                                            font.pixelSize: 11
                                        }
                                        Text {
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                            text: tr("Emisor: %1").arg(modelData.issuerName || modelData.issuer || tr("Desconocido"))
                                            color: cardContents.parent.emphasized
                                                    ? window.certificateSelectionTextColor() : currentTheme.textColor
                                            font.pixelSize: 10
                                            elide: Text.ElideRight
                                        }
                                    }
                                    MouseArea {
                                        anchors.fill: parent
                                        cursorShape: Qt.PointingHandCursor
                                        onClicked: {
                                            const realIndex = window.findCertificateIndexById(window.certificateId(modelData), window.certificates)
                                            window.selectCertificateIndex(realIndex, true)
                                            console.log(tr("QML: Certificado seleccionado:"), (modelData.subjectName || modelData.subject || tr("Certificado")), tr("ID:"), modelData.id)
                                        }
                                    }
                                }
                            }

                            Rectangle {
                                Layout.fillWidth: true
                                Layout.preferredHeight: 270
                                visible: !window.rightSidebarCollapsed && selectedCertIndex !== -1 && window.selectedCertData
                                color: currentTheme.cardColor
                                radius: 10
                                border.color: currentTheme.primaryColor
                                border.width: 1

                                ColumnLayout {
                                    anchors.fill: parent
                                    anchors.margins: 10
                                    spacing: 5

                                    Text {
                                        text: tr("Detalles del certificado")
                                        font.bold: true
                                        color: currentTheme.textColor
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        visible: !!window.selectedCertData && window.selectedCertData.canSign
                                                 && !window.selectedCertData.caducado
                                                 && !!window.selectedCertData.validTo
                                                 && Number(window.selectedCertData.diasCaducidad) >= 0
                                                 && Number(window.selectedCertData.diasCaducidad) <= 60
                                        text: tr("Este certificado caduca en %1 días. Comprueba si debes renovarlo antes de firmar.").arg(window.selectedCertData ? window.selectedCertData.diasCaducidad : 0)
                                        color: "#f2c66d"
                                        wrapMode: Text.WordWrap
                                        font.pixelSize: 11
                                    }
                                    ThemedButton {
                                        visible: window.canRenewFnmt(window.selectedCertData)
                                        text: tr("Renovar en la FNMT")
                                        onClicked: backend.openExternal("https://www.sede.fnmt.gob.es/certificados/persona-fisica/renovar")
                                        Accessible.description: tr("Abre la página oficial de renovación de certificado de persona física de la FNMT.")
                                    }

                                    RowLayout {
                                        Layout.fillWidth: true
                                        spacing: 8

                                        Text {
                                            visible: window.isDefaultCertificate(window.selectedCertData)
                                            text: tr("Predeterminado")
                                            color: "#f1c40f"
                                            font.bold: true
                                            font.pixelSize: 11
                                        }
                                        Item { Layout.fillWidth: true }
                                        ThemedButton {
                                            visible: window.isTemporaryCertificate(window.selectedCertData)
                                            text: tr("Retirar temporal")
                                            onClicked: {
                                                const id = window.certificateId(window.selectedCertData)
                                                window.pendingTemporaryRemovalId = id
                                                backend.removeTemporaryCertificate(id)
                                            }
                                        }
                                        ThemedButton {
                                            text: window.isDefaultCertificate(window.selectedCertData)
                                                  ? tr("Quitar predeterminado")
                                                  : tr("Usar como predeterminado")
                                            onClicked: {
                                                if (window.isDefaultCertificate(window.selectedCertData)) {
                                                    window.clearDefaultCertificate()
                                                } else {
                                                    window.setSelectedCertificateAsDefault()
                                                }
                                            }
                                        }
                                    }

                                    ScrollView {
                                        Layout.fillWidth: true
                                        Layout.fillHeight: true
                                        clip: true

                                        Column {
                                            width: parent.width
                                            spacing: 6

                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Titular: ") + (window.selectedCertData ? (window.selectedCertData.subjectName || window.selectedCertData.subject || "---") : "")
                                                color: currentTheme.textColor
                                                font.pixelSize: 11
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Emisor: ") + (window.selectedCertData ? (window.selectedCertData.issuerName || window.selectedCertData.issuer || "---") : "")
                                                color: currentTheme.textColor
                                                opacity: 0.8
                                                font.pixelSize: 11
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Nº Serie: ") + (window.selectedCertData ? (window.selectedCertData.serialNumber || window.selectedCertData.nif || "") : "")
                                                color: currentTheme.textColor
                                                opacity: 0.8
                                                font.pixelSize: 10
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Válido hasta: ") + (window.selectedCertData ? (window.selectedCertData.validTo || window.selectedCertData.notAfter || "") : "")
                                                color: currentTheme.textColor
                                                opacity: 0.8
                                                font.pixelSize: 10
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Tipo: ") + (window.selectedCertData ? (window.selectedCertData.tipo || tr("Desconocido")) : "")
                                                color: currentTheme.textColor
                                                font.pixelSize: 10
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Organización: ") + (window.selectedCertData ? (window.selectedCertData.organizacion || tr("No indicada")) : "")
                                                color: currentTheme.textColor
                                                font.pixelSize: 10
                                                wrapMode: Text.Wrap
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Estado: ") + window.certificateStatusText(window.selectedCertData)
                                                color: currentTheme.textColor
                                                font.pixelSize: 10
                                                width: parent.width
                                            }
                                            Text {
                                                textFormat: Text.PlainText
                                                text: tr("Huella: ") + (window.selectedCertData ? formatFingerprintForDisplay(window.selectedCertData.fingerprint || "") : "")
                                                color: currentTheme.textColor
                                                opacity: 0.6
                                                font.pixelSize: 9
                                                wrapMode: Text.WrapAnywhere
                                                width: parent.width
                                            }
                                        }
                                    }

                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("Verificar certificado")
                                        palette.button: "#16a085"
                                        palette.buttonText: "white"
                                        onClicked: window.openCertificateValidation(window.selectedCertData, false)
                                    }
                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("Exportar certificado público…")
                                        enabled: isIpcMode && !!window.selectedCertData
                                        onClicked: window.startPublicCertificateExport(false)
                                    }
                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("Refrescar validez online")
                                        enabled: !!window.selectedCertData
                                        onClicked: window.openCertificateValidation(window.selectedCertData, true)
                                    }
                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("Abrir VALIDE")
                                        flat: true
                                        onClicked: backend.openExternal("https://valide.redsara.es/valide/")
                                    }
                                }
                            }
                        }
                    }
                }
            }
            Item {
                id: configTab

                // Service status polled from backend
                property bool svcInstalled: false
                property bool svcRunning: false
                property string svcPlatform: ""
                property string svcMethod: ""
                property string svcMessage: ""
                property bool svcConnected: true
                property bool restServerRunning: false
                property bool restServerChecking: false
                property bool webCompatibilityActive:
                    backend && backend.webCompatibilityActive === true

                Timer {
                    id: statusRetryTimer
                    interval: 1000
                    repeat: false
                    onTriggered: {
                        if (window.activeTab === "config") {
                            backend.getServiceStatus()
                        }
                    }
                }

                Timer {
                    id: restStatusRetryTimer
                    interval: 1500
                    repeat: false
                    onTriggered: configTab.refreshRestServerStatus()
                }

                function refreshServiceStatus() {
                    backend.getServiceStatus()
                    refreshRestServerStatus()
                }

                function refreshRestServerStatus() {
                    var port = (restPortField && restPortField.text && restPortField.text.length > 0) ? restPortField.text : tr("63118")
                    var useHttps = restHttpsCheck ? restHttpsCheck.checked : false
                    restServerChecking = true
                    backend.checkRestHealth("127.0.0.1:" + port, useHttps)
                }

                Connections {
                    target: backend
                    function onServiceStatusReceived(installed, running, platform, method) {
                        configTab.svcConnected = true
                        configTab.svcMessage = ""
                        configTab.svcInstalled = installed
                        configTab.svcRunning   = running
                        configTab.svcPlatform  = platform
                        configTab.svcMethod    = method
                    }
                    function onServiceActionFinished(ok, message) {
                        if (!ok && message && message.indexOf(tr("Sin conexión")) !== -1) {
                            configTab.svcConnected = false
                            configTab.svcMessage = tr("⚠ Desconectado de la interfaz IPC local (modo REST exclusivo, o motor parado).")
                        } else {
                            if (!ok) configTab.svcConnected = true
                            configTab.svcMessage = message
                        }
                        statusRetryTimer.start()
                        configTab.refreshRestServerStatus()
                    }
                    function onRestHealthChecked(running, message) {
                        configTab.restServerChecking = false
                        configTab.restServerRunning = running
                        if (!running && message && message.length > 0) {
                            configTab.svcMessage = tr("REST: ") + message
                        }
                    }
                    function onWebCompatibilityStateChanged(active, durationMinutes, message) {
                        configTab.webCompatibilityActive = active
                        configTab.svcMessage = message || ""
                        if (active && durationMinutes >= 5 && durationMinutes <= 240) {
                            window.webCompatibilityDurationMinutes = durationMinutes
                        }
                        restStatusRetryTimer.restart()
                    }
                }

                // Poll status when tab becomes active
                Connections {
                    target: window
                    function onActiveTabChanged() {
                        if (window.activeTab === "config") configTab.refreshServiceStatus()
                    }
                }

                Rectangle {
                    anchors.fill: parent
                    radius: 16
                    color: currentTheme.cardColor
                    border.color: Qt.rgba(1, 1, 1, currentTheme.borderOpacity)
                    border.width: 1

                    ScrollView {
                        id: configMainOuterScroll
                        anchors.fill: parent
                        leftPadding: 16
                        rightPadding: 26
                        topPadding: 16
                        bottomPadding: 26
                        // Solo desplazamiento vertical: las secciones se ajustan al ancho.
                        contentWidth: availableWidth
                        contentHeight: configMainOuterContent.implicitHeight
                        clip: true
                        ScrollBar.vertical.policy: ScrollBar.AsNeeded
                        ScrollBar.horizontal.policy: ScrollBar.AlwaysOff
                        ScrollBar.vertical.width: 18

                        ColumnLayout {
                        id: configMainOuterContent
                        width: configMainOuterScroll.availableWidth
                        spacing: 25

                        // Título y acciones: en ventanas estrechas las acciones bajan de línea.
                        GridLayout {
                            Layout.fillWidth: true
                            columns: configMainOuterScroll.availableWidth < 560 ? 1 : 2
                            columnSpacing: 12
                            rowSpacing: 8

                            Text {
                                text: tr("Configuración")
                                font.pixelSize: 32
                                font.bold: true
                                color: currentTheme.textColor
                                Layout.fillWidth: true
                                Layout.preferredWidth: 1
                                wrapMode: Text.WordWrap
                            }

                            AdaptiveRow {
                                Layout.preferredWidth: Math.min(parent.width, naturalWidth)
                                Layout.fillWidth: false
                                Layout.alignment: Qt.AlignRight | Qt.AlignVCenter
                                spacing: 12

                                Text {
                                    visible: window.backendSettingsDirty
                                    text: tr("Cambios sin guardar")
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 12
                                    height: 36
                                    verticalAlignment: Text.AlignVCenter
                                }

                                ThemedButton {
                                    text: tr("Guardar preferencias")
                                    enabled: window.backendSettingsDirty
                                    onClicked: saveBackendSettings()
                                }
                                ThemedButton {
                                    text: tr("Descartar cambios")
                                    visible: window.backendSettingsDirty
                                    onClicked: discardBackendSettingsChanges(false)
                                }
                            }
                        }

                        TokenSettingsPanel {
                            Layout.fillWidth: true
                            Layout.minimumWidth: 0
                            theme: currentTheme
                            bridge: backend
                            localIpc: typeof isIpcMode !== "undefined" && isIpcMode
                            translate: function(key) { return window.tr(key) }
                            onVisibleChanged: if (visible && !loaded && !busy) load()
                        }

                        // ── Preferencias de Usuario ─────────────────────────────────
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: prefCol.implicitHeight + 40
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1

                            ColumnLayout {
                                id: prefCol
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 15

                                Text { text: tr("⚙️  Preferencias Generales"); color: currentTheme.textColor; font.bold: true; font.pixelSize: 15 }

                                SettingsRowHighlight {
                                    visible: isIpcMode
                                    Text { text: tr("facturae.enable_label"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: facturaeToolsSwitch
                                        checked: window.facturaeToolsEnabled
                                        Accessible.name: tr("facturae.enable_label")
                                        Accessible.description: tr("facturae.enable_help")
                                        onToggled: {
                                            window.facturaeToolsEnabled = checked
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Binding { target: facturaeToolsSwitch; property: "checked"; value: window.facturaeToolsEnabled }
                                }

                                SettingsRowHighlight {
                                    visible: Qt.platform.os === "linux" && isIpcMode
                                    Text { text: tr("facturae.startup_label"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: startupSwitch
                                        checked: window.startupWithSession
                                        Accessible.name: tr("facturae.startup_label")
                                        Accessible.description: tr("facturae.startup_help")
                                        onClicked: {
                                            if (backend.setStartupEnabled(checked)) {
                                                window.startupWithSession = checked
                                                window.statusMessage = checked ? tr("facturae.startup_on") : tr("facturae.startup_off")
                                            } else {
                                                checked = window.startupWithSession
                                                window.statusMessage = tr("facturae.startup_error")
                                            }
                                        }
                                    }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Idioma"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ComboBox {
                                        id: languageCombo
                                        Layout.preferredWidth: Math.min(260, Math.max(120, parent.width * 0.45))
                                        model: (typeof i18n !== "undefined" && i18n) ? i18n.languages : []
                                        textRole: "name"
                                        function syncCurrentIndex() {
                                            for (let i = 0; i < model.length; ++i) {
                                                if (model[i].code === window.appLanguage) {
                                                    currentIndex = i
                                                    return
                                                }
                                            }
                                        }
                                        Component.onCompleted: syncCurrentIndex()
                                        onModelChanged: syncCurrentIndex()
                                        onActivated: function(index) {
                                            console.log("QML: languageCombo.onActivated", model[index].code)
                                            window.appLanguage = model[index].code
                                            markBackendSettingsDirty()
                                        }
                                    }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Tema visual"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ComboBox {
                                        id: settingsThemeCombo
                                        Layout.preferredWidth: Math.min(260, Math.max(120, parent.width * 0.45))
                                        model: themes.map(function(_, index) { return themeLabel(index) })
                                        currentIndex: window.currentThemeIndex
                                        // Al traducirse los nombres cambia el modelo y el combo volvía al primero.
                                        onModelChanged: currentIndex = Qt.binding(function() { return window.currentThemeIndex })
                                        onActivated: function(index) {
                                            window.currentThemeIndex = index
                                        }
                                    }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Modo Experto"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: expertModeSwitch
                                        checked: backend.expertMode
                                        onToggled: {
                                            backend.expertMode = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Habilita opciones avanzadas de diagnóstico y configuración.")
                                    }
                                    Binding { target: expertModeSwitch; property: "checked"; value: backend.expertMode }
                                }
                                
                                SettingsRowHighlight {
                                    Text { text: tr("Cerrar ventana tras firmar"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: autoCloseSwitch
                                        checked: window.autoClose
                                        onToggled: {
                                            window.autoClose = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Cierra la ventana de la aplicación automáticamente después de una firma exitosa.")
                                    }
                                    Binding { target: autoCloseSwitch; property: "checked"; value: window.autoClose }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Avisar de nuevas versiones"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: checkForUpdatesSwitch
                                        checked: window.checkForUpdates
                                        onToggled: {
                                            window.checkForUpdates = checked
                                            appSettings.checkForUpdatesCached = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Al iniciar, consulta una vez la última GitHub Release. Solo envía a GitHub la conexión HTTPS y la ruta del repositorio oficial; no envía documentos, certificados ni datos de firma.")
                                    }
                                    Binding { target: checkForUpdatesSwitch; property: "checked"; value: window.checkForUpdates }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Algoritmo de huella por defecto"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ComboBox {
                                        id: defaultHashAlgorithmCombo
                                        Layout.preferredWidth: Math.min(180, Math.max(120, parent.width * 0.45))
                                        model: hashAlgorithmOptions()
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.defaultHashAlgorithm)
                                        onActivated: function(index) {
                                            window.defaultHashAlgorithm = model[index].valor
                                            verifyTab.hashAlgorithm = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Define el algoritmo preseleccionado para crear y comprobar huellas digitales.")
                                    }
                                    Binding { target: defaultHashAlgorithmCombo; property: "currentIndex"; value: optionIndexByValue(defaultHashAlgorithmCombo.model, window.defaultHashAlgorithm) }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Copiar huella al portapapeles por defecto"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: defaultHashCopyToClipboardSwitch
                                        checked: window.defaultHashCopyToClipboard
                                        onToggled: {
                                            window.defaultHashCopyToClipboard = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, al generar una huella simple se copiará automáticamente al portapapeles.")
                                    }
                                    Binding { target: defaultHashCopyToClipboardSwitch; property: "checked"; value: window.defaultHashCopyToClipboard }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Formato de huella por defecto para ficheros"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ComboBox {
                                        id: defaultHashFormatFileCombo
                                        Layout.preferredWidth: Math.min(180, Math.max(120, parent.width * 0.45))
                                        model: hashFileFormatOptions()
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.defaultHashFormatFile)
                                        onActivated: function(index) {
                                            window.defaultHashFormatFile = model[index].valor
                                            verifyTab.hashFormatFile = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Define el formato preseleccionado al crear o comprobar huellas de ficheros.")
                                    }
                                    Binding { target: defaultHashFormatFileCombo; property: "currentIndex"; value: optionIndexByValue(defaultHashFormatFileCombo.model, window.defaultHashFormatFile) }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Formato de manifiesto por defecto para directorios"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ComboBox {
                                        id: defaultHashFormatDirectoryCombo
                                        Layout.preferredWidth: Math.min(180, Math.max(120, parent.width * 0.45))
                                        model: hashDirectoryFormatOptions()
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.defaultHashFormatDirectory)
                                        onActivated: function(index) {
                                            window.defaultHashFormatDirectory = model[index].valor
                                            verifyTab.hashFormatDirectory = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Define el formato preseleccionado al crear o comprobar manifiestos de directorio.")
                                    }
                                    Binding { target: defaultHashFormatDirectoryCombo; property: "currentIndex"; value: optionIndexByValue(defaultHashFormatDirectoryCombo.model, window.defaultHashFormatDirectory) }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Recursivo por defecto en directorios"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: defaultHashRecursiveSwitch
                                        checked: window.defaultHashRecursive
                                        onToggled: {
                                            window.defaultHashRecursive = checked
                                            verifyTab.hashRecursive = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, al crear o comprobar huellas de directorios se marcará por defecto el modo recursivo.")
                                    }
                                    Binding { target: defaultHashRecursiveSwitch; property: "checked"; value: window.defaultHashRecursive }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Guardar informe por defecto en directorios"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: defaultHashSaveReportSwitch
                                        checked: window.defaultHashSaveReport
                                        onToggled: {
                                            window.defaultHashSaveReport = checked
                                            verifyTab.hashSaveReport = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, al comprobar huellas de directorios se propondrá por defecto guardar el informe.")
                                    }
                                    Binding { target: defaultHashSaveReportSwitch; property: "checked"; value: window.defaultHashSaveReport }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Confirmar antes de firmar"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: confirmToSignSwitch
                                        checked: window.confirmToSign
                                        onToggled: {
                                            window.confirmToSign = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Pide confirmación explícita antes de lanzar una firma o una firma por lote.")
                                    }
                                    Binding { target: confirmToSignSwitch; property: "checked"; value: window.confirmToSign }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Omitir confirmación al cerrar con cambios sin guardar"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: omitAskOnCloseSwitch
                                        checked: window.omitAskOnClose
                                        onToggled: {
                                            window.omitAskOnClose = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, al cerrar con preferencias pendientes la aplicación intentará guardarlas y cerrará sin mostrar el diálogo de confirmación.")
                                    }
                                    Binding { target: omitAskOnCloseSwitch; property: "checked"; value: window.omitAskOnClose }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Dejar residente al cerrar"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: closeBehaviorSwitch
                                        checked: window.closeBehavior === "resident"
                                        onToggled: {
                                            window.closeBehavior = checked ? "resident" : "exit"
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, al pulsar cerrar la ventana la aplicación se minimizará y seguirá lista para futuras firmas.")
                                    }
                                    Binding { target: closeBehaviorSwitch; property: "checked"; value: window.closeBehavior === "resident" }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Recordar último certificado"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: stickySignerSwitch
                                        checked: window.stickySigner
                                        onToggled: {
                                            window.stickySigner = checked
                                            window.syncCertificateSelection(window.certificates)
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Al iniciar, selecciona automáticamente el último certificado usado para firmar.")
                                    }
                                    Binding { target: stickySignerSwitch; property: "checked"; value: window.stickySigner }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Autoseleccionar si solo hay un certificado"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: autoSelectSingleCertificateSwitch
                                        checked: window.autoSelectSingleCertificate
                                        onToggled: {
                                            window.autoSelectSingleCertificate = checked
                                            window.syncCertificateSelection(window.certificates)
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa y solo hay un certificado disponible, la aplicación lo seleccionará automáticamente al cargar el catálogo.")
                                    }
                                    Binding { target: autoSelectSingleCertificateSwitch; property: "checked"; value: window.autoSelectSingleCertificate }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Preferir certificado predeterminado al iniciar"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: preferDefaultCertificateSwitch
                                        checked: window.preferDefaultCertificate
                                        onToggled: {
                                            window.preferDefaultCertificate = checked
                                            window.syncCertificateSelection(window.certificates)
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, cuando exista un certificado marcado como predeterminado se elegirá antes que el último certificado recordado.")
                                    }
                                    Binding { target: preferDefaultCertificateSwitch; property: "checked"; value: window.preferDefaultCertificate }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Mostrar primero el certificado predeterminado"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: showDefaultCertificateFirstSwitch
                                        checked: window.showDefaultCertificateFirst
                                        onToggled: {
                                            window.showDefaultCertificateFirst = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, el certificado marcado como predeterminado aparecerá al principio de la lista cuando esté visible.")
                                    }
                                    Binding { target: showDefaultCertificateFirstSwitch; property: "checked"; value: window.showDefaultCertificateFirst }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Mostrar primero certificados aptos para firma"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: showUsableCertificatesFirstSwitch
                                        checked: window.showUsableCertificatesFirst
                                        onToggled: {
                                            window.showUsableCertificatesFirst = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, los certificados válidos para firma se mostrarán antes que los no utilizables dentro de la lista visible.")
                                    }
                                    Binding { target: showUsableCertificatesFirstSwitch; property: "checked"; value: window.showUsableCertificatesFirst }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Mostrar primero certificados vigentes"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: showValidCertificatesFirstSwitch
                                        checked: window.showValidCertificatesFirst
                                        onToggled: {
                                            window.showValidCertificatesFirst = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, los certificados no caducados se mostrarán antes que los caducados dentro de la lista visible.")
                                    }
                                    Binding { target: showValidCertificatesFirstSwitch; property: "checked"; value: window.showValidCertificatesFirst }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Recordar búsqueda de certificados"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: rememberCertificateFilterSwitch
                                        checked: window.rememberCertificateFilter
                                        onToggled: {
                                            window.rememberCertificateFilter = checked
                                            if (!checked) {
                                                window.certificateFilterText = ""
                                            }
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, el texto del buscador de certificados se conservará entre sesiones. Si se desactiva, el filtro se limpiará al salir.")
                                    }
                                    Binding { target: rememberCertificateFilterSwitch; property: "checked"; value: window.rememberCertificateFilter }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Mostrar certificados caducados"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: certsExpiredShowSwitch
                                        checked: window.certsExpiredShow
                                        onToggled: {
                                            window.certsExpiredShow = checked
                                            window.syncSelectionWithCurrentCertificateFilters()
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Muestra los certificados que ya han expirado en la lista de selección.")
                                    }
                                    Binding { target: certsExpiredShowSwitch; property: "checked"; value: window.certsExpiredShow }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Mostrar certificados no utilizables"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: certsInvalidShowSwitch
                                        checked: window.certsInvalidShow
                                        onToggled: {
                                            window.certsInvalidShow = checked
                                            window.syncSelectionWithCurrentCertificateFilters()
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Muestra certificados presentes en el almacén pero no aptos para firma. Es útil para diagnóstico, pero normalmente conviene ocultarlos.")
                                    }
                                    Binding { target: certsInvalidShowSwitch; property: "checked"; value: window.certsInvalidShow }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Usar solo certificados de firma"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: useOnlySignatureCertificatesSwitch
                                        checked: window.useOnlySignatureCertificates
                                        onToggled: {
                                            window.useOnlySignatureCertificates = checked
                                            window.syncSelectionWithCurrentCertificateFilters()
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Oculta los certificados presentes en el almacén que no son aptos para firmar. Si además activas la vista de diagnóstico, seguirán pudiendo verse desactivando este filtro.")
                                    }
                                    Binding { target: useOnlySignatureCertificatesSwitch; property: "checked"; value: window.useOnlySignatureCertificates }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Requerir NIF en el certificado"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: certificateRequireNifSwitch
                                        checked: window.certificateRequireNIF
                                        onToggled: {
                                            window.certificateRequireNIF = checked
                                            window.syncSelectionWithCurrentCertificateFilters()
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Oculta certificados que no aportan NIF o identificador equivalente en los datos extraídos por el catálogo.")
                                    }
                                    Binding { target: certificateRequireNifSwitch; property: "checked"; value: window.certificateRequireNIF }
                                }

                                SettingsRowHighlight {
                                    Text { text: tr("Requerir organización en el certificado"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: certificateRequireOrganizationSwitch
                                        checked: window.certificateRequireOrganization
                                        onToggled: {
                                            window.certificateRequireOrganization = checked
                                            window.syncSelectionWithCurrentCertificateFilters()
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Oculta certificados sin organización informada. Útil para priorizar representación, sello o certificados corporativos.")
                                    }
                                    Binding { target: certificateRequireOrganizationSwitch; property: "checked"; value: window.certificateRequireOrganization }
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 8

                                    Text {
                                        text: tr("Filtrar por tipo jurídico")
                                        color: currentTheme.textColor
                                        font.pixelSize: 13
                                    }

                                    Flow {
                                        Layout.fillWidth: true
                                        spacing: 10

                                        ThemedCheckBox {
                                            id: certTypeFisica
                                            text: tr("Persona física")
                                            checked: window.certificateTypeFilterContains("fisica")
                                            onToggled: {
                                                window.toggleCertificateTypeFilter("fisica", checked)
                                                window.syncSelectionWithCurrentCertificateFilters()
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: certTypeFisica; property: "checked"; value: window.certificateTypeFilterContains("fisica") }

                                        ThemedCheckBox {
                                            id: certTypeRepresentacion
                                            text: tr("Representación")
                                            checked: window.certificateTypeFilterContains("representacion")
                                            onToggled: {
                                                window.toggleCertificateTypeFilter("representacion", checked)
                                                window.syncSelectionWithCurrentCertificateFilters()
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: certTypeRepresentacion; property: "checked"; value: window.certificateTypeFilterContains("representacion") }

                                        ThemedCheckBox {
                                            id: certTypeSello
                                            text: tr("Sello")
                                            checked: window.certificateTypeFilterContains("sello")
                                            onToggled: {
                                                window.toggleCertificateTypeFilter("sello", checked)
                                                window.syncSelectionWithCurrentCertificateFilters()
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: certTypeSello; property: "checked"; value: window.certificateTypeFilterContains("sello") }

                                        ThemedCheckBox {
                                            id: certTypeEmpleadoPublico
                                            text: tr("Empleado público")
                                            checked: window.certificateTypeFilterContains("empleado_publico")
                                            onToggled: {
                                                window.toggleCertificateTypeFilter("empleado_publico", checked)
                                                window.syncSelectionWithCurrentCertificateFilters()
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: certTypeEmpleadoPublico; property: "checked"; value: window.certificateTypeFilterContains("empleado_publico") }

                                        ThemedCheckBox {
                                            id: certTypeDesconocido
                                            text: tr("Desconocido")
                                            checked: window.certificateTypeFilterContains("desconocido")
                                            onToggled: {
                                                window.toggleCertificateTypeFilter("desconocido", checked)
                                                window.syncSelectionWithCurrentCertificateFilters()
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: certTypeDesconocido; property: "checked"; value: window.certificateTypeFilterContains("desconocido") }
                                    }
                                }
                            }
                        }

                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: browserRepairCol.implicitHeight + 40
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor
                            border.width: 1

                            ColumnLayout {
                                id: browserRepairCol
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 12

                                Text {
                                    text: tr("🌐  Navegadores y certificados locales")
                                    color: currentTheme.textColor
                                    font.bold: true
                                    font.pixelSize: 15
                                }

                                Text {
                                    text: tr("Repara la integración con Chrome, Chromium, Edge, Brave y Firefox, incluyendo perfiles Snap o Flatpak cuando existan. Usa estas opciones si una web no consigue abrir GrxFirma o si el navegador no confía en la conexión local.")
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                    Layout.fillWidth: true
                                }

                                Flow {
                                    Layout.fillWidth: true
                                    spacing: 10

                                    ThemedButton {
                                        text: tr("Reinstalar conectores de navegadores")
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Vuelve a registrar afirma://, Native Messaging y perfiles Firefox detectados. Después reinicia el navegador.")
                                        onClicked: backend.reinstallBrowserConnectors()
                                    }

                                    ThemedButton {
                                        text: tr("Reinstalar certificados locales")
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Reinstala la confianza TLS local usada por la app para comunicarse con navegadores y servicios locales.")
                                        onClicked: backend.installPublicRoots()
                                    }

                                    ThemedButton {
                                        text: tr("Diagnóstico TLS")
                                        flat: true
                                        onClicked: backend.runTLSDiagnostics()
                                    }
                                }
                            }
                        }

                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: defaultSignCol.implicitHeight + 40
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1

                            ColumnLayout {
                                id: defaultSignCol
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 12

                                Text {
                                    text: tr("✍️  Firma por defecto")
                                    color: currentTheme.textColor
                                    font.bold: true
                                    font.pixelSize: 15
                                }

                                Text {
                                    text: tr("Estas preferencias se aplican al preparar una firma nueva si no cambias manualmente la operación, el formato o la política de salida.")
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                    Layout.fillWidth: true
                                }

                                GridLayout {
                                    columns: configMainOuterScroll.availableWidth < 560 ? 1 : Math.min(3, Math.max(1, Math.floor(configMainOuterScroll.availableWidth / 160)))
                                    columnSpacing: 10
                                    rowSpacing: 10
                                    Layout.fillWidth: true

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Operación por defecto"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ComboBox {
                                            id: settingsSignActionCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Firmar"), valor: "sign" },
                                                { texto: tr("Cofirmar"), valor: "cosign" },
                                                { texto: tr("Contrafirmar"), valor: "countersign" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: signActionIndex()
                                            onActivated: function(index) {
                                                signAction = model[index].valor
                                                markBackendSettingsDirty()
                                            }
                                            ToolTip.visible: hovered
                                            ToolTip.delay: 500
                                            ToolTip.text: tr("Selecciona la operación que aparecerá elegida al preparar una firma nueva.")
                                        }
                                        Binding { target: settingsSignActionCombo; property: "currentIndex"; value: signActionIndex() }
                                    }

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Formato por defecto"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ComboBox {
                                            id: settingsSignFormatCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Auto"), valor: "" },
                                                { texto: tr("PAdES"), valor: "pades" },
                                                { texto: tr("CAdES"), valor: "cades" },
                                                { texto: tr("XAdES"), valor: "xades" },
                                                { texto: tr("XMLdSig"), valor: "xmldsig" },
                                                { texto: tr("ODF"), valor: "odf" },
                                                { texto: tr("OOXML"), valor: "ooxml" },
                                                { texto: tr("FacturaE"), valor: "facturae" },
                                                { texto: tr("ASiC-XAdES"), valor: "asic-xades" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: signFormatIndex()
                                            onActivated: function(index) {
                                                signFormat = model[index].valor
                                                markBackendSettingsDirty()
                                            }
                                            ToolTip.visible: hovered
                                            ToolTip.delay: 500
                                            ToolTip.text: tr("Si no eliges un formato manualmente, esta preferencia se usará al preparar la firma.")
                                        }
                                        Binding { target: settingsSignFormatCombo; property: "currentIndex"; value: signFormatIndex() }
                                    }

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Salida si el fichero ya existe"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ComboBox {
                                            id: settingsSignOverwriteCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Renombrar"), valor: "rename" },
                                                { texto: tr("Error si existe"), valor: "fail" },
                                                { texto: tr("Forzar"), valor: "force" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: signOverwriteIndex()
                                            onActivated: function(index) {
                                                signOverwrite = model[index].valor
                                                markBackendSettingsDirty()
                                            }
                                            ToolTip.visible: hovered
                                            ToolTip.delay: 500
                                            ToolTip.text: tr("Define el comportamiento por defecto cuando el archivo de salida ya existe.")
                                        }
                                        Binding { target: settingsSignOverwriteCombo; property: "currentIndex"; value: signOverwriteIndex() }
                                    }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 10

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Perfil de firma por defecto"); color: currentTheme.textColor }
                                        ComboBox {
                                            id: settingsSignProfileCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Baseline B"), valor: "baseline" },
                                                { texto: tr("Baseline T"), valor: "t" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: optionIndexByValue(model, window.signProfile)
                                            onActivated: function(index) {
                                                window.signProfile = model[index].valor
                                                markBackendSettingsDirty()
                                            }
                                            ToolTip.visible: hovered
                                            ToolTip.delay: 500
                                            ToolTip.text: tr("Define el perfil preseleccionado para CAdES, XAdES y PAdES. El perfil T requiere TSA operativa.")
                                        }
                                        Binding { target: settingsSignProfileCombo; property: "currentIndex"; value: optionIndexByValue(settingsSignProfileCombo.model, window.signProfile) }
                                    }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    Text { text: tr("Firma visible (PAdES) por defecto"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: settingsSignVisibleSealSwitch
                                        onToggled: {
                                            if (window.applyingLoadedSettings) return
                                            window.signVisibleSeal = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, al preparar firmas PDF se propondrá por defecto la firma visible.")
                                    }
                                    Binding { target: settingsSignVisibleSealSwitch; property: "checked"; value: window.signVisibleSeal }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    Text { text: tr("Todas las páginas por defecto en firma visible"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: settingsSignSealAllPagesSwitch
                                        checked: window.signSealAllPages
                                        onToggled: {
                                            window.signSealAllPages = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, al preparar una firma visible PAdES se propondrá por defecto aplicarla en todas las páginas.")
                                    }
                                    Binding { target: settingsSignSealAllPagesSwitch; property: "checked"; value: window.signSealAllPages }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    Text { text: tr("Mantener texto sobre imagen por defecto"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: settingsSignSealKeepTextSwitch
                                        checked: window.signSealKeepText
                                        onToggled: {
                                            window.signSealKeepText = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Si se activa, al usar una imagen en la firma visible se conservará por defecto el texto superpuesto.")
                                    }
                                    Binding { target: settingsSignSealKeepTextSwitch; property: "checked"; value: window.signSealKeepText }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 10
                                    Text { text: tr("sign.seal.opacity"); color: currentTheme.textColor }
                                    Slider {
                                        id: settingsSignSealLogoOpacitySlider
                                        Layout.fillWidth: true
                                        from: 0
                                        to: 100
                                        stepSize: 1
                                        value: window.signSealLogoOpacityPercent
                                        focusPolicy: Qt.StrongFocus
                                        Accessible.name: tr("sign.seal.opacity")
                                        Accessible.description: tr("sign.seal.opacity_help")
                                        ToolTip.visible: hovered
                                        ToolTip.text: tr("sign.seal.opacity_help")
                                        onValueChanged: {
                                            if (!window.applyingLoadedSettings && Math.round(value) !== window.signSealLogoOpacityPercent)
                                                window.signSealLogoOpacityPercent = Math.round(value)
                                        }
                                    }
                                    Text { text: window.signSealLogoOpacityPercent + " %"; color: currentTheme.textColor }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    Text { text: tr("settings.seal_language.label"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ComboBox {
                                        id: settingsSealLanguageCombo
                                        Layout.preferredWidth: Math.min(260, Math.max(120, parent.width * 0.45))
                                        model: window.sealLanguageOptions()
                                        textRole: "name"
                                        Accessible.name: tr("settings.seal_language.label")
                                        Accessible.description: tr("settings.seal_language.help")
                                        function indexForCode(code) {
                                            for (let i = 0; i < model.length; ++i) {
                                                if (model[i].code === code) return i
                                            }
                                            return 0
                                        }
                                        onActivated: function(index) {
                                            if (window.applyingLoadedSettings) return
                                            window.signSealLanguage = model[index].code
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Binding { target: settingsSealLanguageCombo; property: "currentIndex"; value: settingsSealLanguageCombo.indexForCode(window.signSealLanguage) }
                                }

                                Text {
                                    Layout.fillWidth: true
                                    text: tr("settings.seal_language.help")
                                    color: currentTheme.secondaryTextColor
                                    font.pixelSize: 12
                                    wrapMode: Text.WordWrap
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    Text { text: tr("Rotación del sello visible por defecto"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ComboBox {
                                        id: settingsSignSealRotationCombo
                                        Layout.preferredWidth: Math.min(180, Math.max(120, parent.width * 0.45))
                                        model: [
                                            { texto: tr("0°"), valor: 0 },
                                            { texto: tr("90°"), valor: 90 },
                                            { texto: tr("180°"), valor: 180 },
                                            { texto: tr("270°"), valor: 270 }
                                        ]
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.signSealRotation)
                                        onActivated: function(index) {
                                            window.signSealRotation = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Define la rotación preseleccionada para la firma visible PAdES.")
                                    }
                                    Binding { target: settingsSignSealRotationCombo; property: "currentIndex"; value: optionIndexByValue(settingsSignSealRotationCombo.model, window.signSealRotation) }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 10

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Subfiltro PAdES por defecto"); color: currentTheme.textColor }
                                        ComboBox {
                                            id: settingsPadesSubFilterCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("ETSI CAdES detached"), valor: "etsi" },
                                                { texto: tr("Adobe PKCS#7 detached"), valor: "adobe" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: optionIndexByValue(model, window.padesSubFilter)
                                            onActivated: function(index) {
                                                window.padesSubFilter = model[index].valor
                                                markBackendSettingsDirty()
                                            }
                                            ToolTip.visible: hovered
                                            ToolTip.delay: 500
                                            ToolTip.text: tr("Define el subfiltro PAdES preseleccionado para firmas PDF cuando el backend lo soporte.")
                                        }
                                        Binding { target: settingsPadesSubFilterCombo; property: "currentIndex"; value: optionIndexByValue(settingsPadesSubFilterCombo.model, window.padesSubFilter) }
                                    }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 10

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Motivo de firma por defecto"); color: currentTheme.textColor }
                                        ThemedTextField {
                                            id: settingsSignReasonField
                                            Layout.fillWidth: true
                                            text: window.signReason
                                            placeholderText: tr("Ej.: Aprobación del documento")
                                            onTextChanged: {
                                                window.signReason = text
                                                markBackendSettingsDirty()
                                            }
                                            ToolTip.visible: hovered
                                            ToolTip.delay: 500
                                            ToolTip.text: tr("Define el motivo preseleccionado que se incrustará en la firma cuando proceda.")
                                        }
                                        Binding { target: settingsSignReasonField; property: "text"; value: window.signReason; when: !settingsSignReasonField.activeFocus }
                                    }
                                }

                                GridLayout {
                                    columns: configMainOuterScroll.availableWidth < 560 ? 1 : 2
                                    columnSpacing: 10
                                    rowSpacing: 10
                                    Layout.fillWidth: true

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Política FacturaE por defecto"); color: currentTheme.textColor }
                                        ComboBox {
                                            id: settingsFacturaePolicyVersionCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("FacturaE 3.1"), valor: "3.1" },
                                                { texto: tr("FacturaE 3.0"), valor: "3.0" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: optionIndexByValue(model, window.facturaePolicyVersion)
                                            onActivated: function(index) {
                                                window.facturaePolicyVersion = model[index].valor
                                                markBackendSettingsDirty()
                                            }
                                            ToolTip.visible: hovered
                                            ToolTip.delay: 500
                                            ToolTip.text: tr("Define la versión de política preseleccionada al firmar facturas FacturaE.")
                                        }
                                        Binding { target: settingsFacturaePolicyVersionCombo; property: "currentIndex"; value: optionIndexByValue(settingsFacturaePolicyVersionCombo.model, window.facturaePolicyVersion) }
                                    }

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Rol FacturaE por defecto"); color: currentTheme.textColor }
                                        ComboBox {
                                            id: settingsFacturaeSignerRoleCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Emisor"), valor: "emisor" },
                                                { texto: tr("Receptor"), valor: "receptor" },
                                                { texto: tr("Tercero"), valor: "tercero" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: optionIndexByValue(model, window.facturaeSignerRole)
                                            onActivated: function(index) {
                                                window.facturaeSignerRole = model[index].valor
                                                markBackendSettingsDirty()
                                            }
                                            ToolTip.visible: hovered
                                            ToolTip.delay: 500
                                            ToolTip.text: tr("Define el papel preseleccionado del firmante al generar firmas FacturaE.")
                                        }
                                        Binding { target: settingsFacturaeSignerRoleCombo; property: "currentIndex"; value: optionIndexByValue(settingsFacturaeSignerRoleCombo.model, window.facturaeSignerRole) }
                                    }
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 8

                                    Text {
                                        text: tr("Política FacturaE avanzada")
                                        color: currentTheme.textColor
                                        font.pixelSize: 13
                                        font.bold: true
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        text: tr("Opcional. Si informa identificador y hash, se enviarán al backend como política explícita de FacturaE.")
                                        color: currentTheme.secondaryTextColor
                                        font.pixelSize: 11
                                        wrapMode: Text.WordWrap
                                    }
                                    ThemedTextField {
                                        id: settingsFacturaePolicyIdField
                                        Layout.fillWidth: true
                                        text: window.facturaePolicyIdentifier
                                        placeholderText: tr("Identificador de política FacturaE")
                                        onTextChanged: {
                                            window.facturaePolicyIdentifier = text
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Binding { target: settingsFacturaePolicyIdField; property: "text"; value: window.facturaePolicyIdentifier; when: !settingsFacturaePolicyIdField.activeFocus }
                                    ThemedTextField {
                                        id: settingsFacturaePolicyHashField
                                        Layout.fillWidth: true
                                        text: window.facturaePolicyIdentifierHash
                                        placeholderText: tr("Hash Base64 de la política FacturaE")
                                        onTextChanged: {
                                            window.facturaePolicyIdentifierHash = text
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Binding { target: settingsFacturaePolicyHashField; property: "text"; value: window.facturaePolicyIdentifierHash; when: !settingsFacturaePolicyHashField.activeFocus }
                                    ThemedTextField {
                                        id: settingsFacturaePolicyQualifierField
                                        Layout.fillWidth: true
                                        text: window.facturaePolicyQualifier
                                        placeholderText: tr("Qualifier de política FacturaE (SPURI)")
                                        onTextChanged: {
                                            window.facturaePolicyQualifier = text
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Binding { target: settingsFacturaePolicyQualifierField; property: "text"; value: window.facturaePolicyQualifier; when: !settingsFacturaePolicyQualifierField.activeFocus }
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 8

                                    Text {
                                        text: tr("Lugar de firma FacturaE")
                                        color: currentTheme.textColor
                                        font.pixelSize: 13
                                        font.bold: true
                                    }
                                    Text {
                                        Layout.fillWidth: true
                                        text: tr("Opcional. Estos datos se enviarán como SignatureProductionPlace al firmar FacturaE.")
                                        color: currentTheme.secondaryTextColor
                                        font.pixelSize: 11
                                        wrapMode: Text.WordWrap
                                    }
                                    RowLayout {
                                        Layout.fillWidth: true
                                        spacing: 10
                                        ThemedTextField {
                                            id: settingsFacturaeCityField
                                            Layout.fillWidth: true
                                            text: window.facturaeSignatureCity
                                            placeholderText: tr("Ciudad")
                                            onTextChanged: {
                                                window.facturaeSignatureCity = text
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: settingsFacturaeCityField; property: "text"; value: window.facturaeSignatureCity; when: !settingsFacturaeCityField.activeFocus }
                                        ThemedTextField {
                                            id: settingsFacturaeProvinceField
                                            Layout.fillWidth: true
                                            text: window.facturaeSignatureProvince
                                            placeholderText: tr("Provincia")
                                            onTextChanged: {
                                                window.facturaeSignatureProvince = text
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: settingsFacturaeProvinceField; property: "text"; value: window.facturaeSignatureProvince; when: !settingsFacturaeProvinceField.activeFocus }
                                    }
                                    RowLayout {
                                        Layout.fillWidth: true
                                        spacing: 10
                                        ThemedTextField {
                                            id: settingsFacturaePostalCodeField
                                            Layout.fillWidth: true
                                            text: window.facturaeSignaturePostalCode
                                            placeholderText: tr("Código postal")
                                            onTextChanged: {
                                                window.facturaeSignaturePostalCode = text
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: settingsFacturaePostalCodeField; property: "text"; value: window.facturaeSignaturePostalCode; when: !settingsFacturaePostalCodeField.activeFocus }
                                        ThemedTextField {
                                            id: settingsFacturaeCountryField
                                            Layout.fillWidth: true
                                            text: window.facturaeSignatureCountry
                                            placeholderText: tr("País")
                                            onTextChanged: {
                                                window.facturaeSignatureCountry = text
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: settingsFacturaeCountryField; property: "text"; value: window.facturaeSignatureCountry; when: !settingsFacturaeCountryField.activeFocus }
                                    }
                                }
                            }
                        }

                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: autoFormatCol.implicitHeight + 40
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1

                            ColumnLayout {
                                id: autoFormatCol
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 12

                                Text {
                                    text: tr("🗂️  Formato automático por tipo de documento")
                                    color: currentTheme.textColor
                                    font.bold: true
                                    font.pixelSize: 15
                                }

                                Text {
                                    text: tr("Estas preferencias solo se usan cuando el formato por defecto está en Auto. Permiten acercar el comportamiento de V2 a la lógica por tipo documental de AutoFirma 1.9.")
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                    Layout.fillWidth: true
                                }

                                GridLayout {
                                    Layout.fillWidth: true
                                    columns: configMainOuterScroll.availableWidth < 560 ? 1 : 2
                                    rowSpacing: 12
                                    columnSpacing: 14

                                    Text { text: tr("PDF"); color: currentTheme.textColor }
                                    ComboBox {
                                        id: autoFormatPdfCombo
                                        Layout.fillWidth: true
                                        model: [
                                            { texto: tr("PAdES"), valor: "pades" },
                                            { texto: tr("CAdES"), valor: "cades" }
                                        ]
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.autoFormatPdf)
                                        onActivated: function(index) {
                                            window.autoFormatPdf = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                    }

                                    Text { text: tr("OOXML"); color: currentTheme.textColor }
                                    ComboBox {
                                        id: autoFormatOoxmlCombo
                                        Layout.fillWidth: true
                                        model: [
                                            { texto: tr("OOXML"), valor: "ooxml" },
                                            { texto: tr("CAdES"), valor: "cades" }
                                        ]
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.autoFormatOoxml)
                                        onActivated: function(index) {
                                            window.autoFormatOoxml = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                    }

                                    Text { text: tr("FacturaE"); color: currentTheme.textColor }
                                    ComboBox {
                                        id: autoFormatFacturaeCombo
                                        Layout.fillWidth: true
                                        model: [
                                            { texto: tr("FacturaE"), valor: "facturae" },
                                            { texto: tr("XAdES"), valor: "xades" }
                                        ]
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.autoFormatFacturae)
                                        onActivated: function(index) {
                                            window.autoFormatFacturae = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                    }

                                    Text { text: tr("ODF"); color: currentTheme.textColor }
                                    ComboBox {
                                        id: autoFormatOdfCombo
                                        Layout.fillWidth: true
                                        model: [
                                            { texto: tr("ODF"), valor: "odf" },
                                            { texto: tr("CAdES"), valor: "cades" }
                                        ]
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.autoFormatOdf)
                                        onActivated: function(index) {
                                            window.autoFormatOdf = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                    }

                                    Text { text: tr("XML"); color: currentTheme.textColor }
                                    ComboBox {
                                        id: autoFormatXmlCombo
                                        Layout.fillWidth: true
                                        model: [
                                            { texto: tr("XAdES"), valor: "xades" },
                                            { texto: tr("XMLdSig"), valor: "xmldsig" }
                                        ]
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.autoFormatXml)
                                        onActivated: function(index) {
                                            window.autoFormatXml = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                    }

                                    Text { text: tr("Binario / otros"); color: currentTheme.textColor }
                                    ComboBox {
                                        id: autoFormatBinaryCombo
                                        Layout.fillWidth: true
                                        model: [
                                            { texto: tr("CAdES"), valor: "cades" },
                                            { texto: tr("ASiC-XAdES"), valor: "asic-xades" }
                                        ]
                                        textRole: "texto"
                                        currentIndex: optionIndexByValue(model, window.autoFormatBinary)
                                        onActivated: function(index) {
                                            window.autoFormatBinary = model[index].valor
                                            markBackendSettingsDirty()
                                        }
                                    }
                                }
                            }
                        }

                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: signPolicyCol.implicitHeight + 40
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1

                            ColumnLayout {
                                id: signPolicyCol
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 12

                                Text {
                                    text: tr("🛡️  Política de firma por defecto")
                                    color: currentTheme.textColor
                                    font.bold: true
                                    font.pixelSize: 15
                                }

                                Text {
                                    text: tr("Estas preferencias se aplican al construir el payload de firma. Sirven para fijar un baseline más compatible o más permisivo según el entorno.")
                                    color: currentTheme.secondaryTextColor
                                    wrapMode: Text.WordWrap
                                    Layout.fillWidth: true
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    Text { text: tr("Compatibilidad estricta"); color: currentTheme.textColor; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: settingsSignStrictCompatSwitch
                                        checked: window.signStrictCompat
                                        onToggled: {
                                            window.signStrictCompat = checked
                                            markBackendSettingsDirty()
                                        }
                                        ToolTip.visible: hovered
                                        ToolTip.delay: 500
                                        ToolTip.text: tr("Activa perfiles de firma más restrictivos para maximizar la compatibilidad con administraciones públicas y plataformas heredadas.")
                                    }
                                    Binding { target: settingsSignStrictCompatSwitch; property: "checked"; value: window.signStrictCompat }
                                }

                            }
                        }

                        // ── TSA (Sellado de Tiempo) ─────────────────────────────────
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: tsaCol.implicitHeight + 40
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1

                            ColumnLayout {
                                id: tsaCol
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 12

                                RowLayout {
                                    Layout.fillWidth: true
                                    Text { text: tr("⏳  Sellado de Tiempo (TSA)"); color: currentTheme.textColor; font.bold: true; font.pixelSize: 15; Layout.fillWidth: true; ToolTip.text: tr("Habilita el uso de un servidor de sellado de tiempo para añadir una marca de tiempo a las firmas.") ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: tsaEnabledSwitch
                                        checked: window.tsaEnabled
                                        onToggled: {
                                            window.tsaEnabled = checked
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Binding { target: tsaEnabledSwitch; property: "checked"; value: window.tsaEnabled }
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    enabled: window.tsaEnabled
                                    opacity: enabled ? 1.0 : 0.5
                                    spacing: 12

                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        spacing: 4
                                        Text { text: tr("Servidor TSA:"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedComboBox {
                                            id: tsaCombo
                                            Layout.fillWidth: true
                                            editable: true
                                            hasError: window.settingsFieldError("tsa") !== ""
                                            errorColor: currentTheme.errorColor
                                            model: [
                                                "http://tsa.fnmt.es/",
                                                "http://tsa.accv.es/",
                                                "http://tsa.catcert.net/",
                                                "http://tsa.camerfirma.com/",
                                                "http://tsa.izenpe.com/"
                                            ]
                                            onActivated: {
                                                window.tsaUrl = editText
                                                markBackendSettingsDirty()
                                            }
                                            onEditTextChanged: {
                                                window.tsaUrl = editText
                                                markBackendSettingsDirty()
                                                if (window.settingsFieldError("tsa")) window.validateSettingsField("tsa")
                                            }
                                            onActiveFocusChanged: { if (!activeFocus) window.validateSettingsField("tsa") }
                                            Accessible.description: window.settingsFieldError("tsa") ? tr(window.settingsFieldError("tsa")) : ""
                                            Component.onCompleted: {
                                                editText = window.tsaUrl
                                            }
                                            Connections {
                                                target: window
                                                function onTsaUrlChanged() {
                                                    if (tsaCombo.editText !== window.tsaUrl) {
                                                        tsaCombo.editText = window.tsaUrl
                                                    }
                                                }
                                            }
                                        }
                                        Text { Layout.fillWidth: true; visible: window.settingsFieldError("tsa") !== ""; text: "⚠ " + tr(window.settingsFieldError("tsa")); color: currentTheme.errorColor; wrapMode: Text.WordWrap }
                                    }

                                    ThemedButton {
                                        Layout.fillWidth: true
                                        text: tr("🛡️ Instalar confianza TLS local")
                                        palette.button: currentTheme.primaryColor; palette.buttonText: "white"
                                        onClicked: backend.installCamerfirmaCerts()
                                        ToolTip.visible: hovered
                                        ToolTip.text: tr("Instala la confianza del certificado HTTPS local para abrir la consola web REST sin avisos del navegador.")
                                    }
                                }
                            }
                        }

                        // ── Proxy ─────────────────────────────────
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: proxyCol.implicitHeight + 40
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1

                            ColumnLayout {
                                id: proxyCol
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 12

                                RowLayout {
                                    Layout.fillWidth: true
                                    Text { text: tr("🌐  Configuración de Proxy"); color: currentTheme.textColor; font.bold: true; font.pixelSize: 15; Layout.fillWidth: true; ToolTip.text: tr("Habilita el uso de un servidor proxy para las conexiones de red.") ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                    ThemedSwitch {
                                        id: proxyEnabledSwitch
                                        checked: window.proxyEnabled
                                        onToggled: {
                                            window.proxyEnabled = checked
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Binding { target: proxyEnabledSwitch; property: "checked"; value: window.proxyEnabled }
                                }

                                GridLayout {
                                    columns: configMainOuterScroll.availableWidth < 560 ? 1 : Math.min(3, Math.max(1, Math.floor(configMainOuterScroll.availableWidth / 160)))
                                    columnSpacing: 10
                                    rowSpacing: 10
                                    Layout.fillWidth: true
                                    enabled: window.proxyEnabled
                                    opacity: enabled ? 1.0 : 0.5
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Tipo:"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ComboBox {
                                            id: proxyTypeCombo
                                            Layout.fillWidth: true
                                            model: [
                                                { texto: tr("Ninguno"), valor: "none" },
                                                { texto: tr("Manual"), valor: "manual" }
                                            ]
                                            textRole: "texto"
                                            currentIndex: optionIndexByValue(model, window.proxyType)
                                            onActivated: function(index) {
                                                window.proxyType = model[index].valor
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Binding { target: proxyTypeCombo; property: "currentIndex"; value: optionIndexByValue(proxyTypeCombo.model, window.proxyType) }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Host / IP:"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: proxyHostField
                                            Layout.fillWidth: true
                                            text: window.proxyHost
                                            hasError: window.settingsFieldError("proxyHost") !== ""
                                            errorColor: currentTheme.errorColor
                                            Accessible.description: window.settingsFieldError("proxyHost") ? tr(window.settingsFieldError("proxyHost")) : ""
                                            onTextChanged: {
                                                if (window.settingsFieldError("proxyHost")) {
                                                    window.proxyHost = text
                                                    window.validateSettingsField("proxyHost")
                                                }
                                            }
                                            onEditingFinished: {
                                                window.proxyHost = text
                                                window.validateSettingsField("proxyHost")
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Text { Layout.fillWidth: true; visible: window.settingsFieldError("proxyHost") !== ""; text: "⚠ " + tr(window.settingsFieldError("proxyHost")); color: currentTheme.errorColor; wrapMode: Text.WordWrap }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Puerto:"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: proxyPortField
                                            Layout.fillWidth: true
                                            text: window.proxyPort.toString()
                                            validator: IntValidator { bottom: 1; top: 65535 }
                                            hasError: window.settingsFieldError("proxyPort") !== ""
                                            errorColor: currentTheme.errorColor
                                            Accessible.description: window.settingsFieldError("proxyPort") ? tr(window.settingsFieldError("proxyPort")) : ""
                                            onTextChanged: {
                                                if (window.settingsFieldError("proxyPort")) {
                                                    window.proxyPort = parseInt(text)
                                                    window.validateSettingsField("proxyPort")
                                                }
                                            }
                                            onEditingFinished: {
                                                window.proxyPort = parseInt(text)
                                                window.validateSettingsField("proxyPort")
                                                markBackendSettingsDirty()
                                            }
                                        }
                                        Text { Layout.fillWidth: true; visible: window.settingsFieldError("proxyPort") !== ""; text: "⚠ " + tr(window.settingsFieldError("proxyPort")); color: currentTheme.errorColor; wrapMode: Text.WordWrap }
                                    }
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    enabled: window.proxyEnabled
                                    opacity: enabled ? 1.0 : 0.5
                                    spacing: 4

                                    Text { text: tr("URLs excluidas del proxy:"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                    ThemedTextArea {
                                        id: proxyExcludedUrlsArea
                                        Layout.fillWidth: true
                                        Layout.preferredHeight: 88
                                        wrapMode: TextEdit.WrapAnywhere
                                        textFormat: TextEdit.PlainText
                                        placeholderText: tr("Una URL o patrón por línea, por ejemplo:\nlocalhost\n127.0.0.1\n*.dipgra.es")
                                        text: window.proxyExcludedUrlsText()
                                        onTextChanged: {
                                            window.proxyExcludedUrls = parseProxyExcludedUrlsText(text)
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Binding {
                                        target: proxyExcludedUrlsArea
                                        property: "text"
                                        value: window.proxyExcludedUrlsText()
                                        when: !proxyExcludedUrlsArea.activeFocus
                                    }
                                }

                                Rectangle {
                                    Layout.fillWidth: true
                                    implicitHeight: visible ? proxyCredentialsCol.implicitHeight + 24 : 0
                                    visible: isIpcMode && window.proxyEnabled && window.proxyType === "manual"
                                    radius: 8
                                    color: currentTheme.cardColor
                                    border.color: currentTheme.secondaryTextColor
                                    border.width: 1

                                    ColumnLayout {
                                        id: proxyCredentialsCol
                                        anchors.fill: parent
                                        anchors.margins: 12
                                        spacing: 8

                                        RowLayout {
                                            Layout.fillWidth: true
                                            Text {
                                                wrapMode: Text.WordWrap
                                                Layout.preferredWidth: 1
                                                Layout.fillWidth: true
                                                text: tr("Autenticación")
                                                color: currentTheme.textColor
                                                font.bold: true
                                            }
                                            Text {
                                                visible: window.proxyCredentialsConfigured
                                                text: tr("Proxy manual con credenciales protegidas")
                                                color: "#2ecc71"
                                                font.pixelSize: 11
                                            }
                                        }

                                        Text {
                                            Layout.fillWidth: true
                                            wrapMode: Text.Wrap
                                            text: tr("El almacén seguro de secretos de proxy está disponible para este backend local.")
                                            color: currentTheme.secondaryTextColor
                                            font.pixelSize: 11
                                        }

                                        GridLayout {
                                            Layout.fillWidth: true
                                            columns: 3
                                            columnSpacing: 10
                                            rowSpacing: 4
                                            enabled: window.proxySecretStoreAvailable && !window.proxyCredentialBusy
                                            opacity: enabled ? 1.0 : 0.5

                                            Text { text: tr("Dominio"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                            Text { text: tr("Usuario"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                            Text { text: tr("Contraseña"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }

                                            ThemedTextField {
                                                id: proxyRealmField
                                                Layout.fillWidth: true
                                                maximumLength: 256
                                                text: window.proxyCredentialRealm
                                                onTextEdited: window.proxyCredentialRealm = text
                                            }
                                            ThemedTextField {
                                                id: proxyUsernameField
                                                Layout.fillWidth: true
                                                maximumLength: 256
                                                text: window.proxyCredentialUsername
                                                onTextEdited: window.proxyCredentialUsername = text
                                            }
                                            ThemedTextField {
                                                id: proxyPasswordField
                                                Layout.fillWidth: true
                                                maximumLength: 4096
                                                echoMode: TextInput.Password
                                                placeholderText: tr("Contraseña...")
                                            }
                                        }

                                        RowLayout {
                                            Layout.fillWidth: true
                                            spacing: 8
                                            ThemedButton {
                                                text: tr("Guardar cambios")
                                                enabled: window.proxySecretStoreAvailable
                                                         && !window.proxyCredentialBusy
                                                         && proxyRealmField.text.trim() !== ""
                                                         && proxyUsernameField.text.trim() !== ""
                                                         && proxyPasswordField.text.length > 0
                                                onClicked: {
                                                    window.proxyCredentialBusy = true
                                                    backend.storeProxyCredentials(
                                                                proxyRealmField.text,
                                                                proxyUsernameField.text,
                                                                proxyPasswordField.text)
                                                    proxyPasswordField.clear()
                                                }
                                            }
                                            ThemedButton {
                                                text: tr("Quitar")
                                                enabled: window.proxySecretStoreAvailable
                                                         && window.proxyCredentialsConfigured
                                                         && !window.proxyCredentialBusy
                                                onClicked: {
                                                    window.proxyCredentialBusy = true
                                                    proxyPasswordField.clear()
                                                    backend.deleteProxyCredentials()
                                                }
                                            }
                                            BusyIndicator {
                                                running: window.proxyCredentialBusy
                                                visible: running
                                                implicitWidth: 28
                                                implicitHeight: 28
                                            }
                                            Item { Layout.fillWidth: true }
                                        }
                                    }
                                }

                                Rectangle {
                                    Layout.fillWidth: true
                                    implicitHeight: proxySecretStatusCol.implicitHeight + 16
                                    radius: 8
                                    color: window.proxySecretStoreAvailable ? "#1a4a1a" : "#2a2230"
                                    border.color: window.proxySecretStoreAvailable ? "#2ecc71" : currentTheme.primaryColor
                                    border.width: 1

                                    ColumnLayout {
                                        id: proxySecretStatusCol
                                        anchors.fill: parent
                                        anchors.margins: 12
                                        spacing: 4

                                        Text {
                                            wrapMode: Text.WordWrap
                                            Layout.fillWidth: true
                                            color: currentTheme.textColor
                                            font.bold: true
                                            text: window.proxySecretStoreAvailable
                                                  ? tr("Almacén seguro del proxy disponible")
                                                  : tr("Almacén seguro del proxy no disponible")
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            wrapMode: Text.Wrap
                                            color: currentTheme.secondaryTextColor
                                            text: tr("Backend:") + " " + window.proxySecretStoreBackendLabel(window.proxySecretStoreBackend)
                                                  + (window.proxySecretStorePlatform !== "" ? " · " + tr("Plataforma:") + " " + window.proxySecretStorePlatformLabel(window.proxySecretStorePlatform) : "")
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            visible: window.proxyRuntimeMode !== ""
                                            wrapMode: Text.Wrap
                                            color: currentTheme.secondaryTextColor
                                            text: tr("Modo en uso:") + " " + window.proxyRuntimeModeLabel(window.proxyRuntimeMode)
                                        }
                                        Text {
                                            Layout.fillWidth: true
                                            visible: window.proxySecretStoreReason !== ""
                                            wrapMode: Text.Wrap
                                            color: currentTheme.secondaryTextColor
                                            text: window.proxySecretStoreReason
                                        }
                                        RowLayout {
                                            Layout.fillWidth: true
                                            spacing: 8
                                            ThemedButton {
                                                text: tr("Actualizar estado")
                                                onClicked: backend.getProxySecretStoreStatus()
                                            }
                                        }
                                    }
                                }
                            }
                        }

                        // ── Servidor API REST Local ─────────────────
                        Rectangle {
                            Layout.fillWidth: true
                            implicitHeight: restSrvCol.implicitHeight + 40
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1

                            ColumnLayout {
                                id: restSrvCol
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 12

                                Text { text: tr("🌐  Servidor API REST Local"); color: currentTheme.textColor; font.bold: true; font.pixelSize: 15; Layout.fillWidth: true; ToolTip.text: tr("Expone la API REST solo en este equipo, para integraciones locales y consola web.") ; wrapMode: Text.WordWrap }
                                
                                // Estado actual del API REST
                                Rectangle {
                                    Layout.fillWidth: true; height: 44; radius: 8
                                    color: configTab.restServerChecking ? "#1f2937" : (configTab.restServerRunning ? "#1a4a1a" : "#2a0a0a")
                                    border.color: configTab.restServerChecking ? "#9ca3af" : (configTab.restServerRunning ? "#2ecc71" : "#e74c3c")
                                    border.width: 1

                                    RowLayout {
                                        anchors.fill: parent; anchors.margins: 12; spacing: 10
                                        Text {
                                            wrapMode: Text.WordWrap
                                            Layout.preferredWidth: 1
                                            text: configTab.restServerChecking ? tr("● Comprobando estado del servidor...") : (configTab.restServerRunning ? tr("● Servidor API REST en ejecución") : tr("● Servidor detenido"))
                                            color: configTab.restServerChecking ? "#d1d5db" : (configTab.restServerRunning ? "#2ecc71" : "#e74c3c")
                                            font.bold: true; font.pixelSize: 13; Layout.fillWidth: true
                                        }
                                    }
                                }
                                GridLayout {
                                    columns: configMainOuterScroll.availableWidth < 560 ? 1 : 2
                                    columnSpacing: 10
                                    rowSpacing: 10
                                    Layout.fillWidth: true
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Text { text: tr("Puerto:"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: restPortField
                                            Layout.fillWidth: true
                                            text: tr("63118")
                                            validator: IntValidator { bottom: 1024; top: 65535 }
                                        }
                                    }
                                    ColumnLayout {
                                        Layout.fillWidth: true
                                        Layout.preferredWidth: Math.min(300, Math.max(120, parent.width * 0.45))
                                        Text { text: tr("Token de Seguridad:"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                        ThemedTextField {
                                            id: restTokenField
                                            Layout.fillWidth: true
                                            placeholderText: tr("Opcional: token de acceso bearer")
                                        }
                                    }
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    spacing: 4
                                    Text { text: tr("Huellas de Certificados Cliente (SHA-256 CSV):"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                    ThemedTextField {
                                        id: restFingerprintsField
                                        Layout.fillWidth: true
                                        placeholderText: tr("Opcional: 6F:..., 8A:...")
                                    }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    ThemedCheckBox {
                                        id: restHttpsCheck
                                        text: tr("Habilitar HTTPS local")
                                        checked: true
                                    }
                                }

                                RowLayout {
                                    Layout.fillWidth: true
                                    spacing: 10
                                    Text {
                                        text: tr("Duración temporal (minutos):")
                                        color: currentTheme.secondaryTextColor
                                        font.pixelSize: 12
                                    }
                                    ThemedTextField {
                                        id: webCompatibilityDurationField
                                        Layout.preferredWidth: 90
                                        text: String(window.webCompatibilityDurationMinutes)
                                        enabled: !configTab.webCompatibilityActive
                                        validator: IntValidator { bottom: 5; top: 240 }
                                        onEditingFinished: {
                                            const duration = Number(text)
                                            if (!Number.isInteger(duration) || duration < 5 || duration > 240) {
                                                text = String(window.webCompatibilityDurationMinutes)
                                                return
                                            }
                                            window.webCompatibilityDurationMinutes = duration
                                            markBackendSettingsDirty()
                                        }
                                    }
                                    Text {
                                        Layout.preferredWidth: 1
                                        Layout.fillWidth: true
                                        text: tr("Se apagará automáticamente; el estado activo no se guarda.")
                                        color: currentTheme.secondaryTextColor
                                        font.pixelSize: 11
                                        wrapMode: Text.Wrap
                                    }
                                }
                                
                                AdaptiveRow {
                                    Layout.fillWidth: true; spacing: 10
                                     ThemedButton {
                                        text: tr("Iniciar servidor")
                                        palette.button: currentTheme.primaryColor; palette.buttonText: "white"
                                        onClicked: {
                                            configTab.restServerChecking = true
                                            if (!backend || typeof backend.startTemporaryWebCompatibility !== "function") {
                                                configTab.restServerChecking = false
                                                configTab.svcMessage = tr("No se pudo activar la compatibilidad web temporal.")
                                                return
                                            }
                                            backend.startTemporaryWebCompatibility(
                                                "127.0.0.1:" + restPortField.text,
                                                restTokenField.text,
                                                restFingerprintsField.text,
                                                restHttpsCheck.checked,
                                                window.webCompatibilityDurationMinutes)
                                            restStatusRetryTimer.restart()
                                        }
                                    }
                                    ThemedButton {
                                        text: tr("Detener")
                                        onClicked: {
                                            if (!configTab.webCompatibilityActive ||
                                                    !backend ||
                                                    typeof backend.stopWebCompatibility !== "function") {
                                                configTab.svcMessage = tr("ℹ El servidor REST detectado está activo, pero no fue iniciado por esta ventana. Deténgalo desde el tray o desde la instancia que lo arrancó.")
                                                backend.updateStatus(tr("Servidor REST externo detectado: no se puede detener desde esta ventana"))
                                                configTab.refreshRestServerStatus()
                                                return
                                            }
                                            configTab.restServerChecking = true
                                            backend.stopWebCompatibility()
                                            restStatusRetryTimer.restart()
                                        }
                                    }
                                    ThemedButton {
                                        text: tr("Abrir web")
                                        icon.name: "applications-internet"
                                        enabled: configTab.restServerRunning
                                        onClicked: {
                                            var protocol = restHttpsCheck.checked ? "https://" : "http://"
                                            var port = (restPortField.text || "63118").trim()
                                            var targetUrl = protocol + "127.0.0.1:" + port + "/"
                                            console.log(tr("Intentando abrir web en:"), targetUrl)
                                            backend.updateStatus(tr("Abriendo consola web REST en ") + targetUrl)
                                            var opened = false
                                            try {
                                                opened = Qt.openUrlExternally(targetUrl)
                                            } catch (e) {
                                                console.log(tr("Qt.openUrlExternally falló:"), e)
                                            }
                                            if (!opened) {
                                                console.log(tr("Fallback backend.openExternal para:"), targetUrl)
                                                backend.openExternal(targetUrl)
                                            }
                                        }
                                    }
                                }

                                Component.onCompleted: configTab.refreshRestServerStatus()
                            }
                        }

                        // ── Servicio del sistema ──────────────────────────
                        Rectangle {
                            Layout.fillWidth: true
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1
                            height: svcColumn.implicitHeight + 40

                            ColumnLayout {
                                id: svcColumn
                                anchors { top: parent.top; left: parent.left; right: parent.right; margins: 20 }
                                spacing: 16

                                Text {
                                    text: tr("🔧  Servicio de Usuario al Inicio de Sesión")
                                    color: currentTheme.textColor; font.bold: true; font.pixelSize: 15
                                }

                                Text {
                                    text: tr("Instala el motor de firma como servicio de tu usuario (no de sistema),\npara que arranque automáticamente y tenga acceso a tus certificados personales.")
                                    color: currentTheme.secondaryTextColor; font.pixelSize: 12
                                    wrapMode: Text.Wrap; Layout.fillWidth: true
                                }

                                 // Estado actual
                                Rectangle {
                                    Layout.fillWidth: true; height: 44; radius: 8
                                    color: !configTab.svcConnected ? "#333333" : (configTab.svcRunning ? "#1a4a1a" : (configTab.svcInstalled ? "#4a3a0a" : "#2a0a0a"))
                                    border.color: !configTab.svcConnected ? "#aaaaaa" : (configTab.svcRunning ? "#2ecc71" : (configTab.svcInstalled ? "#f39c12" : "#e74c3c"))
                                    border.width: 1

                                    RowLayout {
                                        anchors.fill: parent; anchors.margins: 12; spacing: 10
                                        Text {
                                            wrapMode: Text.WordWrap
                                            Layout.preferredWidth: 1
                                            text: !configTab.svcConnected ? tr("● Estado desconocido (Sin conexión)") :
                                                  configTab.svcRunning ? tr("● Servicio activo y corriendo") :
                                                  configTab.svcInstalled ? tr("● Servicio instalado pero parado") : tr("● Servicio no instalado")
                                            color: !configTab.svcConnected ? "#aaaaaa" : (configTab.svcRunning ? "#2ecc71" : (configTab.svcInstalled ? "#f39c12" : "#e74c3c"))
                                            font.bold: true; font.pixelSize: 13; Layout.fillWidth: true
                                        }
                                        Text {
                                            text: configTab.svcConnected && configTab.svcMethod ? "(" + configTab.svcMethod + ")" : ""
                                            color: currentTheme.secondaryTextColor; font.pixelSize: 11
                                        }
                                    }
                                }

                                // Botones de acción
                                Flow {
                                    Layout.fillWidth: true; spacing: 10

                                     ThemedButton {
                                        text: tr("Instalar servicio")
                                        visible: !configTab.svcInstalled
                                        enabled: configTab.svcConnected
                                        palette.button: currentTheme.primaryColor; palette.buttonText: "white"
                                        onClicked: backend.installService()
                                    }
                                    ThemedButton {
                                        text: tr("Desinstalar servicio")
                                        visible: configTab.svcInstalled
                                        enabled: configTab.svcConnected
                                        palette.button: "#c0392b"; palette.buttonText: "white"
                                        onClicked: backend.uninstallService()
                                    }
                                    ThemedButton {
                                        text: tr("Arrancar")
                                        visible: configTab.svcInstalled && !configTab.svcRunning
                                        enabled: configTab.svcConnected
                                        palette.button: "#27ae60"; palette.buttonText: "white"
                                        onClicked: backend.startService()
                                    }
                                    ThemedButton {
                                        text: tr("Detener")
                                        visible: configTab.svcInstalled && configTab.svcRunning
                                        enabled: configTab.svcConnected
                                        palette.button: "#e67e22"; palette.buttonText: "white"
                                        onClicked: backend.stopService()
                                    }
                                    ThemedButton {
                                        text: tr("↻ Actualizar estado")
                                        flat: true
                                        onClicked: configTab.refreshServiceStatus()
                                    }
                                }

                                // Mensaje de resultado
                                Text {
                                    text: configTab.svcMessage
                                    color: configTab.svcMessage.startsWith(tr("Error")) ? "#e74c3c" : "#2ecc71"
                                    font.pixelSize: 12; wrapMode: Text.Wrap
                                    Layout.fillWidth: true
                                    visible: configTab.svcMessage !== ""
                                }

                                // Nota importante
                                Rectangle {
                                    Layout.fillWidth: true; height: noteText.implicitHeight + 20
                                    radius: 8; color: "#1a1a0a"
                                    border.color: "#f39c12"; border.width: 1

                                    Text {
                                        id: noteText
                                        anchors { fill: parent; margins: 10 }
                                        text: tr("⚠ Importante: se instala como servicio de tu sesión de usuario (no como servicio de sistema), para que tenga acceso a tus certificados del almacén personal. Los servicios de sistema no pueden acceder a los certificados del usuario.")
                                        color: "#f39c12"; font.pixelSize: 11; wrapMode: Text.Wrap
                                    }
                                }
                            }
                        }
                        // ── Restaurar Parámetros ─────────────────────────────────
                        Rectangle {
                            Layout.fillWidth: true
                            height: 70
                            radius: 12
                            color: currentTheme.cardColor
                            border.color: currentTheme.primaryColor; border.width: 1

                            RowLayout {
                                anchors.fill: parent; anchors.margins: 20; spacing: 15
                                Text { text: tr("↺  Valores por defecto"); color: currentTheme.textColor; font.bold: true; font.pixelSize: 15; Layout.fillWidth: true ; wrapMode: Text.WordWrap ; Layout.preferredWidth: 1 }
                                Text { text: tr("Restaura el tema y opciones a fábrica"); color: currentTheme.secondaryTextColor; font.pixelSize: 12 }
                                ThemedButton {
                                    text: tr("Restaurar")
                                    palette.button: "#e74c3c"; palette.buttonText: "white"
                                    onClicked: {
                                        window.currentThemeIndex = 0
                                        backend.expertMode = false
                                        window.markBackendSettingsDirty()
                                    }
                                }
                            }
                        }

                        Item { height: 20 } // spacer
                    }
                }
            }
            }
            Item {
                ColumnLayout {
                    anchors.fill: parent
                    anchors.margins: 30
                    spacing: 20
                    
                    RowLayout {
                        Layout.fillWidth: true
                        ColumnLayout {
                            Layout.fillWidth: true
                            Text { text: tr("Panel de Diagnóstico Experto"); font.pixelSize: 28; font.bold: true; color: currentTheme.textColor }
                            Text { text: tr("Gestión avanzada y resolución de problemas"); color: currentTheme.secondaryTextColor }
                        }
                        ComboBox {
                            id: serverModeCombo
                            model: [{ texto: tr("REST local"), valor: "rest" }]
                            textRole: "texto"
                            currentIndex: 0
                            font.pixelSize: 13
                        }
                        ThemedButton {
                            text: tr("Reiniciar Backend")
                            font.bold: true
                            palette.button: "#e67e22"; palette.buttonText: "white"
                            onClicked: {
                                backend.stopBackend()
                                backend.startBackend("127.0.0.1:63118", "", "rest", "", true)
                            }
                        }
                    }

                    // Botonera Experta
                    ColumnLayout {
                        Layout.fillWidth: true
                        spacing: 15
                        
                        Text { text: tr("SOPORTE Y DOCUMENTACIÓN"); color: currentTheme.primaryColor; font.bold: true; font.pixelSize: 12 }
                        Flow {
                            Layout.fillWidth: true; spacing: 10
                            ThemedButton { text: tr("Gestor Certificados"); onClicked: backend.openCertManager() }
                            ThemedButton { text: tr("Explorar Logs"); onClicked: backend.openLogFolder() }
                            ThemedButton { text: tr("Abrir Ayuda"); onClicked: backend.openHelpManual() }
                            ThemedButton { text: tr("Asistente guiado"); onClicked: window.openSupportAssistant("") }
                            ThemedButton { text: tr("Copiar Diag."); onClicked: backend.exportDiagnosticReport() }
                            ThemedButton { 
                                text: tr("Limpiar Log");
                                palette.button: "#2c3e50"; 
                                onClicked: logArea.text = tr("--- LOGS REINICIADOS [") + new Date().toLocaleTimeString() + "] ---\n"
                            }
                        }
                        
                        Text { text: tr("RED Y SEGURIDAD"); color: currentTheme.primaryColor; font.bold: true; font.pixelSize: 12 }
                        Flow {
                            Layout.fillWidth: true; spacing: 10
                            ThemedButton { text: tr("Diag. TLS"); onClicked: backend.runTLSDiagnostics() }
                            ThemedButton { text: tr("Vaciar Almacén TLS"); onClicked: backend.clearTLSTrustStore() }
                            ThemedButton { text: tr("Reinstalar conectores"); onClicked: backend.reinstallBrowserConnectors() }
                            ThemedButton { text: tr("Reinstalar certificados"); onClicked: backend.installPublicRoots() }
                            ThemedButton {
                                text: tr("Seguridad y Dominios")
                                onClicked: activeTab = "seguridad"
                            }
                        }

                        Text { text: tr("SISTEMA"); color: currentTheme.primaryColor; font.bold: true; font.pixelSize: 12 }
                        Flow {
                            Layout.fillWidth: true; spacing: 10
                            ThemedButton { text: tr("Comprobar Certs"); onClicked: backend.checkCertificates() }
                        }
                    }

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        color: "#050505"
                        radius: 8
                        border.color: currentTheme.primaryColor
                        border.width: 1
                        
                        ScrollView {
                            anchors.fill: parent
                            clip: true
                            TextArea {
                                id: logArea
                                readOnly: true
                                color: "#00ff41"
                                font.family: "Monospace"
                                font.pixelSize: 12
                                wrapMode: TextEdit.Wrap
                                textFormat: TextEdit.PlainText
                                text: tr("--- INICIO DE LOGS ---\n")
                                
                                Connections {
                                    target: backend
                                    function onBackendLogReceived(log) {
                                        logArea.append("[" + new Date().toLocaleTimeString() + "] " + log)
                                    }
                                }
                            }
                        }
                    }
                }
            }

            // TAB: INFORMACIÓN DE CONFIANZA WEB (5)
            Item {
                id: securityTab
                ColumnLayout {
                    anchors.fill: parent; anchors.margins: 40; spacing: 20
                    RowLayout {
                        Layout.fillWidth: true
                        Text {
                            text: tr("Seguridad y Dominios")
                            font.pixelSize: 32
                            font.bold: true
                            color: currentTheme.textColor
                            Layout.fillWidth: true
                        }
                        ThemedButton { text: tr("Volver"); flat: true; onClicked: activeTab = "experto" }
                    }

                    Rectangle {
                        Layout.fillWidth: true; Layout.fillHeight: true; radius: 15; color: currentTheme.cardColor
                        ColumnLayout {
                            anchors.centerIn: parent
                            spacing: 18
                            width: parent.width * 0.8
                            Text {
                                text: tr("Resumen de seguridad")
                                color: currentTheme.textColor
                                font.pixelSize: 22
                                font.bold: true
                                Layout.alignment: Qt.AlignHCenter
                            }
                            Text {
                                text: tr("Esta pantalla es informativa. La confianza web no se configura aquí: la aplican el manejador de protocolo y Native Messaging mediante la política instalada.")
                                color: currentTheme.secondaryTextColor
                                horizontalAlignment: Text.AlignHCenter
                                Layout.fillWidth: true
                                wrapMode: Text.WordWrap
                            }
                            ThemedButton {
                                text: tr("Diagnóstico TLS")
                                Layout.alignment: Qt.AlignHCenter
                                onClicked: backend.runTLSDiagnostics()
                            }
                        }
                    }
                }
            }

            FacturaePanel {
                id: facturaePanel
                localeName: window.appLanguage === "va" ? "ca" : window.appLanguage
                bridge: backend
                reportSaver: function(path, content) { backend.saveTextReport(path, content) }
                theme: currentTheme
                localPath: function(url) { return window.localPathFromUrl(url) }
                translate: function(key) { return window.tr(key) }
                enabled: window.facturaeToolsEnabled && isIpcMode
                onSignRequested: window.activeTab = "firmar"
            }
            EniPanel { localeName: window.appLanguage === "va" ? "ca" : window.appLanguage; bridge: backend; theme: currentTheme; localPath: function(url) { return window.localPathFromUrl(url) }; translate: function(key) { return window.tr(key) }; certificates: window.certificates; certificateId: function(cert) { return window.certificateId(cert) }; enabled: isIpcMode }
        }
    }

    // --- COMPONENTES ---
    component NavButton : Rectangle {
        id: navButtonRoot
        property string text: ""
        property string iconTxt: ""
        property bool active: false
        signal clicked()
        // Al cambiar de sección se limpia la barra de estado: un mensaje de Verificar
        // no debe seguir a la vista en Configuración o en ENI.
        function activate() {
            window.statusMessage = ""
            navButtonRoot.clicked()
        }

        Layout.fillWidth: true
        height: 54
        radius: 8
        activeFocusOnTab: true
        Accessible.role: Accessible.Button
        Accessible.name: navButtonRoot.text
        Accessible.description: active ? tr("Sección activa") : tr("Abrir sección")
        Accessible.onPressAction: navButtonRoot.activate()
        color: active ? Contrast.legibleFill(currentTheme.primaryColor) : "transparent"
        // Texto legible tanto en temas claros como oscuros; la activa, sobre el color principal.
        readonly property color labelColor: active ? Contrast.readableOn(Contrast.legibleFill(currentTheme.primaryColor), "#ffffff") : currentTheme.textColor
        border.color: activeFocus ? (active ? labelColor : currentTheme.focusColor) : "transparent"
        border.width: activeFocus ? 2 : 0

        Keys.onPressed: function(event) {
            if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter || event.key === Qt.Key_Space) {
                navButtonRoot.activate()
                event.accepted = true
            }
        }

        MouseArea {
            id: navButtonMouseArea
            anchors.fill: parent
            onClicked: navButtonRoot.activate()
            hoverEnabled: true
            cursorShape: Qt.PointingHandCursor
            onEntered: if(!navButtonRoot.active) navButtonRoot.opacity = 0.7
            onExited: navButtonRoot.opacity = 1.0
        }

        ToolTip.visible: window.sidebarCollapsed && (navButtonMouseArea.containsMouse || navButtonRoot.activeFocus)
        ToolTip.delay: 350
        ToolTip.text: navButtonRoot.text

        RowLayout {
            anchors.fill: parent
            anchors.margins: 10
            Text {
                text: iconTxt
                color: navButtonRoot.labelColor
                font.bold: true
                // Iconos un 80 % mayores que el texto por defecto para localizarlos de un vistazo.
                font.pixelSize: 24
                Layout.alignment: Qt.AlignHCenter | Qt.AlignVCenter
                Layout.preferredWidth: window.sidebarCollapsed ? parent.width - 20 : 34
                horizontalAlignment: Text.AlignHCenter
                verticalAlignment: Text.AlignVCenter
            }
            Text {
                visible: !window.sidebarCollapsed
                text: navButtonRoot.text
                color: navButtonRoot.labelColor
                font.bold: active
                Layout.fillWidth: true
            }
        }
    }

    // Status Bar - Dynamic
    Rectangle {
        anchors.bottom: parent.bottom
        width: parent.width
        height: 30
        id: statusBar
        readonly property bool showsError: localizedStatusMessage().startsWith(tr("Error"))
        // Fondo opaco: el contenido no debe transparentarse por debajo de la barra.
        color: showsError ? "#c0392b" : currentTheme.sidebarColor
        readonly property color labelColor: showsError ? "#ffffff" : currentTheme.textColor
        
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: 10
            anchors.rightMargin: 10
            spacing: 10
            Text {
                text: localizedStatusMessage().startsWith(tr("Error")) ? "⚠" : "ℹ"
                color: statusBar.labelColor
                font.bold: true
                visible: statusMessage !== ""
                Layout.alignment: Qt.AlignVCenter
            }
            Text {
                text: localizedStatusMessage()
                color: statusBar.labelColor
                font.pixelSize: 12
                font.bold: localizedStatusMessage().startsWith(tr("Error"))
                Layout.fillWidth: true
                Layout.alignment: Qt.AlignVCenter
                elide: Text.ElideRight
            }
            ThemedButton {
                text: window.activeDiagnosticInProgress
                      ? tr("Diagnosticando...")
                      : tr("Diagnosticar ahora")
                flat: true
                palette.window: statusBar.color
                visible: window.activeFailureContext !== null
                enabled: !window.activeDiagnosticInProgress
                onClicked: activeDiagnosticsConsentDialog.open()
            }
            Button {
                id: statusAssistantButton
                text: tr("Asistente")
                flat: true
                visible: true
                Accessible.name: text
                onClicked: window.openSupportAssistant("")
                contentItem: RowLayout {
                    spacing: 6
                    // Círculo con interrogación: identifica el botón de ayuda aunque no se lea el texto.
                    Rectangle {
                        Layout.alignment: Qt.AlignVCenter
                        implicitWidth: 18
                        implicitHeight: 18
                        radius: 9
                        color: "transparent"
                        border.color: statusBar.labelColor
                        border.width: 1.5
                        Text {
                            anchors.centerIn: parent
                            text: "?"
                            color: statusBar.labelColor
                            font.bold: true
                            font.pixelSize: 12
                        }
                    }
                    Text {
                        Layout.alignment: Qt.AlignVCenter
                        text: statusAssistantButton.text
                        color: statusBar.labelColor
                        font.pixelSize: 12
                    }
                }
            }
        }
        
        Behavior on color { ColorAnimation { duration: 300 } }
    }
}
