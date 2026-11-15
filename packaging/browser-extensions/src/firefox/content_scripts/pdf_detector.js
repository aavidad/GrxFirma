// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Detector de documentos PDF para ofrecer la firma.
(function () {
    // Las inyecciones estática y dinámica pueden coincidir en un documento.
    if (globalThis.grxfirmaPDFDetectorActive) return;
    globalThis.grxfirmaPDFDetectorActive = true;
    console.log('GrxFirma: detector PDF activo');
    const t = grxfirmaExt.i18n.message;
    const MAX_PRELOADED_BYTES = Math.floor(2.5 * 1024 * 1024);
    const PDF_FETCH_TIMEOUT_MS = 15 * 1000;

    // Una entrada por URL; los visores internos no duplican el PDF directo.
    const buttons = new Map();
    let candidates = new Map();
    let ownerPort = null;
    let stopped = false;
    let lastRegistration = '';
    const failedURLs = new Set();
    const isDirectPDF = document.contentType === 'application/pdf' ||
        window.location.pathname.toLowerCase().endsWith('.pdf');

    function absoluteURL(candidate) {
        try {
            const url = new URL(candidate, window.location.href);
            url.hash = '';
            return url.href;
        } catch (err) { return null; }
    }

    function embeddedPDFs() {
        const result = new Map();
        for (const element of document.querySelectorAll('embed,object,iframe')) {
            const candidate = element.getAttribute('src') || element.getAttribute('data');
            if (!candidate) continue;
            const url = absoluteURL(candidate);
            if (!url) continue;
            const pdfType = (element.getAttribute('type') || '').toLowerCase() === 'application/pdf';
            if (pdfType || new URL(url).pathname.toLowerCase().endsWith('.pdf')) {
                if (!result.has(url)) result.set(url, []);
                result.get(url).push(element);
            }
        }
        return result;
    }

    // El fondo arbitra toda la pestaña. Ningún mensaje de la página puede
    // conceder un botón ni declarar la confianza de su ventana principal.
    function renderOwners(urls) {
        const owned = new Set(urls);
        for (const [url, controls] of buttons) {
            if (!owned.has(url) || !candidates.has(url)) {
                controls.button.remove();
                controls.notice.remove();
                buttons.delete(url);
            }
        }
        for (const url of owned) {
            if (!candidates.has(url) || buttons.has(url)) continue;
            try { buttons.set(url, createSignButton(url)); }
            catch (_) { failedURLs.add(url); refreshButtons(); }
        }
        let index = 0;
        for (const { button, notice } of buttons.values()) {
            button.style.bottom = `${20 + index * 112}px`;
            notice.style.bottom = `${76 + index * 112}px`;
            index++;
        }
    }

    function refreshButtons(force = false) {
        candidates = isDirectPDF
            ? new Map([[absoluteURL(window.location.href), []]])
            : embeddedPDFs();
        for (const url of failedURLs) candidates.delete(url);
        candidates.delete(null);
        const urls = [...candidates.keys()].slice(0, 256);
        const registration = JSON.stringify(urls);
        // Crear o retirar controles también dispara el MutationObserver.
        if (ownerPort && (force || registration !== lastRegistration)) {
            lastRegistration = registration;
            try { ownerPort.postMessage({ action: 'candidates', urls, direct: isDirectPDF }); }
            catch (_) { renderOwners([]); }
        }
        for (const [url, controls] of buttons) {
            if (!candidates.has(url)) {
                controls.button.remove();
                controls.notice.remove();
                buttons.delete(url);
            }
        }
    }

    function connectOwner() {
        if (stopped || ownerPort) return;
        try {
            const port = grxfirmaExt.runtime.connect({ name: 'grxfirma-pdf-owners' });
            ownerPort = port;
            port.onMessage.addListener((message) => {
                if (ownerPort === port && message.action === 'owners' && Array.isArray(message.urls)) {
                    renderOwners(message.urls);
                }
            });
            port.onDisconnect.addListener(() => {
                if (ownerPort !== port) return;
                ownerPort = null;
                renderOwners([]);
                if (!stopped) setTimeout(connectOwner, 500);
            });
            refreshButtons(true);
        } catch (_) { renderOwners([]); }
    }

    window.addEventListener('pagehide', () => {
        stopped = true;
        renderOwners([]);
        if (ownerPort) ownerPort.disconnect();
        ownerPort = null;
    });
    window.addEventListener('pageshow', () => { stopped = false; connectOwner(); });
    // Recupera la coordinación tras suspender/reiniciar el service worker y
    // revalida navegación y permisos aunque no cambie el DOM.
    setInterval(() => { connectOwner(); refreshButtons(true); }, 15000);

    function inferFileName(url, blob) {
        try {
            const parsed = new URL(url, window.location.href);
            const part = parsed.pathname.split('/').filter(Boolean).pop();
            if (part) {
                return decodeURIComponent(part);
            }
        } catch (err) { }
        const title = (document.title || '').trim();
        if (title) {
            return title.toLowerCase().endsWith('.pdf') ? title : `${title}.pdf`;
        }
        if (blob && blob.type === 'application/pdf') {
            return t('pdfDefaultFilename');
        }
        return t('documentDefaultFilename');
    }

    function blobToBase64(blob) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onload = () => {
                const result = String(reader.result || '');
                const comma = result.indexOf(',');
                resolve(comma >= 0 ? result.slice(comma + 1) : result);
            };
            reader.onerror = () => reject(reader.error || new Error('blob_read_error'));
            reader.readAsDataURL(blob);
        });
    }

    async function buildPendingDocument(pdfUrl) {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), PDF_FETCH_TIMEOUT_MS);
        let reader;
        try {
            const response = await fetch(pdfUrl, { credentials: 'include', cache: 'no-store', signal: controller.signal });
            if (!response.ok || !response.body || typeof response.body.getReader !== 'function') {
                throw new Error('pdf_fetch_unavailable');
            }
            const declared = Number(response.headers.get('content-length'));
            if (declared > MAX_PRELOADED_BYTES) throw new Error('pdf_too_large');
            reader = response.body.getReader();
            const chunks = [];
            let total = 0;
            while (true) {
                const { done, value } = await reader.read();
                if (done) break;
                total += value.byteLength;
                if (total > MAX_PRELOADED_BYTES) throw new Error('pdf_too_large');
                chunks.push(value);
            }
            const blob = new Blob(chunks, { type: response.headers.get('content-type') || 'application/pdf' });
            return {
                name: inferFileName(pdfUrl, blob),
                mimeType: blob.type || 'application/pdf',
                contentBase64: await blobToBase64(blob),
                sourceUrl: pdfUrl
            };
        } finally {
            clearTimeout(timeout);
            if (reader) await reader.cancel().catch(() => {});
        }
    }

    function createSignButton(pdfUrl) {
        const button = document.createElement('button');
        button.id = document.getElementById('grxfirma-sign-trigger') ? '' : 'grxfirma-sign-trigger';
        button.title = inferFileName(pdfUrl);
        button.type = 'button';
        button.setAttribute('aria-label', t('pdfSignButtonAriaLabel'));
        button.setAttribute('aria-description', inferFileName(pdfUrl));

        // Sin icono: una imagen de la extensión exigiría web_accessible_resources
        // y permitiría a cualquier página detectar la extensión.
        const label = document.createElement('span');
        label.textContent = t('pdfSignButtonLabel');
        label.style.fontWeight = 'bold';
        button.append(label);

        // Estilo flotante compartido.
        Object.assign(button.style, {
            position: 'fixed',
            bottom: '20px',
            right: '20px',
            backgroundColor: '#2563eb',
            color: 'white',
            padding: '12px 20px',
            border: '0',
            borderRadius: '50px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
            cursor: 'pointer',
            zIndex: '2147483647', // Encima del contenido de la página.
            fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif',
            fontSize: '14px',
            transition: 'transform 0.2s, background-color 0.2s',
            userSelect: 'none',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center'
        });

        button.onmouseover = () => {
            button.style.transform = 'scale(1.05)';
            button.style.backgroundColor = '#1d4ed8';
        };
        button.onmouseout = () => {
            button.style.transform = 'scale(1)';
            button.style.backgroundColor = '#2563eb';
        };

        const notice = document.createElement('span');
        notice.id = document.getElementById('grxfirma-sign-notice') ? '' : 'grxfirma-sign-notice';
        notice.setAttribute('role', 'status');
        notice.setAttribute('aria-live', 'polite');
        Object.assign(notice.style, {
            position: 'fixed', bottom: '76px', right: '20px', maxWidth: '320px',
            backgroundColor: '#fff', color: '#12324d', padding: '0',
            borderRadius: '8px', boxShadow: 'none', zIndex: '2147483647',
            overflow: 'hidden', height: '0'
        });
        function showNotice(key) {
            notice.textContent = key ? t(key) : '';
            notice.style.padding = key ? '8px 12px' : '0';
            notice.style.boxShadow = key ? '0 2px 8px rgba(0,0,0,.2)' : 'none';
            notice.style.height = key ? 'auto' : '0';
        }
        let opening = false;
        button.onclick = async (event) => {
            if (!event.isTrusted || opening) return;
            opening = true;
            button.disabled = true;
            button.style.opacity = '0.72';
            button.title = t('pdfPreparingDocument');
            showNotice('pdfPreparingDocument');
            let pendingDocument = null;
            let preloadSkipped = false;
            try { pendingDocument = await buildPendingDocument(pdfUrl); }
            catch (error) { preloadSkipped = true; console.warn('GrxFirma: precarga PDF no disponible:', error); }
            try {
                const response = await grxfirmaExt.runtime.sendMessage({
                    action: 'openSigner', pdfUrl, pendingDocument
                });
                if (!response || response.success !== true) {
                    button.title = t('pdfSignerUnavailable');
                    showNotice(response && response.code === 'opening_pending' ? 'pdfOpeningPending' : 'pdfSignerUnavailable');
                    return;
                }
                button.title = inferFileName(pdfUrl);
                if (preloadSkipped || response.preloadSkipped) showNotice('pdfPreloadUnavailable');
                else showNotice('');
            } catch (error) {
                console.error('GrxFirma: error al abrir el firmador:', error);
                button.title = t('pdfSignerUnavailable');
                showNotice('pdfSignerUnavailable');
            } finally {
                opening = false;
                button.disabled = false;
                button.style.opacity = '1';
            }
        };
        document.body.appendChild(notice);

        document.body.appendChild(button);
        return { button, notice };
    }

    connectOwner();
    // Incluye inserciones tardías, cambios de URL y retirada de marcos.
    const observer = new MutationObserver(() => refreshButtons());
    observer.observe(document.documentElement, {
        childList: true, subtree: true, attributes: true,
        attributeFilter: ['src', 'data', 'type']
    });
})();
