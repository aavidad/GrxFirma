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
const correlacion = "a6d77e74-6145-48b9-a932-2624be946e48";

function plano(valor) {
  return JSON.parse(JSON.stringify(valor));
}

async function cargarPuente(variante, respuestaRuntime) {
  const listeners = [];
  const enviados = [];
  const llamadas = [];
  const window = {
    location: { protocol: "https:", origin: "https://sede.dipgra.es" },
    addEventListener(tipo, listener) {
      if (tipo === "message") listeners.push(listener);
    },
    postMessage(mensaje, origen) {
      enviados.push({ mensaje, origen });
    }
  };
  window.top = window;
  const context = vm.createContext({
    atob,
    btoa,
    Object,
    window,
    grxfirmaExt: {
      runtime: {
        async sendMessage(mensaje) {
          llamadas.push(mensaje);
          return respuestaRuntime(mensaje);
        }
      }
    }
  });
  const source = await readFile(
    path.join(extensionRoot, "src", variante, "content_scripts", "identity_bridge.js"),
    "utf8"
  );
  vm.runInContext(source, context, { filename: `${variante}/identity_bridge.js` });
  return {
    enviados,
    llamadas,
    async despachar(datos, origen = window.location.origin, fuente = window) {
      for (const listener of listeners) listener({ data: datos, origin: origen, source: fuente });
      await new Promise((resolve) => setImmediate(resolve));
    }
  };
}

for (const variante of ["chromium", "firefox"]) {
  test(`${variante}: el puente entrega sólo la prueba pública`, async () => {
    const puente = await cargarPuente(variante, async () => ({
      success: true,
      proof: {
        contract: "identidad-reforzada/v1",
        challengeId: correlacion,
        signatureB64: "ZmlybWE=",
        certificateB64: "Y2VydGlmaWNhZG8=",
        chainB64: ["ZW1pc29y"],
        format: "cades-detached",
        signatureAlgorithm: "sha256-rsa-pkcs1v15",
        digestAlgorithm: "sha-256",
        certificateId: "no-debe-salir"
      }
    }));
    await puente.despachar({
      type: "grxfirma-identidad-solicitar/v1",
      correlacion_id: correlacion,
      contenido_canonico_b64: "eyJjYW5vbiI6Im9wYWNvIn0="
    });

    assert.deepEqual(plano(puente.llamadas), [{
      action: "proveIdentity",
      canonicalPayloadB64: "eyJjYW5vbiI6Im9wYWNvIn0="
    }]);
    assert.equal(puente.enviados.length, 1);
    assert.equal(puente.enviados[0].origen, "https://sede.dipgra.es");
    assert.equal(puente.enviados[0].mensaje.exito, true);
    assert.equal(puente.enviados[0].mensaje.prueba.reto_id, correlacion);
    assert.equal(puente.enviados[0].mensaje.prueba.certificateId, undefined);
  });

  test(`${variante}: ignora origen, correlación y campos ambiguos`, async () => {
    const puente = await cargarPuente(variante, async () => ({ success: false }));
    const base = {
      type: "grxfirma-identidad-solicitar/v1",
      correlacion_id: correlacion,
      contenido_canonico_b64: "e30="
    };
    await puente.despachar(base, "https://atacante.invalid");
    await puente.despachar({ ...base, correlacion_id: "no-uuid" });
    await puente.despachar({ ...base, certificate_id: "inyectado" });
    assert.equal(puente.llamadas.length, 0);
    assert.equal(puente.enviados.length, 0);
  });

  test(`${variante}: convierte errores internos en código estable`, async () => {
    const puente = await cargarPuente(variante, async () => ({
      success: false,
      code: "diagnostico.interno",
      error: "ruta o certificado sensible"
    }));
    await puente.despachar({
      type: "grxfirma-identidad-solicitar/v1",
      correlacion_id: correlacion,
      contenido_canonico_b64: "e30="
    });
    assert.deepEqual(plano(puente.enviados[0].mensaje), {
      type: "grxfirma-identidad-resultado/v1",
      correlacion_id: correlacion,
      exito: false,
      codigo: "identidad.agente.no_disponible"
    });
  });
}
