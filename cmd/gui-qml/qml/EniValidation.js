// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

.pragma library

function safe(text, limit) { return text.length <= limit && !/[\x00-\x1f\x7f-\x9f]/.test(text) }
function organError(text) {
    if (!safe(text, 128)) return "eni.validacion.dir3"
    const codes = text.split(",")
    return codes.length <= 128 && codes.every(function(code) { return /^[A-Z][0-9]{8}$/.test(code.trim().toUpperCase()) })
            ? "" : "eni.validacion.dir3"
}
function identifierError(text, required) {
    if (!safe(text, 49)) return "eni.validacion.identifier"
    const value = text.trim()
    if (value === "") return required ? "eni.validacion.source" : ""
    return /^ES_[A-Z][0-9]{8}_[0-9]{4}_[A-Za-z0-9_]{1,30}$/.test(value) ? "" : "eni.validacion.identifier"
}
function classificationError(text) {
    return safe(text, 44) && /^(?:[0-9]{1,30}|[A-Z][0-9]{8}_PRO_[A-Za-z0-9_]{1,30})$/.test(text.trim())
            ? "" : "eni.validacion.classification"
}
function formatError(text) {
    return safe(text, 32) && (text.trim() === "" || /^[A-Za-z0-9.+-]{1,32}$/.test(text.trim())) ? "" : "eni.validacion.format"
}
function interestedError(text) {
    return safe(text, 256) && text.split(",").every(function(value) { return safe(value.trim(), 128) }) ? "" : "eni.validacion.text"
}
function dateError(text) {
    if (!safe(text, 35)) return "eni.validacion.date"
    const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-](\d{2}):(\d{2}))$/.exec(text)
    if (!match) return "eni.validacion.date"
    const y = Number(match[1]), m = Number(match[2]), d = Number(match[3])
    const date = new Date(0)
    date.setUTCFullYear(y, m - 1, d)
    if (y < 1 || m < 1 || m > 12 || d < 1 || date.getUTCFullYear() !== y || date.getUTCMonth() !== m - 1 || date.getUTCDate() !== d
            || Number(match[4]) > 23 || Number(match[5]) > 59 || Number(match[6]) > 59
            || (match[7] !== "Z" && (Number(match[8]) > 14 || Number(match[9]) > 59 || (Number(match[8]) === 14 && Number(match[9]) !== 0)))
            || isNaN(new Date(text).getTime())) return "eni.validacion.date"
    return ""
}
function rfc3339(date) {
    function pad(n) { return String(n).padStart(2, "0") }
    const offset = -date.getTimezoneOffset()
    return String(date.getFullYear()).padStart(4, "0") + "-" + pad(date.getMonth() + 1) + "-" + pad(date.getDate())
            + "T" + pad(date.getHours()) + ":" + pad(date.getMinutes()) + ":" + pad(date.getSeconds())
            + (offset >= 0 ? "+" : "-") + pad(Math.floor(Math.abs(offset) / 60)) + ":" + pad(Math.abs(offset) % 60)
}
function pad2(n) { return String(n).padStart(2, "0") }
// Formato visible: dd/mm/aaaa y hh:mm (24 horas), igual que el selector de WinUI.
function displayDate(date) { return pad2(date.getDate()) + "/" + pad2(date.getMonth() + 1) + "/" + String(date.getFullYear()).padStart(4, "0") }
function displayTime(date) { return pad2(date.getHours()) + ":" + pad2(date.getMinutes()) }
function parseLocal(dateText, timeText) {
    const d = /^\s*(\d{1,2})\/(\d{1,2})\/(\d{4})\s*$/.exec(dateText)
    const t = /^\s*(\d{1,2}):(\d{2})\s*$/.exec(timeText)
    if (!d || !t) return null
    const day = Number(d[1]), month = Number(d[2]), year = Number(d[3])
    const hours = Number(t[1]), minutes = Number(t[2])
    if (year < 1000 || month < 1 || month > 12 || day < 1 || hours > 23 || minutes > 59) return null
    const date = new Date(year, month - 1, day, hours, minutes, 0)
    if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) return null
    return date
}
