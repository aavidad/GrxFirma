// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// Session Synchronization Script for GrxGo Portal
// Runs on https://portal.example/*

(function () {
    console.log("[GrxGo Sync] Session sync script active");

    function syncSession() {
        console.log("[GrxGo Sync] Scanning localStorage for session keys...");

        let sessionId = null;
        let username = null;

        // Exhaustive search: find any key that looks like a JWT or long token
        for (let i = 0; i < localStorage.length; i++) {
            const key = localStorage.key(i);
            const val = localStorage.getItem(key);

            if (!val) continue;

            // Log key found (with value masked for security)
            console.log(`[GrxGo Sync] Found key: ${key} (length: ${val.length})`);

            // If it's a known session key or looks like a token (> 30 chars, alphanumeric/symbols)
            // T112: solo claves de sesion conocidas o JWT bien formados. El
            // heuristico generico anterior (cualquier valor >20 chars sin
            // espacios) podia capturar valores arbitrarios del localStorage.
            if (['grx_session_id', 'session_id', 'token', 'auth_token', 'jwt'].includes(key.toLowerCase()) ||
                (val.length > 30 && /^[a-zA-Z0-9\-_=]+\.[a-zA-Z0-9\-_=]+\.[a-zA-Z0-9\-_=]+$/.test(val))) { // JWT

                if (!sessionId) {
                    console.log(`[GrxGo Sync] => Potential Session ID found in key: ${key}`);
                    sessionId = val;
                }
            }

            if (['grx_username', 'username', 'user_id', 'user'].includes(key.toLowerCase())) {
                if (!username) {
                    console.log(`[GrxGo Sync] => Potential Username found in key: ${key}: ${val}`);
                    username = val;
                }
            }
        }

        if (sessionId) {
            console.log("[GrxGo Sync] Attempting to sync session with background...");
            grxfirmaExt.runtime.sendMessage({
                type: "SYNC_SESSION",
                session_id: sessionId,
                username: username
            }).then((response) => {
                if (response && response.status === "ok") {
                    console.log("[GrxGo Sync] Sync successful!");
                }
            }).catch((error) => {
                console.error("[GrxGo Sync] Message error:", error);
            });
        } else {
            console.warn("[GrxGo Sync] No session token found in localStorage.");
        }
    }

    // Run on load
    syncSession();

    // Also listen for storage changes
    window.addEventListener('storage', (e) => {
        console.log(`[GrxGo Sync] Storage change detected: ${e.key}`);
        syncSession();
    });

    // Periodically sync
    setInterval(syncSession, 30000);
})();
