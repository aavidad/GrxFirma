// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

if (typeof importScripts === 'function') {
    importScripts('compat.js');
    importScripts('config.js');
    importScripts('trusted_sites.js');
}
const LOCAL_REST_URL = CONFIG.LOCAL_REST_URL || 'https://127.0.0.1:63118';

const pendingSignerLaunches = new Map();
const PENDING_SIGNER_LAUNCH_TTL_MS = 5 * 60 * 1000;
const MAX_PENDING_STORAGE_BYTES = 8 * 1024 * 1024;
const MAX_SINGLE_PDF_BASE64_CHARS = Math.ceil((2.5 * 1024 * 1024) / 3) * 4;
const pendingTabOpenings = new Map();
const TAB_OPENING_COOLDOWN_MS = 30 * 1000;
let dynamicPDFSites = new Set();
let dynamicIdentitySites = new Set();

// --- NATIVE HOST CONNECTION START ---
let nativePort = null;
const pendingRequests = new Map();
const chunkBuffers = new Map();
const MAX_NATIVE_RESPONSE_CHUNKS = 4096;
const MAX_NATIVE_CHUNK_CHARS = 1024 * 1024;
const MAX_NATIVE_SIGNATURE_CHARS = 128 * 1024 * 1024;
// Chromium limita a 64 MiB los mensajes enviados al host. Se reserva 1 MiB
// para serializacion y cambios de protocolo antes de recurrir a REST local.
const MAX_NATIVE_REQUEST_BYTES = 63 * 1024 * 1024;
const IDENTITY_CONTRACT = 'identidad-reforzada/v1';
const MAX_IDENTITY_CANON_BYTES = 16 * 1024;
const MAX_IDENTITY_SIGNATURE_BYTES = 512 * 1024;
const MAX_IDENTITY_CERTIFICATE_BYTES = 64 * 1024;
let identityProofInProgress = false;
// Configure the correct Native Host name
const NATIVE_HOST_NAME = "io.github.aavidad.grxfirma";

function nativeChannelUserGuidance() {
    return grxfirmaExt.i18n.message('popupNativeHostUnavailable');
}

function takePendingRequest(requestId) {
    const pending = pendingRequests.get(requestId);
    if (!pending) return null;
    pendingRequests.delete(requestId);
    if (pending.timeoutId) clearTimeout(pending.timeoutId);
    return pending;
}

function rejectPendingRequest(requestId, error) {
    const pending = takePendingRequest(requestId);
    if (pending) pending.reject(error);
}

function buildPendingSignerLaunchKey(token) {
    return `pendingSignerLaunch:${token}`;
}

async function cleanupPendingSignerLaunches() {
    if (!grxfirmaExt.hasStorageSession()) {
        for (const [token, entry] of pendingSignerLaunches) {
            if (!entry || entry.expiresAt <= Date.now()) pendingSignerLaunches.delete(token);
        }
        return [...pendingSignerLaunches.values()];
    }
    const stored = await grxfirmaExt.storageSession.get(null);
    const live = [];
    for (const [key, entry] of Object.entries(stored)) {
        if (!key.startsWith('pendingSignerLaunch:')) continue;
        if (!entry || !Number.isFinite(entry.expiresAt) || entry.expiresAt <= Date.now()) {
            await grxfirmaExt.storageSession.remove(key);
        } else {
            live.push(entry);
        }
    }
    return live;
}

async function storePendingSignerLaunch(token, payload) {
    const live = await cleanupPendingSignerLaunches();
    if (!payload || typeof payload.contentBase64 !== 'string' ||
        payload.contentBase64.length > MAX_SINGLE_PDF_BASE64_CHARS ||
        payload.contentBase64.length === 0 || payload.contentBase64.length % 4 !== 0 ||
        !/^[A-Za-z0-9+/=]+$/.test(payload.contentBase64) ||
        /=/.test(payload.contentBase64.slice(0, -2)) || !/^(?:[A-Za-z0-9+/]{4}|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{2}==)$/.test(payload.contentBase64.slice(-4))) {
        throw new Error('invalid_pending_document');
    }
    const entry = { document: payload, expiresAt: Date.now() + PENDING_SIGNER_LAUNCH_TTL_MS };
    // storage.session puede contar cadenas como UTF-16 y añade metadatos propios.
    const size = (value) => 2 * JSON.stringify(value).length + 1024;
    const keySize = 2 * buildPendingSignerLaunchKey(token).length;
    if (size(entry) + keySize + live.reduce((sum, item) => sum + size(item) + 2 * 84, 0) > MAX_PENDING_STORAGE_BYTES) {
        throw new Error('pending_storage_full');
    }
    if (grxfirmaExt.hasStorageSession()) {
        await grxfirmaExt.storageSession.set({ [buildPendingSignerLaunchKey(token)]: entry });
    } else {
        pendingSignerLaunches.set(token, entry);
    }
}

let pendingStorageWrite = Promise.resolve();
function savePendingSignerLaunch(token, payload) {
    pendingStorageWrite = pendingStorageWrite.catch(() => {}).then(() => storePendingSignerLaunch(token, payload));
    return pendingStorageWrite;
}

async function takePendingSignerLaunch(token) {
    if (typeof token !== 'string' || !/^[0-9a-f]{64}$/.test(token)) return null;
    let entry = null;
    if (grxfirmaExt.hasStorageSession()) {
        const key = buildPendingSignerLaunchKey(token);
        const result = await grxfirmaExt.storageSession.get(key);
        await grxfirmaExt.storageSession.remove(key);
        entry = result[key] || null;
    } else {
        entry = pendingSignerLaunches.get(token) || null;
        pendingSignerLaunches.delete(token);
    }
    if (!entry || !Number.isFinite(entry.expiresAt) || entry.expiresAt <= Date.now()) return null;
    return entry.document || null;
}

function secureRandomHex(byteLength) {
    if (!globalThis.crypto || typeof globalThis.crypto.getRandomValues !== 'function') {
        throw new Error('Web Crypto is required to generate secure request identifiers');
    }
    const bytes = new Uint8Array(byteLength);
    globalThis.crypto.getRandomValues(bytes);
    return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
}

function createLaunchToken() {
    return secureRandomHex(32);
}

function createNativeRequestId() {
    if (globalThis.crypto && typeof globalThis.crypto.randomUUID === 'function') {
        return globalThis.crypto.randomUUID();
    }
    return secureRandomHex(16);
}

function ensureNativeRequestSize(message, maxBytes = MAX_NATIVE_REQUEST_BYTES) {
    const serialized = JSON.stringify(message);
    const byteLength = new TextEncoder().encode(serialized).byteLength;
    if (byteLength > maxBytes) {
        const error = new Error(`Native request exceeds ${maxBytes} bytes`);
        error.code = 'native_request_too_large';
        throw error;
    }
}

function connectNativeHost() {
    if (nativePort) return;

    console.log('[Native] Connecting to host:', NATIVE_HOST_NAME);
    try {
        nativePort = grxfirmaExt.runtime.connectNative(NATIVE_HOST_NAME);

        nativePort.onMessage.addListener((message) => {
            handleNativeResponse(message);
        });

        nativePort.onDisconnect.addListener(() => {
            console.log('[Native] Disconnected');
            const lastError = grxfirmaExt.runtime.lastErrorMessage();
            if (lastError) {
                console.error('[Native] Disconnect error:', lastError);
            }
            const err = new Error(nativeChannelUserGuidance());
            err.code = 'native_secure_channel_unavailable';
            for (const { reject, timeoutId } of pendingRequests.values()) {
                if (timeoutId) clearTimeout(timeoutId);
                reject(err);
            }
            pendingRequests.clear();
            chunkBuffers.clear();
            nativePort = null;
        });
    } catch (e) {
        console.error("[Native] Connection failed:", e);
    }
}

function handleNativeResponse(response) {
    const { requestId, chunk, totalChunks, signature } = response;

    if (!requestId || !pendingRequests.has(requestId)) {
        console.warn('[Native] Ignoring response for unknown requestId');
        return;
    }

    // Handle chunked responses
    if (typeof chunk !== 'undefined' && typeof totalChunks !== 'undefined') {
        const isValidChunkMeta = Number.isInteger(chunk) &&
            Number.isInteger(totalChunks) &&
            totalChunks > 0 &&
            totalChunks <= MAX_NATIVE_RESPONSE_CHUNKS &&
            chunk >= 0 &&
            chunk < totalChunks;
        const chunkText = typeof signature === 'string' ? signature : '';

        if (!isValidChunkMeta || chunkText.length > MAX_NATIVE_CHUNK_CHARS) {
            chunkBuffers.delete(requestId);
            rejectPendingRequest(requestId, new Error('Invalid native response chunk'));
            return;
        }

        const identityProofFingerprint = response.identityProof === undefined
            ? ''
            : JSON.stringify(response.identityProof);
        if (!chunkBuffers.has(requestId)) {
            chunkBuffers.set(requestId, {
                totalChunks,
                totalChars: 0,
                chunks: new Array(totalChunks),
                identityProof: response.identityProof,
                identityProofFingerprint
            });
        }

        const buffer = chunkBuffers.get(requestId);
        if (buffer.totalChunks !== totalChunks ||
            buffer.identityProofFingerprint !== identityProofFingerprint) {
            chunkBuffers.delete(requestId);
            rejectPendingRequest(requestId, new Error('Inconsistent native response chunk count'));
            return;
        }

        if (buffer.chunks[chunk] === undefined) {
            buffer.totalChars += chunkText.length;
        } else {
            buffer.totalChars += chunkText.length - buffer.chunks[chunk].length;
        }

        if (buffer.totalChars > MAX_NATIVE_SIGNATURE_CHARS) {
            chunkBuffers.delete(requestId);
            rejectPendingRequest(requestId, new Error('Native response too large'));
            return;
        }

        buffer.chunks[chunk] = chunkText;

        // Check if complete
        let receivedCount = 0;
        for (let i = 0; i < totalChunks; i++) {
            if (buffer.chunks[i] !== undefined) receivedCount++;
        }

        // Broadcast progress to signer UI if open
        grxfirmaExt.runtime.sendMessage({
            action: 'signingProgress',
            current: chunk + 1,
            total: totalChunks,
            percent: Math.round(((chunk + 1) / totalChunks) * 100)
        }).catch(() => { });

        if (receivedCount === totalChunks) {
            response.signature = buffer.chunks.join('');
            response.identityProof = buffer.identityProof;
            chunkBuffers.delete(requestId);
            console.log(`[Native] Reassembly complete for ${requestId}. Total length: ${response.signature.length}`);

            // Integrity checks
            if (response.success && response.identityProof && response.signature.length % 4 !== 0) {
                rejectPendingRequest(requestId, new Error('Invalid native identity proof encoding'));
                return;
            }
            if (response.success && !response.identityProof && response.signature.length % 4 !== 0) {
                const pad = response.signature.length % 4;
                if (pad > 0) response.signature += '='.repeat(4 - pad);
            }
        } else {
            return; // Wait for more
        }
    }

    if (pendingRequests.has(requestId)) {
        const { resolve, reject } = takePendingRequest(requestId);

        if (response.success) {
            resolve(response);
        } else {
            const error = new Error(response.error || 'Unknown error');
            if (typeof response.code === 'string') error.code = response.code;
            reject(error);
        }
    }
}

function normalizeTrustedRequesterOrigin(raw) {
    try {
        const parsed = new URL(String(raw || ''));
        if (!['https:', 'chrome-extension:', 'moz-extension:'].includes(parsed.protocol)) {
            return '';
        }
        if (parsed.protocol === 'chrome-extension:' || parsed.protocol === 'moz-extension:') {
            return `${parsed.protocol}//${parsed.host}`;
        }
        return parsed.origin;
    } catch (error) {
        return '';
    }
}

function canonicalBase64Limited(value, maximumBytes) {
    if (typeof value !== 'string' || value.length === 0 || value.trim() !== value ||
        value.length > Math.ceil(maximumBytes / 3) * 4 ||
        !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) {
        return false;
    }
    try {
        const decoded = atob(value);
        return decoded.length > 0 && decoded.length <= maximumBytes && btoa(decoded) === value;
    } catch (error) {
        return false;
    }
}

function validateNativeIdentityProof(response) {
    const proof = response && response.identityProof;
    const challengePattern = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
    if (!proof || proof.contract !== IDENTITY_CONTRACT ||
        typeof proof.challengeId !== 'string' || !challengePattern.test(proof.challengeId) ||
        !canonicalBase64Limited(response.signature, MAX_IDENTITY_SIGNATURE_BYTES) ||
        !canonicalBase64Limited(proof.certificateB64, MAX_IDENTITY_CERTIFICATE_BYTES) ||
        !Array.isArray(proof.chainB64) || proof.chainB64.length > 8 ||
        proof.chainB64.some((certificate) => !canonicalBase64Limited(certificate, MAX_IDENTITY_CERTIFICATE_BYTES)) ||
        proof.format !== 'cades-detached' ||
        !['sha256-rsa-pkcs1v15', 'sha256-ecdsa'].includes(proof.signatureAlgorithm) ||
        proof.digestAlgorithm !== 'sha-256') {
        const error = new Error('Invalid native identity proof');
        error.code = 'identity.invalid_proof';
        throw error;
    }
    return {
        contract: IDENTITY_CONTRACT,
        challengeId: proof.challengeId,
        signatureB64: response.signature,
        certificateB64: proof.certificateB64,
        chainB64: [...proof.chainB64],
        format: 'cades-detached',
        signatureAlgorithm: proof.signatureAlgorithm,
        digestAlgorithm: 'sha-256'
    };
}

async function proveIdentityLocal(canonicalPayloadB64, requesterOrigin) {
    if (!canonicalBase64Limited(canonicalPayloadB64, MAX_IDENTITY_CANON_BYTES)) {
        const error = new Error('Invalid identity challenge');
        error.code = 'identity.invalid_request';
        throw error;
    }
    if (identityProofInProgress) {
        const error = new Error('Identity operation already in progress');
        error.code = 'identity.busy';
        throw error;
    }
    identityProofInProgress = true;
    try {
        const response = await sendToNativeHost('proveIdentity', {
            canonicalPayloadB64
        }, requesterOrigin);
        return validateNativeIdentityProof(response);
    } finally {
        identityProofInProgress = false;
    }
}

function sendToNativeHost(action, data = {}, requesterOrigin = '') {
    return new Promise((resolve, reject) => {
        if (!nativePort) connectNativeHost();
        if (!nativePort) {
            const error = new Error(nativeChannelUserGuidance());
            error.code = 'native_secure_channel_unavailable';
            return reject(error);
        }

        const requestId = createNativeRequestId();
        const message = (data && typeof data === 'object' && !Array.isArray(data))
            ? { ...data }
            : {};
        // Estos campos pertenecen al borde de confianza de la extensión. No se
        // permite que un objeto recibido de una página los sobrescriba.
        delete message.requestId;
        delete message.action;
        delete message.requesterApplication;
        delete message.requesterOrigin;
        message.requestId = requestId;
        message.action = action;
        const trustedOrigin = normalizeTrustedRequesterOrigin(requesterOrigin);
        if (trustedOrigin) message.requesterOrigin = trustedOrigin;
        try {
            ensureNativeRequestSize(message);
        } catch (err) {
            return reject(err);
        }
        const timeoutId = setTimeout(() => {
            if (pendingRequests.has(requestId)) {
                pendingRequests.delete(requestId);
                chunkBuffers.delete(requestId);
                const error = new Error(
                    grxfirmaExt.i18n.message('nativeRequestTimeout')
                );
                error.code = 'native_request_timeout';
                reject(error);
            }
        }, 180000);
        pendingRequests.set(requestId, { resolve, reject, timeoutId });

        try {
            nativePort.postMessage(message);
        } catch (err) {
            takePendingRequest(requestId);
            chunkBuffers.delete(requestId);
            return reject(err);
        }
    });
}
// --- NATIVE HOST CONNECTION END ---

const INTERNAL_RUNTIME_ACTIONS = new Set([
    'signDocument',
    'openSigner',
    'ping',
    'verifySignature',
    'refreshTrustedSites'
]);

function senderDocumentURL(sender) {
    for (const candidate of [sender && sender.url, sender && sender.origin, sender && sender.tab && sender.tab.url]) {
        if (typeof candidate === 'string' && candidate.trim()) return candidate.trim();
    }
    return '';
}

const FACTORY_PORTAL_PATTERNS = ['https://*.dipgra.es/*', 'https://*.savia.net/*'];
function matchesManagedPortalPattern(parsed, pattern) {
    const match = /^https:\/\/(\*\.)?([^/:*]+)\/\*$/.exec(pattern);
    if (!match) return false;
    const expectedHost = match[2].toLowerCase();
    const actualHost = parsed.hostname.toLowerCase();
    return (match[1] ? actualHost === expectedHost || actualHost.endsWith(`.${expectedHost}`) : actualHost === expectedHost) &&
        (parsed.port === '' || parsed.port === '443');
}
function isFactoryPortalURL(parsed) {
    return parsed.protocol === 'https:' && FACTORY_PORTAL_PATTERNS.some((pattern) => matchesManagedPortalPattern(parsed, pattern));
}
function isDynamicSiteURL(parsed, sites) {
    return [...sites].some((site) => grxfirmaTrustedSites.matches(parsed.href, site));
}
function isPDFPortalURL(parsed) {
    return isFactoryPortalURL(parsed) || isDynamicSiteURL(parsed, dynamicPDFSites);
}
function isIdentityPortalURL(parsed) {
    return isFactoryPortalURL(parsed) || isDynamicSiteURL(parsed, dynamicIdentitySites);
}
function scriptId(site, kind) {
    return `grxfirma-${kind}-${Array.from(site, (char) => char.charCodeAt(0).toString(16).padStart(2, '0')).join('')}`;
}
async function reconcileTrustedSites() {
    const local = await grxfirmaExt.storageLocal.get('sitiosConfianza');
    let managed = {};
    try { managed = await grxfirmaExt.storageManaged.get('sitiosConfianza'); } catch (_) { /* Sin política. */ }
    const userSites = grxfirmaTrustedSites.normalizeList(local.sitiosConfianza);
    const managedSites = grxfirmaTrustedSites.normalizeList(managed.sitiosConfianza);
    const desired = [];
    const pdfSites = new Set();
    const identitySites = new Set();
    for (const site of new Set([...userSites, ...managedSites])) {
        if (grxfirmaTrustedSites.FACTORY_SITES.includes(site)) continue;
        const matches = [grxfirmaTrustedSites.pattern(site)];
        if (!await grxfirmaExt.permissions.contains({ origins: matches })) continue;
        pdfSites.add(site);
        desired.push({ id: scriptId(site, 'pdf'), matches, js: ['compat.js', 'content_scripts/pdf_detector.js'], runAt: 'document_end', allFrames: true });
        if (managedSites.includes(site)) {
            identitySites.add(site);
            desired.push({ id: scriptId(site, 'identity'), matches, js: ['compat.js', 'content_scripts/identity_bridge.js'], runAt: 'document_end' });
        }
    }
    const current = await grxfirmaExt.scripting.getRegisteredContentScripts({});
    const ids = current.filter((item) => item.id.startsWith('grxfirma-')).map((item) => item.id);
    if (ids.length) await grxfirmaExt.scripting.unregisterContentScripts({ ids });
    if (desired.length) await grxfirmaExt.scripting.registerContentScripts(desired);
    dynamicPDFSites = pdfSites;
    dynamicIdentitySites = identitySites;
    return { userSites, managedSites };
}
let trustedSitesRefresh = Promise.resolve();
function refreshTrustedSites() {
    trustedSitesRefresh = trustedSitesRefresh.catch(() => {}).then(reconcileTrustedSites);
    return trustedSitesRefresh;
}
if (grxfirmaExt.runtime.onInstalled) grxfirmaExt.runtime.onInstalled.addListener(() => { refreshTrustedSites().catch(console.error); });
if (grxfirmaExt.runtime.onStartup) grxfirmaExt.runtime.onStartup.addListener(() => { refreshTrustedSites().catch(console.error); });
if (grxfirmaExt.permissions && grxfirmaExt.permissions.onAdded) grxfirmaExt.permissions.onAdded.addListener(() => { refreshTrustedSites().catch(console.error); });
if (grxfirmaExt.permissions && grxfirmaExt.permissions.onRemoved) grxfirmaExt.permissions.onRemoved.addListener(() => { refreshTrustedSites().catch(console.error); });
if (grxfirmaExt.storageOnChanged) grxfirmaExt.storageOnChanged.addListener((changes, area) => {
    if ((area === 'local' || area === 'managed') && changes.sitiosConfianza) refreshTrustedSites().catch(console.error);
});
refreshTrustedSites().catch(console.error);

function authorizeRuntimeMessage(request, sender) {
    const action = request && typeof request.action === 'string' ? request.action : '';
    const runtimeID = String(grxfirmaExt.runtime.id || '');
    if (!action || !runtimeID || !sender || sender.id !== runtimeID) {
        return { allowed: false, reason: 'sender_not_authorized', origin: '' };
    }

    const rawURL = senderDocumentURL(sender);
    let parsed;
    try {
        parsed = new URL(rawURL);
    } catch (error) {
        return { allowed: false, reason: 'sender_origin_invalid', origin: '' };
    }

    const extensionBase = grxfirmaExt.runtime.getURL('');
    if (rawURL.startsWith(extensionBase) && INTERNAL_RUNTIME_ACTIONS.has(action)) {
        return {
            allowed: true,
            kind: 'extension',
            origin: normalizeTrustedRequesterOrigin(extensionBase)
        };
    }

    if ((action === 'openSigner' && isPDFPortalURL(parsed)) ||
        (action === 'proveIdentity' && isIdentityPortalURL(parsed))) {
        return { allowed: true, kind: 'portal', origin: parsed.origin };
    }

    if (parsed.origin === 'https://127.0.0.1:63118' &&
        (parsed.pathname === '/signer' || parsed.pathname === '/firmador') &&
        action === 'takePendingSignerLaunch') {
        return { allowed: true, kind: 'local-signer', origin: parsed.origin };
    }

    return { allowed: false, reason: 'sender_action_not_authorized', origin: '' };
}

// Mensajes internos de la extension y del puente del firmador local.
function processRuntimeMessage(request, sender, sendResponse) {
    const authorization = authorizeRuntimeMessage(request, sender);
    if (!authorization.allowed) {
        sendResponse({
            success: false,
            code: authorization.reason,
            error: grxfirmaExt.i18n.message('runtimeRequestRejected')
        });
        return false;
    }

    if (request.action === 'refreshTrustedSites') {
        refreshTrustedSites().then(() => sendResponse({ success: true })).catch(err => sendResponse({ success: false, error: err.message }));
        return true;
    }
    // Operaciones del firmador heredado y del puente local.
    else if (request.action === 'signDocument') {
        signWithSystemCertificate(
            request.data,
            request.certificateId,
            request.pin,
            request.format,
            request.signatureOptions,
            authorization.origin
        ).then(signature => {
            sendResponse({ success: true, signature });
        }).catch(err => {
            sendResponse({ success: false, error: err.message });
        });
        return true;
    }
    else if (request.action === 'openSigner') {
        (async () => {
            const tabId = sender && sender.tab && sender.tab.id;
            const now = Date.now();
            for (const [id, until] of pendingTabOpenings) if (until <= now) pendingTabOpenings.delete(id);
            if (authorization.kind === 'portal' && Number.isInteger(tabId)) {
                if (pendingTabOpenings.has(tabId)) {
                    sendResponse({ success: false, code: 'opening_pending', error: grxfirmaExt.i18n.message('pdfOpeningPending') });
                    return;
                }
                pendingTabOpenings.set(tabId, now + TAB_OPENING_COOLDOWN_MS);
            }
            let url = LOCAL_REST_URL.replace(/\/+$/, '') + '/signer';
            let preloadSkipped = false;
            if (request.pendingDocument) {
                try {
                    const token = createLaunchToken();
                    await savePendingSignerLaunch(token, request.pendingDocument);
                    url += '?launchToken=' + encodeURIComponent(token);
                } catch (error) {
                    preloadSkipped = true;
                    console.warn('GrxFirma: PDF preload unavailable:', error);
                }
            }
            try { await grxfirmaExt.tabs.create({ url }); }
            catch (error) { if (Number.isInteger(tabId)) pendingTabOpenings.delete(tabId); throw error; }
            sendResponse({ success: true, preloadSkipped });
        })().catch(err => sendResponse({ success: false, error: err.message || String(err) }));
        return true;
    }
    else if (request.action === 'proveIdentity') {
        proveIdentityLocal(request.canonicalPayloadB64, authorization.origin).then(proof => {
            sendResponse({ success: true, proof });
        }).catch(err => {
            sendResponse({
                success: false,
                code: typeof err.code === 'string' ? err.code : 'identity.unavailable'
            });
        });
        return true;
    }
    else if (request.action === 'takePendingSignerLaunch') {
        (async () => {
            const payload = await takePendingSignerLaunch(request.token);
            if (!payload) {
                sendResponse({ success: false, error: 'pending_document_not_found' });
                return;
            }
            sendResponse({ success: true, document: payload });
        })().catch(err => {
            sendResponse({ success: false, error: err.message || String(err) });
        });
        return true;
    }
    else if (request.action === 'ping') {
        pingLocalIntegration().then(result => {
            sendResponse(result);
        }).catch(err => {
            sendResponse({ success: false, error: err.message || String(err) });
        });
        return true;
    }
    else if (request.action === 'verifySignature') {
        verifyWithSystemCertificate(
            request.signatureData || request.data,
            request.originalData,
            request.format,
            request.mimeType
        ).then(result => {
            sendResponse({ success: true, result });
        }).catch(err => {
            sendResponse({ success: false, error: err.message || String(err) });
        });
        return true;
    }
}
grxfirmaExt.runtime.onMessage.addListener((request, sender, sendResponse) => {
    if (request && (request.action === 'openSigner' || request.action === 'proveIdentity')) {
        trustedSitesRefresh.then(() => processRuntimeMessage(request, sender, sendResponse))
            .catch(() => sendResponse({ success: false, code: 'sites_unavailable' }));
        return true;
    }
    return processRuntimeMessage(request, sender, sendResponse);
});

async function pingLocalIntegration() {
    try {
        const response = await sendToNativeHost('ping');
        return { success: true, message: 'pong', mode: 'native', native: response };
    } catch (nativeError) {
        try {
            await restJSON('/certificates', { method: 'GET' }, 30000);
            return { success: true, message: 'rest-ok', mode: 'rest' };
        } catch (restError) {
            throw fallbackError(nativeError, restError);
        }
    }
}

async function signWithSystemCertificate(data, certificateId, pin, format = 'cades', signatureOptions = null, requesterOrigin = '') {
    try {
        const message = { data, certificateId, pin, format };
        if (signatureOptions) message.signatureOptions = signatureOptions;

        const response = await sendToNativeHost('sign', message, requesterOrigin);
        return response.signature;
    } catch (error) {
        return signWithSystemCertificateRest(data, certificateId, format, signatureOptions, error);
    }
}

async function verifyWithSystemCertificate(signatureData, originalData, format = 'cades', mimeType = 'application/octet-stream') {
    try {
        const message = { signatureData, format, mimeType };
        if (originalData) message.originalData = originalData;
        const response = await sendToNativeHost('verify', message);
        return response.result;
    } catch (error) {
        return verifyWithSystemCertificateRest(signatureData, originalData, format, mimeType, error);
    }
}

function localRestBase() {
    return (LOCAL_REST_URL || 'https://127.0.0.1:63118').replace(/\/+$/, '');
}

function localRestBearer() {
    return (CONFIG.LOCAL_REST_BEARER || '').trim();
}

async function restJSON(path, options = {}, timeoutMs = 180000) {
    const headers = Object.assign({ Accept: 'application/json' }, options.headers || {});
    const bearer = localRestBearer();
    if (!bearer) {
        throw new Error('REST fallback deshabilitado: no hay un Bearer efimero configurado');
    }
    headers.Authorization = `Bearer ${bearer}`;
    if (options.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';

    let timer = null;
    let signal = options.signal;
    if (!signal && typeof AbortController !== 'undefined') {
        const controller = new AbortController();
        signal = controller.signal;
        timer = setTimeout(() => controller.abort(), timeoutMs);
    }

    try {
        const response = await fetch(localRestBase() + path, Object.assign({}, options, {
            headers,
            signal,
            cache: 'no-store'
        }));
        const text = await response.text();
        const payload = text ? JSON.parse(text) : {};
        if (!response.ok) {
            throw new Error(payload.error || `REST ${path} devolvio HTTP ${response.status}`);
        }
        return payload;
    } finally {
        if (timer) clearTimeout(timer);
    }
}

function restSignOptions(signatureOptions) {
    const options = {};
    if (!signatureOptions || typeof signatureOptions !== 'object') return options;
    for (const [key, value] of Object.entries(signatureOptions)) {
        if (value === undefined || value === null || typeof value === 'object') continue;
        options[key] = String(value);
    }
    return options;
}

async function signWithSystemCertificateRest(data, certificateId, format, signatureOptions, nativeError) {
    try {
        const normalizedFormat = (format || 'cades').toLowerCase();
        const body = {
            name: `documento.${normalizedFormat}`,
            content_base64: data,
            mime_type: normalizedFormat === 'pades' ? 'application/pdf' : 'application/octet-stream',
            format: normalizedFormat,
            action: 'sign',
            certificate_id: certificateId || '',
            options: restSignOptions(signatureOptions),
            returnSignatureB64: true
        };
        const response = await restJSON('/sign', {
            method: 'POST',
            body: JSON.stringify(body)
        });
        return response.signed_content_base64;
    } catch (restError) {
        throw fallbackError(nativeError, restError);
    }
}

async function verifyWithSystemCertificateRest(signatureData, originalData, format, mimeType, nativeError) {
    try {
        const normalizedFormat = (format || 'cades').toLowerCase();
        const detached = !!signatureData;
        const body = {
            name: detached ? `firma.${normalizedFormat === 'xades' ? 'xml' : 'p7s'}` : 'documento-firmado.pdf',
            content_base64: detached ? signatureData : originalData,
            mime_type: detached ? 'application/pkcs7-signature' : (mimeType || 'application/pdf'),
            original_content_base64: detached ? (originalData || '') : ''
        };
        return restJSON('/verify', {
            method: 'POST',
            body: JSON.stringify(body)
        });
    } catch (restError) {
        throw fallbackError(nativeError, restError);
    }
}

function fallbackError(nativeError, restError) {
    if (nativeError && restError) {
        return new Error(`${nativeError.message || nativeError}; REST fallback: ${restError.message || restError}`);
    }
    return restError || nativeError || new Error('Integracion local no disponible');
}
