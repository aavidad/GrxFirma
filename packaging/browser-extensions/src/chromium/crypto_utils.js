// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

/**
 * crypto_utils.js
 * Hybrid Encryption (RSA-OAEP + AES-GCM) using Web Crypto API
 * H-03: Vault and Identity Protection with Key Derivation
 */

var CryptoUtils = globalThis.CryptoUtils = {
    AES_ALGO: "AES-GCM",
    RSA_ALGO: "RSA-OAEP",
    KEY_LEN: 256,
    IV_LEN: 12,
    
    // H-03: Cache de claves derivadas en memoria (solo sesión actual)
    // Se descarta al cerrar la extensión
    _derivedKeyCache: null,
    _sessionPassword: null,

    /**
     * --- SYMMETRIC (VAULT) ---
     */
    
    // H-03: Derivar clave AES con PBKDF2-SHA-256 (≥600k iteraciones, OWASP 2023)
    async _deriveKeyFromPassword(password, salt) {
        // Si no hay salt, generar uno aleatorio
        if (!salt) {
            salt = crypto.getRandomValues(new Uint8Array(32));
        }
        
        const enc = new TextEncoder();
        const passwordKey = await crypto.subtle.importKey(
            "raw",
            enc.encode(password),
            "PBKDF2",
            false,
            ["deriveBits"]
        );
        
        const derivedBits = await crypto.subtle.deriveBits(
            {
                name: "PBKDF2",
                hash: "SHA-256",
                salt: salt,
                iterations: 600000 // OWASP 2023 minimum
            },
            passwordKey,
            this.KEY_LEN
        );
        
        const key = await crypto.subtle.importKey(
            "raw",
            derivedBits,
            this.AES_ALGO,
            true,
            ["encrypt", "decrypt"]
        );
        
        return { key, salt };
    },
    
    // H-03: Contraseña de sesión aleatoria para proteger el vault (no hardcodeada)
    async _getSessionPassword() {
        if (this._sessionPassword) {
            return this._sessionPassword;
        }
        // Intentar recuperar contraseña de sesión ya generada
        const sessionResult = grxfirmaExt.hasStorageSession()
            ? await grxfirmaExt.storageSession.get(['grx_vault_pw'])
            : {};
        if (sessionResult.grx_vault_pw) {
            this._sessionPassword = sessionResult.grx_vault_pw;
            return this._sessionPassword;
        }
        // Generar contraseña aleatoria nueva para esta sesión
        const randomBytes = crypto.getRandomValues(new Uint8Array(32));
        const pw = btoa(String.fromCharCode(...randomBytes));
        if (grxfirmaExt.hasStorageSession()) {
            await grxfirmaExt.storageSession.set({ grx_vault_pw: pw });
        }
        this._sessionPassword = pw;
        return pw;
    },
    
    async getSecretKey() {
        // H-03: Si la key derivada está en cache de sesión, usarla
        if (this._derivedKeyCache) {
            return this._derivedKeyCache;
        }
        
        const sessionResult = grxfirmaExt.hasStorageSession()
            ? await grxfirmaExt.storageSession.get(['grx_crypto_salt'])
            : {};

        if (sessionResult.grx_crypto_salt) {
            const password = await this._getSessionPassword();
            const salt = Uint8Array.from(atob(sessionResult.grx_crypto_salt), c => c.charCodeAt(0));
            const { key } = await this._deriveKeyFromPassword(password, salt);
            this._derivedKeyCache = key;
            return key;
        }

        const result = await grxfirmaExt.storageLocal.get(['grx_crypto_key']);
        if (result.grx_crypto_key) {
            console.warn("[CryptoUtils] Ignoring legacy unprotected vault key from chrome.storage.local");
            await grxfirmaExt.storageLocal.remove('grx_crypto_key');
        }

        const password = await this._getSessionPassword();
        const { key, salt } = await this._deriveKeyFromPassword(password);
        const saltB64 = btoa(String.fromCharCode(...salt));

        if (grxfirmaExt.hasStorageSession()) {
            await grxfirmaExt.storageSession.set({ grx_crypto_salt: saltB64 });
        }

        this._derivedKeyCache = key;
        return key;
    },

    async encrypt(text) {
        const key = await this.getSecretKey();
        const iv = crypto.getRandomValues(new Uint8Array(this.IV_LEN));
        const encoded = new TextEncoder().encode(text);
        const encrypted = await crypto.subtle.encrypt({ name: this.AES_ALGO, iv }, key, encoded);
        const combined = new Uint8Array(iv.length + encrypted.byteLength);
        combined.set(iv);
        combined.set(new Uint8Array(encrypted), iv.length);
        return btoa(String.fromCharCode(...combined));
    },

    async decrypt(encryptedB64) {
        try {
            const key = await this.getSecretKey();
            const combined = Uint8Array.from(atob(encryptedB64), c => c.charCodeAt(0));
            const iv = combined.slice(0, this.IV_LEN);
            const data = combined.slice(this.IV_LEN);
            const decrypted = await crypto.subtle.decrypt({ name: this.AES_ALGO, iv }, key, data);
            return new TextDecoder().decode(decrypted);
        } catch (e) {
            console.error("Decryption failed", e);
            return null;
        }
    },

    /**
     * --- ASYMMETRIC (IDENTITY) ---
     */

    // Generate RSA-OAEP KeyPair for Identity
    async generateIdentity() {
        const keyPair = await crypto.subtle.generateKey(
            {
                name: "RSA-OAEP",
                modulusLength: 2048,
                publicExponent: new Uint8Array([1, 0, 1]),
                hash: "SHA-256"
            },
            true,
            ["encrypt", "decrypt"]
        );

        // Export Public Key (SPKI) for sharing
        const exportedPub = await crypto.subtle.exportKey("spki", keyPair.publicKey);
        const pubPem = this.ab2str(exportedPub);

        // Export Private Key (PKCS8) for local storage
        const exportedPriv = await crypto.subtle.exportKey("pkcs8", keyPair.privateKey);
        const privPem = this.ab2str(exportedPriv);

        // H-03: Guardar en session storage si está disponible (se borra al cerrar la extensión).
        // Fallback: cifrar la clave privada con la clave de vault antes de guardar en local.
        if (grxfirmaExt.hasStorageSession()) {
            await grxfirmaExt.storageSession.set({
                grx_identity_pub: pubPem,
                grx_identity_priv: privPem
            });
        } else {
            const encryptedPriv = await this.encrypt(privPem);
            console.warn("[CryptoUtils] chrome.storage.session unavailable — storing identity encrypted with vault key");
            await grxfirmaExt.storageLocal.set({
                grx_identity_pub: pubPem,
                grx_identity_priv_enc: encryptedPriv
            });
        }

        return { pub: pubPem, priv: privPem };
    },

    async getIdentity() {
        if (grxfirmaExt.hasStorageSession()) {
            const sessionIdentity = await grxfirmaExt.storageSession.get(['grx_identity_pub', 'grx_identity_priv']);
            if (sessionIdentity.grx_identity_pub && sessionIdentity.grx_identity_priv) {
                return { pub: sessionIdentity.grx_identity_pub, priv: sessionIdentity.grx_identity_priv };
            }
        }

        const res = await grxfirmaExt.storageLocal.get(['grx_identity_pub', 'grx_identity_priv_enc', 'grx_identity_priv']);
        if (!res.grx_identity_pub) {
            return null;
        }

        let privPem = null;
        if (res.grx_identity_priv_enc) {
            privPem = await this.decrypt(res.grx_identity_priv_enc);
        } else if (res.grx_identity_priv) {
            console.warn("[CryptoUtils] Migrating unencrypted legacy identity from chrome.storage.local");
            privPem = res.grx_identity_priv;
            await grxfirmaExt.storageLocal.remove(['grx_identity_priv']);
        }

        if (!privPem) {
            return null;
        }

        if (grxfirmaExt.hasStorageSession()) {
            await grxfirmaExt.storageSession.set({
                grx_identity_pub: res.grx_identity_pub,
                grx_identity_priv: privPem
            });
            await grxfirmaExt.storageLocal.remove(['grx_identity_pub', 'grx_identity_priv_enc']);
        }

        return { pub: res.grx_identity_pub, priv: privPem };
    },

    async importPublicKey(pem) {
        const binaryDer = this.str2ab(pem);
        return await crypto.subtle.importKey(
            "spki",
            binaryDer,
            { name: "RSA-OAEP", hash: "SHA-256" },
            true,
            ["encrypt"]
        );
    },

    async importPrivateKey(pem) {
        const binaryDer = this.str2ab(pem);
        return await crypto.subtle.importKey(
            "pkcs8",
            binaryDer,
            { name: "RSA-OAEP", hash: "SHA-256" },
            true,
            ["decrypt"]
        );
    },

    /**
     * --- HYBRID ENCRYPTION ---
     * 1. Generate ephemeral AES key.
     * 2. Encrypt data with AES.
     * 3. Encrypt AES key with Recipient's RSA Public Key.
     * 4. Return { encKey, iv, data }
     */
    async encryptHybrid(text, recipientPublicKeyPem) {
        try {
            // 1. Generate Ephemeral AES Key
            const aesKey = await crypto.subtle.generateKey(
                { name: "AES-GCM", length: 256 },
                true,
                ["encrypt"]
            );

            // 2. Encrypt Data with AES
            const iv = crypto.getRandomValues(new Uint8Array(12));
            const encodedData = new TextEncoder().encode(text);
            const encryptedData = await crypto.subtle.encrypt(
                { name: "AES-GCM", iv: iv },
                aesKey,
                encodedData
            );

            // 3. Encrypt AES Key with RSA
            const recipientKey = await this.importPublicKey(recipientPublicKeyPem);
            const rawAesKey = await crypto.subtle.exportKey("raw", aesKey);
            const encryptedKey = await crypto.subtle.encrypt(
                { name: "RSA-OAEP" },
                recipientKey,
                rawAesKey
            );

            // 4. Pack
            return {
                iv: this.ab2str(iv),
                key: this.ab2str(encryptedKey),
                data: this.ab2str(encryptedData)
            };
        } catch (e) {
            console.error("Hybrid Encryption Failed", e);
            throw e;
        }
    },

    async decryptHybrid(packet, privateKeyPem) {
        try {
            // 1. Decrypt AES Key with RSA
            const privateKey = await this.importPrivateKey(privateKeyPem);
            const rawEncryptedKey = this.str2ab(packet.key);
            const rawAesKey = await crypto.subtle.decrypt(
                { name: "RSA-OAEP" },
                privateKey,
                rawEncryptedKey
            );

            // 2. Import AES Key
            const aesKey = await crypto.subtle.importKey(
                "raw",
                rawAesKey,
                "AES-GCM",
                true,
                ["decrypt"]
            );

            // 3. Decrypt Data
            const iv = this.str2ab(packet.iv);
            const rawData = this.str2ab(packet.data);
            const decryptedData = await crypto.subtle.decrypt(
                { name: "AES-GCM", iv: iv },
                aesKey,
                rawData
            );

            return new TextDecoder().decode(decryptedData);
        } catch (e) {
            console.error("Hybrid Decryption Failed", e);
            return null;
        }
    },

    /**
     * UTILS (Base64 <-> ArrayBuffer)
     */
    ab2str(buf) {
        return btoa(String.fromCharCode(...new Uint8Array(buf)));
    },

    str2ab(str) {
        const binary_string = atob(str);
        const len = binary_string.length;
        const bytes = new Uint8Array(len);
        for (let i = 0; i < len; i++) {
            bytes[i] = binary_string.charCodeAt(i);
        }
        return bytes.buffer;
    },

    // --- Backup Utils (Legacy/Keep) ---
    async deriveKeyFromPassword(password, salt) {
        const encoder = new TextEncoder();
        const baseKey = await crypto.subtle.importKey(
            "raw",
            encoder.encode(password),
            "PBKDF2",
            false,
            ["deriveKey"]
        );

        return await crypto.subtle.deriveKey(
            {
                name: "PBKDF2",
                salt: salt,
                iterations: 600000,
                hash: "SHA-256"
            },
            baseKey,
            { name: "AES-GCM", length: 256 },
            true,
            ["encrypt", "decrypt"]
        );
    },

    async encryptWithPassword(text, password) {
        const salt = crypto.getRandomValues(new Uint8Array(16));
        const iv = crypto.getRandomValues(new Uint8Array(this.IV_LEN));
        const key = await this.deriveKeyFromPassword(password, salt);
        const encoded = new TextEncoder().encode(text);

        const encrypted = await crypto.subtle.encrypt(
            { name: "AES-GCM", iv },
            key,
            encoded
        );

        const combined = new Uint8Array(salt.length + iv.length + encrypted.byteLength);
        combined.set(salt);
        combined.set(iv, salt.length);
        combined.set(new Uint8Array(encrypted), salt.length + iv.length);

        return btoa(String.fromCharCode(...combined));
    },

    async decryptWithPassword(encryptedB64, password) {
        try {
            const combined = Uint8Array.from(atob(encryptedB64), c => c.charCodeAt(0));
            const salt = combined.slice(0, 16);
            const iv = combined.slice(16, 16 + this.IV_LEN);
            const data = combined.slice(16 + this.IV_LEN);

            const key = await this.deriveKeyFromPassword(password, salt);
            const decrypted = await crypto.subtle.decrypt(
                { name: "AES-GCM", iv },
                key,
                data
            );

            return new TextDecoder().decode(decrypted);
        } catch (e) {
            console.error("Password decryption failed (wrong password?)", e);
            return null;
        }
    }
};
