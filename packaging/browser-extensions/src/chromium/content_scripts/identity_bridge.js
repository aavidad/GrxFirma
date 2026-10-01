// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

(() => {
  "use strict";

  const tipoSolicitud = "grxfirma-identidad-solicitar/v1";
  const tipoResultado = "grxfirma-identidad-resultado/v1";
  const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
  const base64Canonico = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/;
  let operacionActiva = false;

  if (window.top !== window || window.location.protocol !== "https:") {
    return;
  }

  function base64Acotado(valor, maximoBytes) {
    if (typeof valor !== "string" || valor.length === 0 || valor.trim() !== valor ||
      valor.length > Math.ceil(maximoBytes / 3) * 4 || !base64Canonico.test(valor)) {
      return false;
    }
    try {
      const contenido = atob(valor);
      return contenido.length > 0 && contenido.length <= maximoBytes && btoa(contenido) === valor;
    } catch (error) {
      return false;
    }
  }

  function solicitudValida(datos) {
    if (!datos || Object.prototype.toString.call(datos) !== "[object Object]") {
      return false;
    }
    const claves = Object.keys(datos).sort();
    return claves.length === 3 && claves[0] === "contenido_canonico_b64" &&
      claves[1] === "correlacion_id" && claves[2] === "type" &&
      datos.type === tipoSolicitud && typeof datos.correlacion_id === "string" &&
      uuidV4.test(datos.correlacion_id) && base64Acotado(datos.contenido_canonico_b64, 16 * 1024);
  }

  function codigoPublico(codigo) {
    switch (codigo) {
      case "identity.invalid_request":
      case "identity.invalid_proof":
        return "identidad.agente.prueba_invalida";
      case "identity.busy":
      case "identity.unavailable":
      default:
        return "identidad.agente.no_disponible";
    }
  }

  function publicarResultado(correlacionId, resultado) {
    window.postMessage({
      type: tipoResultado,
      correlacion_id: correlacionId,
      ...resultado
    }, window.location.origin);
  }

  function reconstruirPrueba(respuesta) {
    const prueba = respuesta && respuesta.proof;
    if (!respuesta || respuesta.success !== true || !prueba) {
      return null;
    }
    return {
      reto_id: prueba.challengeId,
      contrato: prueba.contract,
      firma_b64: prueba.signatureB64,
      certificado_b64: prueba.certificateB64,
      cadena_b64: Array.isArray(prueba.chainB64) ? [...prueba.chainB64] : [],
      formato: prueba.format,
      algoritmo_firma: prueba.signatureAlgorithm,
      algoritmo_huella: prueba.digestAlgorithm
    };
  }

  async function procesarSolicitud(datos) {
    if (operacionActiva) {
      publicarResultado(datos.correlacion_id, {
        exito: false,
        codigo: "identidad.agente.no_disponible"
      });
      return;
    }
    operacionActiva = true;
    try {
      const respuesta = await grxfirmaExt.runtime.sendMessage({
        action: "proveIdentity",
        canonicalPayloadB64: datos.contenido_canonico_b64
      });
      const prueba = reconstruirPrueba(respuesta);
      if (!prueba) {
        publicarResultado(datos.correlacion_id, {
          exito: false,
          codigo: codigoPublico(respuesta && respuesta.code)
        });
        return;
      }
      publicarResultado(datos.correlacion_id, { exito: true, prueba });
    } catch (error) {
      publicarResultado(datos.correlacion_id, {
        exito: false,
        codigo: "identidad.agente.no_disponible"
      });
    } finally {
      operacionActiva = false;
    }
  }

  window.addEventListener("message", (evento) => {
    if (evento.source !== window || evento.origin !== window.location.origin ||
      !solicitudValida(evento.data)) {
      return;
    }
    void procesarSolicitud(evento.data);
  });
})();
