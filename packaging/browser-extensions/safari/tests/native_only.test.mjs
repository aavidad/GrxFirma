// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const shimUrl = new URL("../SafariNativeOnly.js.in", import.meta.url);
const shim = await readFile(shimUrl, "utf8");

function loadShim() {
  const nativeCalls = [];
  const context = {
    Error,
    getSystemCertificatesFromRest: async () => "unsafe-rest",
    signWithSystemCertificateRest: async () => "unsafe-rest",
    verifyWithSystemCertificateRest: async () => "unsafe-rest",
    pingLocalIntegration: async () => ({ mode: "unsafe-rest" }),
    sendToNativeHost: async (action) => {
      nativeCalls.push(action);
      return { requestId: "test", success: true };
    },
  };
  vm.runInNewContext(shim, context, { filename: "SafariNativeOnly.js.in" });
  return { context, nativeCalls };
}

test("Safari disables every JavaScript REST fallback", async () => {
  const { context } = loadShim();
  const nativeError = new Error("native unavailable");

  await assert.rejects(
    context.getSystemCertificatesFromRest(nativeError),
    (error) => error === nativeError,
  );
  await assert.rejects(
    context.signWithSystemCertificateRest("data", "cert", "cades", null, nativeError),
    (error) => error === nativeError,
  );
  await assert.rejects(
    context.verifyWithSystemCertificateRest("signature", "original", "cades", "application/octet-stream", nativeError),
    (error) => error === nativeError,
  );
});

test("Safari ping uses only the native handler", async () => {
  const { context, nativeCalls } = loadShim();
  const response = await context.pingLocalIntegration();

  assert.deepEqual(nativeCalls, ["ping"]);
  assert.equal(response.success, true);
  assert.equal(response.mode, "safari-native");
  assert.equal(response.native.success, true);
});
