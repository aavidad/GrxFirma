// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Los permisos y scripts dinámicos solo usan patrones validados aquí.
(function (global) {
  'use strict';
  const PUBLIC_SUFFIXES = new Set([
    'es', 'gob.es', 'com.es', 'nom.es', 'org.es', 'edu.es', 'com', 'org', 'net', 'eu', 'edu', 'gov', 'int', 'info', 'biz',
    'uk', 'co.uk', 'gov.uk', 'ac.uk', 'org.uk', 'fr', 'gouv.fr', 'pt', 'gov.pt', 'it', 'gov.it', 'de', 'io', 'app', 'dev',
    // Alojamientos compartidos: cada subdominio pertenece a un tercero distinto.
    'github.io', 'gitlab.io', 'pages.dev', 'workers.dev', 'netlify.app', 'vercel.app', 'web.app', 'firebaseapp.com',
    'herokuapp.com', 'azurewebsites.net', 'cloudfront.net', 'blogspot.com', 'appspot.com', 'amazonaws.com', 'onrender.com'
  ]);
  const FACTORY_SITES = Object.freeze(['*.dipgra.es', '*.savia.net']);

  function normalizeSite(input) {
    if (typeof input !== 'string') throw new Error('invalid_site');
    const raw = input.trim().toLowerCase();
    if (!raw || /[\s\/:?#@\\%\u0000-\u001f\u007f]/u.test(raw) || raw.includes('..') || raw.endsWith('.')) throw new Error('invalid_site');
    const wildcard = raw.startsWith('*.');
    const hostInput = wildcard ? raw.slice(2) : raw;
    if (hostInput.includes('*') || !hostInput.includes('.')) throw new Error('invalid_site');
    let host;
    try { host = new URL(`https://${hostInput}/`).hostname.toLowerCase(); }
    catch (_) { throw new Error('invalid_site'); }
    if (!host || host.length > 253 || /^\d+\.\d+\.\d+\.\d+$/.test(host)) throw new Error('invalid_site');
    const labels = host.split('.');
    if (labels.length < 2 || labels.some((label) => !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))) throw new Error('invalid_site');
    if (host === 'localhost' || host.endsWith('.localhost') || labels.at(-1).match(/^\d+$/)) throw new Error('invalid_site');
    if (PUBLIC_SUFFIXES.has(host)) throw new Error('invalid_site');
    return `${wildcard ? '*.' : ''}${host}`;
  }

  function pattern(site) { return `https://${normalizeSite(site)}/*`; }
  function matches(url, site) {
    let parsed;
    try { parsed = new URL(url); } catch (_) { return false; }
    if (parsed.protocol !== 'https:' || (parsed.port && parsed.port !== '443')) return false;
    const valid = normalizeSite(site);
    if (valid.startsWith('*.')) {
      const base = valid.slice(2);
      return parsed.hostname === base || parsed.hostname.endsWith(`.${base}`);
    }
    return parsed.hostname === valid;
  }
  function normalizeList(items) {
    if (!Array.isArray(items)) return [];
    const result = new Set();
    for (const item of items) { try { result.add(normalizeSite(item)); } catch (_) { /* Se ignora política inválida. */ } }
    return [...result].sort();
  }
  global.grxfirmaTrustedSites = { normalizeSite, normalizeList, pattern, matches, FACTORY_SITES };
})(globalThis);
