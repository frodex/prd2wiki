import { Editor, rootCtx, defaultValueCtx } from '@milkdown/kit/core'
import { commonmark } from '@milkdown/kit/preset/commonmark'
import { gfm } from '@milkdown/kit/preset/gfm'
import { listener, listenerCtx } from '@milkdown/kit/plugin/listener'
import { getMarkdown } from '@milkdown/kit/utils'
import { nord } from '@milkdown/theme-nord'
import '@milkdown/theme-nord/style.css'

let editorInstance = null
let getMarkdownFn = null

export async function initEditor(containerId, fallbackId) {
    const container = document.getElementById(containerId)
    const fallback = document.getElementById(fallbackId)
    if (!container) return false

    const initialContent = fallback ? fallback.value : ''

    try {
        const editor = await Editor.make()
            .config(nord)
            .config((ctx) => {
                ctx.set(rootCtx, container)
                ctx.set(defaultValueCtx, initialContent)
            })
            .use(commonmark)
            .use(gfm)
            .use(listener)
            .create()

        editorInstance = editor
        getMarkdownFn = () => editor.action(getMarkdown())

        // Hide fallback textarea, show editor
        if (fallback) fallback.style.display = 'none'
        container.style.display = ''

        return true
    } catch (err) {
        console.error('Milkdown init failed:', err)
        // Show fallback
        container.style.display = 'none'
        if (fallback) fallback.style.display = ''
        return false
    }
}

export function getEditorContent() {
    if (getMarkdownFn) {
        return getMarkdownFn()
    }
    const ta = document.getElementById('editor-fallback')
    return ta ? ta.value : ''
}

// Expose globally for wiki.js to call
window.prd2wikiEditor = { initEditor, getEditorContent }
