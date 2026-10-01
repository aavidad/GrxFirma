// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

(() => {
  "use strict";

  const isLocalSignerPage = window.location.origin === "https://127.0.0.1:63118" &&
    (window.location.pathname === "/signer" || window.location.pathname === "/firmador");

  if (!isLocalSignerPage) {
    return;
  }

  const params = new URLSearchParams(window.location.search);
  const token = params.get("launchToken");
  if (!token) {
    return;
  }

  grxfirmaExt.runtime.sendMessage({ action: "takePendingSignerLaunch", token }).then((response) => {
    if (!response || !response.success || !response.document) {
      window.postMessage({
        type: "grxfirma-extension-document-error",
        error: (response && response.error) || "missing_pending_document"
      }, window.location.origin);
      return;
    }
    window.postMessage({
      type: "grxfirma-extension-document",
      document: response.document
    }, window.location.origin);
  }).catch((error) => {
      window.postMessage({
        type: "grxfirma-extension-document-error",
        error: error.message || "bridge_error"
      }, window.location.origin);
  });
})();
