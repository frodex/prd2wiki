// prd2wiki editor — wiki.js

let editorInstance = null;
let getMarkdownFn = null;
let usingFallback = false;

async function initMilkdown() {
    const container = document.getElementById('milkdown-editor');
    const fallback = document.getElementById('editor-fallback');
    if (!container) return;

    const initialContent = fallback ? fallback.value : '';

    try {
        const [coreModule, commonmarkModule, gfmModule, listenerModule, nordModule, utilsModule] = await Promise.all([
            import('https://esm.sh/@milkdown/kit@7/core'),
            import('https://esm.sh/@milkdown/kit@7/preset/commonmark'),
            import('https://esm.sh/@milkdown/kit@7/preset/gfm'),
            import('https://esm.sh/@milkdown/kit@7/plugin/listener'),
            import('https://esm.sh/@milkdown/kit@7/theme/nord'),
            import('https://esm.sh/@milkdown/kit@7/utils'),
        ]);

        const { Editor, rootCtx, defaultValueCtx } = coreModule;
        const { commonmark } = commonmarkModule;
        const { gfm } = gfmModule;
        const { listener, listenerCtx } = listenerModule;
        const { nord } = nordModule;
        const { getMarkdown } = utilsModule;

        const editor = await Editor.make()
            .config(nord)
            .config((ctx) => {
                ctx.set(rootCtx, container);
                ctx.set(defaultValueCtx, initialContent);
            })
            .use(commonmark)
            .use(gfm)
            .use(listener)
            .create();

        editorInstance = editor;
        getMarkdownFn = () => editor.action(getMarkdown());
        container.style.display = '';
        console.log('Milkdown editor initialized');
    } catch (err) {
        console.warn('Milkdown failed to load, falling back to textarea:', err);
        usingFallback = true;
        container.style.display = 'none';
        if (fallback) {
            fallback.style.display = '';
        }
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
        frontmatter: {
            id: form.querySelector('#field-id').value.trim(),
            title: form.querySelector('#field-title').value.trim(),
            type: form.querySelector('#field-type').value,
            status: form.querySelector('#field-status').value,
            tags: tags,
        },
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
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

async function submitPage(intent) {
    const data = collectFormData(intent);
    if (!data) return;

    if (!data.frontmatter.id) {
        showResults('ID is required.', true);
        return;
    }
    if (!data.frontmatter.title) {
        showResults('Title is required.', true);
        return;
    }

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
            showResults(result.error || ('HTTP ' + resp.status), true);
            return;
        }

        showResults(result, false);

        // On success, redirect to page view after a short delay
        if (result.page_id || result.path) {
            const pageId = result.page_id || data.frontmatter.id;
            setTimeout(() => {
                window.location.href = '/projects/' + encodeURIComponent(project) + '/pages/' + encodeURIComponent(pageId);
            }, 1500);
        }
    } catch (err) {
        showResults('Network error: ' + err.message, true);
    }
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
});
