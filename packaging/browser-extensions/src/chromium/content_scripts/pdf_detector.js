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
    let otherFrameOwnsDocument = false;
    let documentCanRender = false;
    const readyFrames = new Map();
    const discoveredPDFs = new WeakMap();
    const isDirectPDF = document.contentType === 'application/pdf' ||
        window.location.pathname.toLowerCase().endsWith('.pdf');
    const channel = 'grxfirma-pdf-owner-v1';
    const documentKeys = new Map();
    // La huella permite comparar referencias sin revelar al padre la URL
    // final de un PDF de otro origen (puede incluir credenciales temporales).
    function documentKey(url) {
        if (!documentKeys.has(url)) {
            documentKeys.set(url, (async () => {
                try {
                    const bytes = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(url));
                    return [...new Uint8Array(bytes)].map((value) => value.toString(16).padStart(2, '0')).join('');
                } catch (err) { return null; }
            })());
        }
        return documentKeys.get(url);
    }

    async function probeParent() {
        if (!isDirectPDF || window.parent === window || !documentCanRender) return;
        const key = await documentKey(absoluteURL(window.location.href));
        if (key) window.parent.postMessage({ channel, action: 'probe', key }, '*');
    }

    function absoluteURL(candidate) {
        try {
            const url = new URL(candidate, window.location.href);
            url.hash = '';
            return url.href;
        } catch (err) { return null; }
    }

    function embeddedPDFs(includeUnknown = false) {
        const result = new Map();
        for (const element of document.querySelectorAll('embed,object,iframe')) {
            const candidate = element.getAttribute('src') || element.getAttribute('data');
            if (!candidate) continue;
            const url = absoluteURL(candidate);
            if (!url) continue;
            const pdfType = (element.getAttribute('type') || '').toLowerCase() === 'application/pdf';
            if (pdfType || new URL(url).pathname.toLowerCase().endsWith('.pdf') ||
                discoveredPDFs.get(element) === url || includeUnknown) {
                if (!result.has(url)) result.set(url, []);
                result.get(url).push(element);
            }
        }
        return result;
    }

    // El marco PDF gana si ha podido crear sus controles; el contenedor
    // conserva el botón de respaldo si el visor no admite scripts. Solo se
    // intercambian control y huellas, sin contenido ni URL del documento.
    window.addEventListener('message', async (event) => {
        const message = event.data;
        if (!message || message.channel !== channel) return;
        if (['owned', 'selected'].includes(message.action) && isDirectPDF && event.source === window.parent &&
            typeof message.key === 'string' && message.key === await documentKey(absoluteURL(window.location.href))) {
            const ownedElsewhere = message.action === 'owned';
            if (otherFrameOwnsDocument !== ownedElsewhere) {
                otherFrameOwnsDocument = ownedElsewhere;
                refreshButtons(false);
            }
            return;
        }
        if (message.action === 'probe-request' && isDirectPDF && event.source === window.parent) {
            await probeParent();
            return;
        }
        if (message.action !== 'probe' || !event.source || event.source === window) return;
        let child = false;
        try { child = event.source.parent === window; } catch (err) { return; }
        if (!child) return;
        if (typeof message.key !== 'string' || !/^[a-f0-9]{64}$/.test(message.key)) return;
        for (const [url, elements] of embeddedPDFs(true)) {
            const matchingFrame = elements.some((element) => element.contentWindow === event.source);
            const matchingDocument = message.key === await documentKey(url);
            // Los embed/object no siempre exponen contentWindow. En ese caso
            // se compara la huella; en iframe se verifica además el emisor.
            const anonymousHosts = elements.filter((element) => !element.contentWindow &&
                ['EMBED', 'OBJECT'].includes(element.tagName.toUpperCase()));
            const anonymousHost = anonymousHosts.length > 0;
            if (!matchingFrame && !(anonymousHost && matchingDocument)) continue;
            for (const element of elements) discoveredPDFs.set(element, url);
            if (!readyFrames.has(url)) readyFrames.set(url, new Map());
            const hosts = elements.filter((element) => element.contentWindow === event.source ||
                (anonymousHosts.includes(element) && matchingDocument));
            readyFrames.get(url).set(event.source, { key: message.key, hosts });
            refreshButtons(false);
            return;
        }
    });

    function refreshButtons(negotiate = true) {
        const candidates = isDirectPDF
            ? new Map([[absoluteURL(window.location.href), []]])
            : embeddedPDFs();
        // Si se repite una URL en varios marcos, se elige uno. Al retirarlo,
        // otro marco ya preparado recupera el botón; si no queda ninguno,
        // vuelve el respaldo de la página.
        for (const [url, frames] of readyFrames) {
            const currentHosts = embeddedPDFs().get(url) || [];
            for (const [frame, state] of frames) {
                // Los visores de plugins no siempre aparecen en window.frames;
                // se conserva la relación con su elemento real, no con un índice.
                const active = state.hosts.some((host) => currentHosts.includes(host));
                let closed = false;
                try { closed = frame.closed === true; } catch (err) { closed = true; }
                if (!active || closed) frames.delete(frame);
            }
            if (!candidates.has(url) || frames.size === 0) {
                readyFrames.delete(url);
                continue;
            }
            const owner = frames.keys().next().value;
            for (const [frame, state] of frames) {
                frame.postMessage({ channel, key: state.key,
                    action: !otherFrameOwnsDocument && frame === owner ? 'selected' : 'owned' }, '*');
            }
            candidates.delete(url);
        }
        if (isDirectPDF && otherFrameOwnsDocument) candidates.clear();
        for (const [url, controls] of buttons) {
            if (!candidates.has(url)) {
                controls.button.remove();
                controls.notice.remove();
                buttons.delete(url);
            }
        }
        for (const url of candidates.keys()) {
            if (url && !buttons.has(url)) {
                buttons.set(url, createSignButton(url));
                if (isDirectPDF) documentCanRender = true;
            }
        }
        // Repite la negociación si el padre se inyecta después del marco.
        if (negotiate && embeddedPDFs().size > 0) {
            for (let frameIndex = 0; frameIndex < window.frames.length; frameIndex++) {
                window.frames[frameIndex].postMessage({ channel, action: 'probe-request' }, '*');
            }
        }
        // Evita que los botones de distintos documentos se superpongan.
        let index = 0;
        for (const { button, notice } of buttons.values()) {
            button.style.bottom = `${20 + index * 112}px`;
            notice.style.bottom = `${76 + index * 112}px`;
            index++;
        }
        if (negotiate) void probeParent();
    }

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

    refreshButtons();
    // Incluye inserciones tardías, cambios de URL y retirada de marcos.
    const observer = new MutationObserver(() => refreshButtons());
    observer.observe(document.documentElement, {
        childList: true, subtree: true, attributes: true,
        attributeFilter: ['src', 'data', 'type']
    });
})();
