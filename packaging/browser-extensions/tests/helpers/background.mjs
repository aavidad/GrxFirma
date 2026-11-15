// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import { readFile } from 'node:fs/promises';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';
const extensionRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');

function storageArea() {
  const values = new Map();
  return {
    async get(keys) {
      const result = {};
      for (const key of keys === null ? values.keys() : (Array.isArray(keys) ? keys : [keys])) {
        if (values.has(key)) result[key] = values.get(key);
      }
      return result;
    },
    async set(items) {
      for (const [key, value] of Object.entries(items)) values.set(key, value);
    },
    async remove(keys) {
      for (const key of Array.isArray(keys) ? keys : [keys]) values.delete(key);
    }
  };
}

export async function loadBackground(variant, options = {}) {
  const nativeMessageListeners = [];
  const connectListeners = [];
  const contentPorts = [];
  const debugLogs = [];
  const frames = options.frames || [{ frameId: 0, parentFrameId: -1, url: 'https://sede.dipgra.es/documento.pdf' }];
  const runtimeMessageListeners = [];
  const disconnectListeners = [];
  const liveTimers = new Set();
  const localStorage = storageArea();
  const sessionStorage = storageArea();
  const managedStorage = storageArea();
  const grantedOrigins = new Set(options.grantedOrigins || []);
  const registeredScripts = new Map();
  if (options.managedSites) await managedStorage.set({ sitiosConfianza: options.managedSites });
  if (options.userSites) await localStorage.set({ sitiosConfianza: options.userSites });
  const postedMessages = [];
  const openedTabs = [];
  const runtimeId = options.runtimeId || "abcdefghijklmnopabcdefghijklmnop";
  const extensionScheme = variant === "firefox" ? "moz-extension" : "chrome-extension";
  const extensionBase = `${extensionScheme}://${runtimeId}/`;

  const port = {
    onMessage: {
      addListener(listener) {
        nativeMessageListeners.push(listener);
      }
    },
    onDisconnect: {
      addListener(listener) {
        disconnectListeners.push(listener);
      }
    },
    postMessage(message) {
      postedMessages.push(message);
      if (options.postMessageError) throw options.postMessageError;
      if (options.nativeResponse) {
        queueMicrotask(() => {
          const response = options.nativeResponse(message);
          const responses = Array.isArray(response) ? response : [response];
          for (const item of responses) {
            for (const listener of nativeMessageListeners) listener(item);
          }
        });
      }
    }
  };

  const context = vm.createContext({
    AbortController,
    URL,
    atob,
    btoa,
    crypto: globalThis.crypto,
    TextEncoder,
    console: { log() {}, warn() {}, error() {}, debug(message) { debugLogs.push(message); } },
    queueMicrotask,
    fetch: options.fetch || (async () => {
      throw new Error("unexpected fetch");
    }),
    setTimeout(callback, delay) {
      const handle = setTimeout(callback, delay);
      liveTimers.add(handle);
      return handle;
    },
    clearTimeout(handle) {
      liveTimers.delete(handle);
      clearTimeout(handle);
    },
    CONFIG: {
      API_URL: "https://api.example.invalid",
      LOCAL_REST_URL: "https://127.0.0.1:63118",
      LOCAL_REST_BEARER: options.restBearer || ""
    },
    CryptoUtils: {
      async decrypt() { return null; },
      async getIdentity() { return null; },
      async generateIdentity() { return { pub: "test" }; }
    },
    grxfirmaExt: {
      i18n: {
        message(key) {
          const messages = {
            popupNativeHostUnavailable: "No se pudo establecer el canal seguro. Reinstale el conector oficial y reinicie el navegador.",
            nativeRequestTimeout: "La operación agotó el tiempo. Repítala; si persiste, reinicie el navegador.",
            runtimeRequestRejected: "Solicitud bloqueada por seguridad. Use el conector oficial desde una sede autorizada."
          };
          return messages[key] || key;
        }
      },
      runtime: {
        id: runtimeId,
        getURL(relative = "") {
          return extensionBase + String(relative).replace(/^\/+/, "");
        },
        getManifest() {
          return {
            host_permissions: [
              "https://*.dipgra.es/*",
              "https://*.savia.net/*",
              "https://127.0.0.1/*"
            ]
          };
        },
        connectNative() {
          if (options.connectError) throw options.connectError;
          return port;
        },
        lastErrorMessage() {
          return options.disconnectError || "";
        },
        async sendMessage() {},
        onConnect: { addListener(listener) { connectListeners.push(listener); } },
        onInstalled: { addListener() {} },
        onStartup: { addListener() {} },
        onMessage: {
          addListener(listener) {
            runtimeMessageListeners.push(listener);
          }
        }
      },
      tabs: {
        async create(properties) {
          openedTabs.push(properties);
        }
      },
      permissions: {
        async contains({ origins }) { return origins.every((origin) => grantedOrigins.has(origin)); },
        onAdded: { addListener() {} }, onRemoved: { addListener() {} }
      },
      scripting: {
        async getRegisteredContentScripts() { return [...registeredScripts.values()]; },
        async registerContentScripts(items) { for (const item of items) registeredScripts.set(item.id, item); },
        async unregisterContentScripts({ ids }) { for (const id of ids) registeredScripts.delete(id); }
      },
      storageOnChanged: { addListener() {} },
      storageManaged: managedStorage,
      storageLocal: localStorage,
      storageSession: sessionStorage,
      hasStorageSession() { return true; }
    }
  });

  const trustedSource = await readFile(path.join(extensionRoot, "src", variant, "trusted_sites.js"), "utf8");
  vm.runInContext(trustedSource, context, { filename: `${variant}/trusted_sites.js` });
  const source = await readFile(path.join(extensionRoot, "src", variant, "background.js"), "utf8");
  vm.runInContext(source, context, { filename: `${variant}/background.js` });
  await vm.runInContext('trustedSitesRefresh', context);

  return {
    context, frames, debugLogs,
    // Simula la destrucción de documentos por el navegador. No es una API
    // disponible para el fondo: solo provoca la desconexión de sus puertos.
    navigation() {
      for (const { sender, disconnect } of contentPorts) {
        const frame = frames.find((item) => item.frameId === sender.frameId);
        if (!frame || frame.url !== sender.url || frame.documentId !== sender.documentId ||
            frames.find((item) => item.frameId === 0)?.url !== sender.tab?.url) disconnect();
      }
    },
    connectContent(sender) {
      sender = { ...sender, tab: sender.tab && { ...sender.tab } };
      const clientMessages = [], serverMessages = [], clientDisconnect = [], serverDisconnect = [];
      let active = true;
      const disconnect = () => {
        if (!active) return;
        active = false;
        queueMicrotask(() => { for (const listener of [...serverDisconnect, ...clientDisconnect]) listener(); });
      };
      const endpoint = (messages, disconnections, receivers) => ({
        name: 'grxfirma-pdf-owners', sender,
        onMessage: { addListener(listener) { messages.push(listener); } },
        onDisconnect: { addListener(listener) { disconnections.push(listener); } },
        postMessage(message) {
          if (!active) throw new Error('port disconnected');
          queueMicrotask(() => { if (active) for (const listener of receivers) listener(message); });
        },
        disconnect
      });
      contentPorts.push({ sender, disconnect });
      const client = endpoint(clientMessages, clientDisconnect, serverMessages);
      const server = endpoint(serverMessages, serverDisconnect, clientMessages);
      for (const listener of connectListeners) listener(server);
      return client;
    },
    postedMessages,
    openedTabs,
    localStorage, sessionStorage, managedStorage, grantedOrigins, registeredScripts,
    async settle() {
      for (let i = 0; i < 4; i++) {
        await new Promise((resolve) => setImmediate(resolve));
        await vm.runInContext('Promise.all([...pdfTabs.values()].map((state) => state.pending))', context);
      }
    },
    internalSender(relative = "popup.html") {
      return {
        id: runtimeId,
        url: extensionBase + relative.replace(/^\/+/, "")
      };
    },
    dispatchRuntimeMessage(request, sender) {
      const effectiveSender = sender || {
        id: runtimeId,
        url: extensionBase + "popup.html"
      };
      return new Promise((resolve, reject) => {
        if (runtimeMessageListeners.length === 0) {
          resolve(undefined);
          return;
        }
        let settled = false;
        const sendResponse = (response) => {
          if (!settled) {
            settled = true;
            resolve(response);
          }
        };
        try {
          const keepOpen = runtimeMessageListeners.some(
            (listener) => listener(request, effectiveSender, sendResponse) === true
          );
          if (!keepOpen && !settled) {
            settled = true;
            resolve(undefined);
          }
        } catch (error) {
          reject(error);
        }
      });
    },
    disconnect() {
      for (const listener of disconnectListeners) listener();
    },
    liveTimers
  };
}

