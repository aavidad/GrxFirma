// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Content script to detect PDF files and offer signing
(function () {
    console.log('GrxFirma: PDF detector active');
    const t = grxfirmaExt.i18n.message;
    const MAX_PRELOADED_BYTES = Math.floor(2.5 * 1024 * 1024);
    const PDF_FETCH_TIMEOUT_MS = 15 * 1000;

    function currentPDFURL() {
        const embedded = document.querySelector('embed[type="application/pdf"],object[type="application/pdf"]');
        if (embedded) {
            const candidate = embedded.getAttribute('src') || embedded.getAttribute('data') || embedded.src || embedded.data;
            if (candidate) {
                return candidate;
            }
        }
        return window.location.href;
    }

    function isPDF() {
        // Check URL extension
        if (window.location.pathname.toLowerCase().endsWith('.pdf')) return true;

        // Check content type if available
        if (document.contentType === 'application/pdf') return true;

        // Check for PDF viewer elements (Chrome/Edge/Firefox)
        if (document.querySelector('embed[type="application/pdf"]') ||
            document.querySelector('object[type="application/pdf"]')) {
            return true;
        }

        return false;
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

    async function buildPendingDocument() {
        const pdfUrl = currentPDFURL();
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

    // Con all_frames, un PDF incrustado con <embed>/<object> se detecta dos
    // veces: en la página que lo contiene y en el documento PDF interno. Solo
    // pone el botón el marco que contiene el elemento; un PDF en <iframe> lo
    // pone el propio marco, porque la página superior no lo detecta.
    function parentFrameHandlesPDF() {
        let host = null;
        try { host = window.frameElement; } catch (err) { return false; }
        if (!host || typeof host.tagName !== 'string') return false;
        const tag = host.tagName.toUpperCase();
        return tag === 'EMBED' || tag === 'OBJECT';
    }

    if (isPDF() && !parentFrameHandlesPDF()) {
        console.log('GrxFirma: PDF detected!');
        createSignButton();
    }

    function createSignButton() {
        // Check if already exists
        if (document.getElementById('grxfirma-sign-trigger')) return;

        const button = document.createElement('button');
        button.id = 'grxfirma-sign-trigger';
        button.type = 'button';
        button.setAttribute('aria-label', t('pdfSignButtonAriaLabel'));

        // Sin icono: una imagen de la extensión exigiría web_accessible_resources
        // y permitiría a cualquier página detectar la extensión.
        const label = document.createElement('span');
        label.textContent = t('pdfSignButtonLabel');
        label.style.fontWeight = 'bold';
        button.append(label);

        // Modern floating style
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
            zIndex: '2147483647', // Max z-index
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
        notice.id = 'grxfirma-sign-notice';
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
            try { pendingDocument = await buildPendingDocument(); }
            catch (error) { preloadSkipped = true; console.warn('GrxFirma: PDF preload unavailable:', error); }
            try {
                const response = await grxfirmaExt.runtime.sendMessage({
                    action: 'openSigner', pdfUrl: currentPDFURL(), pendingDocument
                });
                if (!response || response.success !== true) {
                    button.title = t('pdfSignerUnavailable');
                    showNotice(response && response.code === 'opening_pending' ? 'pdfOpeningPending' : 'pdfSignerUnavailable');
                    return;
                }
                button.title = '';
                if (preloadSkipped || response.preloadSkipped) showNotice('pdfPreloadUnavailable');
                else showNotice('');
            } catch (error) {
                console.error('GrxFirma: Error opening signer:', error);
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
    }
})();
