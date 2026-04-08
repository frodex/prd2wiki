// prd2wiki editor — wiki.js

let editorInstance = null;
let getMarkdownFn = null;
let usingFallback = false;

async function initMilkdown() {
    const container = document.getElementById('milkdown-editor');
    const fallback = document.getElementById('editor-fallback');
    if (!container) return;

    if (window.prd2wikiEditor) {
        try {
            const form = document.getElementById('page-form');
            const project = form ? form.dataset.project : '';
            const pageId = document.getElementById('field-id') ? document.getElementById('field-id').value : '';
            const ok = await window.prd2wikiEditor.initEditor('milkdown-editor', 'editor-fallback', project, pageId);
            if (ok) {
                getMarkdownFn = () => window.prd2wikiEditor.getEditorContent();
                // Prevent browser default drag behavior on the whole editor area
                container.addEventListener('dragover', (e) => { e.preventDefault(); e.stopPropagation(); });
                container.addEventListener('drop', (e) => { e.stopPropagation(); });
                console.log('Milkdown editor initialized (bundled)');
                return;
            }
        } catch (err) {
            console.warn('Bundled Milkdown failed:', err);
        }
    }

    // Fallback to textarea with paste-to-upload support
    console.warn('Milkdown not available, using textarea fallback');
    usingFallback = true;
    container.style.display = 'none';
    if (fallback) {
        fallback.style.display = '';
        // Prevent browser default drag — handle file drops for upload
        fallback.addEventListener('dragover', (e) => { e.preventDefault(); e.stopPropagation(); fallback.classList.add('drag-over'); });
        fallback.addEventListener('dragleave', () => { fallback.classList.remove('drag-over'); });
        fallback.addEventListener('drop', async (e) => {
            e.preventDefault();
            e.stopPropagation();
            fallback.classList.remove('drag-over');
            const files = e.dataTransfer?.files;
            if (!files || files.length === 0) return;
            const form = document.getElementById('page-form');
            const project = form ? form.dataset.project : '';
            const pageId = document.getElementById('field-id') ? document.getElementById('field-id').value : '';
            if (!project || !pageId) { alert('Save the page first before uploading images.'); return; }
            for (let i = 0; i < files.length; i++) {
                const file = files[i];
                if (!file.type.startsWith('image/')) continue;
                const formData = new FormData();
                formData.append('file', file);
                try {
                    const resp = await fetch(`/api/projects/${encodeURIComponent(project)}/pages/${encodeURIComponent(pageId)}/attachments`, { method: 'POST', body: formData });
                    if (!resp.ok) { alert('Upload failed: ' + resp.status); return; }
                    const { url } = await resp.json();
                    const pos = fallback.selectionStart;
                    const md = `![${file.name}](${url})`;
                    fallback.value = fallback.value.slice(0, pos) + md + fallback.value.slice(pos);
                    fallback.selectionStart = fallback.selectionEnd = pos + md.length;
                } catch (err) { alert('Upload error: ' + err.message); }
            }
        });
        // Add clipboard paste handler for images in textarea mode
        fallback.addEventListener('paste', async (e) => {
            const items = e.clipboardData?.items;
            if (!items) return;
            for (let i = 0; i < items.length; i++) {
                if (items[i].type.startsWith('image/')) {
                    e.preventDefault();
                    const file = items[i].getAsFile();
                    if (!file) continue;
                    const form = document.getElementById('page-form');
                    const project = form ? form.dataset.project : '';
                    const pageId = document.getElementById('field-id') ? document.getElementById('field-id').value : '';
                    if (!project || !pageId) { alert('Save the page first before uploading images.'); return; }
                    const formData = new FormData();
                    formData.append('file', file);
                    try {
                        const resp = await fetch(`/api/projects/${encodeURIComponent(project)}/pages/${encodeURIComponent(pageId)}/attachments`, { method: 'POST', body: formData });
                        if (!resp.ok) { alert('Upload failed: ' + resp.status); return; }
                        const { url } = await resp.json();
                        const pos = fallback.selectionStart;
                        const md = `![screenshot](${url})`;
                        fallback.value = fallback.value.slice(0, pos) + md + fallback.value.slice(pos);
                        fallback.selectionStart = fallback.selectionEnd = pos + md.length;
                    } catch (err) { alert('Upload error: ' + err.message); }
                    return;
                }
            }
        });
    }
}

function getEditorMarkdown() {
    if (usingFallback) {
        const ta = document.getElementById('editor-fallback');
        return ta ? ta.value : '';
    }
    if (getMarkdownFn) {
        return getMarkdownFn();
    }
    // Last resort
    const ta = document.getElementById('editor-fallback');
    return ta ? ta.value : '';
}

function collectFormData(intent) {
    const form = document.getElementById('page-form');
    if (!form) return null;

    const tagsRaw = form.querySelector('#field-tags').value;
    const tags = tagsRaw
        .split(',')
        .map(t => t.trim())
        .filter(t => t.length > 0);

    return {
        id: form.querySelector('#field-id').value.trim(),
        title: form.querySelector('#field-title').value.trim(),
        type: form.querySelector('#field-type').value,
        status: form.querySelector('#field-status').value,
        tags: tags,
        body: getEditorMarkdown(),
        intent: intent,
    };
}

function showResults(data, isError) {
    const area = document.getElementById('results-area');
    const content = document.getElementById('results-content');
    if (!area || !content) return;

    area.style.display = '';

    if (isError) {
        content.innerHTML = '<div class="result-error">' + escapeHtml(String(data)) + '</div>';
        return;
    }

    let html = '';
    if (data.issues && data.issues.length > 0) {
        html += '<div class="result-issues"><h3>Issues</h3><ul>';
        data.issues.forEach(i => {
            html += '<li class="issue-' + escapeHtml(i.severity || 'warning') + '">'
                + escapeHtml(i.message || String(i)) + '</li>';
        });
        html += '</ul></div>';
    }
    if (data.warnings && data.warnings.length > 0) {
        html += '<div class="result-warnings"><h3>Warnings</h3><ul>';
        data.warnings.forEach(w => {
            html += '<li>' + escapeHtml(String(w)) + '</li>';
        });
        html += '</ul></div>';
    }
    if (data.page_id || data.path) {
        html += '<div class="result-success">Page saved successfully.</div>';
    }
    if (!html) {
        html = '<div class="result-success">Operation completed.</div>';
    }

    content.innerHTML = html;
    area.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// Store the last submission data so we can resubmit as verbatim
let lastSubmitData = null;

function showDiffPreview(original, result, project) {
    const area = document.getElementById('results-area');
    const content = document.getElementById('results-content');
    if (!area || !content) return;

    area.style.display = '';

    let html = '<div class="diff-preview">';
    html += '<h3>Changes Preview</h3>';

    // Show warnings
    if (result.warnings && result.warnings.length > 0) {
        html += '<div class="diff-warnings"><strong>Warnings:</strong><ul>';
        result.warnings.forEach(w => {
            html += '<li>' + escapeHtml(String(w)) + '</li>';
        });
        html += '</ul></div>';
    }

    // Show issues
    if (result.issues && result.issues.length > 0) {
        html += '<div class="diff-issues"><strong>Issues:</strong><ul>';
        result.issues.forEach(i => {
            const sev = i.severity || 'info';
            const field = i.field ? '[' + escapeHtml(i.field) + '] ' : '';
            html += '<li class="issue-' + escapeHtml(sev) + '">'
                + field + escapeHtml(i.message || String(i)) + '</li>';
        });
        html += '</ul></div>';
    }

    // Show field-by-field diff between what was submitted and what was returned
    const fields = [
        { label: 'Title', orig: original.title, cur: result.title },
        { label: 'Status', orig: original.status, cur: result.status },
    ];
    let hasDiff = false;
    let diffHtml = '<table class="diff-table"><thead><tr><th>Field</th><th>Submitted</th><th>Returned</th></tr></thead><tbody>';

    fields.forEach(f => {
        if (f.orig && f.cur && f.orig !== f.cur) {
            hasDiff = true;
            diffHtml += '<tr>'
                + '<td>' + escapeHtml(f.label) + '</td>'
                + '<td class="diff-removed">' + escapeHtml(f.orig) + '</td>'
                + '<td class="diff-added">' + escapeHtml(f.cur) + '</td>'
                + '</tr>';
        }
    });
    diffHtml += '</tbody></table>';

    if (hasDiff) {
        html += diffHtml;
    }

    // Action buttons
    html += '<div class="diff-actions">';
    html += '<button class="btn btn-primary" onclick="acceptChanges()">Accept Changes</button>';
    html += '<button class="btn" onclick="editMore()">Edit More</button>';
    html += '<button class="btn" onclick="saveAsIs()">Save As-Is</button>';
    html += '</div>';

    html += '</div>';
    content.innerHTML = html;
    area.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

// Make diff action functions globally accessible for inline onclick handlers (module scope)
window.acceptChanges = function() {
    if (!lastSubmitData) return;
    const currentBody = getEditorMarkdown();
    if (currentBody !== lastSubmitData.body) {
        showResults('You made additional edits after the format check. Click a submit button again to include your new changes.', true);
        return;
    }
    const project = document.getElementById('page-form').dataset.project;
    const pageId = lastSubmitData.id;
    window.location.href = '/projects/' + encodeURIComponent(project) + '/pages/' + encodeURIComponent(pageId);
};

window.editMore = function() {
    // Hide the results area and let the user continue editing
    const area = document.getElementById('results-area');
    if (area) area.style.display = 'none';
};

window.saveAsIs = function() {
    // Resubmit the original data as verbatim (no mutation)
    if (!lastSubmitData) return;
    lastSubmitData.intent = 'verbatim';
    submitPage('verbatim');
};

async function submitPage(intent) {
    const data = collectFormData(intent);
    if (!data) return;

    if (!data.title) {
        showResults('Title is required.', true);
        return;
    }
    // ID is auto-generated by the server if left blank

    lastSubmitData = data;

    const project = document.getElementById('page-form').dataset.project;
    const url = '/api/projects/' + encodeURIComponent(project) + '/pages';

    try {
        const resp = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data),
        });

        const result = await resp.json();

        if (!resp.ok) {
            if (result.issues && result.issues.length > 0) {
                showResults(result, false);  // show structured issues, not generic error
            } else {
                showResults(result.error || ('HTTP ' + resp.status), true);
            }
            return;
        }

        // For conform/integrate, show diff preview if there are warnings or issues
        if ((intent === 'conform' || intent === 'integrate') &&
            ((result.warnings && result.warnings.length > 0) ||
             (result.issues && result.issues.length > 0))) {
            showDiffPreview(data, result, project);
            return;
        }

        showResults(result, false);

        // On success, clear auto-saved draft and redirect to page view
        clearAutosaveDraft();
        if (result.id || result.path) {
            const pageId = result.id || result.path?.replace('pages/','').replace('.md','') || data.id;
            setTimeout(() => {
                window.location.href = '/projects/' + encodeURIComponent(project) + '/pages/' + encodeURIComponent(pageId);
            }, 1500);
        }
    } catch (err) {
        showResults('Network error: ' + err.message, true);
    }
}

// Reference tree expansion — attach to window for inline onclick access
window.expandRef = async function expandRef(project, refId, toggleEl) {
    const li = toggleEl.closest('.ref-node');
    if (!li) return;

    // If already expanded, toggle collapse
    const existing = li.querySelector('.ref-tree');
    if (existing) {
        existing.remove();
        toggleEl.innerHTML = '&#9654;'; // right arrow
        toggleEl.classList.remove('ref-expanded');
        return;
    }

    toggleEl.innerHTML = '&#9660;'; // down arrow
    toggleEl.classList.add('ref-expanded');

    try {
        const resp = await fetch('/api/projects/' + encodeURIComponent(project) +
            '/pages/' + encodeURIComponent(refId) + '/references?depth=1');
        if (!resp.ok) {
            toggleEl.innerHTML = '&#9654;';
            toggleEl.classList.remove('ref-expanded');
            return;
        }
        const data = await resp.json();
        const children = data.children || [];

        if (children.length === 0) {
            const empty = document.createElement('ul');
            empty.className = 'ref-tree';
            empty.innerHTML = '<li class="ref-empty">No child references</li>';
            li.appendChild(empty);
            return;
        }

        const ul = document.createElement('ul');
        ul.className = 'ref-tree';

        children.forEach(child => {
            const childLi = document.createElement('li');
            childLi.className = 'ref-node';

            let statusIcon = '';
            if (child.status === 'valid') {
                statusIcon = '<span class="ref-status ref-valid" title="valid">&#10003;</span>';
            } else if (child.status === 'stale' || child.status === 'contested') {
                statusIcon = '<span class="ref-status ref-warn" title="' + escapeHtml(child.status) + '">&#9888;</span>';
            } else if (child.status) {
                statusIcon = '<span class="badge badge-' + escapeHtml(child.status) + '">' + escapeHtml(child.status) + '</span>';
            }

            const version = child.version ? '<span class="ref-version">v' + child.version + '</span>' : '';
            const title = child.title ? ' <span class="ref-title">' + escapeHtml(child.title) + '</span>' : '';

            childLi.innerHTML =
                '<span class="ref-toggle" onclick="expandRef(\'' + escapeHtml(project) + '\', \'' + escapeHtml(child.ref) + '\', this)">&#9654;</span>' +
                '<span class="ref-label">' + escapeHtml(child.ref) + title + version + ' ' + statusIcon + '</span>';

            ul.appendChild(childLi);
        });

        li.appendChild(ul);
    } catch (err) {
        toggleEl.innerHTML = '&#9654;';
        toggleEl.classList.remove('ref-expanded');
    }
};

// Auto-save draft to localStorage
function autosaveDraftKey() {
    const id = document.getElementById('field-id')?.value || 'new';
    return 'prd2wiki-draft-' + id;
}

function autosaveDraft() {
    const data = collectFormData('draft');
    if (data && data.body) {
        data._savedAt = Date.now();
        localStorage.setItem(autosaveDraftKey(), JSON.stringify(data));
    }
}

function clearAutosaveDraft() {
    localStorage.removeItem(autosaveDraftKey());
}

// Wire up submit buttons
document.addEventListener('DOMContentLoaded', () => {
    document.querySelectorAll('.btn-submit').forEach(btn => {
        btn.addEventListener('click', () => {
            const intent = btn.dataset.intent;
            submitPage(intent);
        });
    });

    // Initialize editor
    initMilkdown();

    // Auto-save: clear any old draft on editor load (server content is authoritative),
    // then start saving in case the browser crashes mid-edit.
    const form = document.getElementById('page-form');
    if (form) {
        clearAutosaveDraft();
        setInterval(autosaveDraft, 5000);
    }
});
