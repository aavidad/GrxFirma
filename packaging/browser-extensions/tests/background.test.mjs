// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";
import vm from "node:vm";
import { fileURLToPath } from "node:url";

const testDir = path.dirname(fileURLToPath(import.meta.url));
const extensionRoot = path.resolve(testDir, "..");

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

async function loadBackground(variant, options = {}) {
  const nativeMessageListeners = [];
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
    console: { log() {}, warn() {}, error() {} },
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
    context,
    postedMessages,
    openedTabs,
    localStorage, sessionStorage, managedStorage, grantedOrigins, registeredScripts,
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

for (const variant of ["chromium", "firefox"]) {
  test(`${variant}: no persiste documentos del firmador en storage.local`, async () => {
    const signer = await readFile(
      path.join(extensionRoot, "src", variant, "signer", "signer.js"),
      "utf8"
    );

    assert.doesNotMatch(signer, /pendingFile/);
    assert.doesNotMatch(signer, /readAsDataURL\(selectedFile\)/);
    assert.match(signer, /MAX_EXTENSION_TRANSFER_BYTES = 46 \* 1024 \* 1024/);
    assert.match(signer, /ensureExtensionTransferSize\(selectedFile\.size\)/);
  });

  test(`${variant}: genera tokens de lanzamiento criptograficos`, async () => {
    const harness = await loadBackground(variant);
    const tokens = vm.runInContext(
      "Array.from({ length: 128 }, () => createLaunchToken())",
      harness.context
    );

    assert.equal(new Set(tokens).size, tokens.length);
    for (const token of tokens) assert.match(token, /^[0-9a-f]{64}$/);
  });

  test(`${variant}: el documento pendiente es de un solo uso y caduca`, async () => {
    const harness = await loadBackground(variant);

    const first = await vm.runInContext(
      `(async () => {
        await savePendingSignerLaunch("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", { name: "documento.pdf", contentBase64: "YQ==" });
        return takePendingSignerLaunch("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
      })()`,
      harness.context
    );
    assert.equal(first.name, "documento.pdf");
    assert.equal(
      await vm.runInContext('takePendingSignerLaunch("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")', harness.context),
      null
    );

    const expired = await vm.runInContext(
      `(async () => {
        await grxfirmaExt.storageSession.set({
          "pendingSignerLaunch:expired-token": {
            document: { name: "secreto.pdf" },
            expiresAt: Date.now() - 1
          }
        });
        return takePendingSignerLaunch("expired-token");
      })()`,
      harness.context
    );
    assert.equal(expired, null);
  });

  test(`${variant}: limita la cuota y limpia precargas caducadas`, async () => {
    const harness = await loadBackground(variant);
    await harness.sessionStorage.set({
      'pendingSignerLaunch:expired': { document: { contentBase64: 'YQ==' }, expiresAt: Date.now() - 1 }
    });
    const result = await harness.dispatchRuntimeMessage({
      action: 'openSigner', pendingDocument: { name: 'grande.pdf', contentBase64: 'A'.repeat(7 * 1024 * 1024 + 4) }
    }, harness.internalSender());
    assert.equal(result.success, true);
    assert.equal(result.preloadSkipped, true);
    assert.equal(harness.openedTabs[0].url, 'https://127.0.0.1:63118/signer');
    assert.deepEqual(await harness.sessionStorage.get('pendingSignerLaunch:expired'), {});
    await assert.rejects(vm.runInContext('savePendingSignerLaunch("' + 'c'.repeat(64) + '", { contentBase64: "AA=A" })', harness.context), /invalid_pending_document/);
    harness.context.safePayload = { name: 'límite.pdf', contentBase64: 'A'.repeat(Math.ceil((2.5 * 1024 * 1024) / 3) * 4) };
    await vm.runInContext('savePendingSignerLaunch("' + 'd'.repeat(64) + '", safePayload)', harness.context);
    assert.ok((await harness.sessionStorage.get(null))['pendingSignerLaunch:' + 'd'.repeat(64)]);
  });

  test(`${variant}: serializa las reservas de cuota concurrentes`, async () => {
    const harness = await loadBackground(variant);
    harness.context.largePayload = { name: 'pdf', contentBase64: 'A'.repeat(2 * 1024 * 1024 + 4) };
    // Dos escrituras en el mismo worker no pueden reservar el mismo espacio.
    const first = vm.runInContext('savePendingSignerLaunch("' + 'a'.repeat(64) + '", largePayload)', harness.context);
    const second = vm.runInContext('savePendingSignerLaunch("' + 'b'.repeat(64) + '", largePayload)', harness.context);
    await first;
    await assert.rejects(second, /pending_storage_full/);
  });

  test(`${variant}: separa PDF de identidad en sitios de usuario y política`, async () => {
    const harness = await loadBackground(variant, {
      userSites: ['*.usuario.org'], managedSites: ['sede.organismo.es', 'sinpermiso.org'],
      grantedOrigins: ['https://*.usuario.org/*', 'https://sede.organismo.es/*']
    });
    assert.equal(harness.registeredScripts.size, 3);
    assert.notEqual(vm.runInContext("scriptId('foo-bar.com', 'pdf')", harness.context), vm.runInContext("scriptId('foo.bar-com', 'pdf')", harness.context));
    const userScripts = [...harness.registeredScripts.values()].filter((script) => script.matches[0].includes('usuario.org'));
    assert.equal(userScripts.length, 1);
    assert.deepEqual([...userScripts[0].js], ['compat.js', 'content_scripts/pdf_detector.js']);
    const user = { id: 'abcdefghijklmnopabcdefghijklmnop', url: 'https://x.usuario.org/archivo.pdf', tab: { id: 12 } };
    assert.equal((await harness.dispatchRuntimeMessage({ action: 'openSigner' }, user)).success, true);
    assert.equal((await harness.dispatchRuntimeMessage({ action: 'proveIdentity', canonicalPayloadB64: 'e30=' }, user)).success, false);
    assert.equal((await harness.dispatchRuntimeMessage({ action: 'openSigner' }, user)).code, 'opening_pending');
    const managed = { ...user, url: 'https://sede.organismo.es/' };
    harness.context.managedSender = managed;
    assert.equal(vm.runInContext("authorizeRuntimeMessage({ action: 'proveIdentity' }, managedSender).allowed", harness.context), true);
    harness.context.managedSender = { ...user, url: 'https://sinpermiso.org/' };
    assert.equal(vm.runInContext("authorizeRuntimeMessage({ action: 'proveIdentity' }, managedSender).allowed", harness.context), false);
    harness.grantedOrigins.delete('https://sede.organismo.es/*');
    await vm.runInContext('refreshTrustedSites()', harness.context);
    assert.equal(harness.registeredScripts.size, 1);
    harness.context.managedSender = managed;
    assert.equal(vm.runInContext("authorizeRuntimeMessage({ action: 'proveIdentity' }, managedSender).allowed", harness.context), false);
  });

  test(`${variant}: rechaza solicitudes nativas por bytes UTF-8 antes de enviarlas`, async () => {
    const harness = await loadBackground(variant);

    assert.throws(
      () => vm.runInContext("ensureNativeRequestSize({ data: 'áááá' }, 8)", harness.context),
      /Native request exceeds 8 bytes/
    );
    assert.equal(vm.runInContext("pendingRequests.size", harness.context), 0);
  });

  test(`${variant}: ping nativo resuelve y cancela su timeout`, async () => {
    const harness = await loadBackground(variant, {
      nativeResponse(message) {
        return { requestId: message.requestId, success: true, pong: true };
      }
    });

    const result = await vm.runInContext("pingLocalIntegration()", harness.context);
    assert.equal(result.success, true);
    assert.equal(result.mode, "native");
    assert.equal(harness.liveTimers.size, 0);
    assert.equal(vm.runInContext("pendingRequests.size", harness.context), 0);
  });

  test(`${variant}: desconexion rechaza peticiones y cancela timeouts`, async () => {
    const harness = await loadBackground(variant);
    const pending = vm.runInContext("sendToNativeHost('sign', { data: 'AA==' })", harness.context);
    harness.disconnect();

    await assert.rejects(pending, /Reinstale el conector oficial/);
    assert.equal(harness.liveTimers.size, 0);
    assert.equal(vm.runInContext("pendingRequests.size", harness.context), 0);
  });

  test(`${variant}: el borde runtime deniega otra extension, origen y accion`, async () => {
    const harness = await loadBackground(variant);

    const anotherExtension = await harness.dispatchRuntimeMessage(
      { action: "ping" },
      {
        id: "ponmlkjihgfedcbaponmlkjihgfedcba",
        url: "https://portal.dipgra.es/documento.pdf"
      }
    );
    assert.equal(anotherExtension.success, false);
    assert.equal(anotherExtension.code, "sender_not_authorized");
    assert.match(anotherExtension.error, /bloqueada por seguridad/);

    const foreignOrigin = await harness.dispatchRuntimeMessage(
      { action: "openSigner" },
      {
        id: "abcdefghijklmnopabcdefghijklmnop",
        url: "https://dipgra.es.example.invalid/documento.pdf"
      }
    );
    assert.equal(foreignOrigin.success, false);

    const portalSign = await harness.dispatchRuntimeMessage(
      { action: "signDocument", data: "AA==" },
      {
        id: "abcdefghijklmnopabcdefghijklmnop",
        url: "https://sede.dipgra.es/documento.pdf"
      }
    );
    assert.equal(portalSign.success, false);
    assert.equal(portalSign.code, "sender_action_not_authorized");
    assert.equal(harness.postedMessages.length, 0);
  });

  test(`${variant}: portal solo abre firmador y puente local solo consume su token`, async () => {
    const harness = await loadBackground(variant);
    const portalSender = {
      id: "abcdefghijklmnopabcdefghijklmnop",
      url: "https://sede.dipgra.es/documento.pdf"
    };
    const opened = await harness.dispatchRuntimeMessage(
      { action: "openSigner" },
      portalSender
    );
    assert.equal(opened.success, true);
    assert.equal(harness.openedTabs.length, 1);
    assert.equal(harness.openedTabs[0].url, "https://127.0.0.1:63118/signer");

    await vm.runInContext(
      'savePendingSignerLaunch("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", { name: "documento.pdf", contentBase64: "YQ==" })',
      harness.context
    );
    const localSender = {
      id: "abcdefghijklmnopabcdefghijklmnop",
      url: "https://127.0.0.1:63118/signer?launchToken=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    };
    const taken = await harness.dispatchRuntimeMessage(
      { action: "takePendingSignerLaunch", token: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" },
      localSender
    );
    assert.equal(taken.success, true);
    assert.equal(taken.document.name, "documento.pdf");

    const wrongPath = await harness.dispatchRuntimeMessage(
      { action: "takePendingSignerLaunch", token: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" },
      {
        id: "abcdefghijklmnopabcdefghijklmnop",
        url: "https://127.0.0.1:63118/otra-ruta"
      }
    );
    assert.equal(wrongPath.success, false);
    assert.equal(wrongPath.code, "sender_action_not_authorized");
  });

  test(`${variant}: portal obtiene prueba de identidad sin enumerar certificados`, async () => {
    const metadata = {
      contract: "identidad-reforzada/v1",
      challengeId: "018f47de-88c0-4d35-bd91-76a73848d111",
      certificateB64: "Y2VydGlmaWNhZG8=",
      chainB64: ["ZW1pc29y"],
      format: "cades-detached",
      signatureAlgorithm: "sha256-rsa-pkcs1v15",
      digestAlgorithm: "sha-256"
    };
    const harness = await loadBackground(variant, {
      nativeResponse(message) {
        assert.equal(message.action, "proveIdentity");
        return [
          { requestId: message.requestId, success: true, signature: "Zmly",
            identityProof: metadata, chunk: 0, totalChunks: 2 },
          { requestId: message.requestId, success: true, signature: "bWE=",
            identityProof: metadata, chunk: 1, totalChunks: 2 }
        ];
      }
    });
    const sender = {
      id: "abcdefghijklmnopabcdefghijklmnop",
      url: "https://sede.dipgra.es/operacion"
    };
    const response = await harness.dispatchRuntimeMessage({
      action: "proveIdentity",
      canonicalPayloadB64: "eyJjYW5vbiI6Im9wYWNvIn0="
    }, sender);

    assert.equal(response.success, true);
    assert.equal(response.proof.signatureB64, "ZmlybWE=");
    assert.equal(response.proof.challengeId, metadata.challengeId);
    assert.equal(harness.postedMessages.length, 1);
    assert.deepEqual(
      Object.keys(harness.postedMessages[0]).sort(),
      ["action", "canonicalPayloadB64", "requestId", "requesterOrigin"].sort()
    );
    assert.equal(harness.postedMessages[0].requesterOrigin, "https://sede.dipgra.es");
    assert.equal(harness.liveTimers.size, 0);
  });

  test(`${variant}: identidad rechaza reto no canónico antes del host`, async () => {
    const harness = await loadBackground(variant);
    const response = await harness.dispatchRuntimeMessage({
      action: "proveIdentity",
      canonicalPayloadB64: "***"
    }, {
      id: "abcdefghijklmnopabcdefghijklmnop",
      url: "https://sede.dipgra.es/operacion"
    });
    assert.equal(response.success, false);
    assert.equal(response.code, "identity.invalid_request");
    assert.equal(harness.postedMessages.length, 0);
  });

  test(`${variant}: identidad rechaza metadatos cambiantes entre fragmentos`, async () => {
    const base = {
      contract: "identidad-reforzada/v1",
      challengeId: "018f47de-88c0-4d35-bd91-76a73848d111",
      certificateB64: "Yw==",
      chainB64: [],
      format: "cades-detached",
      signatureAlgorithm: "sha256-ecdsa",
      digestAlgorithm: "sha-256"
    };
    const harness = await loadBackground(variant, {
      nativeResponse(message) {
        return [
          { requestId: message.requestId, success: true, signature: "Zg==",
            identityProof: base, chunk: 0, totalChunks: 2 },
          { requestId: message.requestId, success: true, signature: "Zg==",
            identityProof: { ...base, challengeId: "a6d77e74-6145-48b9-a932-2624be946e48" },
            chunk: 1, totalChunks: 2 }
        ];
      }
    });
    await assert.rejects(
      vm.runInContext(
        `sendToNativeHost("proveIdentity", { canonicalPayloadB64: "e30=" }, "https://sede.dipgra.es")`,
        harness.context
      ),
      /Inconsistent native response chunk count/
    );
    assert.equal(harness.liveTimers.size, 0);
  });

  test(`${variant}: los campos de control nativos no se pueden sobrescribir`, async () => {
    const harness = await loadBackground(variant, {
      nativeResponse(message) {
        return { requestId: message.requestId, success: true };
      }
    });
    await vm.runInContext(
      `sendToNativeHost("ping", {
        requestId: "falseado",
        action: "sign",
        requesterApplication: "falseada",
        requesterOrigin: "https://evil.invalid"
      }, "https://sede.dipgra.es/ruta")`,
      harness.context
    );

    assert.equal(harness.postedMessages.length, 1);
    const message = harness.postedMessages[0];
    assert.notEqual(message.requestId, "falseado");
    assert.equal(message.action, "ping");
    assert.equal(message.requesterApplication, undefined);
    assert.equal(message.requesterOrigin, "https://sede.dipgra.es");
  });

  test(`${variant}: firma interna conserva el origen acreditado por runtime`, async () => {
    const harness = await loadBackground(variant, {
      nativeResponse(message) {
        return {
          requestId: message.requestId,
          success: true,
          signature: "Zg==",
          chunk: 0,
          totalChunks: 1
        };
      }
    });

    const response = await harness.dispatchRuntimeMessage({
      action: "signDocument",
      data: "aG9sYQ==",
      certificateId: "cert-1",
      format: "cades"
    });
    assert.equal(response.success, true);
    assert.equal(harness.postedMessages.length, 1);
    assert.match(
      harness.postedMessages[0].requesterOrigin,
      /^(chrome-extension|moz-extension):\/\//
    );
  });

  test(`${variant}: no anuncia REST sin un Bearer aprovisionado`, async () => {
    const requested = [];
    const harness = await loadBackground(variant, {
      connectError: new Error("native host unavailable"),
      async fetch(url) {
        requested.push(String(url));
        throw new Error("unexpected fetch");
      }
    });

    await assert.rejects(
      vm.runInContext("pingLocalIntegration()", harness.context),
      /Reinstale el conector oficial.*REST fallback: REST fallback deshabilitado/
    );
    assert.deepEqual(requested, []);
    assert.equal(harness.liveTimers.size, 0);
  });

  test(`${variant}: ping valida el fallback REST autenticado`, async () => {
    const requested = [];
    const harness = await loadBackground(variant, {
      connectError: new Error("native host unavailable"),
      restBearer: "ephemeral-test-token",
      async fetch(url, init) {
        requested.push({ url: String(url), authorization: init.headers.Authorization });
        return {
          ok: true,
          status: 200,
          async text() {
            return JSON.stringify({ certificates: [] });
          }
        };
      }
    });

    const result = await vm.runInContext("pingLocalIntegration()", harness.context);
    assert.equal(result.success, true);
    assert.equal(result.mode, "rest");
    assert.deepEqual(requested, [{
      url: "https://127.0.0.1:63118/certificates",
      authorization: "Bearer ephemeral-test-token"
    }]);
    assert.equal(harness.liveTimers.size, 0);
  });
}
