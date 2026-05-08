// ─── Fix date inputs to use client local date ──────────────────────────
(function() {
    var now = new Date();
    var localToday = now.getFullYear() + '-' +
        String(now.getMonth() + 1).padStart(2, '0') + '-' +
        String(now.getDate()).padStart(2, '0');
    document.querySelectorAll('input[type="date"]').forEach(function(input) {
        // Only fix inputs that default to "today" (server-rendered)
        if (input.value && input.getAttribute('max')) {
            input.value = localToday;
            input.setAttribute('max', localToday);
        } else if (input.value && !input.getAttribute('data-start')) {
            input.value = localToday;
        }
    });
})();

// ─── Nav Toggle ─────────────────────────────────────────────────────────
function toggleNav() {
    document.getElementById('nav-links').classList.toggle('show');
}

// ─── Category Selection (Symptom Form) ──────────────────────────────────
function selectCategory(btn) {
    // Update active pill
    btn.parentElement.querySelectorAll('.pill').forEach(function(p) {
        p.classList.remove('active');
    });
    btn.classList.add('active');

    // Show matching symptom group
    var cat = btn.getAttribute('data-category');
    document.getElementById('category-input').value = cat;

    document.querySelectorAll('.symptom-group').forEach(function(g) {
        g.style.display = g.getAttribute('data-category') === cat ? 'flex' : 'none';
    });

    // Clear all symptom selections
    document.querySelectorAll('.pill.sym').forEach(function(p) {
        p.classList.remove('active');
    });
    updateSymptomsInput();
}

// ─── Symptom Multi-Select ───────────────────────────────────────────────
function toggleSymptom(btn) {
    btn.classList.toggle('active');
    updateSymptomsInput();
}

function updateSymptomsInput() {
    var selected = [];
    document.querySelectorAll('.pill.sym.active').forEach(function(p) {
        selected.push(p.textContent.trim());
    });
    var input = document.getElementById('symptoms-input');
    if (input) input.value = selected.join(',');

    // Update the selected display
    var display = document.getElementById('selected-symptoms-display');
    var list = document.getElementById('selected-list');
    var customInput = document.getElementById('custom-symptom-input');
    var customVal = (customInput && customInput.value.trim()) ? customInput.value.trim() : '';

    if (display && list) {
        var allSelected = selected.slice();
        if (customVal) allSelected.push(customVal + ' (custom)');
        if (allSelected.length > 0) {
            display.style.display = 'block';
            list.textContent = allSelected.join(', ');
        } else {
            display.style.display = 'none';
        }
    }
}

// Legacy single-select (kept for backwards compatibility)
function selectSymptom(btn) {
    toggleSymptom(btn);
}

// ─── Custom Symptom Toggle ──────────────────────────────────────────────
function toggleCustomSymptom() {
    var input = document.getElementById('custom-symptom-input');
    if (!input) return;
    if (input.style.display === 'none') {
        input.style.display = 'block';
        input.focus();
        input.addEventListener('input', updateSymptomsInput);
    } else {
        input.style.display = 'none';
        input.value = '';
        updateSymptomsInput();
    }
}

// ─── Severity Slider ────────────────────────────────────────────────────
function updateSeverityDisplay(value) {
    var display = document.getElementById('severity-display');
    if (!display) return;
    var filled = '';
    for (var i = 0; i < 5; i++) {
        filled += i < value ? '●' : '○';
    }
    display.textContent = filled;
}

// ─── Init ───────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
    // ─── PWA: Service Worker Registration + Update Detection ────────────
    if ('serviceWorker' in navigator) {
        navigator.serviceWorker.register('/sw.js', { scope: '/' }).then(function(registration) {
            // Check for updates immediately and every 60 seconds
            registration.update();
            setInterval(function() { registration.update(); }, 60000);

            // Detect new service worker waiting
            function showUpdateBanner(worker) {
                if (document.getElementById('bloom-update-banner')) return;
                var banner = document.createElement('div');
                banner.id = 'bloom-update-banner';
                banner.innerHTML = '<div class="pwa-banner pwa-update-banner">' +
                    '<span>🌸 A new version of Bloom is available!</span>' +
                    '<button id="bloom-update-btn" class="btn btn-sm btn-primary">Refresh</button>' +
                    '<button id="bloom-update-dismiss" class="btn btn-sm" style="margin-left:8px;background:transparent;color:inherit;">Later</button>' +
                    '</div>';
                document.body.appendChild(banner);
                document.getElementById('bloom-update-btn').addEventListener('click', function() {
                    worker.postMessage({ type: 'SKIP_WAITING' });
                });
                document.getElementById('bloom-update-dismiss').addEventListener('click', function() {
                    banner.remove();
                });
            }

            if (registration.waiting) {
                showUpdateBanner(registration.waiting);
            }

            registration.addEventListener('updatefound', function() {
                var newWorker = registration.installing;
                newWorker.addEventListener('statechange', function() {
                    if (newWorker.state === 'installed' && navigator.serviceWorker.controller) {
                        showUpdateBanner(newWorker);
                    }
                });
            });

            // Reload when new SW takes over
            var refreshing = false;
            navigator.serviceWorker.addEventListener('controllerchange', function() {
                if (!refreshing) {
                    refreshing = true;
                    window.location.reload();
                }
            });
        }).catch(function(err) {
            console.warn('SW registration failed:', err);
        });
    }

    // ─── PWA: Install Prompt ────────────────────────────────────────────
    var deferredPrompt = null;
    var installDismissed = sessionStorage.getItem('bloom-install-dismissed');

    window.addEventListener('beforeinstallprompt', function(e) {
        e.preventDefault();
        deferredPrompt = e;
        if (!installDismissed && !window.matchMedia('(display-mode: standalone)').matches) {
            showInstallBanner();
        }
    });

    function showInstallBanner() {
        if (document.getElementById('bloom-install-banner')) return;
        var banner = document.createElement('div');
        banner.id = 'bloom-install-banner';
        banner.innerHTML = '<div class="pwa-banner pwa-install-banner">' +
            '<span>🌸 Install Bloom for the best experience!</span>' +
            '<button id="bloom-install-btn" class="btn btn-sm btn-primary">Install</button>' +
            '<button id="bloom-install-dismiss" class="btn btn-sm" style="margin-left:8px;background:transparent;color:inherit;">Not now</button>' +
            '</div>';
        document.body.appendChild(banner);
        document.getElementById('bloom-install-btn').addEventListener('click', function() {
            banner.remove();
            if (deferredPrompt) {
                deferredPrompt.prompt();
                deferredPrompt.userChoice.then(function() {
                    deferredPrompt = null;
                });
            }
        });
        document.getElementById('bloom-install-dismiss').addEventListener('click', function() {
            banner.remove();
            sessionStorage.setItem('bloom-install-dismissed', '1');
        });
    }

    // iOS install hint (no beforeinstallprompt on Safari)
    var isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent) && !window.MSStream;
    var isStandalone = window.navigator.standalone === true;
    if (isIOS && !isStandalone && !sessionStorage.getItem('bloom-ios-dismissed')) {
        setTimeout(function() {
            var banner = document.createElement('div');
            banner.id = 'bloom-ios-banner';
            banner.innerHTML = '<div class="pwa-banner pwa-install-banner">' +
                '<span>🌸 Install Bloom: tap <strong>Share</strong> then <strong>"Add to Home Screen"</strong></span>' +
                '<button id="bloom-ios-dismiss" class="btn btn-sm" style="margin-left:8px;background:transparent;color:inherit;">✕</button>' +
                '</div>';
            document.body.appendChild(banner);
            document.getElementById('bloom-ios-dismiss').addEventListener('click', function() {
                banner.remove();
                sessionStorage.setItem('bloom-ios-dismissed', '1');
            });
        }, 2000);
    }

    // Severity slider
    var slider = document.getElementById('severity');
    if (slider) {
        updateSeverityDisplay(slider.value);
        slider.addEventListener('input', function() {
            updateSeverityDisplay(this.value);
        });
    }

    // Auto-dismiss flash messages
    var flash = document.getElementById('flash');
    if (flash) {
        setTimeout(function() {
            flash.style.opacity = '0';
            flash.style.transform = 'translateY(-10px)';
            setTimeout(function() {
                flash.remove();
            }, 300);
        }, 4000);
    }

    // Symptom form validation
    var symptomForm = document.querySelector('.symptom-form');
    if (symptomForm) {
        symptomForm.addEventListener('submit', function(e) {
            var symptomsInput = document.getElementById('symptoms-input');
            var customInput = document.getElementById('custom-symptom-input');
            var hasPills = symptomsInput && symptomsInput.value.trim();
            var hasCustom = customInput && customInput.value.trim();
            if (!hasPills && !hasCustom) {
                e.preventDefault();
                alert('Please select at least one symptom or type a custom one');
            }
        });
    }

    // Action item toggle (partner dashboard)
    document.querySelectorAll('.action-item').forEach(function(item) {
        item.addEventListener('click', function() {
            this.classList.toggle('done');
            var check = this.querySelector('.action-check');
            if (check) {
                check.textContent = this.classList.contains('done') ? '✓' : '○';
            }
        });
    });

    // Rating selector buttons
    document.querySelectorAll('.rating-selector').forEach(function(selector) {
        var buttons = selector.querySelectorAll('.rating-btn');
        var hiddenInput = selector.querySelector('input[type="hidden"]');
        buttons.forEach(function(btn) {
            btn.addEventListener('click', function() {
                buttons.forEach(function(b) { b.classList.remove('selected'); });
                btn.classList.add('selected');
                hiddenInput.value = btn.getAttribute('data-value');
            });
        });
    });

    var storageRadios = document.querySelectorAll('input[name="storage_mode"]');
    if (storageRadios.length) {
        var cloudFields = document.getElementById('cloud-signup-fields');
        var submitBtn = document.querySelector('.auth-form button[type="submit"]');
        var requiredIds = ['name', 'email', 'password', 'confirm'];

        function syncStorageModeUI() {
            var selected = document.querySelector('input[name="storage_mode"]:checked');
            var cloud = selected && selected.value === 'cloud';

            requiredIds.forEach(function(id) {
                var input = document.getElementById(id);
                if (!input) return;
                input.required = cloud;
                if (!cloud) {
                    input.value = '';
                }
            });

            if (cloudFields) {
                cloudFields.style.display = cloud ? 'block' : 'none';
            }
            if (submitBtn) {
                submitBtn.textContent = cloud ? 'Create Cloud Account' : 'Continue in Local-Only Mode';
            }

            ['storage-local-card', 'storage-cloud-card'].forEach(function(id) {
                var card = document.getElementById(id);
                if (!card) return;
                card.classList.remove('active');
            });
            var activeCardId = cloud ? 'storage-cloud-card' : 'storage-local-card';
            var activeCard = document.getElementById(activeCardId);
            if (activeCard) {
                activeCard.classList.add('active');
            }
        }

        storageRadios.forEach(function(radio) {
            radio.addEventListener('change', syncStorageModeUI);
        });
        syncStorageModeUI();
    }
});
