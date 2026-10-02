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
const variants = ["chromium", "firefox"];

async function readSource(variant, relativePath) {
  return readFile(path.join(extensionRoot, "src", variant, relativePath), "utf8");
}

async function readJSON(variant, relativePath) {
  return JSON.parse(await readSource(variant, relativePath));
}

function messageKeysFromManifest(manifest) {
  return new Set(
    [...JSON.stringify(manifest).matchAll(/__MSG_([A-Za-z][A-Za-z0-9_]*)__\/?/g)]
      .map((match) => match[1])
  );
}

function messageKeysFromHTML(source) {
  return new Set(
    [...source.matchAll(/data-i18n(?:-(?:aria-label|placeholder|title))?="([^"]+)"/g)]
      .map((match) => match[1])
  );
}

function messageKeysFromJavaScript(source) {
  return new Set(
    [...source.matchAll(/(?:\bt|\bsay|\bshowNotice|\.i18n\.message|\.i18n\.getMessage)\(\s*["']([A-Za-z][A-Za-z0-9_]*)["']/g)]
      .map((match) => match[1])
  );
}

for (const variant of variants) {
  test(`${variant}: los catalogos i18n publicados tienen paridad y cubren cada referencia`, async () => {
    const manifest = await readJSON(variant, "manifest.json");
    const spanish = await readJSON(variant, "_locales/es/messages.json");
    const english = await readJSON(variant, "_locales/en/messages.json");
    const popupHTML = await readSource(variant, "popup.html");
    const optionsHTML = await readSource(variant, "options.html");
    const optionsJS = await readSource(variant, "options.js");
    const popupJS = await readSource(variant, "popup.js");
    const backgroundJS = await readSource(variant, "background.js");
    const pdfDetector = await readSource(variant, "content_scripts/pdf_detector.js");
    const signerJS = await readSource(variant, "signer/signer.js");

    assert.equal(manifest.default_locale, "es");
    assert.deepEqual(Object.keys(english).sort(), Object.keys(spanish).sort());
    for (const [locale, catalog] of [["es", spanish], ["en", english]]) {
      for (const [key, entry] of Object.entries(catalog)) {
        assert.equal(typeof entry.message, "string", `${locale}.${key} no contiene message`);
        assert.notEqual(entry.message.trim(), "", `${locale}.${key} esta vacia`);
      }
    }

    const referenced = new Set([
      ...messageKeysFromManifest(manifest),
      ...messageKeysFromHTML(popupHTML),
      ...messageKeysFromHTML(optionsHTML),
      ...messageKeysFromJavaScript(optionsJS),
      ...messageKeysFromJavaScript(popupJS),
      ...messageKeysFromJavaScript(backgroundJS),
      ...messageKeysFromJavaScript(pdfDetector),
      ...messageKeysFromJavaScript(signerJS)
    ]);
    assert.deepEqual([...referenced].sort(), Object.keys(spanish).sort());
  });

  test(`${variant}: la superficie visible publicada no introduce literales fuera de i18n`, async () => {
    const popupHTML = await readSource(variant, "popup.html");
    const popupJS = await readSource(variant, "popup.js");
    const optionsHTML = await readSource(variant, "options.html");
    const optionsJS = await readSource(variant, "options.js");
    const pdfDetector = await readSource(variant, "content_scripts/pdf_detector.js");

    const visibleHTML = (popupHTML + optionsHTML)
      .replace(/<!--[\s\S]*?-->/g, "")
      .replace(/<style[\s\S]*?<\/style>/gi, "")
      .replace(/<script[\s\S]*?<\/script>/gi, "");
    const directText = [...visibleHTML.matchAll(/>([^<>]+)</g)]
      .map((match) => match[1].trim())
      .filter((value) => value);
    assert.deepEqual(directText, []);

    const directAttributes = [
      ...visibleHTML.matchAll(/\s(?:alt|aria-label|placeholder|title)="([^"]+)"/g)
    ]
      .map((match) => match[1].trim())
      .filter((value) => value);
    assert.deepEqual(directAttributes, []);

    for (const [relativePath, source] of [
      ["popup.js", popupJS],
      ["options.js", optionsJS],
      ["content_scripts/pdf_detector.js", pdfDetector]
    ]) {
      const directAssignments = [
        ...source.matchAll(/\.(?:textContent|title)\s*=\s*(["'`])([^"'`\n]*)\1/g)
      ]
        .map((match) => match[2].trim())
        .filter((value) => /[A-Za-zÁÉÍÓÚÑáéíóúñ]/.test(value));
      assert.deepEqual(
        directAssignments,
        [],
        `${relativePath} asigna texto visible literal: ${directAssignments.join(", ")}`
      );

      assert.doesNotMatch(
        source,
        /setAttribute\(\s*["'](?:aria-label|title)["']\s*,\s*["'][^"']*[A-Za-zÁÉÍÓÚÑáéíóúñ]/,
        `${relativePath} asigna un atributo visible literal`
      );
    }
  });
}

test("compat localiza texto, atributos y el idioma del documento", async () => {
  const source = await readSource("chromium", "compat.js");
  const textNode = {
    textContent: "",
    getAttribute(name) {
      return name === "data-i18n" ? "popupIntro" : null;
    }
  };
  const titleNode = {
    title: "",
    getAttribute(name) {
      return name === "data-i18n-title" ? "pdfPreparingDocument" : null;
    },
    setAttribute(name, value) {
      this[name] = value;
    }
  };
  const document = {
    documentElement: { lang: "es" },
    querySelectorAll(selector) {
      if (selector === "[data-i18n]") return [textNode];
      if (selector === "[data-i18n-title]") return [titleNode];
      return [];
    }
  };
  const translations = {
    popupIntro: "Secure web integration",
    pdfPreparingDocument: "Preparing document"
  };
  const storage = { get() {}, set() {}, remove() {} };
  const context = vm.createContext({
    document,
    chrome: {
      i18n: {
        getMessage(key) {
          return translations[key] || "";
        },
        getUILanguage() {
          return "en_GB";
        }
      },
      runtime: {
        onMessage: {}
      },
      storage: {
        local: storage
      },
      tabs: {}
    }
  });

  vm.runInContext(source, context, { filename: "chromium/compat.js" });
  vm.runInContext("grxfirmaExt.i18n.localizeDocument()", context);

  assert.equal(document.documentElement.lang, "en-GB");
  assert.equal(textNode.textContent, "Secure web integration");
  assert.equal(titleNode.title, "Preparing document");
  assert.equal(
    vm.runInContext("grxfirmaExt.i18n.message('unknownMessage')", context),
    "unknownMessage"
  );
});
