// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

document.addEventListener('DOMContentLoaded', () => {
  grxfirmaExt.i18n.localizeDocument();
  const t = grxfirmaExt.i18n.message;
  const input = document.getElementById('siteInput');
  const list = document.getElementById('siteList');
  const feedback = document.getElementById('feedback');
  let managedSites = [];
  let userSites = [];
  let managedMissing = new Set();

  function say(key) { feedback.textContent = t(key); }
  async function readSites() {
    const local = await grxfirmaExt.storageLocal.get('sitiosConfianza');
    let managed = {};
    try { managed = await grxfirmaExt.storageManaged.get('sitiosConfianza'); } catch (_) { /* Sin política. */ }
    userSites = grxfirmaTrustedSites.normalizeList(local.sitiosConfianza);
    managedSites = grxfirmaTrustedSites.normalizeList(managed.sitiosConfianza);
    managedMissing = new Set();
    for (const site of managedSites) {
      if (grxfirmaTrustedSites.FACTORY_SITES.includes(site)) continue;
      try {
        if (!await grxfirmaExt.permissions.contains({ origins: [grxfirmaTrustedSites.pattern(site)] })) managedMissing.add(site);
      } catch (_) { managedMissing.add(site); }
    }
  }
  function render() {
    list.replaceChildren();
    for (const site of new Set([...grxfirmaTrustedSites.FACTORY_SITES, ...managedSites, ...userSites])) {
      const item = document.createElement('li');
      const name = document.createElement('span');
      name.textContent = site;
      item.appendChild(name);
      const note = document.createElement('span');
      note.className = 'muted';
      if (grxfirmaTrustedSites.FACTORY_SITES.includes(site)) note.textContent = t('optionsFactorySite');
      else if (managedSites.includes(site)) note.textContent = t('optionsManagedSite');
      if (note.textContent) item.appendChild(note);
      if (managedMissing.has(site)) {
        const missing = document.createElement('span');
        missing.className = 'muted';
        missing.textContent = t('optionsManagedPermissionMissing');
        item.appendChild(missing);
      }
      if (userSites.includes(site) && !managedSites.includes(site) && !grxfirmaTrustedSites.FACTORY_SITES.includes(site)) {
        const remove = document.createElement('button');
        remove.type = 'button';
        remove.className = 'secondary';
        remove.textContent = t('optionsRemoveSite');
        remove.setAttribute('aria-label', t('optionsRemoveSite') + ': ' + site);
        remove.addEventListener('click', async () => {
          try {
            await grxfirmaExt.permissions.remove({ origins: [grxfirmaTrustedSites.pattern(site)] });
            userSites = userSites.filter((value) => value !== site);
            await grxfirmaExt.storageLocal.set({ sitiosConfianza: userSites });
            await grxfirmaExt.runtime.sendMessage({ action: 'refreshTrustedSites' });
            render();
            say('optionsSiteRemoved');
            input.focus();
          } catch (_) { say('optionsSaveFailed'); }
        });
        item.appendChild(remove);
      }
      list.appendChild(item);
    }
  }
  async function refresh() {
    await readSites();
    render();
  }
  document.getElementById('addSiteForm').addEventListener('submit', async (event) => {
    event.preventDefault();
    let site;
    try { site = grxfirmaTrustedSites.normalizeSite(input.value); }
    catch (_) { say('optionsInvalidSite'); input.focus(); return; }
    if (userSites.includes(site) || managedSites.includes(site) || grxfirmaTrustedSites.FACTORY_SITES.includes(site)) {
      say('optionsSiteExists'); input.focus(); return;
    }
    // La petición de permiso empieza dentro del gesto del usuario.
    let granted;
    try { granted = await grxfirmaExt.permissions.request({ origins: [grxfirmaTrustedSites.pattern(site)] }); }
    catch (_) { say('optionsPermissionDenied'); return; }
    if (!granted) { say('optionsPermissionDenied'); return; }
    try {
      userSites.push(site);
      userSites.sort();
      await grxfirmaExt.storageLocal.set({ sitiosConfianza: userSites });
      const result = await grxfirmaExt.runtime.sendMessage({ action: 'refreshTrustedSites' });
      if (!result || !result.success) throw new Error('registration_failed');
      render();
      input.value = '';
      say('optionsSiteAdded');
      input.focus();
    } catch (_) {
      userSites = userSites.filter((value) => value !== site);
      await grxfirmaExt.storageLocal.set({ sitiosConfianza: userSites }).catch(() => {});
      await grxfirmaExt.permissions.remove({ origins: [grxfirmaTrustedSites.pattern(site)] }).catch(() => {});
      say('optionsSaveFailed');
    }
  });
  if (grxfirmaExt.storageOnChanged) grxfirmaExt.storageOnChanged.addListener((changes, area) => {
    if ((area === 'local' || area === 'managed') && changes.sitiosConfianza) refresh().catch(() => say('optionsLoadFailed'));
  });
  refresh().catch(() => say('optionsLoadFailed'));
});
