// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

.pragma library

// Firmantes de una verificación escritos como los lee una persona, igual que
// el informe del motor (informeverificacion/nombre.go) y WinUI: nombre, NIF,
// organización, emisor por su nombre y fecha de la firma con su origen. El
// DN completo queda para los detalles técnicos. Aquí no hay textos: las
// etiquetas llegan ya traducidas desde main.qml.

// Separa un DN (RFC 4514) por comas, punto y coma o «+» no escapados.
function dnComponents(dn) {
    const text = String(dn || "")
    const out = []
    let current = ""
    for (let i = 0; i < text.length; i++) {
        const c = text.charAt(i)
        if (c === "\\" && i + 1 < text.length) {
            current += c + text.charAt(i + 1)
            i++
            continue
        }
        if (c === "," || c === ";" || c === "+") {
            out.push(current)
            current = ""
            continue
        }
        current += c
    }
    out.push(current)
    return out
}

function isHex(c) {
    return /^[0-9A-Fa-f]$/.test(c)
}

// Bytes a texto UTF-8; si no es UTF-8 válido, nada.
function utf8(bytes) {
    let escaped = ""
    for (let i = 0; i < bytes.length; i++)
        escaped += "%" + (bytes[i] < 16 ? "0" : "") + bytes[i].toString(16)
    try {
        return decodeURIComponent(escaped)
    } catch (e) {
        return ""
    }
}

// Un valor «#…» es el DER del atributo en hexadecimal (crypto/x509 lo escribe
// así para los OID que no conoce, como nombre y apellidos). Solo se aceptan
// cadenas de texto ASN.1 que ocupen el valor entero.
function decodeDerString(hex) {
    if (hex.length < 4 || hex.length % 2 !== 0 || !/^[0-9A-Fa-f]+$/.test(hex))
        return ""
    const bytes = []
    for (let i = 0; i < hex.length; i += 2)
        bytes.push(parseInt(hex.substr(i, 2), 16))
    const tag = bytes[0]
    let length = bytes[1]
    let offset = 2
    if (length & 0x80) {
        const count = length & 0x7f
        if (count < 1 || count > 2 || bytes.length < 2 + count)
            return ""
        length = 0
        for (let i = 0; i < count; i++)
            length = length * 256 + bytes[2 + i]
        offset = 2 + count
    }
    if (offset + length !== bytes.length)
        return ""
    const body = bytes.slice(offset)
    switch (tag) {
    case 0x0c: // UTF8String
        return utf8(body)
    case 0x12: // NumericString
    case 0x13: // PrintableString
    case 0x16: // IA5String
    case 0x1a: // VisibleString
    case 0x1b: // GeneralString
        for (let i = 0; i < body.length; i++)
            if (body[i] > 0x7f) return ""
        return String.fromCharCode.apply(null, body)
    case 0x14: // T61String: se lee como Latin-1, igual que encoding/asn1
        return String.fromCharCode.apply(null, body)
    case 0x1e: { // BMPString
        if (body.length % 2 !== 0) return ""
        let text = ""
        for (let i = 0; i < body.length; i += 2)
            text += String.fromCharCode(body[i] * 256 + body[i + 1])
        return text
    }
    default:
        return ""
    }
}

// Deshace los escapes RFC 4514 («\,» o «\c3\b1») y decodifica «#…».
function dnValue(raw) {
    const value = String(raw || "").trim()
    if (value.charAt(0) === "#")
        return decodeDerString(value.substring(1))
    const bytes = []
    const pushText = function(text) {
        let encoded = ""
        try {
            encoded = encodeURIComponent(text)
        } catch (e) {
            return
        }
        for (let k = 0; k < encoded.length; k++) {
            if (encoded.charAt(k) === "%") {
                bytes.push(parseInt(encoded.substr(k + 1, 2), 16))
                k += 2
            } else {
                bytes.push(encoded.charCodeAt(k))
            }
        }
    }
    for (let i = 0; i < value.length; i++) {
        const c = value.charAt(i)
        if (c === "\\" && i + 1 < value.length) {
            if (i + 2 < value.length && isHex(value.charAt(i + 1)) && isHex(value.charAt(i + 2))) {
                bytes.push(parseInt(value.substr(i + 1, 2), 16))
                i += 2
                continue
            }
            pushText(value.charAt(i + 1))
            i++
            continue
        }
        pushText(c)
    }
    const decoded = utf8(bytes)
    return decoded !== "" || bytes.length === 0 ? decoded : value
}

// Atributos que una persona necesita leer de un DN.
function parseDistinguishedName(dn) {
    const result = { common: "", identifier: "", organization: "", givenName: "", surname: "" }
    const parts = dnComponents(dn)
    for (let i = 0; i < parts.length; i++) {
        const cut = parts[i].indexOf("=")
        if (cut <= 0) continue
        const type = parts[i].substring(0, cut).trim().toUpperCase()
        const value = dnValue(parts[i].substring(cut + 1))
        if (value === "") continue
        let field = ""
        if (type === "CN" || type === "2.5.4.3") field = "common"
        else if (type === "SERIALNUMBER" || type === "2.5.4.5") field = "identifier"
        else if (type === "O" || type === "2.5.4.10") field = "organization"
        else if (type === "GIVENNAME" || type === "GN" || type === "2.5.4.42") field = "givenName"
        else if (type === "SN" || type === "SURNAME" || type === "2.5.4.4") field = "surname"
        if (field !== "" && result[field] === "")
            result[field] = value
    }
    return result
}

// El CN o, si falta, nombre y apellidos; en último caso, el DN recibido.
function readableName(dn) {
    const parsed = parseDistinguishedName(dn)
    if (parsed.common !== "") return parsed.common
    const full = (parsed.givenName + " " + parsed.surname).trim()
    return full !== "" ? full : String(dn || "").trim()
}

// Emisor por su nombre y, si es distinta, su organización entre paréntesis.
function readableIssuer(dn) {
    const parsed = parseDistinguishedName(dn)
    let name = readableName(dn)
    if (parsed.organization !== "" && parsed.organization !== name)
        name += " (" + parsed.organization + ")"
    return name
}

function pad(n) {
    return n < 10 ? "0" + n : String(n)
}

// Escribe la fecha en hora local con un patrón de Go («02/01/2006 15:04:05»),
// el mismo que usa el catálogo para el informe, más la zona horaria.
function formatGoLayout(date, layout) {
    const tokens = {
        "2006": String(date.getFullYear()),
        "01": pad(date.getMonth() + 1),
        "02": pad(date.getDate()),
        "15": pad(date.getHours()),
        "04": pad(date.getMinutes()),
        "05": pad(date.getSeconds())
    }
    const text = String(layout).replace(/2006|01|02|15|04|05/g, function(token) { return tokens[token] })
    const offset = -date.getTimezoneOffset()
    if (offset === 0) return text + " (UTC)"
    const sign = offset > 0 ? "+" : "-"
    const abs = Math.abs(offset)
    return text + " (UTC" + sign + pad(Math.floor(abs / 60)) + ":" + pad(abs % 60) + ")"
}

// Fecha de la firma tal como la entrega el motor (RFC 3339) solo si su
// origen es uno conocido; si no, nada: no se inventa una fecha.
function signingTimeText(raw, source, layout) {
    if (source !== "timestamp" && source !== "signed_attribute")
        return ""
    const text = String(raw || "")
    if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/.test(text))
        return ""
    const date = new Date(text)
    if (isNaN(date.getTime()))
        return ""
    return formatGoLayout(date, layout || "02/01/2006 15:04:05")
}
