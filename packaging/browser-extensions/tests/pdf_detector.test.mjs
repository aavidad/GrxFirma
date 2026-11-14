// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import test from 'node:test';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
async function detector(variant, fetch, frame = {}) {
  const elements = new Map();
  const controls = [];
  const sent = [];
  const posted = [];
  let changed;
  const document = {
    contentType: frame.contentType || 'application/pdf', title: 'documento', documentElement: {},
    querySelectorAll() { return frame.embedded || []; },
    getElementById(id) { return elements.get(id) || null; },
    createElement(tag) {
      return {
        tagName: tag, style: {}, children: [], id: '', title: '', textContent: '', attributes: {},
        setAttribute(key, value) { this.attributes[key] = value; },
        append(...nodes) { this.children.push(...nodes); },
        remove() { elements.delete(this.id); controls.splice(controls.indexOf(this), 1); }
      };
    },
    body: { appendChild(element) { if (element.id) elements.set(element.id, element); controls.push(element); } }
  };
  const listeners = [];
  const window = {
    location: frame.location || { href: 'https://sede.dipgra.es/archivo.pdf', pathname: '/archivo.pdf' },
    frames: { length: 0 },
    addEventListener(type, callback) { if (type === 'message') listeners.push(callback); },
    async dispatch(data, source) { await Promise.all(listeners.map((callback) => callback({ data, source }))); }
  };
  window.parent = window;
  if (frame.parent) {
    const parent = frame.parent.window;
    const parentProxy = { postMessage(data) { posted.push(data); void parent.dispatch(data, childProxy); } };
    const childProxy = { parent, postMessage(data) { frame.parent.posted.push(data); void window.dispatch(data, parentProxy); } };
    window.parent = parentProxy;
    parent.frames[parent.frames.length++] = childProxy;
    if (frame.element?.tagName === 'IFRAME') frame.element.contentWindow = childProxy;
  }
  const context = vm.createContext({
    document, window, URL, Blob, AbortController, setTimeout, clearTimeout, TextEncoder,
    crypto: { subtle: { async digest(algorithm, bytes) {
      return Uint8Array.from(createHash('sha256').update(bytes).digest()).buffer;
    } } },
    MutationObserver: class { constructor(callback) { changed = callback; } observe() {} },
    console: { log() {}, warn() {}, error() {} }, fetch,
    grxfirmaExt: {
      i18n: { message(key) { return key; } },
      runtime: { getURL(path) { return 'moz-extension://test/' + path; },
        async sendMessage(message) { sent.push(message); return { success: true }; } }
    }
  });
  const source = await readFile(path.join(root, variant, 'content_scripts/pdf_detector.js'), 'utf8');
  const start = async () => {
    vm.runInContext(source, context);
    await new Promise((resolve) => setImmediate(resolve));
  };
  if (!frame.defer) await start();
  return {
    get button() { return controls.find((node) => node.tagName === 'button'); },
    get notice() { return controls.find((node) => node.attributes.role === 'status'); },
    get buttons() { return controls.filter((node) => node.tagName === 'button'); },
    sent, posted, window, start, changed() { changed(); }
  };
}

function pdfElement(tagName, url = '/archivo.pdf', type = 'application/pdf') {
  return { tagName: tagName.toUpperCase(), getAttribute(key) {
    if (key === 'type') return type;
    return key === (tagName === 'object' ? 'data' : 'src') ? url : null;
  } };
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

  test(`${variant}: el botón no carga recursos de la extensión en la página`, async () => {
    const h = await detector(variant, async () => { throw new Error('sin red'); });
    assert.ok(h.button.children.length > 0);
    assert.ok(h.button.children.every((node) => node.tagName !== 'img'));
    // C-06: sin recursos de la extensión en la página, el manifiesto no expone ninguno.
    const manifest = JSON.parse(await readFile(path.join(root, variant, 'manifest.json'), 'utf8'));
    assert.equal(manifest.web_accessible_resources, undefined);
  });
}

// Regresión: ambos marcos negocian con independencia del orden de inyección.
const paginaConPDF = {
  contentType: 'text/html',
  location: { href: 'https://sede.dipgra.es/expediente', pathname: '/expediente' }
};
for (const variant of ['chromium', 'firefox']) {
  const sinRed = async () => { throw new Error('sin red'); };
  for (const tagName of ['embed', 'object', 'iframe']) {
    for (const parentFirst of [true, false]) {
      test(`${variant}: <${tagName}>, padre primero=${parentFirst}: un botón`, async () => {
        const element = pdfElement(tagName);
        const parent = await detector(variant, sinRed, {
          ...paginaConPDF, embedded: [element], defer: !parentFirst
        });
        const child = await detector(variant, sinRed, { parent, element });
        if (!parentFirst) await parent.start();
        assert.equal(parent.buttons.length + child.buttons.length, 1);
        assert.ok(child.button);
        await child.button.onclick({ isTrusted: true });
        assert.equal(child.sent[0].pdfUrl, 'https://sede.dipgra.es/archivo.pdf');
      });
    }
  }
  test(`${variant}: mantiene el botón si el visor interno no admite scripts`, async () => {
    const h = await detector(variant, sinRed, { ...paginaConPDF, embedded: [pdfElement('iframe')] });
    assert.equal(h.buttons.length, 1);
  });
  test(`${variant}: varios PDF distintos y varias referencias al mismo PDF`, async () => {
    const elements = [pdfElement('embed', '/uno.pdf'), pdfElement('object', '/dos.pdf'),
      pdfElement('iframe', '/uno.pdf#page=2')];
    const h = await detector(variant, sinRed, { ...paginaConPDF, embedded: elements });
    assert.equal(h.buttons.length, 2);
    assert.notEqual(h.buttons[0].style.bottom, h.buttons[1].style.bottom);
    for (const button of h.buttons) await button.onclick({ isTrusted: true });
    assert.deepEqual(h.sent.map((message) => message.pdfUrl),
      ['https://sede.dipgra.es/uno.pdf', 'https://sede.dipgra.es/dos.pdf']);
  });
  test(`${variant}: iframe con URL opaca, el marco detecta el tipo PDF`, async () => {
    const element = pdfElement('iframe', '/descarga', '');
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [element] });
    const child = await detector(variant, sinRed, { parent, element, location: {
      href: 'https://sede.dipgra.es/descarga', pathname: '/descarga'
    } });
    assert.equal(parent.buttons.length, 0);
    assert.equal(child.buttons.length, 1);
  });
  test(`${variant}: el PDF de otro origen negocia sin acceder al DOM del padre`, async () => {
    const element = pdfElement('object', 'https://otro.savia.net/archivo.pdf');
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [element] });
    const child = await detector(variant, sinRed, { parent, element, location: {
      href: 'https://otro.savia.net/archivo.pdf', pathname: '/archivo.pdf'
    } });
    assert.equal(parent.buttons.length + child.buttons.length, 1);
  });
  test(`${variant}: sin detector en el padre, el marco conserva su botón`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF, defer: true });
    const child = await detector(variant, sinRed, { parent });
    assert.equal(child.buttons.length, 1);
  });
  test(`${variant}: ignora confirmaciones de otros marcos y páginas`, async () => {
    const child = await detector(variant, sinRed);
    await child.window.dispatch({ channel: 'grxfirma-pdf-owner-v1', action: 'owned',
      key: createHash('sha256').update('https://sede.dipgra.es/archivo.pdf').digest('hex') }, {});
    assert.equal(child.buttons.length, 1);
  });
  test(`${variant}: PDF directo con embed interno, una sola URL`, async () => {
    const h = await detector(variant, sinRed, { embedded: [pdfElement('embed', '/otro.pdf')] });
    assert.equal(h.buttons.length, 1);
    await h.button.onclick({ isTrusted: true });
    assert.equal(h.sent[0].pdfUrl, 'https://sede.dipgra.es/archivo.pdf');
  });
  test(`${variant}: inserción, cambio de URL y retirada de PDF`, async () => {
    const embedded = [];
    const h = await detector(variant, sinRed, { ...paginaConPDF, embedded });
    assert.equal(h.buttons.length, 0);
    embedded.push(pdfElement('embed'));
    h.changed();
    assert.equal(h.buttons.length, 1);
    embedded[0] = pdfElement('object', '/nuevo.pdf');
    h.changed();
    await h.button.onclick({ isTrusted: true });
    assert.equal(h.sent[0].pdfUrl, 'https://sede.dipgra.es/nuevo.pdf');
    embedded.length = 0;
    h.changed();
    assert.equal(h.buttons.length, 0);
  });
  test(`${variant}: una segunda inyección no duplica botones ni observadores`, async () => {
    const h = await detector(variant, sinRed);
    await h.start();
    assert.equal(h.buttons.length, 1);
  });

  test(`${variant}: la negociación no revela URL ni contenido del PDF`, async () => {
    const url = 'https://otro.savia.net/archivo.pdf?token=credencial-temporal';
    const element = pdfElement('object', url);
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [element] });
    const child = await detector(variant, sinRed, { parent, element, location: {
      href: url, pathname: '/archivo.pdf'
    } });
    assert.equal(parent.buttons.length + child.buttons.length, 1);
    const messages = [...parent.posted, ...child.posted];
    assert.ok(messages.some((message) => message.action === 'selected'));
    for (const message of messages) {
      assert.equal(message.url, undefined);
      assert.equal(message.pendingDocument, undefined);
      if (message.key) assert.match(message.key, /^[a-f0-9]{64}$/);
      assert.ok(!JSON.stringify(message).includes('credencial-temporal'));
    }
  });
  test(`${variant}: transmite la propiedad del botón a un visor PDF anidado`, async () => {
    const outer = pdfElement('iframe');
    const inner = pdfElement('embed');
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [outer] });
    const child = await detector(variant, sinRed, { parent, element: outer, embedded: [inner] });
    const viewer = await detector(variant, sinRed, { parent: child, element: inner });
    assert.equal(parent.buttons.length + child.buttons.length + viewer.buttons.length, 1);
  });

  test(`${variant}: dos marcos del mismo PDF eligen uno y recuperan el otro al retirarlo`, async () => {
    const first = pdfElement('iframe');
    const second = pdfElement('iframe');
    const embedded = [first, second];
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded });
    const child1 = await detector(variant, sinRed, { parent, element: first });
    const child2 = await detector(variant, sinRed, { parent, element: second });
    assert.equal(parent.buttons.length + child1.buttons.length + child2.buttons.length, 1);
    assert.ok(child1.button);
    embedded.shift();
    parent.window.frames[0] = parent.window.frames[1];
    delete parent.window.frames[1];
    parent.window.frames.length = 1;
    parent.changed();
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(parent.buttons.length, 0);
    assert.equal(child2.buttons.length, 1);
  });

  test(`${variant}: agrupa también dos iframe con PDF sin extensión`, async () => {
    const first = pdfElement('iframe', '/descarga', '');
    const second = pdfElement('iframe', '/descarga', '');
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [first, second] });
    const location = { href: 'https://sede.dipgra.es/descarga', pathname: '/descarga' };
    const child1 = await detector(variant, sinRed, { parent, element: first, location });
    const child2 = await detector(variant, sinRed, { parent, element: second, location });
    assert.equal(parent.buttons.length + child1.buttons.length + child2.buttons.length, 1);
  });

  test(`${variant}: un visor de plugin puede no aparecer en window.frames`, async () => {
    const element = pdfElement('embed');
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [element] });
    const child = await detector(variant, sinRed, { parent, element });
    parent.window.frames.length = 0;
    parent.changed();
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(parent.buttons.length + child.buttons.length, 1);
    assert.ok(child.button);
  });

}
