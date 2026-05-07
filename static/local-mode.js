// Bloom Local Mode — Full offline encrypted period tracker
// AES-256-GCM + PBKDF2-SHA256 (210k iterations)
// Zero server contact after page load

(function() {
'use strict';

// ═══════════════════════════════════════════════════════════════════
// CRYPTO ENGINE
// ═══════════════════════════════════════════════════════════════════

const STORAGE_KEY = 'bloom_vault';
const SALT_KEY = 'bloom_salt';
const PBKDF2_ITERATIONS = 210000;

let cryptoKey = null;
let autoLockTimer = null;
let data = null; // decrypted app data

const defaultData = () => ({
    settings: { cycleLength: 28, periodLength: 5, showFertility: true, autoLock: 5 },
    periods: [],    // [{id, startDate, endDate}]
    symptoms: [],   // [{id, date, category, symptoms[], severity, notes}]
    journal: [],    // [{id, date, mood, title, content}]
});

async function deriveKey(passphrase, salt) {
    const enc = new TextEncoder();
    const keyMaterial = await crypto.subtle.importKey('raw', enc.encode(passphrase), 'PBKDF2', false, ['deriveKey']);
    return crypto.subtle.deriveKey(
        { name: 'PBKDF2', salt, iterations: PBKDF2_ITERATIONS, hash: 'SHA-256' },
        keyMaterial,
        { name: 'AES-GCM', length: 256 },
        false,
        ['encrypt', 'decrypt']
    );
}

async function encrypt(key, plaintext) {
    const iv = crypto.getRandomValues(new Uint8Array(12));
    const enc = new TextEncoder();
    const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, enc.encode(plaintext));
    const buf = new Uint8Array(iv.length + ct.byteLength);
    buf.set(iv);
    buf.set(new Uint8Array(ct), iv.length);
    return btoa(String.fromCharCode(...buf));
}

async function decrypt(key, ciphertext) {
    const raw = Uint8Array.from(atob(ciphertext), c => c.charCodeAt(0));
    const iv = raw.slice(0, 12);
    const ct = raw.slice(12);
    const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ct);
    return new TextDecoder().decode(pt);
}

async function saveData() {
    if (!cryptoKey || !data) return;
    const ct = await encrypt(cryptoKey, JSON.stringify(data));
    localStorage.setItem(STORAGE_KEY, ct);
}

async function loadData(passphrase) {
    const saltHex = localStorage.getItem(SALT_KEY);
    if (!saltHex) throw new Error('No vault found');
    const salt = Uint8Array.from(saltHex.match(/.{2}/g).map(b => parseInt(b, 16)));
    const key = await deriveKey(passphrase, salt);
    const ct = localStorage.getItem(STORAGE_KEY);
    if (!ct) throw new Error('No data');
    const json = await decrypt(key, ct);
    cryptoKey = key;
    data = JSON.parse(json);
    // Migrate old formats
    if (!data.settings) data.settings = defaultData().settings;
    if (!data.periods) data.periods = [];
    if (!data.symptoms) data.symptoms = [];
    if (!data.journal) data.journal = [];
}

async function createVault(passphrase) {
    const salt = crypto.getRandomValues(new Uint8Array(16));
    const saltHex = Array.from(salt).map(b => b.toString(16).padStart(2, '0')).join('');
    localStorage.setItem(SALT_KEY, saltHex);
    cryptoKey = await deriveKey(passphrase, salt);
    data = defaultData();
    await saveData();
}

function hasVault() {
    return !!localStorage.getItem(SALT_KEY) && !!localStorage.getItem(STORAGE_KEY);
}

function lockVault() {
    cryptoKey = null;
    data = null;
    clearTimeout(autoLockTimer);
    document.getElementById('app-main').style.display = 'none';
    document.getElementById('lock-screen').style.display = '';
    document.getElementById('lock-setup').style.display = 'none';
    document.getElementById('lock-unlock').style.display = '';
    document.getElementById('lock-error').textContent = '';
    document.getElementById('unlock-pass').value = '';
}

// ═══════════════════════════════════════════════════════════════════
// AUTO-LOCK
// ═══════════════════════════════════════════════════════════════════

function resetAutoLock() {
    clearTimeout(autoLockTimer);
    if (!data || !data.settings.autoLock) return;
    autoLockTimer = setTimeout(lockVault, data.settings.autoLock * 60000);
}

['click', 'keydown', 'touchstart', 'scroll', 'mousemove'].forEach(e =>
    document.addEventListener(e, resetAutoLock, { passive: true })
);

// ═══════════════════════════════════════════════════════════════════
// CYCLE CALCULATIONS (mirrors server-side cycle.go)
// ═══════════════════════════════════════════════════════════════════

function parseLocalDate(d) {
    if (d instanceof Date) return d;
    if (typeof d === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(d)) {
        const [y, m, day] = d.split('-').map(Number);
        return new Date(y, m - 1, day);
    }
    return new Date(d);
}

function midnight(d) {
    const t = parseLocalDate(d);
    t.setHours(0, 0, 0, 0);
    return t;
}

function daysBetween(a, b) {
    return Math.round((midnight(b) - midnight(a)) / 86400000);
}

function formatDate(d) {
    const dt = parseLocalDate(d);
    return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
}

function formatDateInput(d) {
    const dt = parseLocalDate(d);
    return dt.getFullYear() + '-' + String(dt.getMonth() + 1).padStart(2, '0') + '-' + String(dt.getDate()).padStart(2, '0');
}

function getLastPeriodStart() {
    if (!data || !data.periods.length) return null;
    const sorted = [...data.periods].sort((a, b) => parseLocalDate(b.startDate) - parseLocalDate(a.startDate));
    return sorted[0];
}

function getActivePeriod() {
    return data.periods.find(p => !p.endDate);
}

function calculateCycleInfo() {
    const last = getLastPeriodStart();
    if (!last) return null;

    const cycleLen = data.settings.cycleLength;
    const periodLen = data.settings.periodLength;
    const today = midnight(new Date());
    const lastStart = midnight(last.startDate);

    let daysSince = daysBetween(lastStart, today);
    let currentCycleStart = lastStart;
    if (daysSince >= cycleLen) {
        const completed = Math.floor(daysSince / cycleLen);
        currentCycleStart = new Date(lastStart.getTime() + completed * cycleLen * 86400000);
    }

    const cycleDay = daysBetween(currentCycleStart, today) + 1;
    let ovulationDay = cycleLen - 14;
    if (ovulationDay < periodLen + 1) ovulationDay = periodLen + 1;

    const daysUntilPeriod = cycleLen - cycleDay + 1;
    const nextPeriodDate = new Date(today.getTime() + daysUntilPeriod * 86400000);

    let phase;
    if (cycleDay <= periodLen) phase = 'menstruation';
    else if (cycleDay < ovulationDay - 1) phase = 'follicular';
    else if (cycleDay <= ovulationDay + 1) phase = 'ovulation';
    else phase = 'luteal';

    const fertileStart = new Date(currentCycleStart.getTime() + (ovulationDay - 6) * 86400000);
    const fertileEnd = new Date(currentCycleStart.getTime() + ovulationDay * 86400000);
    const inFertile = today >= fertileStart && today <= fertileEnd;
    const ovulationDate = new Date(currentCycleStart.getTime() + (ovulationDay - 1) * 86400000);

    return { phase, cycleDay, daysUntilPeriod, nextPeriodDate, inFertile, fertileStart, fertileEnd, ovulationDate, currentCycleStart, cycleLen, periodLen, ovulationDay };
}

const phaseData = {
    menstruation: {
        emoji: '🌺', color: 'menstruation',
        desc: 'Your period is here. Take it easy, stay hydrated, and listen to your body.',
        encouragement: "You're doing great. Rest is productive. Your body is working hard right now. 💛",
        exercise: { tip: 'Gentle movement helps with cramps and mood.', examples: ['Walking', 'Yoga', 'Stretching', 'Light swimming'] },
        nutrition: { tip: 'Iron-rich and anti-inflammatory foods.', nutrients: ['Iron', 'Magnesium', 'Omega-3'], foods: ['Spinach', 'Dark chocolate', 'Salmon', 'Ginger tea'] }
    },
    follicular: {
        emoji: '🌱', color: 'follicular',
        desc: 'Energy is rising! Great time for new projects and activities.',
        encouragement: 'Your energy is building — ride the wave! This is your time to shine. ✨',
        exercise: { tip: 'Your body recovers faster now. Push yourself!', examples: ['Strength training', 'HIIT', 'Running', 'Dance'] },
        nutrition: { tip: 'Fuel rising energy with lean proteins and complex carbs.', nutrients: ['B Vitamins', 'Zinc', 'Vitamin E'], foods: ['Eggs', 'Avocado', 'Quinoa', 'Broccoli'] }
    },
    ovulation: {
        emoji: '🌸', color: 'ovulation',
        desc: 'Peak energy and confidence! You may feel more social and creative.',
        encouragement: "You're radiant right now. Confidence suits you! 🌟",
        exercise: { tip: "Peak performance! You're strongest this week.", examples: ['High intensity', 'Spin class', 'Sprint intervals', 'Boxing'] },
        nutrition: { tip: 'Antioxidant-rich whole foods and extra hydration.', nutrients: ['Antioxidants', 'B Vitamins', 'Zinc'], foods: ['Berries', 'Leafy greens', 'Nuts', 'Seeds'] }
    },
    luteal: {
        emoji: '🌙', color: 'luteal',
        desc: 'Winding down. You might crave comfort foods and need more rest.',
        encouragement: "Be gentle with yourself. It's okay to slow down. You deserve softness. 🌙",
        exercise: { tip: 'Lower intensity helps manage PMS symptoms.', examples: ['Pilates', 'Walking', 'Swimming', 'Gentle yoga'] },
        nutrition: { tip: "Complex carbs help serotonin. Don't fight the cravings — work with them.", nutrients: ['Magnesium', 'Calcium', 'B6'], foods: ['Sweet potatoes', 'Whole grains', 'Bananas', 'Dark chocolate'] }
    }
};

// ═══════════════════════════════════════════════════════════════════
// TAB NAVIGATION
// ═══════════════════════════════════════════════════════════════════

function switchTab(tabName) {
    document.querySelectorAll('.local-tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.local-nav-btn').forEach(b => b.classList.remove('active'));
    document.getElementById('tab-' + tabName).classList.add('active');
    document.querySelector(`.local-nav-btn[data-tab="${tabName}"]`).classList.add('active');
    if (tabName === 'dashboard') renderDashboard();
    else if (tabName === 'log-period') renderPeriods();
    else if (tabName === 'symptoms') renderSymptoms();
    else if (tabName === 'journal') renderJournal();
    else if (tabName === 'calendar') renderCalendar();
    else if (tabName === 'trends') renderTrends();
    else if (tabName === 'settings') renderSettings();
}
window.switchTab = switchTab;

// ═══════════════════════════════════════════════════════════════════
// DASHBOARD
// ═══════════════════════════════════════════════════════════════════

function renderDashboard() {
    const info = calculateCycleInfo();
    if (!info) {
        document.getElementById('dash-empty').style.display = '';
        document.getElementById('dash-content').style.display = 'none';
        return;
    }
    document.getElementById('dash-empty').style.display = 'none';
    document.getElementById('dash-content').style.display = '';

    const pd = phaseData[info.phase];
    const greeting = data.settings.displayName ? `Hi ${escHtml(data.settings.displayName)}! ` : '';

    document.getElementById('dash-phase-hero').className = 'phase-hero phase-' + pd.color;
    document.getElementById('dash-phase-hero').innerHTML = `
        <div class="phase-emoji">${pd.emoji}</div>
        <h1>${greeting}${info.phase.charAt(0).toUpperCase() + info.phase.slice(1)} Phase</h1>
        <p class="phase-desc">${pd.desc}</p>`;

    let stats = `
        <div class="stat-card"><div class="stat-number">Day ${info.cycleDay}</div><div class="stat-label">of your cycle</div></div>
        <div class="stat-card"><div class="stat-number">${info.daysUntilPeriod}</div><div class="stat-label">days until period</div></div>`;
    if (data.settings.showFertility) {
        stats += `<div class="stat-card ${info.inFertile ? 'stat-fertile' : ''}"><div class="stat-number">${info.inFertile ? '🌿 Yes' : 'No'}</div><div class="stat-label">fertile window</div></div>`;
    }
    stats += `<div class="stat-card"><div class="stat-number">${formatDate(info.nextPeriodDate)}</div><div class="stat-label">next period</div></div>`;
    document.getElementById('dash-stats').innerHTML = stats;

    // Active period banner
    const active = getActivePeriod();
    document.getElementById('dash-active-period').innerHTML = active ?
        `<div class="banner banner-period"><div class="banner-text"><span class="banner-icon">🩸</span> Period started ${formatDate(active.startDate)}</div><button class="btn btn-sm btn-white" onclick="endPeriod('${active.id}')">Mark as Ended</button></div>` : '';

    document.getElementById('dash-encouragement').className = 'encouragement-card phase-' + pd.color + '-soft';
    document.getElementById('dash-encouragement').innerHTML = `<div class="encouragement-icon">💛</div><p class="encouragement-text">${pd.encouragement}</p>`;

    document.getElementById('dash-wellness').innerHTML = `
        <h2>✨ Your Body This Phase</h2>
        <div class="wellness-grid">
            <div class="wellness-card wellness-exercise">
                <div class="wellness-icon">🏃‍♀️</div><h3>Exercise</h3>
                <p class="wellness-tip">${pd.exercise.tip}</p>
                <div class="wellness-tags">${pd.exercise.examples.map(e => `<span class="wellness-tag exercise-tag">${e}</span>`).join('')}</div>
            </div>
            <div class="wellness-card wellness-nutrition">
                <div class="wellness-icon">🥗</div><h3>Nutrition</h3>
                <p class="wellness-tip">${pd.nutrition.tip}</p>
                <div class="wellness-tags">${pd.nutrition.foods.map(f => `<span class="wellness-tag food-tag">${f}</span>`).join('')}</div>
            </div>
            <div class="wellness-card wellness-nutrients">
                <div class="wellness-icon">💊</div><h3>Key Nutrients</h3>
                <p class="wellness-tip">Focus on these nutrients during this phase:</p>
                <div class="wellness-tags">${pd.nutrition.nutrients.map(n => `<span class="wellness-tag nutrient-tag">${n}</span>`).join('')}</div>
            </div>
        </div>`;

    // Predictions
    const predictions = generatePredictions(info);
    document.getElementById('dash-predictions').innerHTML = predictions.length ? `
        <h2>📅 Upcoming Predictions</h2>
        <div class="predictions-grid">${predictions.map(p => `
            <div class="prediction-card">
                <div class="prediction-header">🩸 Period</div>
                <div class="prediction-dates">${formatDate(p.periodStart)} — ${formatDate(p.periodEnd)}</div>
                ${data.settings.showFertility ? `<div class="prediction-details">
                    <span>🌸 Ovulation: ${formatDate(p.ovulation)}</span>
                    <span>🌿 Fertile: ${formatDate(p.fertileStart)} — ${formatDate(p.fertileEnd)}</span>
                </div>` : ''}
            </div>`).join('')}</div>` : '';

    // Recent symptoms on dashboard
    const recentSyms = [...data.symptoms].sort((a, b) => parseLocalDate(b.date) - parseLocalDate(a.date)).slice(0, 5);
    document.getElementById('dash-recent-symptoms').innerHTML = recentSyms.length ? `
        <h2>📝 Recent Symptoms</h2>
        <div class="symptom-list">${recentSyms.map(s => {
            const syms = s.symptoms || (s.symptom ? [s.symptom] : []);
            const dots = '●'.repeat(s.severity) + '○'.repeat(5 - s.severity);
            return `<div class="symptom-row">
                <span class="symptom-date">${parseLocalDate(s.date).toLocaleDateString('en-US', {month:'short', day:'numeric'})}</span>
                <span class="symptom-badge cat-${s.category}">${s.category}</span>
                <span class="symptom-name">${syms.join(', ')}</span>
                <span class="severity-dots">${dots}</span>
            </div>`;
        }).join('')}</div>` : '';
}

function generatePredictions(info) {
    if (!info) return [];
    const predictions = [];
    const lastPeriod = getLastPeriodStart();
    if (!lastPeriod) return [];
    const baseStart = midnight(lastPeriod.startDate);
    for (let i = 1; i <= 3; i++) {
        const cycleStart = new Date(baseStart.getTime() + i * info.cycleLen * 86400000);
        const periodEnd = new Date(cycleStart.getTime() + (info.periodLen - 1) * 86400000);
        const ovulation = new Date(cycleStart.getTime() + (info.ovulationDay - 1) * 86400000);
        const fertileStart = new Date(cycleStart.getTime() + (info.ovulationDay - 6) * 86400000);
        const fertileEnd = new Date(cycleStart.getTime() + info.ovulationDay * 86400000);
        if (cycleStart > new Date()) {
            predictions.push({ periodStart: cycleStart, periodEnd, ovulation, fertileStart, fertileEnd });
        }
    }
    return predictions;
}

// ═══════════════════════════════════════════════════════════════════
// PERIODS
// ═══════════════════════════════════════════════════════════════════

function renderPeriods() {
    document.getElementById('period-start').value = formatDateInput(new Date());
    const active = getActivePeriod();
    document.getElementById('period-active-banner').innerHTML = active ?
        `<div class="banner banner-period"><div class="banner-text"><span class="banner-icon">🩸</span> Active period started ${formatDate(active.startDate)}</div><button class="btn btn-sm btn-white" onclick="endPeriod('${active.id}')">Mark as Ended</button></div>` : '';

    const sorted = [...data.periods].sort((a, b) => parseLocalDate(b.startDate) - parseLocalDate(a.startDate));
    document.getElementById('period-history').innerHTML = sorted.length === 0 ? '<p class="form-hint">No periods logged yet.</p>' :
        sorted.map(p => `<div class="history-item">
            <div class="history-dates">
                <span class="history-start">${formatDate(p.startDate)}</span>
                ${p.endDate ? `<span class="history-arrow">→</span><span class="history-end">${formatDate(p.endDate)}</span>` : '<span class="history-badge active">Ongoing</span>'}
            </div>
            <div class="history-actions">
                ${!p.endDate ? `<button class="btn btn-xs btn-secondary" onclick="endPeriod('${p.id}')">End</button>` : ''}
                <button class="btn btn-xs btn-danger" onclick="deletePeriod('${p.id}')">✕</button>
            </div>
        </div>`).join('');
}

window.endPeriod = function(id) {
    const p = data.periods.find(x => x.id === id);
    if (p) { p.endDate = formatDateInput(new Date()); saveData(); renderPeriods(); renderDashboard(); }
};

window.deletePeriod = function(id) {
    if (!confirm('Delete this period entry?')) return;
    data.periods = data.periods.filter(x => x.id !== id);
    saveData(); renderPeriods(); renderDashboard();
};

// ═══════════════════════════════════════════════════════════════════
// SYMPTOMS
// ═══════════════════════════════════════════════════════════════════

const symptomSets = {
    physical: ['Cramps', 'Headache', 'Bloating', 'Fatigue', 'Back Pain', 'Breast Tenderness', 'Nausea', 'Acne', 'Dizziness'],
    emotional: ['Mood Swings', 'Irritability', 'Anxiety', 'Sadness', 'Happy', 'Brain Fog', 'Low Motivation'],
    flow: ['Heavy Flow', 'Medium Flow', 'Light Flow', 'Spotting', 'Clots'],
    other: ['Cravings', 'Insomnia', 'Hot Flashes', 'Increased Appetite', 'Low Appetite']
};

let selectedCategory = 'physical';
let selectedSymptoms = [];

function renderSymptomPills() {
    const pills = symptomSets[selectedCategory] || [];
    document.getElementById('sym-pills').innerHTML = `<div class="pill-group">${pills.map(s =>
        `<button type="button" class="pill sym ${selectedSymptoms.includes(s) ? 'active' : ''}" onclick="toggleSym(this, '${s}')">${s}</button>`
    ).join('')}</div>`;
    updateSymSelected();
}

function updateSymSelected() {
    const el = document.getElementById('sym-selected');
    const list = document.getElementById('sym-selected-list');
    if (selectedSymptoms.length) {
        el.style.display = '';
        list.textContent = selectedSymptoms.join(', ');
    } else {
        el.style.display = 'none';
    }
}

window.toggleSym = function(btn, name) {
    const idx = selectedSymptoms.indexOf(name);
    if (idx >= 0) { selectedSymptoms.splice(idx, 1); btn.classList.remove('active'); }
    else { selectedSymptoms.push(name); btn.classList.add('active'); }
    updateSymSelected();
};

function renderSymptoms() {
    document.getElementById('sym-date').value = formatDateInput(new Date());
    renderSymptomPills();

    const sorted = [...data.symptoms].sort((a, b) => parseLocalDate(b.date) - parseLocalDate(a.date)).slice(0, 20);
    document.getElementById('symptom-history').innerHTML = sorted.length === 0 ? '<p class="form-hint">No symptoms logged yet.</p>' :
        sorted.map(s => {
            const syms = s.symptoms || (s.symptom ? [s.symptom] : []);
            return `<div class="history-item">
            <div><strong>${formatDate(s.date)}</strong> — ${syms.join(', ')} <span class="form-hint">(${s.severity}/5)</span></div>
            <button class="btn btn-xs btn-danger" onclick="deleteSymptom('${s.id}')">✕</button>
        </div>`;
        }).join('');
}

window.deleteSymptom = function(id) {
    data.symptoms = data.symptoms.filter(x => x.id !== id);
    saveData(); renderSymptoms();
};

// ═══════════════════════════════════════════════════════════════════
// JOURNAL
// ═══════════════════════════════════════════════════════════════════

let selectedMood = '😐';

function renderJournal() {
    document.getElementById('jrn-date').value = formatDateInput(new Date());

    const sorted = [...data.journal].sort((a, b) => parseLocalDate(b.date) - parseLocalDate(a.date));
    document.getElementById('journal-history').innerHTML = sorted.length === 0 ? '<p class="form-hint">No journal entries yet.</p>' :
        sorted.map(j => `<div class="journal-entry">
            <div class="journal-entry-header">
                <div class="journal-meta">
                    ${j.mood ? `<span class="journal-mood">${j.mood}</span>` : ''}
                    <span class="journal-date">${formatDate(j.date)}</span>
                </div>
                <button class="btn-icon" onclick="deleteJournal('${j.id}')" title="Delete">🗑️</button>
            </div>
            ${j.title ? `<h3 class="journal-title">${escHtml(j.title)}</h3>` : ''}
            <p class="journal-content">${escHtml(j.content)}</p>
        </div>`).join('');
}

window.deleteJournal = function(id) {
    if (!confirm('Delete this journal entry?')) return;
    data.journal = data.journal.filter(x => x.id !== id);
    saveData(); renderJournal();
};

function escHtml(s) {
    const d = document.createElement('div');
    d.textContent = s || '';
    return d.innerHTML;
}

// ═══════════════════════════════════════════════════════════════════
// CALENDAR
// ═══════════════════════════════════════════════════════════════════

let calYear, calMonth;

function renderCalendar() {
    if (!calYear) { const now = new Date(); calYear = now.getFullYear(); calMonth = now.getMonth(); }

    const monthNames = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'];
    document.getElementById('cal-title').textContent = `${monthNames[calMonth]} ${calYear}`;

    const info = calculateCycleInfo();
    const firstDay = new Date(calYear, calMonth, 1);
    const lastDay = new Date(calYear, calMonth + 1, 0);
    const startPad = firstDay.getDay();

    // Build period date set
    const periodDates = new Set();
    data.periods.forEach(p => {
        if (!p.startDate) return;
        const s = midnight(p.startDate);
        const e = p.endDate ? midnight(p.endDate) : s;
        const numDays = daysBetween(s, e);
        for (let i = 0; i <= numDays; i++) {
            periodDates.add(formatDateInput(new Date(s.getTime() + i * 86400000)));
        }
    });

    // Symptom dates
    const symptomDates = new Set(data.symptoms.map(s => s.date));

    // Predicted/fertile/ovulation dates
    const predictedDates = new Set();
    const fertileDates = new Set();
    const ovulationDates = new Set();
    if (info) {
        // Generate predictions for display range (forward and backward)
        const last = getLastPeriodStart();
        if (last) {
            const start = midnight(last.startDate);
            // Forward predictions
            for (let cycle = 0; cycle < 6; cycle++) {
                const cycleStart = new Date(start.getTime() + cycle * info.cycleLen * 86400000);
                for (let d = 0; d < info.periodLen; d++) {
                    const pd = new Date(cycleStart.getTime() + d * 86400000);
                    if (!periodDates.has(formatDateInput(pd))) predictedDates.add(formatDateInput(pd));
                }
                const ovDay = info.ovulationDay;
                const ov = new Date(cycleStart.getTime() + (ovDay - 1) * 86400000);
                ovulationDates.add(formatDateInput(ov));
                for (let d = ovDay - 6; d <= ovDay; d++) {
                    fertileDates.add(formatDateInput(new Date(cycleStart.getTime() + d * 86400000)));
                }
            }
            // Backward predictions
            for (let cycle = 1; cycle <= 12; cycle++) {
                const cycleStart = new Date(start.getTime() - cycle * info.cycleLen * 86400000);
                for (let d = 0; d < info.periodLen; d++) {
                    const pd = new Date(cycleStart.getTime() + d * 86400000);
                    if (!periodDates.has(formatDateInput(pd))) predictedDates.add(formatDateInput(pd));
                }
                const ovDay = info.ovulationDay;
                const ov = new Date(cycleStart.getTime() + (ovDay - 1) * 86400000);
                ovulationDates.add(formatDateInput(ov));
                for (let d = ovDay - 6; d <= ovDay; d++) {
                    fertileDates.add(formatDateInput(new Date(cycleStart.getTime() + d * 86400000)));
                }
            }
        }
    }

    const today = formatDateInput(new Date());
    let html = '';
    let dayNum = 1 - startPad;
    for (let week = 0; week < 6; week++) {
        if (dayNum > lastDay.getDate()) break;
        html += '<tr>';
        for (let dow = 0; dow < 7; dow++, dayNum++) {
            if (dayNum < 1 || dayNum > lastDay.getDate()) {
                html += '<td class="cal-day other-month"></td>';
            } else {
                const dateStr = formatDateInput(new Date(calYear, calMonth, dayNum));
                let cls = 'cal-day';
                if (dateStr === today) cls += ' today';
                if (periodDates.has(dateStr)) cls += ' period';
                else if (predictedDates.has(dateStr)) cls += ' predicted';
                if (ovulationDates.has(dateStr) && data.settings.showFertility) cls += ' ovulation';
                else if (fertileDates.has(dateStr) && data.settings.showFertility) cls += ' fertile';
                const symCount = data.symptoms.filter(s => s.date === dateStr).length;
                html += `<td class="${cls}"><span class="day-num">${dayNum}</span>${symCount ? `<span class="day-badge">${symCount}</span>` : ''}</td>`;
            }
        }
        html += '</tr>';
    }
    document.getElementById('cal-body').innerHTML = html;
}

// ═══════════════════════════════════════════════════════════════════
// TRENDS
// ═══════════════════════════════════════════════════════════════════

function renderTrends() {
    if (data.periods.length < 2) {
        document.getElementById('trends-empty').style.display = '';
        document.getElementById('trends-content').style.display = 'none';
        return;
    }
    document.getElementById('trends-empty').style.display = 'none';
    document.getElementById('trends-content').style.display = '';

    const sorted = [...data.periods].filter(p => p.startDate).sort((a, b) => parseLocalDate(a.startDate) - parseLocalDate(b.startDate));
    const cycles = [];
    for (let i = 1; i < sorted.length; i++) {
        const len = daysBetween(parseLocalDate(sorted[i - 1].startDate), parseLocalDate(sorted[i].startDate));
        if (len <= 0 || len > 90) continue; // skip bad data
        const pDays = sorted[i - 1].endDate ? daysBetween(parseLocalDate(sorted[i - 1].startDate), parseLocalDate(sorted[i - 1].endDate)) : null;
        const month = parseLocalDate(sorted[i - 1].startDate).toLocaleDateString('en-US', { month: 'short' });
        cycles.push({ len, pDays, month });
    }

    if (!cycles.length) {
        document.getElementById('trends-empty').style.display = '';
        document.getElementById('trends-content').style.display = 'none';
        return;
    }

    const avgLen = (cycles.reduce((s, c) => s + c.len, 0) / cycles.length).toFixed(1);
    const maxLen = Math.max(...cycles.map(c => c.len));
    const shortestCycle = Math.min(...cycles.map(c => c.len));
    const longestCycle = maxLen;

    document.getElementById('trends-stats').innerHTML = `
        <div class="stat-card"><div class="stat-number">${avgLen}</div><div class="stat-label">avg cycle length</div></div>
        <div class="stat-card"><div class="stat-number">${shortestCycle}–${longestCycle}</div><div class="stat-label">cycle range (days)</div></div>
        <div class="stat-card"><div class="stat-number">${data.periods.length}</div><div class="stat-label">periods logged</div></div>
        <div class="stat-card"><div class="stat-number">${data.symptoms.length}</div><div class="stat-label">symptoms logged</div></div>`;

    // Cycle length chart
    document.getElementById('trends-cycle-chart').innerHTML = `
        <h2>📏 Cycle Length Over Time</h2>
        <p class="section-subtitle">How your cycle length varies month to month</p>
        <div class="chart-card"><div class="bar-chart">${cycles.slice(-12).map(c => `
            <div class="bar-col">
                <div class="bar-value">${c.len}d</div>
                <div class="bar-track"><div class="bar-fill cycle-bar" style="height:${(c.len / maxLen * 100).toFixed(0)}%"></div></div>
                <div class="bar-label">${c.month}</div>
            </div>`).join('')}</div></div>`;

    // Period duration chart
    const withDuration = cycles.filter(c => c.pDays && c.pDays > 0 && c.pDays <= 15);
    const maxPDays = withDuration.length ? Math.max(...withDuration.map(c => c.pDays)) : 10;
    document.getElementById('trends-period-chart').innerHTML = withDuration.length ? `
        <h2>🩸 Period Duration</h2>
        <p class="section-subtitle">How many days each period lasted</p>
        <div class="chart-card"><div class="bar-chart">${withDuration.slice(-12).map(c => `
            <div class="bar-col">
                <div class="bar-value">${c.pDays}d</div>
                <div class="bar-track"><div class="bar-fill period-bar" style="height:${(c.pDays / maxPDays * 100).toFixed(0)}%"></div></div>
                <div class="bar-label">${c.month}</div>
            </div>`).join('')}</div></div>` : '';

    // Symptom frequency — horizontal bar style like server
    const symFreq = {};
    const symSev = {};
    data.symptoms.forEach(s => {
        const syms = s.symptoms || (s.symptom ? [s.symptom] : []);
        syms.forEach(name => {
            symFreq[name] = (symFreq[name] || 0) + 1;
            symSev[name] = (symSev[name] || []);
            symSev[name].push(s.severity || 3);
        });
    });
    const topSymptoms = Object.entries(symFreq).sort((a, b) => b[1] - a[1]).slice(0, 10);
    const maxSym = topSymptoms.length ? topSymptoms[0][1] : 1;

    document.getElementById('trends-symptom-chart').innerHTML = topSymptoms.length ? `
        <h2>📝 Most Common Symptoms</h2>
        <p class="section-subtitle">Your most frequently logged symptoms</p>
        <div class="symptom-trend-list">${topSymptoms.map(([name, count]) => {
            const avg = (symSev[name].reduce((a, b) => a + b, 0) / symSev[name].length).toFixed(1);
            return `<div class="symptom-trend-row">
                <div class="symptom-trend-info">
                    <span class="symptom-trend-name">${name}</span>
                </div>
                <div class="symptom-trend-stats">
                    <span class="trend-count">${count}×</span>
                    <div class="trend-bar-track">
                        <div class="trend-bar-fill" style="width:${(count / maxSym * 100).toFixed(0)}%"></div>
                    </div>
                    <span class="trend-avg">avg ${avg}/5</span>
                </div>
            </div>`;
        }).join('')}</div>` : '';

    // Month-by-month overview
    const monthData = {};
    data.periods.forEach(p => {
        if (!p.startDate) return;
        const d = parseLocalDate(p.startDate);
        const key = `${d.getFullYear()}-${d.getMonth()}`;
        if (!monthData[key]) monthData[key] = { periods: 0, symptoms: 0, month: d.toLocaleDateString('en-US', { month: 'short' }), year: d.getFullYear() };
        monthData[key].periods++;
    });
    data.symptoms.forEach(s => {
        if (!s.date) return;
        const d = parseLocalDate(s.date);
        const key = `${d.getFullYear()}-${d.getMonth()}`;
        if (!monthData[key]) monthData[key] = { periods: 0, symptoms: 0, month: d.toLocaleDateString('en-US', { month: 'short' }), year: d.getFullYear() };
        monthData[key].symptoms++;
    });
    const months = Object.values(monthData).slice(-12);
    document.getElementById('trends-months').innerHTML = months.length > 2 ? `
        <h2>📅 Month by Month</h2>
        <p class="section-subtitle">Activity over the last 12 months</p>
        <div class="month-grid">${months.map(m => `
            <div class="month-cell ${m.periods ? 'has-data' : ''}">
                <div class="month-name">${m.month}</div>
                <div class="month-year">${m.year}</div>
                ${m.periods ? `<div class="month-stat">🩸 ${m.periods}</div>` : ''}
                ${m.symptoms ? `<div class="month-stat">📝 ${m.symptoms}</div>` : ''}
            </div>`).join('')}</div>` : '';

    // Symptom × Phase Correlation
    renderPhaseCorrelation(sorted, topSymptoms);

    // Year at a Glance
    renderYearAtGlance();
}

function renderPhaseCorrelation(sortedPeriods, topSymptoms) {
    if (!topSymptoms.length || sortedPeriods.length < 2) {
        document.getElementById('trends-correlation').innerHTML = '';
        return;
    }

    const cycleLen = data.settings.cycleLength;
    const periodLen = data.settings.periodLength;
    const ovDay = Math.max(cycleLen - 14, periodLen + 1);
    const phases = ['menstruation', 'follicular', 'ovulation', 'luteal'];
    const phaseEmoji = { menstruation: '🌺', follicular: '🌱', ovulation: '🌸', luteal: '🌙' };

    function getPhaseForDate(dateStr) {
        const date = midnight(dateStr);
        // Find which cycle this date belongs to
        for (let i = sortedPeriods.length - 1; i >= 0; i--) {
            const start = midnight(sortedPeriods[i].startDate);
            const diff = daysBetween(start, date);
            if (diff >= 0 && diff < cycleLen) {
                const day = diff + 1;
                if (day <= periodLen) return 'menstruation';
                if (day < ovDay - 1) return 'follicular';
                if (day <= ovDay + 1) return 'ovulation';
                return 'luteal';
            }
        }
        return null;
    }

    // Count symptoms per phase
    const corr = {};
    topSymptoms.forEach(([name]) => { corr[name] = { menstruation: 0, follicular: 0, ovulation: 0, luteal: 0 }; });

    data.symptoms.forEach(s => {
        const phase = getPhaseForDate(s.date);
        if (!phase) return;
        const syms = s.symptoms || (s.symptom ? [s.symptom] : []);
        syms.forEach(name => { if (corr[name]) corr[name][phase]++; });
    });

    const maxCorr = Math.max(1, ...Object.values(corr).flatMap(p => Object.values(p)));

    document.getElementById('trends-correlation').innerHTML = `
        <h2>🔬 Symptom × Phase Correlation</h2>
        <p class="section-subtitle">Which symptoms appear in which cycle phases</p>
        <div class="correlation-grid">
            <div class="correlation-header">
                <div class="corr-label">Symptom</div>
                ${phases.map(p => `<div class="corr-phase">${phaseEmoji[p]}</div>`).join('')}
            </div>
            ${topSymptoms.slice(0, 8).map(([name]) => `
            <div class="correlation-row">
                <div class="corr-label"><span class="symptom-badge">${name}</span></div>
                ${phases.map(p => {
                    const count = corr[name][p];
                    const level = count === 0 ? 0 : Math.min(3, Math.ceil(count / maxCorr * 3));
                    return `<div class="corr-cell"><span class="corr-dot corr-level-${level}" title="${count}×">${count || ''}</span></div>`;
                }).join('')}
            </div>`).join('')}
        </div>`;
}

function renderYearAtGlance() {
    const today = new Date();
    const periodDates = new Set();
    data.periods.forEach(p => {
        if (!p.startDate) return;
        const s = midnight(p.startDate);
        const e = p.endDate ? midnight(p.endDate) : s;
        const numDays = daysBetween(s, e);
        for (let i = 0; i <= Math.min(numDays, 15); i++) {
            periodDates.add(formatDateInput(new Date(s.getTime() + i * 86400000)));
        }
    });

    const monthsHtml = [];
    for (let m = 0; m < 12; m++) {
        const monthDate = new Date(today.getFullYear(), today.getMonth() - 11 + m, 1);
        const year = monthDate.getFullYear();
        const month = monthDate.getMonth();
        const name = monthDate.toLocaleDateString('en-US', { month: 'short' });
        const firstDay = new Date(year, month, 1).getDay();
        const lastDate = new Date(year, month + 1, 0).getDate();
        const todayStr = formatDateInput(today);

        let days = '';
        for (let pad = 0; pad < firstDay; pad++) days += '<span class="mini-day empty"></span>';
        for (let d = 1; d <= lastDate; d++) {
            const ds = formatDateInput(new Date(year, month, d));
            let cls = 'mini-day';
            if (ds === todayStr) cls += ' today';
            if (periodDates.has(ds)) cls += ' period';
            days += `<span class="${cls}">${d}</span>`;
        }

        monthsHtml.push(`<div class="mini-month">
            <div class="mini-month-name">${name} ${year !== today.getFullYear() ? year : ''}</div>
            <div class="mini-cal-header"><span>S</span><span>M</span><span>T</span><span>W</span><span>T</span><span>F</span><span>S</span></div>
            <div class="mini-cal-grid">${days}</div>
        </div>`);
    }

    document.getElementById('trends-year').innerHTML = `
        <h2>📅 Year at a Glance</h2>
        <p class="section-subtitle">Your cycle patterns across the last 12 months</p>
        <div class="year-grid">${monthsHtml.join('')}</div>`;
}

// ═══════════════════════════════════════════════════════════════════
// SETTINGS
// ═══════════════════════════════════════════════════════════════════

function renderSettings() {
    document.getElementById('set-cycle-len').value = data.settings.cycleLength;
    document.getElementById('set-period-len').value = data.settings.periodLength;
    document.getElementById('set-fertility').checked = data.settings.showFertility;
    document.getElementById('set-autolock').value = data.settings.autoLock || 5;
    document.getElementById('set-name').value = data.settings.displayName || '';
    document.getElementById('set-theme').value = data.settings.theme || 'bloom';
}

// ═══════════════════════════════════════════════════════════════════
// EXPORT / IMPORT
// ═══════════════════════════════════════════════════════════════════

async function exportEncrypted() {
    const pass = prompt('Choose a backup passphrase (8+ chars):');
    if (!pass || pass.length < 8) { alert('Passphrase must be at least 8 characters.'); return; }
    const salt = crypto.getRandomValues(new Uint8Array(16));
    const key = await deriveKey(pass, salt);
    const ct = await encrypt(key, JSON.stringify(data));
    const backup = { type: 'bloom-encrypted-backup', version: 2, salt: Array.from(salt).map(b => b.toString(16).padStart(2, '0')).join(''), data: ct };
    download(JSON.stringify(backup), 'bloom-backup-encrypted.json');
}

function exportJSON() {
    download(JSON.stringify(data, null, 2), 'bloom-export.json');
}

async function importFile(file) {
    const text = await file.text();
    let imported;
    try {
        imported = JSON.parse(text);
    } catch { alert('Invalid file format.'); return; }

    if (imported.type === 'bloom-encrypted-backup') {
        const pass = prompt('Enter the backup passphrase:');
        if (!pass) return;
        try {
            const salt = Uint8Array.from(imported.salt.match(/.{2}/g).map(b => parseInt(b, 16)));
            const key = await deriveKey(pass, salt);
            const json = await decrypt(key, imported.data);
            imported = JSON.parse(json);
        } catch { alert('Wrong passphrase or corrupted backup.'); return; }
    }

    // Normalize server export format (snake_case → camelCase)
    const normPeriods = (imported.periods || []).map(p => ({
        id: p.id || uid(),
        startDate: p.startDate || p.start_date,
        endDate: p.endDate || p.end_date || null
    }));
    const normSymptoms = (imported.symptoms || []).map(s => ({
        id: s.id || uid(),
        date: s.date,
        category: s.category || 'physical',
        symptoms: s.symptoms || (s.symptom ? [s.symptom] : []),
        severity: s.severity || 3,
        notes: s.notes || ''
    }));
    const normJournal = (imported.journal || []).map(j => ({
        id: j.id || uid(),
        date: j.date,
        mood: j.mood || '',
        title: j.title || '',
        content: j.content || ''
    }));

    // Merge or replace
    if (normPeriods.length || normSymptoms.length || normJournal.length) {
        const merge = confirm('Merge with existing data? (Cancel = replace all)');
        if (merge) {
            const existingPeriodDates = new Set(data.periods.map(p => p.startDate));
            normPeriods.forEach(p => { if (!existingPeriodDates.has(p.startDate)) data.periods.push(p); });
            normSymptoms.forEach(s => data.symptoms.push(s));
            normJournal.forEach(j => data.journal.push(j));
        } else {
            data.periods = normPeriods;
            data.symptoms = normSymptoms;
            data.journal = normJournal;
            if (imported.settings) data.settings = { ...data.settings, ...imported.settings };
            if (imported.user) {
                if (imported.user.cycle_length) data.settings.cycleLength = imported.user.cycle_length;
                if (imported.user.period_length) data.settings.periodLength = imported.user.period_length;
            }
        }
        await saveData();
        alert('Import complete!');
        renderDashboard();
    } else {
        alert('Unrecognized file format. Expected Bloom export/backup.');
    }
}

function download(content, filename) {
    const a = document.createElement('a');
    a.href = URL.createObjectURL(new Blob([content], { type: 'application/json' }));
    a.download = filename;
    a.click();
    URL.revokeObjectURL(a.href);
}

function uid() {
    return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

// ═══════════════════════════════════════════════════════════════════
// EVENT BINDINGS
// ═══════════════════════════════════════════════════════════════════

function init() {
    // Lock screen logic
    if (hasVault()) {
        document.getElementById('lock-setup').style.display = 'none';
        document.getElementById('lock-unlock').style.display = '';
    } else {
        document.getElementById('lock-setup').style.display = '';
        document.getElementById('lock-unlock').style.display = 'none';
    }

    // Setup form
    document.getElementById('setup-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const pass = document.getElementById('setup-pass').value;
        const confirm = document.getElementById('setup-confirm').value;
        if (pass !== confirm) { showLockError('Passphrases don\'t match.'); return; }
        if (pass.length < 8) { showLockError('Must be 8+ characters.'); return; }
        await createVault(pass);
        unlockUI();
    });

    // Unlock form
    document.getElementById('unlock-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const pass = document.getElementById('unlock-pass').value;
        try {
            await loadData(pass);
            unlockUI();
        } catch {
            showLockError('Wrong passphrase.');
        }
    });

    // Nav tabs
    document.querySelectorAll('.local-nav-btn').forEach(btn => {
        btn.addEventListener('click', () => switchTab(btn.dataset.tab));
    });

    // Period form
    document.getElementById('period-form').addEventListener('submit', (e) => {
        e.preventDefault();
        const startDate = document.getElementById('period-start').value;
        if (!startDate) return;
        data.periods.push({ id: uid(), startDate, endDate: null });
        saveData(); renderPeriods(); renderDashboard();
    });

    // Symptom category buttons
    document.getElementById('sym-categories').addEventListener('click', (e) => {
        const btn = e.target.closest('[data-cat]');
        if (!btn) return;
        document.querySelectorAll('#sym-categories .pill').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        selectedCategory = btn.dataset.cat;
        renderSymptomPills();
    });

    // Symptom form
    document.getElementById('symptom-form').addEventListener('submit', (e) => {
        e.preventDefault();
        if (!selectedSymptoms.length) { alert('Select at least one symptom.'); return; }
        data.symptoms.push({
            id: uid(),
            date: document.getElementById('sym-date').value,
            category: selectedCategory,
            symptoms: [...selectedSymptoms],
            severity: parseInt(document.getElementById('sym-severity').value),
            notes: document.getElementById('sym-notes').value
        });
        selectedSymptoms = [];
        document.getElementById('sym-notes').value = '';
        saveData(); renderSymptoms();
    });

    // Journal mood picker
    document.getElementById('jrn-moods').addEventListener('click', (e) => {
        const btn = e.target.closest('[data-mood]');
        if (!btn) return;
        document.querySelectorAll('#jrn-moods .mood-option').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        selectedMood = btn.dataset.mood;
    });

    // Journal form
    document.getElementById('journal-form').addEventListener('submit', (e) => {
        e.preventDefault();
        const content = document.getElementById('jrn-content').value.trim();
        if (!content) { alert('Write something first!'); return; }
        data.journal.push({
            id: uid(),
            date: document.getElementById('jrn-date').value,
            mood: selectedMood,
            title: document.getElementById('jrn-title').value.trim(),
            content
        });
        document.getElementById('jrn-title').value = '';
        document.getElementById('jrn-content').value = '';
        saveData(); renderJournal();
    });

    // Calendar nav
    document.getElementById('cal-prev').addEventListener('click', () => { calMonth--; if (calMonth < 0) { calMonth = 11; calYear--; } renderCalendar(); });
    document.getElementById('cal-next').addEventListener('click', () => { calMonth++; if (calMonth > 11) { calMonth = 0; calYear++; } renderCalendar(); });

    // Settings form
    document.getElementById('settings-form').addEventListener('submit', (e) => {
        e.preventDefault();
        data.settings.cycleLength = parseInt(document.getElementById('set-cycle-len').value) || 28;
        data.settings.periodLength = parseInt(document.getElementById('set-period-len').value) || 5;
        data.settings.showFertility = document.getElementById('set-fertility').checked;
        data.settings.displayName = document.getElementById('set-name').value.trim();
        data.settings.theme = document.getElementById('set-theme').value;
        document.body.setAttribute('data-theme', data.settings.theme);
        saveData();
        alert('Settings saved!');
    });

    // Theme change preview
    document.getElementById('set-theme').addEventListener('change', (e) => {
        document.body.setAttribute('data-theme', e.target.value);
    });

    // Auto-lock select
    document.getElementById('set-autolock').addEventListener('change', (e) => {
        data.settings.autoLock = parseInt(e.target.value);
        saveData(); resetAutoLock();
    });

    // Security buttons
    document.getElementById('btn-lock').addEventListener('click', lockVault);
    document.getElementById('btn-lock-nav').addEventListener('click', lockVault);
    document.getElementById('btn-change-pass').addEventListener('click', async () => {
        const newPass = prompt('New passphrase (8+ chars):');
        if (!newPass || newPass.length < 8) { alert('Must be 8+ characters.'); return; }
        const confirmPass = prompt('Confirm new passphrase:');
        if (newPass !== confirmPass) { alert('Passphrases don\'t match.'); return; }
        const salt = crypto.getRandomValues(new Uint8Array(16));
        const saltHex = Array.from(salt).map(b => b.toString(16).padStart(2, '0')).join('');
        localStorage.setItem(SALT_KEY, saltHex);
        cryptoKey = await deriveKey(newPass, salt);
        await saveData();
        alert('Passphrase changed! All data re-encrypted.');
    });

    // Export/Import
    document.getElementById('btn-export-enc').addEventListener('click', exportEncrypted);
    document.getElementById('btn-export-json').addEventListener('click', exportJSON);
    document.getElementById('import-file').addEventListener('change', (e) => {
        if (e.target.files[0]) importFile(e.target.files[0]);
        e.target.value = '';
    });

    // Erase
    document.getElementById('btn-erase').addEventListener('click', () => {
        if (!confirm('PERMANENTLY delete all local data? This cannot be undone!')) return;
        if (!confirm('Are you absolutely sure?')) return;
        localStorage.removeItem(STORAGE_KEY);
        localStorage.removeItem(SALT_KEY);
        cryptoKey = null; data = null;
        location.reload();
    });
}

function unlockUI() {
    document.getElementById('lock-screen').style.display = 'none';
    document.getElementById('app-main').style.display = '';
    if (data.settings.theme) document.body.setAttribute('data-theme', data.settings.theme);
    resetAutoLock();
    renderDashboard();
}

function showLockError(msg) {
    document.getElementById('lock-error').textContent = msg;
}

// Start
init();
})();
