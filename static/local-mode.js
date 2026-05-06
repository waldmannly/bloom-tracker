(function () {
    'use strict';

    // ─── Constants ──────────────────────────────────────────────────────────
    var STORAGE_KEY = 'bloom-local-encrypted-v1';
    var SETTINGS_KEY = 'bloom-local-settings-v1';
    var LEGACY_STORAGE_KEY = 'bloom-local-v1';
    var PBKDF2_ITERATIONS = 210000;
    var ENCRYPTED_BACKUP_HEADER = 'BLOOM-ENC-BACKUP-V1';

    // ─── Session State ──────────────────────────────────────────────────────
    var currentState = null;
    var cryptoSession = null;
    var autoLockTimer = null;

    // ─── Settings (stored unencrypted — no sensitive data) ──────────────────
    function loadSettings() {
        try {
            return JSON.parse(localStorage.getItem(SETTINGS_KEY) || '{}');
        } catch (e) {
            return {};
        }
    }

    function saveSettings(settings) {
        localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
    }

    function getAutoLockMinutes() {
        var s = loadSettings();
        if (s.autoLockMinutes === 0) return 0;
        return s.autoLockMinutes || 5;
    }

    // ─── Utility ────────────────────────────────────────────────────────────
    function todayString() {
        return new Date().toISOString().slice(0, 10);
    }

    function defaultState() {
        return {
            profile: { cycleLength: 28, periodLength: 5 },
            periods: [],
            symptoms: [],
            journal: [],
            updatedAt: new Date().toISOString()
        };
    }

    function escapeHtml(value) {
        return String(value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    // ─── Status Messages ────────────────────────────────────────────────────
    function statusEl() {
        return document.getElementById('local-crypto-status');
    }

    function showStatus(message, isError) {
        var el = statusEl();
        if (!el) return;
        el.textContent = message;
        el.className = isError ? 'security-error' : 'security-ok';
    }

    function clearStatus() {
        var el = statusEl();
        if (!el) return;
        el.textContent = '';
        el.className = 'form-hint';
    }

    // ─── Base64 Helpers ─────────────────────────────────────────────────────
    function toBase64(bytes) {
        var str = '';
        for (var i = 0; i < bytes.length; i++) {
            str += String.fromCharCode(bytes[i]);
        }
        return btoa(str);
    }

    function fromBase64(value) {
        var binary = atob(value);
        var out = new Uint8Array(binary.length);
        for (var i = 0; i < binary.length; i++) {
            out[i] = binary.charCodeAt(i);
        }
        return out;
    }

    // ─── Envelope (encrypted localStorage blob) ─────────────────────────────
    function loadEnvelope() {
        var raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return null;
        try {
            var env = JSON.parse(raw);
            if (!env || !env.salt || !env.iv || !env.ciphertext) return null;
            return env;
        } catch (e) {
            return null;
        }
    }

    function saveEnvelope(envelope) {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(envelope));
    }

    // ─── UI Lock State ──────────────────────────────────────────────────────
    function setLockState(unlocked) {
        var setupForm = document.getElementById('local-setup-form');
        var unlockForm = document.getElementById('local-unlock-form');
        var app = document.getElementById('local-app-content');
        var actions = document.getElementById('local-security-actions');
        if (!setupForm || !unlockForm || !app || !actions) return;

        if (unlocked) {
            setupForm.style.display = 'none';
            unlockForm.style.display = 'none';
            app.style.display = 'block';
            actions.style.display = 'flex';
            resetAutoLock();
            return;
        }

        app.style.display = 'none';
        actions.style.display = 'none';
        clearAutoLock();
        if (loadEnvelope()) {
            setupForm.style.display = 'none';
            unlockForm.style.display = 'block';
        } else {
            setupForm.style.display = 'block';
            unlockForm.style.display = 'none';
        }
    }

    // ─── Auto-Lock Timer ────────────────────────────────────────────────────
    function clearAutoLock() {
        if (autoLockTimer) {
            clearTimeout(autoLockTimer);
            autoLockTimer = null;
        }
    }

    function resetAutoLock() {
        clearAutoLock();
        var minutes = getAutoLockMinutes();
        if (minutes <= 0) return;
        autoLockTimer = setTimeout(function () {
            if (currentState) {
                lockData();
                showStatus('Auto-locked after ' + minutes + ' min of inactivity.', false);
            }
        }, minutes * 60 * 1000);
    }

    function onUserActivity() {
        if (currentState) {
            resetAutoLock();
        }
    }

    // ─── Web Crypto ─────────────────────────────────────────────────────────
    function subtle() {
        return window.crypto && window.crypto.subtle;
    }

    function randomBytes(length) {
        var bytes = new Uint8Array(length);
        window.crypto.getRandomValues(bytes);
        return bytes;
    }

    function deriveKey(passphrase, saltBytes, iterations) {
        var enc = new TextEncoder();
        return subtle().importKey(
            'raw', enc.encode(passphrase), 'PBKDF2', false, ['deriveKey']
        ).then(function (material) {
            return subtle().deriveKey(
                { name: 'PBKDF2', salt: saltBytes, iterations: iterations, hash: 'SHA-256' },
                material,
                { name: 'AES-GCM', length: 256 },
                false,
                ['encrypt', 'decrypt']
            );
        });
    }

    function encryptData(key, saltBytes, iterations, plaintext) {
        var ivBytes = randomBytes(12);
        var enc = new TextEncoder();
        return subtle().encrypt(
            { name: 'AES-GCM', iv: ivBytes }, key, enc.encode(plaintext)
        ).then(function (cipherBuffer) {
            return {
                v: 1,
                kdf: 'PBKDF2-SHA256',
                iterations: iterations,
                salt: toBase64(saltBytes),
                iv: toBase64(ivBytes),
                ciphertext: toBase64(new Uint8Array(cipherBuffer))
            };
        });
    }

    function decryptData(envelope, passphrase) {
        var saltBytes = fromBase64(envelope.salt);
        var ivBytes = fromBase64(envelope.iv);
        var cipherBytes = fromBase64(envelope.ciphertext);
        var iterations = Number(envelope.iterations) || PBKDF2_ITERATIONS;
        var dec = new TextDecoder();

        return deriveKey(passphrase, saltBytes, iterations).then(function (key) {
            return subtle().decrypt(
                { name: 'AES-GCM', iv: ivBytes }, key, cipherBytes
            ).then(function (plainBuffer) {
                return { plaintext: dec.decode(plainBuffer), key: key, salt: saltBytes, iterations: iterations };
            });
        });
    }

    // ─── Encrypt/Decrypt State ──────────────────────────────────────────────
    function encryptState(state) {
        if (!cryptoSession) return Promise.reject(new Error('No session'));
        state.updatedAt = new Date().toISOString();
        return encryptData(cryptoSession.key, cryptoSession.salt, cryptoSession.iterations, JSON.stringify(state))
            .then(function (env) {
                env.updatedAt = state.updatedAt;
                return env;
            });
    }

    function decryptEnvelope(envelope, passphrase) {
        return decryptData(envelope, passphrase).then(function (result) {
            var parsed = JSON.parse(result.plaintext);
            if (!parsed.profile || !Array.isArray(parsed.periods) || !Array.isArray(parsed.symptoms) || !Array.isArray(parsed.journal)) {
                throw new Error('Invalid payload');
            }
            cryptoSession = { key: result.key, salt: result.salt, iterations: result.iterations };
            return parsed;
        });
    }

    function persistState() {
        if (!currentState) return;
        encryptState(currentState).then(function (envelope) {
            saveEnvelope(envelope);
        }).catch(function () {
            showStatus('Could not save encrypted data.', true);
        });
    }

    function lockData() {
        currentState = null;
        cryptoSession = null;
        setLockState(false);
    }

    // ─── Encrypted Backup Export ────────────────────────────────────────────
    function exportEncryptedBackup() {
        if (!currentState) return;

        var pass = prompt('Enter a passphrase to protect this backup file:\n(Can be different from your device passphrase, min 8 chars)');
        if (!pass || pass.length < 8) {
            alert('Backup passphrase must be at least 8 characters.');
            return;
        }

        var saltBytes = randomBytes(16);
        deriveKey(pass, saltBytes, PBKDF2_ITERATIONS).then(function (key) {
            return encryptData(key, saltBytes, PBKDF2_ITERATIONS, JSON.stringify(currentState));
        }).then(function (envelope) {
            envelope.header = ENCRYPTED_BACKUP_HEADER;
            envelope.exportedAt = new Date().toISOString();
            var blob = new Blob([JSON.stringify(envelope, null, 2)], { type: 'application/json' });
            var url = URL.createObjectURL(blob);
            var link = document.createElement('a');
            link.href = url;
            link.download = 'bloom-encrypted-backup-' + todayString() + '.json';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            URL.revokeObjectURL(url);
            showStatus('Encrypted backup exported successfully.', false);
        }).catch(function () {
            alert('Failed to create encrypted backup.');
        });
    }

    // ─── Encrypted Backup Import ────────────────────────────────────────────
    function importEncryptedBackup(file) {
        var reader = new FileReader();
        reader.onload = function () {
            try {
                var envelope = JSON.parse(String(reader.result || '{}'));

                // Detect plain JSON backup (unencrypted export)
                if (envelope.profile && Array.isArray(envelope.periods)) {
                    currentState.profile = envelope.profile;
                    currentState.periods = envelope.periods || [];
                    currentState.symptoms = envelope.symptoms || [];
                    currentState.journal = envelope.journal || [];
                    persistState();
                    syncInputsFromState();
                    renderAll(currentState);
                    showStatus('Plain backup imported successfully.', false);
                    return;
                }

                if (!envelope.salt || !envelope.ciphertext) {
                    alert('Invalid backup file format.');
                    return;
                }

                var pass = prompt('Enter the passphrase used when exporting this backup:');
                if (!pass) return;

                decryptData(envelope, pass).then(function (result) {
                    var imported = JSON.parse(result.plaintext);
                    if (!imported.profile || !Array.isArray(imported.periods)) {
                        alert('Decrypted data is invalid.');
                        return;
                    }
                    currentState.profile = imported.profile;
                    currentState.periods = imported.periods || [];
                    currentState.symptoms = imported.symptoms || [];
                    currentState.journal = imported.journal || [];
                    persistState();
                    syncInputsFromState();
                    renderAll(currentState);
                    showStatus('Encrypted backup imported successfully.', false);
                }).catch(function () {
                    alert('Wrong passphrase or corrupted backup file.');
                });
            } catch (err) {
                alert('Could not read backup file.');
            }
        };
        reader.readAsText(file);
    }

    // ─── Passphrase Change ──────────────────────────────────────────────────
    function changePassphrase() {
        if (!currentState || !cryptoSession) {
            alert('Unlock your data first.');
            return;
        }

        var newPass = prompt('Enter your NEW passphrase (min 8 characters):');
        if (!newPass || newPass.length < 8) {
            alert('Passphrase must be at least 8 characters.');
            return;
        }
        var confirmPass = prompt('Confirm your new passphrase:');
        if (newPass !== confirmPass) {
            alert('Passphrases do not match.');
            return;
        }

        var newSalt = randomBytes(16);
        deriveKey(newPass, newSalt, PBKDF2_ITERATIONS).then(function (newKey) {
            cryptoSession = { key: newKey, salt: newSalt, iterations: PBKDF2_ITERATIONS };
            return encryptState(currentState);
        }).then(function (envelope) {
            saveEnvelope(envelope);
            showStatus('Passphrase changed! Data re-encrypted with new passphrase.', false);
        }).catch(function () {
            alert('Failed to change passphrase. Old passphrase still active.');
        });
    }

    // ─── Cycle Calculations ─────────────────────────────────────────────────
    function calculateCycleSummary(state) {
        var periods = (state.periods || []).slice().sort(function (a, b) {
            return a.date < b.date ? 1 : -1;
        });
        if (periods.length === 0) {
            return { phase: 'No data yet', nextPeriod: 'Log your first period', dayInCycle: '—' };
        }

        var lastDate = new Date(periods[0].date + 'T00:00:00');
        var cycleLength = state.profile.cycleLength || 28;
        var periodLength = state.profile.periodLength || 5;
        var now = new Date();
        var daysSince = Math.floor((now - lastDate) / (1000 * 60 * 60 * 24));
        if (daysSince < 0) daysSince = 0;
        var dayInCycle = (daysSince % cycleLength) + 1;

        var phase = 'Luteal';
        if (dayInCycle <= periodLength) phase = 'Menstruation';
        else if (dayInCycle <= Math.max(periodLength + 7, 12)) phase = 'Follicular';
        else if (Math.abs(dayInCycle - (cycleLength - 14)) <= 2) phase = 'Ovulation';

        var nextPeriodDate = new Date(lastDate);
        nextPeriodDate.setDate(nextPeriodDate.getDate() + cycleLength);

        return {
            phase: phase,
            nextPeriod: nextPeriodDate.toISOString().slice(0, 10),
            dayInCycle: dayInCycle
        };
    }

    // ─── Rendering ──────────────────────────────────────────────────────────
    function renderSummary(state) {
        var el = document.getElementById('local-summary');
        if (!el) return;
        var s = calculateCycleSummary(state);
        el.innerHTML =
            '<div class="stats-grid">' +
            '<div class="stat-card"><div class="stat-number">' + s.dayInCycle + '</div><div class="stat-label">Cycle Day</div></div>' +
            '<div class="stat-card"><div class="stat-number">' + s.phase + '</div><div class="stat-label">Phase</div></div>' +
            '<div class="stat-card"><div class="stat-number">' + s.nextPeriod + '</div><div class="stat-label">Next Period</div></div>' +
            '<div class="stat-card"><div class="stat-number">' + state.periods.length + '</div><div class="stat-label">Logged</div></div>' +
            '</div>';
    }

    function renderList(elId, items, formatter) {
        var el = document.getElementById(elId);
        if (!el) return;
        if (!items || items.length === 0) {
            el.innerHTML = '<p class="card-desc">No entries yet.</p>';
            return;
        }
        el.innerHTML = '<div class="symptom-list">' + items.slice(0, 10).map(formatter).join('') + '</div>';
    }

    function renderAll(state) {
        renderSummary(state);

        renderList('local-period-list', state.periods.slice().sort(function (a, b) {
            return a.date < b.date ? 1 : -1;
        }), function (p) {
            var notes = p.notes ? '<div class="symptom-notes">' + escapeHtml(p.notes) + '</div>' : '';
            return '<div class="symptom-row"><div class="symptom-info"><strong>' + p.date + '</strong>' + notes + '</div></div>';
        });

        renderList('local-symptom-list', state.symptoms.slice().sort(function (a, b) {
            return a.date < b.date ? 1 : -1;
        }), function (s) {
            var notes = s.notes ? '<div class="symptom-notes">' + escapeHtml(s.notes) + '</div>' : '';
            return '<div class="symptom-row"><div class="symptom-info"><span class="pill-tag">' + escapeHtml(s.category) + '</span> <strong>' + escapeHtml(s.name) + '</strong> <span>Sev ' + s.severity + '</span> <span class="symptom-date">' + s.date + '</span>' + notes + '</div></div>';
        });

        renderList('local-journal-list', state.journal.slice().sort(function (a, b) {
            return a.date < b.date ? 1 : -1;
        }), function (j) {
            var title = j.title ? '<strong>' + escapeHtml(j.title) + '</strong>' : '<strong>Entry</strong>';
            var mood = j.mood ? ' ' + escapeHtml(j.mood) : '';
            return '<div class="symptom-row"><div class="symptom-info">' + title + mood + ' <span class="symptom-date">' + j.date + '</span><div class="symptom-notes">' + escapeHtml(j.content) + '</div></div></div>';
        });

        renderSettingsPanel();
    }

    // ─── Settings Panel ─────────────────────────────────────────────────────
    function renderSettingsPanel() {
        var el = document.getElementById('local-settings-content');
        if (!el) return;
        var settings = loadSettings();
        var autoLock = settings.autoLockMinutes !== undefined ? settings.autoLockMinutes : 5;

        el.innerHTML =
            '<div class="form-stack">' +
            '  <div class="form-group">' +
            '    <label for="setting-autolock">Auto-lock after inactivity</label>' +
            '    <select id="setting-autolock">' +
            '      <option value="0"' + (autoLock === 0 ? ' selected' : '') + '>Never (stay unlocked)</option>' +
            '      <option value="2"' + (autoLock === 2 ? ' selected' : '') + '>2 minutes</option>' +
            '      <option value="5"' + (autoLock === 5 ? ' selected' : '') + '>5 minutes (recommended)</option>' +
            '      <option value="10"' + (autoLock === 10 ? ' selected' : '') + '>10 minutes</option>' +
            '      <option value="30"' + (autoLock === 30 ? ' selected' : '') + '>30 minutes</option>' +
            '    </select>' +
            '  </div>' +
            '  <div class="form-group">' +
            '    <label>Storage Mode</label>' +
            '    <p class="form-hint" style="margin:0.25rem 0">Currently: <strong>Local-only (offline, encrypted on device)</strong></p>' +
            '    <p class="form-hint" style="margin:0.25rem 0">Want cross-device sync? <a href="/register">Create a cloud account</a>, then export your data here and import it on the server version.</p>' +
            '  </div>' +
            '</div>';

        document.getElementById('setting-autolock').addEventListener('change', function () {
            var val = Number(this.value);
            var s = loadSettings();
            s.autoLockMinutes = val;
            saveSettings(s);
            resetAutoLock();
            showStatus('Auto-lock: ' + (val === 0 ? 'disabled' : val + ' min') + '.', false);
        });
    }

    function syncInputsFromState() {
        if (!currentState || !currentState.profile) return;
        var cl = document.getElementById('local-cycle-length');
        var pl = document.getElementById('local-period-length');
        if (cl) cl.value = currentState.profile.cycleLength || 28;
        if (pl) pl.value = currentState.profile.periodLength || 5;
    }

    // ─── Bind Forms ─────────────────────────────────────────────────────────
    function bindForms() {
        var state = currentState;
        if (!state) return;

        var cycleLength = document.getElementById('local-cycle-length');
        var periodLength = document.getElementById('local-period-length');
        cycleLength.value = state.profile.cycleLength || 28;
        periodLength.value = state.profile.periodLength || 5;

        var periodDate = document.getElementById('local-period-date');
        var symptomDate = document.getElementById('local-symptom-date');
        var journalDate = document.getElementById('local-journal-date');
        periodDate.value = todayString();
        symptomDate.value = todayString();
        journalDate.value = todayString();

        document.getElementById('local-profile-form').addEventListener('submit', function (e) {
            e.preventDefault();
            currentState.profile.cycleLength = Number(cycleLength.value) || 28;
            currentState.profile.periodLength = Number(periodLength.value) || 5;
            persistState();
            renderAll(currentState);
            showStatus('Settings saved.', false);
        });

        document.getElementById('local-period-form').addEventListener('submit', function (e) {
            e.preventDefault();
            currentState.periods.push({
                date: document.getElementById('local-period-date').value,
                notes: document.getElementById('local-period-notes').value.trim()
            });
            persistState();
            renderAll(currentState);
            e.target.reset();
            document.getElementById('local-period-date').value = todayString();
        });

        document.getElementById('local-symptom-form').addEventListener('submit', function (e) {
            e.preventDefault();
            currentState.symptoms.push({
                date: document.getElementById('local-symptom-date').value,
                category: document.getElementById('local-symptom-category').value,
                name: document.getElementById('local-symptom-name').value.trim(),
                severity: Number(document.getElementById('local-symptom-severity').value) || 3,
                notes: document.getElementById('local-symptom-notes').value.trim()
            });
            persistState();
            renderAll(currentState);
            e.target.reset();
            document.getElementById('local-symptom-date').value = todayString();
            document.getElementById('local-symptom-severity').value = 3;
        });

        document.getElementById('local-journal-form').addEventListener('submit', function (e) {
            e.preventDefault();
            currentState.journal.push({
                date: document.getElementById('local-journal-date').value,
                mood: document.getElementById('local-journal-mood').value.trim(),
                title: document.getElementById('local-journal-title').value.trim(),
                content: document.getElementById('local-journal-content').value.trim()
            });
            persistState();
            renderAll(currentState);
            e.target.reset();
            document.getElementById('local-journal-date').value = todayString();
        });

        // Backup actions
        document.getElementById('local-export-btn').addEventListener('click', function () {
            if (!currentState) return;
            var blob = new Blob([JSON.stringify(currentState, null, 2)], { type: 'application/json' });
            var url = URL.createObjectURL(blob);
            var link = document.createElement('a');
            link.href = url;
            link.download = 'bloom-backup-' + todayString() + '.json';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
            URL.revokeObjectURL(url);
            showStatus('Plain backup exported.', false);
        });

        document.getElementById('local-export-encrypted-btn').addEventListener('click', exportEncryptedBackup);

        document.getElementById('local-import-input').addEventListener('change', function (e) {
            var file = e.target.files && e.target.files[0];
            if (!file) return;
            importEncryptedBackup(file);
            e.target.value = '';
        });

        document.getElementById('local-clear-btn').addEventListener('click', function () {
            if (!window.confirm('Delete ALL local Bloom data?\n\nThis cannot be undone. Export a backup first.')) return;
            localStorage.removeItem(STORAGE_KEY);
            localStorage.removeItem(SETTINGS_KEY);
            localStorage.removeItem(LEGACY_STORAGE_KEY);
            location.reload();
        });

        // Security actions
        document.getElementById('local-change-passphrase-btn').addEventListener('click', changePassphrase);
    }

    var formsInitialized = false;
    function initializeFormsOnce() {
        if (formsInitialized) return;
        bindForms();
        formsInitialized = true;
    }

    // ─── Unlock Flow ────────────────────────────────────────────────────────
    function unlockWithState(state) {
        currentState = state;
        setLockState(true);
        clearStatus();
        initializeFormsOnce();
        syncInputsFromState();
        renderAll(currentState);
    }

    function handleSetupSubmit(e) {
        e.preventDefault();
        var pass = document.getElementById('local-passphrase').value;
        var confirm = document.getElementById('local-passphrase-confirm').value;

        if (pass.length < 8) {
            showStatus('Passphrase must be at least 8 characters.', true);
            return;
        }
        if (pass !== confirm) {
            showStatus('Passphrases do not match.', true);
            return;
        }

        var saltBytes = randomBytes(16);
        deriveKey(pass, saltBytes, PBKDF2_ITERATIONS).then(function (key) {
            cryptoSession = { key: key, salt: saltBytes, iterations: PBKDF2_ITERATIONS };

            // Auto-migrate legacy unencrypted data
            var legacy = null;
            try {
                var raw = localStorage.getItem(LEGACY_STORAGE_KEY);
                if (raw) legacy = JSON.parse(raw);
            } catch (e) { /* ignore */ }

            if (legacy && legacy.profile) {
                currentState = legacy;
            } else {
                currentState = defaultState();
            }

            return encryptState(currentState);
        }).then(function (envelope) {
            saveEnvelope(envelope);
            localStorage.removeItem(LEGACY_STORAGE_KEY);
            unlockWithState(currentState);
            showStatus('Encryption enabled! Data is now protected.', false);
            document.getElementById('local-setup-form').reset();
        }).catch(function () {
            showStatus('Could not initialize encryption.', true);
        });
    }

    function handleUnlockSubmit(e) {
        e.preventDefault();
        var pass = document.getElementById('local-unlock-passphrase').value;
        var envelope = loadEnvelope();
        if (!envelope) { setLockState(false); return; }

        decryptEnvelope(envelope, pass).then(function (state) {
            unlockWithState(state);
            showStatus('Unlocked. Data encrypted at rest.', false);
            document.getElementById('local-unlock-form').reset();
        }).catch(function () {
            showStatus('Wrong passphrase.', true);
        });
    }

    // ─── Bind Security Panel ────────────────────────────────────────────────
    function bindSecurityActions() {
        document.getElementById('local-setup-form').addEventListener('submit', handleSetupSubmit);
        document.getElementById('local-unlock-form').addEventListener('submit', handleUnlockSubmit);
        document.getElementById('local-lock-btn').addEventListener('click', function () {
            lockData();
            showStatus('Locked.', false);
        });
    }

    // ─── Activity Listeners for Auto-Lock ───────────────────────────────────
    function bindActivityListeners() {
        ['click', 'keydown', 'touchstart', 'scroll'].forEach(function (evt) {
            document.addEventListener(evt, onUserActivity, { passive: true });
        });
    }

    // ─── Init ───────────────────────────────────────────────────────────────
    document.addEventListener('DOMContentLoaded', function () {
        if (!subtle()) {
            showStatus('Browser does not support Web Crypto.', true);
            setLockState(false);
            return;
        }

        bindSecurityActions();
        bindActivityListeners();

        setLockState(false);
        if (loadEnvelope()) {
            showStatus('Enter passphrase to unlock.', false);
        } else if (localStorage.getItem(LEGACY_STORAGE_KEY)) {
            showStatus('Unencrypted data found — create a passphrase to encrypt it.', false);
            document.getElementById('local-setup-form').style.display = 'block';
        } else {
            showStatus('Create a passphrase to start tracking securely.', false);
        }
    });
})();
