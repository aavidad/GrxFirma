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
import { loadBackground } from './helpers/background.mjs';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'src');
async function detector(variant, fetch, frame = {}) {
  const elements = new Map();
  const controls = [];
  const sent = [];
  const posted = [];
  const events = new Map();
  const intervals = [];
  const reconnects = [];
  const ports = [];
  let changed;
  const background = frame.background || frame.parent?.background || await loadBackground(variant, frame.options);
  background.detectors ||= [];
  background.nextFrameId ||= 0;
  const frameId = frame.parent ? ++background.nextFrameId : 0;
  const location = frame.location || { href: 'https://sede.dipgra.es/archivo.pdf', pathname: '/archivo.pdf' };
  const navigationFrame = { frameId, parentFrameId: frame.parent?.frameId ?? -1, url: location.href,
    documentId: `doc-${frameId}` };
  if (!frame.parent) background.frames.length = 0;
  background.frames.push(navigationFrame);
  const sender = { id: 'abcdefghijklmnopabcdefghijklmnop', url: location.href, frameId,
    documentId: navigationFrame.documentId, tab: { id: 12, url: background.frames[0].url } };
  const document = {
    contentType: frame.contentType || 'application/pdf', title: 'documento', documentElement: {},
    querySelectorAll() { return frame.embedded || []; },
    getElementById(id) { return elements.get(id) || null; },
    createElement(tag) {
      return {
        tagName: tag, style: {}, children: [], id: '', title: '', textContent: '', attributes: {},
        setAttribute(key, value) { this.attributes[key] = value; },
        append(...nodes) { this.children.push(...nodes); },
        remove() { elements.delete(this.id); const i = controls.indexOf(this); if (i >= 0) controls.splice(i, 1); }
      };
    },
    body: { appendChild(element) { if (element.id) elements.set(element.id, element); controls.push(element); } }
  };
  if (frame.noBody) document.body = null;
  const window = {
    location,
    addEventListener(type, callback) {
      if (!events.has(type)) events.set(type, []);
      events.get(type).push(callback);
    },
    async dispatch(data, source) { for (const callback of events.get('message') || []) await callback({ data, source }); }
  };
  const context = vm.createContext({
    document, window, URL, Blob, AbortController, clearTimeout, TextEncoder,
    setTimeout(callback, delay) {
      if (delay === 500) { reconnects.push(callback); return 0; }
      return setTimeout(callback, delay);
    },
    setInterval(callback) { intervals.push(callback); },
    MutationObserver: class { constructor(callback) { changed = callback; } observe() {} },
    console: { log() {}, warn() {}, error() {} }, fetch,
    grxfirmaExt: {
      i18n: { message(key) { return key; } },
      runtime: {
        connect() { const port = background.connectContent(sender); ports.push(port); return port; },
        async sendMessage(message) { sent.push(message); return background.dispatchRuntimeMessage(message, sender); }
      }
    }
  });
  const source = await readFile(path.join(root, variant, 'content_scripts/pdf_detector.js'), 'utf8');
  const start = async () => { vm.runInContext(source, context); await background.settle(); };
  const h = {
    get button() { return controls.find((node) => node.tagName === 'button'); },
    get notice() { return controls.find((node) => node.attributes.role === 'status'); },
    get buttons() { return controls.filter((node) => node.tagName === 'button'); },
    sent, posted, window, start, background, frameId, parent: frame.parent,
    async changed() { changed(); await background.settle(); },
    async heartbeat() { intervals.forEach((callback) => callback()); await background.settle(); },
    async disconnect() { ports.at(-1).disconnect(); await background.settle(); },
    async reconnect() { reconnects.splice(0).forEach((callback) => callback()); await background.settle(); },
    async remove() {
      for (const child of background.detectors.filter((item) => item.parent === h)) await child.remove();
      for (const callback of events.get('pagehide') || []) callback();
      const i = background.frames.findIndex((item) => item.frameId === frameId);
      if (i >= 0) background.frames.splice(i, 1);
      await background.settle();
    }
  };
  background.detectors.push(h);
  if (!frame.defer) await start();
  return h;
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

// Las regresiones usan el fondo y los detectores reales, con las API del
// navegador simuladas; no acceden a la red ni arrancan un navegador.
const paginaConPDF = {
  contentType: 'text/html',
  location: { href: 'https://sede.dipgra.es/expediente', pathname: '/expediente' }
};
const sinRed = async () => { throw new Error('sin red'); };
const count = (...frames) => frames.reduce((sum, frame) => sum + frame.buttons.length, 0);
for (const variant of ['chromium', 'firefox']) {
  for (const tagName of ['embed', 'object', 'iframe']) {
    for (const parentFirst of [true, false]) {
      test(`${variant}: <${tagName}>, padre primero=${parentFirst}: gana el propio PDF`, async () => {
        const parent = await detector(variant, sinRed, {
          ...paginaConPDF, embedded: [pdfElement(tagName)], defer: !parentFirst
        });
        const child = await detector(variant, sinRed, { parent });
        if (!parentFirst) await parent.start();
        assert.equal(count(parent, child), 1);
        assert.ok(child.button);
        await child.button.onclick({ isTrusted: true });
        assert.equal(child.sent[0].pdfUrl, 'https://sede.dipgra.es/archivo.pdf');
      });
    }
  }
  for (const otherOrigin of [false, true]) {
    for (const parentFirst of [true, false]) {
      test(`${variant}: envoltorios HTML, otro origen=${otherOrigin}, principal primero=${parentFirst}`, async () => {
        const parent = await detector(variant, sinRed, {
          ...paginaConPDF, embedded: [pdfElement('embed')], defer: !parentFirst
        });
        const wrappers = [];
        const pdfs = [];
        for (let i = 0; i < 2; i++) {
          const wrapper = await detector(variant, sinRed, { ...paginaConPDF, parent,
            location: { href: `https://${otherOrigin ? 'otro.savia.net' : 'sede.dipgra.es'}/visor${i}`, pathname: `/visor${i}` },
            embedded: [pdfElement('embed', 'https://sede.dipgra.es/archivo.pdf')]
          });
          wrappers.push(wrapper);
          pdfs.push(await detector(variant, sinRed, { parent: wrapper }));
        }
        if (!parentFirst) await parent.start();
        assert.equal(count(parent, ...wrappers, ...pdfs), 1);
        assert.ok(pdfs[0].button);
        // Una mutación o una nueva elección no cambia el dueño estable.
        await wrappers[1].heartbeat();
        assert.equal(count(parent, ...wrappers, ...pdfs), 1);
        assert.ok(pdfs[0].button);
      });
    }
  }
  test(`${variant}: sin detector en el padre, conserva el botón en el PDF`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF, defer: true });
    const child = await detector(variant, sinRed, { parent });
    assert.equal(child.buttons.length, 1);
  });
  test(`${variant}: respaldo si el visor interno no admite scripts`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [pdfElement('iframe')] });
    assert.equal(parent.buttons.length, 1);
  });
  test(`${variant}: varios PDF y referencias con fragmentos`, async () => {
    const h = await detector(variant, sinRed, { ...paginaConPDF, embedded: [
      pdfElement('embed', '/uno.pdf'), pdfElement('object', '/dos.pdf'), pdfElement('iframe', '/uno.pdf#page=2')
    ] });
    assert.equal(h.buttons.length, 2);
    assert.notEqual(h.buttons[0].style.bottom, h.buttons[1].style.bottom);
    for (const button of h.buttons) await button.onclick({ isTrusted: true });
    assert.deepEqual(h.sent.map((message) => message.pdfUrl),
      ['https://sede.dipgra.es/dos.pdf', 'https://sede.dipgra.es/uno.pdf']);
  });
  test(`${variant}: PDF sin extensión en dos marcos`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF });
    const location = { href: 'https://sede.dipgra.es/descarga', pathname: '/descarga' };
    const child1 = await detector(variant, sinRed, { parent, location });
    const child2 = await detector(variant, sinRed, { parent, location });
    assert.equal(count(parent, child1, child2), 1);
    assert.ok(child1.button);
    await child1.remove();
    assert.equal(child2.buttons.length, 1);
  });
  test(`${variant}: retirada del dueño HTML y de su subárbol, traspaso al hermano`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF });
    const first = await detector(variant, sinRed, { ...paginaConPDF, parent, embedded: [pdfElement('embed')] });
    const nested = await detector(variant, sinRed, { parent: first });
    const second = await detector(variant, sinRed, { ...paginaConPDF, parent, embedded: [pdfElement('object')] });
    assert.equal(count(parent, first, nested, second), 1);
    assert.ok(nested.button);
    await first.remove();
    assert.equal(count(parent, first, nested, second), 1);
    assert.ok(second.button);
    await second.remove();
    assert.equal(count(parent, first, nested, second), 0);
  });
  test(`${variant}: retirar el marco PDF devuelve el respaldo al HTML`, async () => {
    const embedded = [pdfElement('embed')];
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded });
    const child = await detector(variant, sinRed, { parent });
    assert.ok(child.button);
    await child.remove();
    assert.equal(count(parent, child), 1);
    assert.ok(parent.button);
  });
  test(`${variant}: principal sin confianza, PDF de confianza oculto y apertura rechazada`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF,
      location: { href: 'https://prueba.ejemplo.org/', pathname: '/' }, defer: true });
    const child = await detector(variant, sinRed, { parent });
    assert.equal(count(parent, child), 0);
    assert.ok(child.background.debugLogs.includes('pdfTopPageNotTrusted'));
    const response = await child.background.dispatchRuntimeMessage({ action: 'openSigner',
      pdfUrl: child.window.location.href, topURL: 'https://sede.dipgra.es/' }, {
      id: 'abcdefghijklmnopabcdefghijklmnop', frameId: child.frameId, url: child.window.location.href,
      tab: { id: 12, url: parent.window.location.href }
    });
    assert.equal(response.success, false);
    assert.equal(response.code, 'pdf_top_page_not_trusted');
    assert.equal(child.background.openedTabs.length, 0);
  });
  for (const position of ['top', 'frame', 'pdf']) {
    test(`${variant}: puerto 9443 en ${position}, sin botón ni apertura`, async () => {
      const location = { href: 'https://sede.dipgra.es:9443/archivo.pdf', pathname: '/archivo.pdf' };
      const parent = await detector(variant, sinRed, { ...paginaConPDF,
        ...(position === 'top' ? { location } : {}),
        embedded: position === 'frame' ? [] : [pdfElement('embed', position === 'pdf' ? location.href : '/archivo.pdf')]
      });
      const child = position === 'frame' ? await detector(variant, sinRed, { parent, location }) : null;
      assert.equal(count(parent, ...(child ? [child] : [])), 0);
      const response = await parent.background.dispatchRuntimeMessage({ action: 'openSigner',
        pdfUrl: location.href }, {
        id: 'abcdefghijklmnopabcdefghijklmnop', url: child ? location.href : parent.window.location.href,
        tab: { id: 12, url: parent.window.location.href }
      });
      assert.equal(response.success, false);
      assert.equal(parent.background.openedTabs.length, 0);
    });
  }
  test(`${variant}: fallo de webNavigation cierra el acceso`, async () => {
    const h = await detector(variant, sinRed, { options: { framesError: true } });
    assert.equal(h.buttons.length, 0);
  });
  test(`${variant}: cambio de confianza de la principal retira el botón`, async () => {
    const h = await detector(variant, sinRed);
    assert.ok(h.button);
    h.background.frames[0].url = 'https://sinconfianza.example/';
    h.background.navigation();
    await h.background.settle();
    assert.equal(h.buttons.length, 0);
  });
  test(`${variant}: desconexión del puerto y recuperación del detector`, async () => {
    const h = await detector(variant, sinRed);
    await h.disconnect();
    assert.equal(h.buttons.length, 0);
    await h.reconnect();
    assert.equal(h.buttons.length, 1);
  });
  test(`${variant}: inserción, cambio de URL y retirada de PDF`, async () => {
    const embedded = [];
    const h = await detector(variant, sinRed, { ...paginaConPDF, embedded });
    assert.equal(h.buttons.length, 0);
    embedded.push(pdfElement('embed'));
    await h.changed();
    assert.equal(h.buttons.length, 1);
    embedded[0] = pdfElement('object', '/nuevo.pdf');
    await h.changed();
    await h.button.onclick({ isTrusted: true });
    assert.equal(h.sent[0].pdfUrl, 'https://sede.dipgra.es/nuevo.pdf');
    embedded.length = 0;
    await h.changed();
    assert.equal(h.buttons.length, 0);
  });
  test(`${variant}: una segunda inyección no duplica controles`, async () => {
    const h = await detector(variant, sinRed);
    await h.start();
    assert.equal(h.buttons.length, 1);
  });
  test(`${variant}: visor PDF anidado y embed interno, una URL`, async () => {
    const h = await detector(variant, sinRed, { embedded: [pdfElement('embed', '/otro.pdf')] });
    const child = await detector(variant, sinRed, { parent: h });
    assert.equal(count(h, child), 1);
    await h.button.onclick({ isTrusted: true });
    assert.equal(h.sent[0].pdfUrl, 'https://sede.dipgra.es/archivo.pdf');
  });
  test(`${variant}: no expone la negociación a páginas ni acepta su postMessage`, async () => {
    const h = await detector(variant, sinRed, { location: {
      href: 'https://sede.dipgra.es/archivo.pdf?token=temporal', pathname: '/archivo.pdf'
    } });
    await h.window.dispatch({ channel: 'grxfirma-pdf-owner-v1', action: 'owned',
      key: createHash('sha256').update(h.window.location.href).digest('hex') }, {});
    assert.equal(h.buttons.length, 1);
    assert.deepEqual(h.posted, []);
  });
}

for (const variant of ['chromium', 'firefox']) {
  for (const kind of ['userSites', 'managedSites']) {
    test(`${variant}: PDF de fábrica con principal ${kind} y retirada del permiso`, async () => {
      const parent = await detector(variant, sinRed, { ...paginaConPDF, defer: true,
        location: { href: 'https://portal.organismo.es/expediente', pathname: '/expediente' },
        options: { [kind]: ['portal.organismo.es'], grantedOrigins: ['https://portal.organismo.es/*'] }
      });
      const child = await detector(variant, sinRed, { parent });
      assert.ok(child.button);
      child.background.grantedOrigins.clear();
      await vm.runInContext('refreshTrustedSites()', child.background.context);
      await child.background.settle();
      assert.equal(child.buttons.length, 0);
    });
  }
  test(`${variant}: frameId reutilizado no hereda el dueño de otro documento`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF });
    const first = await detector(variant, sinRed, { parent });
    const second = await detector(variant, sinRed, { parent });
    assert.ok(first.button);
    first.background.frames.find((frame) => frame.frameId === first.frameId).documentId = 'nuevo-documento';
    first.background.navigation();
    await first.background.settle();
    assert.equal(count(parent, first, second), 1);
    assert.ok(second.button);
  });
  test(`${variant}: comprueba principal real aunque el puerto conserve una URL anterior`, async () => {
    const h = await detector(variant, sinRed);
    h.background.frames[0].url = 'https://ajeno.example/';
    await h.heartbeat();
    assert.equal(h.buttons.length, 0);
    assert.ok(h.background.debugLogs.includes('pdfTopPageNotTrusted'));
  });
}

for (const variant of ['chromium', 'firefox']) {
  test(`${variant}: entre HTML gana el menos profundo aunque se registre después`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF,
      embedded: [pdfElement('embed')], defer: true });
    const wrapper = await detector(variant, sinRed, { ...paginaConPDF, parent,
      embedded: [pdfElement('object')] });
    assert.ok(wrapper.button);
    await parent.start();
    assert.equal(count(parent, wrapper), 1);
    assert.ok(parent.button);
    await parent.heartbeat();
    assert.ok(parent.button);
  });
  test(`${variant}: entre PDF directos gana el menos profundo aunque llegue después`, async () => {
    const parent = await detector(variant, sinRed, { defer: true });
    const child = await detector(variant, sinRed, { parent });
    assert.ok(child.button);
    await parent.start();
    assert.equal(count(parent, child), 1);
    assert.ok(parent.button);
  });
}

for (const variant of ['chromium', 'firefox']) {
  test(`${variant}: si el PDF no puede crear controles, cede el respaldo al HTML`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [pdfElement('embed')] });
    const child = await detector(variant, sinRed, { parent, noBody: true });
    assert.equal(count(parent, child), 1);
    assert.ok(parent.button);
  });
}

for (const variant of ['chromium', 'firefox']) {
  test(`${variant}: desconectar el puerto del dueño cede al respaldo sin pagehide`, async () => {
    const parent = await detector(variant, sinRed, { ...paginaConPDF, embedded: [pdfElement('embed')] });
    const child = await detector(variant, sinRed, { parent });
    assert.ok(child.button);
    await child.disconnect();
    assert.equal(count(parent, child), 1);
    assert.ok(parent.button);
  });
}
