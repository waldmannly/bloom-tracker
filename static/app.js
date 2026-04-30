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
    // Clear custom input if selecting pills
    var customInput = document.getElementById('custom-symptom-input');
    if (customInput && customInput.style.display !== 'none') {
        customInput.style.display = 'none';
        customInput.value = '';
    }
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
    if (display && list) {
        if (selected.length > 0) {
            display.style.display = 'block';
            list.textContent = selected.join(', ');
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
        // Clear pill selections
        document.querySelectorAll('.pill.sym').forEach(function(p) {
            p.classList.remove('active');
        });
        document.getElementById('symptom-input').value = 'custom';
    } else {
        input.style.display = 'none';
        input.value = '';
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
            if (customInput && customInput.value.trim()) {
                return; // Custom symptom takes priority
            }
            if (!symptomsInput || !symptomsInput.value.trim()) {
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
});
