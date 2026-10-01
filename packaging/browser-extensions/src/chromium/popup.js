// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

document.addEventListener('DOMContentLoaded', async () => {
  grxfirmaExt.i18n.localizeDocument();
  const t = grxfirmaExt.i18n.message;
  const status = document.getElementById('status');
  const localURL = (CONFIG.LOCAL_REST_URL || 'https://127.0.0.1:63118').replace(/\/+$/, '');
  document.getElementById('openApp').addEventListener('click', () => {
    grxfirmaExt.tabs.create({ url: localURL + '/' });
  });
  document.getElementById('openOptions').addEventListener('click', () => {
    grxfirmaExt.tabs.create({ url: grxfirmaExt.runtime.getURL('options.html') });
  });
  try {
    const response = await grxfirmaExt.runtime.sendMessage({ action: 'ping' });
    if (!response || !response.success) throw new Error('host_unavailable');
    status.className = 'status ok';
    status.textContent = response.mode === 'rest' ? t('popupRestAvailable') : t('popupNativeHostConnected');
  } catch (_) {
    status.className = 'status bad';
    status.textContent = t('popupNativeHostUnavailable');
  }
});
