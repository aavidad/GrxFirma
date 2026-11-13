// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Carga el background.js real de Chromium (el que recibe Safari) y después el
// parche, como hace postprocess.py. Así la prueba falla si el parche nombra
// funciones que el fondo no declara (hallazgo C-01).

import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const shim = await readFile(new URL("../SafariNativeOnly.js.in", import.meta.url), "utf8");
const srcRoot = new URL("../../src/chromium/", import.meta.url);
const background = await readFile(new URL("background.js", srcRoot), "utf8");
const trustedSites = await readFile(new URL("trusted_sites.js", srcRoot), "utf8");

function storageArea() {
  return { async get() { return {}; }, async set() {}, async remove() {} };
}

function loadPatchedBackground({ nativeFails = false } = {}) {
  const posted = [];
  const fetches = [];
  const nativeListeners = [];
  const disconnectListeners = [];
  const timers = new Set();
  const port = {
    onMessage: { addListener(listener) { nativeListeners.push(listener); } },
    onDisconnect: { addListener(listener) { disconnectListeners.push(listener); } },
    postMessage(message) {
      posted.push(message);
      queueMicrotask(() => {
        for (const listener of nativeListeners) {
          listener({ requestId: message.requestId, success: true, chunk: 0, totalChunks: 1 });
        }
      });
    },
  };
  const context = vm.createContext({
    URL, atob, btoa, TextEncoder, AbortController, queueMicrotask,
    crypto: globalThis.crypto,
    console: { log() {}, warn() {}, error() {} },
    setTimeout(callback, delay) { const handle = setTimeout(callback, delay); timers.add(handle); return handle; },
    clearTimeout(handle) { timers.delete(handle); clearTimeout(handle); },
    async fetch(url) { fetches.push(String(url)); throw new Error("unexpected fetch"); },
    CONFIG: { LOCAL_REST_URL: "https://127.0.0.1:63118", LOCAL_REST_BEARER: "bearer-que-no-debe-usarse" },
    grxfirmaExt: {
      i18n: { message(key) { return key; } },
      runtime: {
        id: "abcdefghijklmnopabcdefghijklmnop",
        getURL(relative = "") { return "safari-web-extension://abcdefghijklmnopabcdefghijklmnop/" + relative; },
        connectNative() { if (nativeFails) throw new Error("native unavailable"); return port; },
        lastErrorMessage() { return ""; },
        async sendMessage() {},
        onMessage: { addListener() {} },
      },
      tabs: { async create() {} },
      permissions: { async contains() { return false; } },
      scripting: {
        async getRegisteredContentScripts() { return []; },
        async registerContentScripts() {},
        async unregisterContentScripts() {},
      },
      storageManaged: storageArea(),
      storageLocal: storageArea(),
      storageSession: storageArea(),
      hasStorageSession() { return true; },
    },
  });
  vm.runInContext(trustedSites, context, { filename: "trusted_sites.js" });
  vm.runInContext(background, context, { filename: "background.js" });
  // En un service worker, asignar en modo estricto un nombre que el fondo no
  // declara lanza ReferenceError. Dentro de vm esa asignacion no lanza: crea un
  // global nuevo. Por eso se exige que el parche no cree ningun global.
  const before = new Set(vm.runInContext("Object.getOwnPropertyNames(globalThis)", context));
  vm.runInContext(shim, context, { filename: "SafariNativeOnly.js.in" });
  const created = vm.runInContext("Object.getOwnPropertyNames(globalThis)", context)
    .filter((name) => !before.has(name)).map(String);
  // Copia al reino de la prueba para compararla con deepEqual.
  const createdHere = [...created];
  return { context, posted, fetches, timers, created: createdHere };
}

function clearTimers(timers) {
  for (const handle of timers) clearTimeout(handle);
}

test("el parche se aplica sobre el background.js real sin lanzar", () => {
  const { context, timers, created } = loadPatchedBackground();
  assert.deepEqual(created, [], "el parche asigna nombres que background.js no declara");
  assert.equal(vm.runInContext("pingLocalIntegration.name", context), "pingThroughSafariHandler");
  assert.equal(vm.runInContext("restJSON.name", context), "disableRestFallback");
  clearTimers(timers);
});

test("Safari no usa REST desde JavaScript aunque haya Bearer", async () => {
  const { context, fetches, timers } = loadPatchedBackground();
  await assert.rejects(vm.runInContext("restJSON('/certificates', { method: 'GET' })", context), /native bridge only/);
  assert.deepEqual(fetches, []);
  clearTimers(timers);
});

test("el ping de Safari solo usa el manejador nativo", async () => {
  const { context, posted, timers } = loadPatchedBackground();
  const response = await vm.runInContext("pingLocalIntegration()", context);
  assert.deepEqual(posted.map((message) => message.action), ["ping"]);
  assert.equal(response.success, true);
  assert.equal(response.mode, "safari-native");
  clearTimers(timers);
});

test("si falla el nativo, el ping de Safari no recurre a REST", async () => {
  const { context, fetches, timers } = loadPatchedBackground({ nativeFails: true });
  await assert.rejects(vm.runInContext("pingLocalIntegration()", context));
  assert.deepEqual(fetches, []);
  clearTimers(timers);
});
