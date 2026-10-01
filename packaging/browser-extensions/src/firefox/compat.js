// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

(function (global) {
  "use strict";

  const api = (typeof browser !== "undefined")
    ? browser
    : ((typeof chrome !== "undefined") ? chrome : null);
  const usesPromiseNamespace = typeof browser !== "undefined";

  if (!api || !api.runtime) {
    throw new Error("WebExtension runtime API unavailable");
  }

  function lastErrorMessage() {
    return (typeof chrome !== "undefined" && chrome.runtime && chrome.runtime.lastError)
      ? chrome.runtime.lastError.message
      : "";
  }

  function call(target, method, args) {
    if (usesPromiseNamespace) {
      return target[method](...args);
    }
    return new Promise((resolve, reject) => {
      try {
        target[method](...args, (result) => {
          const err = lastErrorMessage();
          if (err) {
            reject(new Error(err));
            return;
          }
          resolve(result);
        });
      } catch (err) {
        reject(err);
      }
    });
  }

  function storageArea(name) {
    const storage = api.storage || {};
    const area = name === "managed" ? storage.managed : ((name === "session" && storage.session) ? storage.session : storage.local);
    return {
      get(keys) {
        return call(area, "get", [keys]);
      },
      set(items) {
        return call(area, "set", [items]);
      },
      remove(keys) {
        return call(area, "remove", [keys]);
      }
    };
  }

  function localizedMessage(key, substitutions) {
    if (api.i18n && typeof api.i18n.getMessage === "function") {
      const value = api.i18n.getMessage(key, substitutions);
      if (value) return value;
    }
    return key;
  }

  function localizeDocument(root = global.document) {
    if (!root) return;

    if (root.documentElement && api.i18n && typeof api.i18n.getUILanguage === "function") {
      const language = api.i18n.getUILanguage();
      if (language) root.documentElement.lang = language.replaceAll("_", "-");
    }

    for (const node of root.querySelectorAll("[data-i18n]")) {
      node.textContent = localizedMessage(node.getAttribute("data-i18n"));
    }
    for (const attribute of ["aria-label", "placeholder", "title"]) {
      const marker = `data-i18n-${attribute}`;
      for (const node of root.querySelectorAll(`[${marker}]`)) {
        node.setAttribute(attribute, localizedMessage(node.getAttribute(marker)));
      }
    }
  }

  global.grxfirmaExt = {
    i18n: {
      message: localizedMessage,
      localizeDocument
    },
    runtime: {
      id: api.runtime.id,
      connectNative(name) {
        return api.runtime.connectNative(name);
      },
      getURL(path) {
        return api.runtime.getURL(path);
      },
      getManifest() {
        return api.runtime.getManifest();
      },
      sendMessage(message) {
        return call(api.runtime, "sendMessage", [message]);
      },
      lastErrorMessage,
      onMessage: api.runtime.onMessage,
      onInstalled: api.runtime.onInstalled,
      onStartup: api.runtime.onStartup
    },
    tabs: {
      create(createProperties) {
        return call(api.tabs, "create", [createProperties]);
      }
    },
    permissions: {
      contains(value) { return call(api.permissions, "contains", [value]); },
      request(value) { return call(api.permissions, "request", [value]); },
      remove(value) { return call(api.permissions, "remove", [value]); },
      onAdded: api.permissions && api.permissions.onAdded,
      onRemoved: api.permissions && api.permissions.onRemoved
    },
    scripting: {
      getRegisteredContentScripts(value = {}) { return call(api.scripting, "getRegisteredContentScripts", [value]); },
      registerContentScripts(value) { return call(api.scripting, "registerContentScripts", [value]); },
      unregisterContentScripts(value) { return call(api.scripting, "unregisterContentScripts", [value]); }
    },
    storageManaged: storageArea("managed"),
    storageOnChanged: api.storage.onChanged,
    storageLocal: storageArea("local"),
    storageSession: storageArea("session"),
    hasStorageSession() {
      return !!(api.storage && api.storage.session);
    }
  };
})(globalThis);
