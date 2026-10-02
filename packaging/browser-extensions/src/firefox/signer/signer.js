// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Popup script for Native Messaging extension
let signatureImageB64 = null;

document.addEventListener('DOMContentLoaded', async () => {
    const statusDiv = document.getElementById('status');
    const getCertsBtn = document.getElementById('getCerts');
    const openOptionsBtn = document.getElementById('openOptions');
    const certList = document.getElementById('certList');

    // Test connection
    testConnection();

    // Auto-load certificates on open
    loadCertificates();

    // Get certificates button (now acts as refresh)
    getCertsBtn.addEventListener('click', loadCertificates);

    // Open options button
    openOptionsBtn.addEventListener('click', () => {
        chrome.runtime.openOptionsPage();
    });


    // Help modal (includes About section)
    const showHelpBtn = document.getElementById('showHelp');
    const helpModal = document.getElementById('helpModal');
    const closeHelpBtn = document.getElementById('closeHelp');

    showHelpBtn.addEventListener('click', () => {
        helpModal.style.display = 'block';
    });

    closeHelpBtn.addEventListener('click', () => {
        helpModal.style.display = 'none';
    });

    // Close modal on click outside
    helpModal.addEventListener('click', (e) => {
        if (e.target === helpModal) helpModal.style.display = 'none';
    });

    // Help button hover effects (CSP-compliant)
    showHelpBtn.addEventListener('mouseover', () => {
        showHelpBtn.style.color = '#2563eb';
    });

    showHelpBtn.addEventListener('mouseout', () => {
        showHelpBtn.style.color = '#6b7280';
    });

    // Options button hover effects (CSP-compliant) - reuse existing openOptionsBtn
    openOptionsBtn.addEventListener('mouseover', () => {
        openOptionsBtn.style.color = '#2563eb';
    });

    openOptionsBtn.addEventListener('mouseout', () => {
        openOptionsBtn.style.color = '#6b7280';
    });

    // ===== Signature Options Setup =====
    setupSignatureOptions();

    // Check for PDF URL in query parameters (from detector)
    const urlParams = new URLSearchParams(window.location.search);
    const pdfUrl = urlParams.get('pdfUrl');
    if (pdfUrl) {
        autoFetchPDF(pdfUrl);
    }
});

function showUntrustedStatus(container, message) {
    const status = document.createElement('div');
    status.className = 'status disconnected';
    status.textContent = message;
    container.replaceChildren(status);
}

// Automatically fetch a PDF from a URL
async function autoFetchPDF(url) {
    const fileName = document.getElementById('fileName');
    const signResult = document.getElementById('signResult');

    try {
        console.log('Fetching PDF from URL:', url);
        fileName.textContent = '⏳ Descargando PDF...';
        fileName.style.display = 'block';

        const response = await fetch(url);
        if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`);

        const blob = await response.blob();
        ensureExtensionTransferSize(blob.size);
        const urlParts = url.split('/');
        const name = urlParts[urlParts.length - 1] || 'documento.pdf';

        selectedFile = new File([blob], name, { type: 'application/pdf' });
        fileName.textContent = `📄 ${selectedFile.name} (${(selectedFile.size / 1024).toFixed(1)} KB)`;
        console.log('PDF fetched successfully:', selectedFile.name);

    } catch (error) {
        selectedFile = null;
        console.error('Error fetching PDF:', error);
        showUntrustedStatus(signResult, `❌ Error al descargar PDF: ${error.message}`);
    }
}

// Setup signature options UI
function setupSignatureOptions() {
    const formatSelect = document.getElementById('formatSelect');
    const signatureOptions = document.getElementById('signatureOptions');
    const enableToggle = document.getElementById('enableVisibleSignature');
    const signatureConfig = document.getElementById('signatureConfig');
    const positionSelect = document.getElementById('signaturePosition');
    const customPositionInputs = document.getElementById('customPositionInputs');

    // Load saved preferences
    loadSignaturePreferences();

    // Show/hide signature options based on format
    formatSelect.addEventListener('change', () => {
        const isPDF = formatSelect.value === 'pades';
        signatureOptions.style.display = isPDF ? 'block' : 'none';
    });

    // Trigger initial state
    const isPDF = formatSelect.value === 'pades';
    signatureOptions.style.display = isPDF ? 'block' : 'none';

    // Enable/disable signature config
    enableToggle.addEventListener('change', () => {
        signatureConfig.style.display = enableToggle.checked ? 'block' : 'none';
        saveSignaturePreferences();
    });

    // Show/hide custom position inputs
    positionSelect.addEventListener('change', () => {
        const isCustom = positionSelect.value === 'custom';
        customPositionInputs.style.display = isCustom ? 'block' : 'none';
        saveSignaturePreferences();
    });

    // Pro: QR and Metadata toggles
    const enableQR = document.getElementById('enableQR');
    const qrConfig = document.getElementById('qrConfig');
    enableQR.addEventListener('change', () => {
        qrConfig.style.display = enableQR.checked ? 'block' : 'none';
        saveSignaturePreferences();
    });

    const enableMetadata = document.getElementById('enableMetadata');
    const metadataConfig = document.getElementById('metadataConfig');
    enableMetadata.addEventListener('change', () => {
        metadataConfig.style.display = enableMetadata.checked ? 'flex' : 'none';
        saveSignaturePreferences();
    });

    // Save preferences on any change
    const preferenceInputs = [
        'sigX', 'sigY', 'sigWidth', 'sigHeight', 'signaturePage', 'keepSignatureText',
        'enableQR', 'qrContent', 'enableMetadata', 'metaTitle', 'metaSubject'
    ];
    preferenceInputs.forEach(id => {
        const element = document.getElementById(id);
        if (element) {
            element.addEventListener('change', saveSignaturePreferences);
            if (element.type === 'text') {
                element.addEventListener('input', saveSignaturePreferences);
            }
        }
    });

    // Image Signature Handling
    setupImageSignature();
}

// Handler for image signature selection and preview
function setupImageSignature() {
    const sigImageInput = document.getElementById('sigImageInput');
    const selectSigImageBtn = document.getElementById('selectSigImage');
    const removeSigImageBtn = document.getElementById('removeSigImage');
    const previewContainer = document.getElementById('sigImagePreviewContainer');
    const previewImg = document.getElementById('sigImagePreview');

    selectSigImageBtn.addEventListener('click', () => {
        sigImageInput.click();
    });

    sigImageInput.addEventListener('change', (e) => {
        const file = e.target.files[0];
        if (!file) return;

        if (!file.type.startsWith('image/')) {
            alert('Por favor, selecciona un archivo de imagen (PNG o JPG).');
            return;
        }

        const reader = new FileReader();
        reader.onload = (event) => {
            signatureImageB64 = event.target.result;
            previewImg.src = signatureImageB64;
            previewContainer.style.display = 'block';
            saveSignaturePreferences();
        };
        reader.readAsDataURL(file);
    });

    removeSigImageBtn.addEventListener('click', () => {
        signatureImageB64 = null;
        previewImg.src = '';
        previewContainer.style.display = 'none';
        sigImageInput.value = '';
        saveSignaturePreferences();
    });
}

// Load signature preferences from storage
async function loadSignaturePreferences() {
    try {
        const result = await chrome.storage.local.get('signaturePreferences');
        const prefs = result.signaturePreferences || {
            enabled: true,
            position: 'bottom-right',
            customX: 50,
            customY: 50,
            width: 200,
            height: 100,
            page: 'last'
        };

        // Apply to UI
        document.getElementById('enableVisibleSignature').checked = prefs.enabled;
        document.getElementById('signaturePosition').value = prefs.position;
        document.getElementById('sigX').value = prefs.customX;
        document.getElementById('sigY').value = prefs.customY;
        document.getElementById('sigWidth').value = prefs.width;
        document.getElementById('sigHeight').value = prefs.height;
        document.getElementById('signaturePage').value = prefs.page;
        document.getElementById('keepSignatureText').checked = prefs.keepText !== false;

        // QR and Metadata
        document.getElementById('enableQR').checked = prefs.enableQR || false;
        document.getElementById('qrContent').value = prefs.qrContent || '';
        document.getElementById('enableMetadata').checked = prefs.enableMetadata || false;
        document.getElementById('metaTitle').value = prefs.metaTitle || '';
        document.getElementById('metaSubject').value = prefs.metaSubject || '';

        // Load image if present
        if (prefs.imageB64) {
            signatureImageB64 = prefs.imageB64;
            const previewImg = document.getElementById('sigImagePreview');
            const previewContainer = document.getElementById('sigImagePreviewContainer');
            if (previewImg && previewContainer) {
                previewImg.src = signatureImageB64;
                previewContainer.style.display = 'block';
            }
        }

        // Update UI state
        document.getElementById('signatureConfig').style.display = prefs.enabled ? 'block' : 'none';
        document.getElementById('customPositionInputs').style.display = prefs.position === 'custom' ? 'block' : 'none';
        document.getElementById('qrConfig').style.display = prefs.enableQR ? 'block' : 'none';
        document.getElementById('metadataConfig').style.display = prefs.enableMetadata ? 'flex' : 'none';
    } catch (error) {
        console.error('Error loading signature preferences:', error);
    }
}

// Save signature preferences to storage
async function saveSignaturePreferences() {
    try {
        const prefs = {
            enabled: document.getElementById('enableVisibleSignature').checked,
            position: document.getElementById('signaturePosition').value,
            customX: parseInt(document.getElementById('sigX').value),
            customY: parseInt(document.getElementById('sigY').value),
            width: parseInt(document.getElementById('sigWidth').value),
            height: parseInt(document.getElementById('sigHeight').value),
            page: document.getElementById('signaturePage').value,
            keepText: document.getElementById('keepSignatureText').checked,
            imageB64: signatureImageB64,
            enableQR: document.getElementById('enableQR').checked,
            qrContent: document.getElementById('qrContent').value,
            enableMetadata: document.getElementById('enableMetadata').checked,
            metaTitle: document.getElementById('metaTitle').value,
            metaSubject: document.getElementById('metaSubject').value
        };

        await chrome.storage.local.set({ signaturePreferences: prefs });
        console.log('Signature preferences saved:', prefs);
    } catch (error) {
        console.error('Error saving signature preferences:', error);
    }
}

// Get current signature options for signing
function getSignatureOptions() {
    const enabled = document.getElementById('enableVisibleSignature').checked;

    if (!enabled) {
        return { enabled: false };
    }

    return {
        enabled: true,
        position: document.getElementById('signaturePosition').value,
        customX: parseInt(document.getElementById('sigX').value),
        customY: parseInt(document.getElementById('sigY').value),
        width: parseInt(document.getElementById('sigWidth').value),
        height: parseInt(document.getElementById('sigHeight').value),
        page: document.getElementById('signaturePage').value,
        keepText: document.getElementById('keepSignatureText').checked,
        imageB64: signatureImageB64,
        enableQR: document.getElementById('enableQR').checked,
        qrContent: document.getElementById('qrContent').value,
        enableMetadata: document.getElementById('enableMetadata').checked,
        metaTitle: document.getElementById('metaTitle').value,
        metaSubject: document.getElementById('metaSubject').value
    };
}

async function loadCertificates() {
    const getCertsBtn = document.getElementById('getCerts');
    const certList = document.getElementById('certList');

    getCertsBtn.disabled = true;
    getCertsBtn.textContent = '⏳ Cargando certificados...';
    certList.innerHTML = '<div class="loading">Cargando...</div>';

    try {
        const response = await chrome.runtime.sendMessage({
            action: 'getSystemCertificates'
        });

        if (response.success) {
            displayCertificates(response.certificates);
        } else {
            showUntrustedStatus(certList, `❌ Error: ${response.error}`);
        }
    } catch (error) {
        showUntrustedStatus(certList, `❌ Error: ${error.message}`);
    } finally {
        getCertsBtn.disabled = false;
        getCertsBtn.textContent = '🔄 Recargar Certificados';
    }
}

async function testConnection() {
    const statusDiv = document.getElementById('status');

    try {
        const response = await chrome.runtime.sendMessage({ action: 'ping' });

        if (response.success) {
            statusDiv.className = 'status connected';
            statusDiv.textContent = '✅ Conectado con native host';
        } else {
            statusDiv.className = 'status disconnected';
            statusDiv.textContent = '⚠️ Native host no responde';
        }
    } catch (error) {
        statusDiv.className = 'status disconnected';
        statusDiv.textContent = '❌ Error: ' + error.message;
    }
}

function displayCertificates(certificates) {
    const certList = document.getElementById('certList');
    const certSelect = document.getElementById('certSelect');

    if (!certificates || certificates.length === 0) {
        certList.innerHTML = '<div class="status">ℹ️ No se encontraron certificados</div>';
        return;
    }

    // Clear and populate certificate selector
    certSelect.innerHTML = '<option value="">Seleccionar certificado...</option>';

    // Create collapsible header
    certList.innerHTML = `
        <div style="display: flex; align-items: center; justify-content: space-between; cursor: pointer; padding: 10px; background: #f3f4f6; border-radius: 5px; margin: 10px 0;" id="certListToggle">
            <h3 style="font-size: 14px; margin: 0;" id="certListCount"></h3>
            <span id="certListArrow" style="font-size: 18px;">▶</span>
        </div>
        <div id="certListContent" style="display: none;"></div>
    `;

    document.getElementById('certListCount').textContent = `Certificados encontrados (${certificates.length})`;
    const certListContent = document.getElementById('certListContent');
    const toggle = document.getElementById('certListToggle');
    const arrow = document.getElementById('certListArrow');

    // Toggle visibility
    toggle.onclick = () => {
        const isHidden = certListContent.style.display === 'none';
        certListContent.style.display = isHidden ? 'block' : 'none';
        arrow.textContent = isHidden ? '▼' : '▶';
    };

    certificates.forEach(cert => {
        const div = document.createElement('div');
        div.className = 'cert-item';

        const sourceClass = cert.source === 'dnie' ? 'dnie' : 'system';
        const sourceLabel = cert.source === 'dnie' ? 'DNIe' :
            cert.source === 'smartcard' ? 'Tarjeta' : 'Sistema';

        const subject = document.createElement('strong');
        subject.textContent = cert.subject?.CN || 'Sin nombre';
        const source = document.createElement('span');
        source.className = `cert-source ${sourceClass}`;
        source.textContent = sourceLabel;
        const issuer = document.createElement('small');
        issuer.textContent = `Emisor: ${cert.issuer?.CN || cert.issuer?.O || 'Desconocido'}`;
        const validity = document.createElement('small');
        validity.textContent = `Válido: ${new Date(cert.validFrom).toLocaleDateString()} - ${new Date(cert.validTo).toLocaleDateString()}`;
        div.append(subject, source, document.createElement('br'), issuer, document.createElement('br'), validity);

        certListContent.appendChild(div);

        // Add to selector
        const option = document.createElement('option');
        option.value = cert.id;
        option.textContent = cert.subject.CN || 'Sin nombre';
        option.dataset.cert = JSON.stringify(cert);
        certSelect.appendChild(option);
    });

    // Enable signing UI
    setupSigningUI();
}

// Setup signing UI handlers
let signingUISetup = false;
let selectedFile = null;
const MAX_EXTENSION_TRANSFER_BYTES = 46 * 1024 * 1024;

function ensureExtensionTransferSize(...sizes) {
    const total = sizes.reduce((sum, size) => sum + (Number(size) || 0), 0);
    if (total > MAX_EXTENSION_TRANSFER_BYTES) {
        throw new Error('El navegador admite hasta 46 MiB por operación de firma o verificación');
    }
}

function setupSigningUI() {
    // Only setup once
    if (signingUISetup) return;
    signingUISetup = true;

    const fileInput = document.getElementById('fileInput');
    const fileName = document.getElementById('fileName');
    const signBtn = document.getElementById('signDocument');
    const signResult = document.getElementById('signResult');
    const progressContainer = document.getElementById('progressContainer');
    const progressBar = document.getElementById('progressBar');
    const progressText = document.getElementById('progressText');

    // Listen for progress messages
    chrome.runtime.onMessage.addListener((message) => {
        if (message.action === 'signingProgress') {
            progressContainer.style.display = 'block';
            progressBar.style.width = `${message.percent}%`;
            progressText.textContent = `${message.percent}% (${message.current}/${message.total})`;
        }
    });

    // File input change handler
    fileInput.onchange = async (e) => {
        e.preventDefault();
        e.stopPropagation();
        try {
            selectedFile = e.target.files[0];
            if (selectedFile) {
                ensureExtensionTransferSize(selectedFile.size);
                fileName.textContent = `📄 ${selectedFile.name} (${(selectedFile.size / 1024).toFixed(1)} KB)`;
                fileName.style.display = 'block';
                console.log('File selected:', selectedFile.name);

            }
        } catch (error) {
            selectedFile = null;
            e.target.value = '';
            console.error('Error selecting file:', error);
            showUntrustedStatus(signResult, `❌ Error al seleccionar archivo: ${error.message}`);
        }
        return false;
    };

    // Sign button
    signBtn.onclick = async () => {
        const certId = document.getElementById('certSelect').value;
        const format = document.getElementById('formatSelect').value;
        // Password handled automatically by backend

        if (!certId) {
            signResult.innerHTML = '<div class="status disconnected">⚠️ Selecciona un certificado</div>';
            return;
        }

        if (!selectedFile) {
            signResult.innerHTML = '<div class="status disconnected">⚠️ Selecciona un archivo</div>';
            return;
        }

        // Password not required for system store

        signBtn.disabled = true;
        signBtn.textContent = '⏳ Firmando...';
        signResult.innerHTML = '<div class="loading">Firmando documento...</div>';

        // Reset progress
        progressContainer.style.display = 'block';
        progressBar.style.width = '0%';
        progressText.textContent = '0%';

        try {
            ensureExtensionTransferSize(selectedFile.size);
            // Read file as base64
            const fileData = await readFileAsBase64(selectedFile);

            // Get signature options (only for PDF)
            const signatureOptions = format === 'pades' ? getSignatureOptions() : null;
            console.log('[Popup] Sending signatureOptions:', signatureOptions);

            // Send to background for signing
            const response = await chrome.runtime.sendMessage({
                action: 'signDocument',
                certificateId: certId,
                data: fileData,
                pin: '', // Not used
                format: format,
                signatureOptions: signatureOptions
            });

            if (response.success) {
                // Download signed file
                let mimeType = 'application/octet-stream';
                let filename = selectedFile.name;

                if (format === 'pades') {
                    mimeType = 'application/pdf';
                    const cleanName = filename.replace(/\.pdf$/i, '');
                    filename = `${cleanName}_firmado.pdf`;
                } else {
                    mimeType = 'application/pkcs7-signature'; // Standard for p7s
                    const extension = getSignedFileExtension(format, selectedFile.name);
                    filename = `${filename}.${extension}`;
                }

                console.log(`Downloading ${filename} (${mimeType}), size: ${response.signature.length} base64 chars`);

                // Calculate expected binary size (base64 to binary is roughly 3/4)
                const expectedSize = Math.floor(response.signature.length * 3 / 4);
                console.log(`Expected binary size: ~${expectedSize} bytes (${(expectedSize / 1024).toFixed(2)} KB)`);

                const signedData = base64ToBlob(response.signature, mimeType);
                console.log(`Blob size: ${signedData.size} bytes (${(signedData.size / 1024).toFixed(2)} KB)`);

                // CRITICAL: Verify blob wasn't truncated
                if (signedData.size < expectedSize * 0.95) {
                    throw new Error(`Truncamiento de archivo detectado. Se esperaban ~${expectedSize} bytes, se recibieron ${signedData.size} bytes. La conversión falló.`);
                }

                // Diagnostic Check
                if (mimeType === 'application/pdf') {
                    const reader = new FileReader();
                    reader.onload = () => {
                        const text = reader.result;
                        const header = text.substring(0, 20);
                        const tail = text.substring(text.length - 20);
                        console.log(`🔍 PDF Diagnostics:`);
                        console.log(`   Header (Hex): ${text.substring(0, 8).split('').map(c => c.charCodeAt(0).toString(16).padStart(2, '0')).join(' ')}`);
                        console.log(`   Header (Txt): ${header.replace(/\n/g, '\\n')}`);
                        console.log(`   Tail   (Txt): ${tail.replace(/\n/g, '\\n')}`);

                        if (!header.startsWith('%PDF-')) {
                            console.error('❌ CRITICAL: File does not start with %PDF-');
                            signResult.innerHTML += '<div style="color:red; font-size:10px; margin-top:5px;">⚠️ Advertencia: El PDF generado parece inválido (Cabecera incorrecta).</div>';
                        }
                    };
                    reader.readAsBinaryString(signedData.slice(0, 100)); // Read start
                    // Also read fail? We can't read twice easily here without chain.
                    // Just read first 100 bytes is enough for header.
                }

                if (signedData.size < 100) {
                    throw new Error("El archivo generado está vacío o es demasiado pequeño");
                }

                downloadFile(signedData, filename);

                signResult.innerHTML = '<div class="status connected">✅ Documento firmado correctamente</div>';
                progressText.textContent = '100% (Completado)';
                progressBar.style.width = '100%';
            } else {
                showUntrustedStatus(signResult, `❌ Error: ${response.error}`);
                progressContainer.style.display = 'none';
            }
        } catch (error) {
            showUntrustedStatus(signResult, `❌ Error: ${error.message}`);
            progressContainer.style.display = 'none';
        } finally {
            signBtn.disabled = false;
            signBtn.textContent = '✍️ Firmar Documento';
        }
    };
}

// Helper functions
function readFileAsBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
            const base64 = reader.result.split(',')[1];
            resolve(base64);
        };
        reader.onerror = reject;
        reader.readAsDataURL(file);
    });
}

function base64ToBlob(base64, mimeType = 'application/octet-stream') {
    // CRITICAL FIX: atob() has size limits in some browsers (~512KB base64)
    // Use chunked decoding for large strings
    const chunkSize = 8192; // Process 8KB of base64 at a time
    const len = base64.length;
    let bytes;

    if (len > chunkSize) {
        // Chunked approach for large data
        console.log(`Using chunked base64 decode (${len} chars, ${Math.ceil(len / chunkSize)} chunks)`);
        const chunks = [];
        for (let i = 0; i < len; i += chunkSize) {
            const chunk = base64.substring(i, i + chunkSize);
            chunks.push(atob(chunk));
        }
        const binary = chunks.join('');
        bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
    } else {
        // Direct approach for small data
        const binary = atob(base64);
        bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
    }

    return new Blob([bytes], { type: mimeType });
}

function getSignedFileExtension(format, originalName) {
    switch (format) {
        case 'pades':
            return 'pdf';
        case 'xades':
            return 'xml';
        case 'cades':
            return 'p7s';
        default:
            return 'signed';
    }
}

function downloadFile(blob, filename) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// Setup verification UI
document.addEventListener('DOMContentLoaded', () => {
    setupVerificationUI();
});

let verifyOriginalFileData = null;
let verifySignatureFileData = null;

function setupVerificationUI() {
    const formatSelect = document.getElementById('verifyFormat');
    const signatureContainer = document.getElementById('verifySignatureContainer');
    const originalLabel = document.getElementById('verifyOriginalLabel');

    const selectOriginalBtn = document.getElementById('selectOriginalFile');
    const originalFileInput = document.getElementById('verifyOriginalFile');
    const originalFileName = document.getElementById('originalFileName');

    const selectSignatureBtn = document.getElementById('selectSignatureFile');
    const signatureFileInput = document.getElementById('verifySignatureFile');
    const signatureFileName = document.getElementById('signatureFileName');

    const verifyBtn = document.getElementById('verifySignature');
    const verifyResult = document.getElementById('verifyResult');

    // Handle format change
    formatSelect.onchange = () => {
        const format = formatSelect.value;
        if (format === 'pades') {
            signatureContainer.style.display = 'none';
            originalLabel.textContent = 'PDF Firmado:';
            selectOriginalBtn.textContent = '📄 Seleccionar PDF Firmado';
        } else {
            signatureContainer.style.display = 'block';
            originalLabel.textContent = 'Archivo Original:';
            selectOriginalBtn.textContent = '📄 Seleccionar Archivo Original';
        }
    };

    // Trigger initial state
    formatSelect.onchange();

    // Select original file
    selectOriginalBtn.onclick = () => {
        originalFileInput.click();
    };

    originalFileInput.onchange = async (e) => {
        const file = e.target.files[0];
        if (file) {
            try {
                ensureExtensionTransferSize(file.size, verifySignatureFileData?.byteLength);
                verifyOriginalFileData = await readFileAsArrayBuffer(file);
                originalFileName.textContent = `📄 ${file.name} (${(file.size / 1024).toFixed(1)} KB)`;
            } catch (error) {
                verifyOriginalFileData = null;
                originalFileInput.value = '';
                showUntrustedStatus(verifyResult, `❌ ${error.message}`);
            }
        }
    };

    // Select signature file
    selectSignatureBtn.onclick = () => {
        signatureFileInput.click();
    };

    signatureFileInput.onchange = async (e) => {
        const file = e.target.files[0];
        if (file) {
            try {
                ensureExtensionTransferSize(file.size, verifyOriginalFileData?.byteLength);
                verifySignatureFileData = await readFileAsArrayBuffer(file);
                signatureFileName.textContent = `📝 ${file.name} (${(file.size / 1024).toFixed(1)} KB)`;
            } catch (error) {
                verifySignatureFileData = null;
                signatureFileInput.value = '';
                showUntrustedStatus(verifyResult, `❌ ${error.message}`);
            }
        }
    };

    // Verify button
    verifyBtn.onclick = async () => {
        const format = formatSelect.value;

        if (!verifyOriginalFileData) {
            verifyResult.innerHTML = '<div class="status disconnected">⚠️ Selecciona el archivo principal</div>';
            return;
        }

        if (format !== 'pades' && !verifySignatureFileData) {
            verifyResult.innerHTML = '<div class="status disconnected">⚠️ Selecciona el archivo de firma (.p7s)</div>';
            return;
        }

        verifyBtn.disabled = true;
        verifyBtn.textContent = '⏳ Verificando...';
        verifyResult.innerHTML = '<div class="loading">Verificando firma...</div>';

        try {
            ensureExtensionTransferSize(
                verifyOriginalFileData.byteLength,
                verifySignatureFileData?.byteLength
            );
            const originalB64 = arrayBufferToBase64(verifyOriginalFileData);
            const signatureB64 = verifySignatureFileData ? arrayBufferToBase64(verifySignatureFileData) : '';

            const response = await chrome.runtime.sendMessage({
                action: 'verifySignature',
                originalData: originalB64,
                signatureData: signatureB64,
                format: format
            });

            if (response.success && response.result.valid) {
                verifyResult.innerHTML = '<div class="status connected"><strong>✅ Firma válida</strong><br><small class="verify-signer"></small><br><small class="verify-date"></small><br><small class="verify-format"></small><br><small class="verify-algorithm"></small></div>';
                verifyResult.querySelector('.verify-signer').textContent = `Firmante: ${response.result.signerName || 'Desconocido'}`;
                verifyResult.querySelector('.verify-date').textContent = `Fecha: ${response.result.timestamp || 'No disponible'}`;
                verifyResult.querySelector('.verify-format').textContent = `Formato: ${response.result.format || format.toUpperCase()}`;
                verifyResult.querySelector('.verify-algorithm').textContent = `Algoritmo: ${response.result.algorithm || 'SHA256withRSA'}`;
            } else {
                const rawReason = response.result?.reason || response.error || 'Error desconocido';
                const reason = rawReason === 'revocación no concluyente'
                    ? (chrome.i18n.getMessage('revocationInconclusive') || rawReason)
                    : rawReason;
                verifyResult.innerHTML = '<div class="status disconnected"><strong>❌ Firma inválida</strong><br><small class="verify-reason"></small></div>';
                verifyResult.querySelector('.verify-reason').textContent = reason;
            }
        } catch (error) {
            verifyResult.innerHTML = '<div class="status disconnected"></div>';
            verifyResult.firstElementChild.textContent = `❌ Error: ${error.message}`;
        } finally {
            verifyBtn.disabled = false;
            verifyBtn.textContent = '✓ Verificar Firma';
        }
    };
}

function readFileAsArrayBuffer(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(reader.result);
        reader.onerror = reject;
        reader.readAsArrayBuffer(file);
    });
}

function arrayBufferToBase64(buffer) {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.length; i++) {
        binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary);
}
