// Derechos de autor (C) 2026 Alberto Avidad Fernández.
// Autoría: Alberto Avidad Fernández
// Licencia: EUPL 1.2 o posterior
// SPDX-License-Identifier: EUPL-1.2

// GrxGo Auto-Login Script

function hostMatches(host, expectedDomain) {
    const normalizedHost = (host || "").toLowerCase();
    const normalizedDomain = (expectedDomain || "").toLowerCase();
    return normalizedHost === normalizedDomain || normalizedHost.endsWith(`.${normalizedDomain}`);
}

(async function () {
    // Query Background for credentials matching this domain
    try {
        const response = await grxfirmaExt.runtime.sendMessage({ type: "GET_CREDENTIALS" });

        if (response && response.creds) {
            console.log("[GrxGo] Credentials received from extension background");
            const creds = response.creds;

            const host = window.location.hostname;
            const path = window.location.pathname;

            if (hostMatches(host, "cronos.example")) {
                handleCronos(creds);
            } else if (hostMatches(host, "savia.net")) {
                handleSavia(creds);
            } else if (hostMatches(host, "incidencias.example")) {
                handleGLPI(creds);
            } else if (hostMatches(host, "correoweb.example")) {
                handleOWA(creds);
            } else {
                // Default to generic handler for vault-matched sites
                console.log("[GrxGo] No specific handler for this host, trying generic...");
                handleGeneric(creds);
            }
        }
    } catch (err) {
        // No handler or error, ignore
    }
})();

function handleCronos(creds) {
    // Need to find the frame with the login form
    // The structure is Frameset -> Frame (principal) -> Frameset -> Frame (cuerpo) -> Frame (validar.php)
    // Content scripts run in ALL frames if configured (need `all_frames: true` in manifest).

    const userField = document.querySelector('input[name="USUARIO"]');
    const passField = document.querySelector('input[name="CONTRASENA"]');
    // Button might be an image or script submission.
    // Form usually has <input type=button onclick=validar()> or submit
    // Let's look for a form

    if (userField && passField) {
        userField.value = creds.user;
        passField.value = creds.pass;
        console.log("[GrxGo] Filled Cronos form");

        // Submit
        // document.forms[0].submit();
        // Or find the button
        // In Validar.php locally?
        // Let's try form submit
        const form = userField.closest("form");
        if (form) {
            setTimeout(() => form.submit(), 500);
        }
    }
}

function handleSavia(creds) {
    // Angular form.
    // Need to find inputs and inject values + trigger events.

    // Wait for inputs
    const interval = setInterval(() => {
        const userField = document.querySelector('input[name="userName"]');
        const passField = document.querySelector('input[name="password"]');
        const submitBtn = document.querySelector('button[type="submit"]');

        if (userField && passField && submitBtn) {
            clearInterval(interval);

            // React/Angular often need input events
            nativeFill(userField, creds.user);
            nativeFill(passField, creds.pass);

            setTimeout(() => submitBtn.click(), 500);
        }
    }, 500);
}

function nativeFill(el, value) {
    if (!el) return;
    el.focus();
    const lastValue = el.value;
    el.value = value;

    // Trigger standard events for modern frameworks
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));

    // Some systems (like OWA) might need key events
    el.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Enter' }));
    el.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true, key: 'Enter' }));

    const tracker = el._valueTracker;
    if (tracker) {
        tracker.setValue(lastValue);
    }
    el.blur();
}

function handleGLPI(creds) {
    // GLPI standard login fields are login_name and login_password
    const interval = setInterval(() => {
        const userField = document.querySelector('input[name="login_name"]');
        const passField = document.querySelector('input[name="login_password"]');
        const submitBtn = document.querySelector('button[name="submit"]');

        if (userField && passField) {
            clearInterval(interval);
            userField.value = creds.user;
            passField.value = creds.pass;
            console.log("[GrxGo] Filled GLPI form");

            // Some GLPI versions use a button, others just the form submit
            if (submitBtn) {
                setTimeout(() => submitBtn.click(), 500);
            } else {
                const form = userField.closest("form");
                if (form) setTimeout(() => form.submit(), 500);
            }
        }
    }, 500);
}

function handleCorreweb(creds) {
    // Roundcube commonly uses _user and _pass
    // Some older systems might use user/pass or rcmloginuser/rcmloginpwd
    const interval = setInterval(() => {
        const userField = document.querySelector('input[name="_user"], input[name="user"], input[name="rcmloginuser"]');
        const passField = document.querySelector('input[name="_pass"], input[name="pass"], input[name="rcmloginpwd"]');
        const submitBtn = document.querySelector('button[type="submit"], input[type="submit"]');

        if (userField && passField) {
            clearInterval(interval);
            userField.value = creds.user;
            passField.value = creds.pass;
            console.log("[GrxGo] Filled Correweb form");

            if (submitBtn) {
                setTimeout(() => submitBtn.click(), 500);
            } else {
                const form = userField.closest("form");
                if (form) setTimeout(() => form.submit(), 500);
            }
        }
    }, 500);
}

function handleOWA(creds) {
    const isLogonPage = window.location.pathname.includes("logon.aspx");
    console.log(`[GrxGo] OWA Autologin: ${isLogonPage ? 'Logon Page detected' : 'Searching fields...'}`);

    const interval = setInterval(() => {
        // Form-Based Authentication (FBA) common names and IDs
        const userField = document.getElementById("username") ||
            document.getElementById("txtUsername") ||
            document.querySelector('input[name="username"]') ||
            document.querySelector('input[id*="user" i]');

        const passField = document.getElementById("password") ||
            document.getElementById("txtPassword") ||
            document.querySelector('input[name="password"]') ||
            document.querySelector('input[type="password"]');

        // OWA button can be a div, input, or button
        const submitBtn = document.querySelector(".signinbutton") ||
            document.getElementById("signinbutton") ||
            document.getElementById("signInBtn") ||
            document.querySelector('input[type="submit"]') ||
            document.querySelector('.signinBtn') ||
            document.querySelector('[role="button"][id*="signIn" i]');

        if (userField && passField) {
            clearInterval(interval);

            let username = creds.user;
            if (!username.includes("@")) {
                username += "@grx";
            }

            console.log(`[GrxGo] OWA: Filling credentials for ${username}...`);

            nativeFill(userField, username);

            // Short delay before password and click to ensure OWA's JS has processed the username
            setTimeout(() => {
                nativeFill(passField, creds.pass);

                if (submitBtn) {
                    setTimeout(() => {
                        console.log("[GrxGo] OWA: Attempting sign-in click...");
                        submitBtn.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
                        submitBtn.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
                        submitBtn.dispatchEvent(new MouseEvent('click', { bubbles: true }));
                        if (typeof submitBtn.click === 'function') submitBtn.click();
                    }, 800);
                } else {
                    const form = userField.closest("form");
                    if (form) {
                        console.log("[GrxGo] OWA: Submitting form...");
                        setTimeout(() => form.submit(), 800);
                    }
                }
            }, 400);
        }
    }, 1000);
}

function handleGeneric(creds) {
    const interval = setInterval(() => {
        // Common usernames
        const userField = document.querySelector('input[name*="user" i], input[id*="user" i], input[name*="usuario" i], input[id*="usuario" i], input[name="u"], input[name="login"]');
        // Common passwords
        const passField = document.querySelector('input[type="password"], input[name*="pass" i], input[id*="pass" i], input[name*="clave" i], input[id*="clave" i], input[name="p"]');
        // Common buttons
        const submitBtn = document.querySelector('button[type="submit"], input[type="submit"], button[id*="login" i], input[id*="login" i], .login-button, .btn-login');

        if (userField && passField) {
            clearInterval(interval);
            console.log("[GrxGo] Generic handler: Found login fields, filling...");

            nativeFill(userField, creds.user);
            setTimeout(() => {
                nativeFill(passField, creds.pass);

                if (submitBtn) {
                    console.log("[GrxGo] Generic handler: Clicking submit button...");
                    setTimeout(() => submitBtn.click(), 500);
                } else {
                    const form = userField.closest("form");
                    if (form) {
                        console.log("[GrxGo] Generic handler: Submitting form...");
                        setTimeout(() => form.submit(), 500);
                    }
                }
            }, 300);
        }
    }, 1000);

    // Stop checking after 20 seconds
    setTimeout(() => clearInterval(interval), 20000);
}

