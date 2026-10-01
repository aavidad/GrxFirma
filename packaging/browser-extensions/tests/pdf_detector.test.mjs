// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
async function detector(variant, fetch) {
  const elements = new Map();
  const sent = [];
  const document = {
    contentType: 'application/pdf', title: 'documento',
    querySelector() { return null; },
    getElementById(id) { return elements.get(id) || null; },
    createElement(tag) {
      return {
        tagName: tag, style: {}, children: [], id: '', title: '', textContent: '',
        setAttribute() {}, append(...nodes) { this.children.push(...nodes); }
      };
    },
    body: { appendChild(element) { elements.set(element.id, element); } }
  };
  const context = vm.createContext({
    document, window: { location: { href: 'https://sede.dipgra.es/archivo.pdf', pathname: '/archivo.pdf' } },
    URL, Blob, AbortController, setTimeout, clearTimeout,
    console: { log() {}, warn() {}, error() {} }, fetch,
    grxfirmaExt: {
      i18n: { message(key) { return key; } },
      runtime: { getURL(path) { return 'moz-extension://test/' + path; },
        async sendMessage(message) { sent.push(message); return { success: true }; } }
    }
  });
  vm.runInContext(await readFile(path.join(root, variant, 'content_scripts/pdf_detector.js'), 'utf8'), context);
  return { button: elements.get('grxfirma-sign-trigger'), notice: elements.get('grxfirma-sign-notice'), sent };
}
for (const variant of ['chromium', 'firefox']) {
  test(`${variant}: ignora clic sintético y bloquea reentrada`, async () => {
    let requested = 0;
    let rejectFetch;
    const h = await detector(variant, () => {
      requested++;
      return new Promise((_, reject) => { rejectFetch = reject; });
    });
    await h.button.onclick({ isTrusted: false });
    assert.equal(requested, 0);
    const first = h.button.onclick({ isTrusted: true });
    await h.button.onclick({ isTrusted: true });
    assert.equal(requested, 1);
    rejectFetch(new Error('sin acceso'));
    await first;
    assert.equal(h.sent.length, 1);
    assert.equal(h.sent[0].pendingDocument, null);
    assert.equal(h.notice.textContent, 'pdfPreloadUnavailable');
  });

  test(`${variant}: corta la lectura incremental al superar 2,5 MiB`, async () => {
    let reads = 0;
    const h = await detector(variant, async () => ({
      ok: true, headers: { get() { return null; } },
      body: { getReader() { return {
        async read() { reads++; return { done: false, value: new Uint8Array(1024 * 1024) }; },
        async cancel() {}
      }; } },
      blob() { throw new Error('No se debe usar blob()'); }
    }));
    await h.button.onclick({ isTrusted: true });
    assert.equal(reads, 3);
    assert.equal(h.sent[0].pendingDocument, null);
    assert.equal(h.notice.textContent, 'pdfPreloadUnavailable');
  });
}
