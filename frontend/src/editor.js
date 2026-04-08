import { Editor, rootCtx, defaultValueCtx } from '@milkdown/kit/core'
import { commonmark } from '@milkdown/kit/preset/commonmark'
import { gfm } from '@milkdown/kit/preset/gfm'
import { listener, listenerCtx } from '@milkdown/kit/plugin/listener'
import { upload, uploadConfig } from '@milkdown/plugin-upload'
import { getMarkdown } from '@milkdown/kit/utils'
import { nord } from '@milkdown/theme-nord'
import '@milkdown/theme-nord/style.css'

let editorInstance = null
let getMarkdownFn = null

// Custom uploader that POSTs images to the wiki attachments API
function createUploader(project, pageId) {
    return async (files, schema) => {
        const nodes = []
        for (let i = 0; i < files.length; i++) {
            const file = files.item(i)
            if (!file || !file.type.startsWith('image/')) continue

            const formData = new FormData()
            formData.append('file', file)

            try {
                const resp = await fetch(
                    `/api/projects/${encodeURIComponent(project)}/pages/${encodeURIComponent(pageId)}/attachments`,
                    { method: 'POST', body: formData }
                )
                if (!resp.ok) {
                    console.error('Upload failed:', resp.status)
                    continue
                }
                const { url } = await resp.json()
                const node = schema.nodes.image.createAndFill({ src: url, alt: file.name || 'screenshot' })
                if (node) nodes.push(node)
            } catch (err) {
                console.error('Upload error:', err)
            }
        }
        return nodes
    }
}

export async function initEditor(containerId, fallbackId, project, pageId) {
    const container = document.getElementById(containerId)
    const fallback = document.getElementById(fallbackId)
    if (!container) return false

    const initialContent = fallback ? fallback.value : ''

    // Don't enable upload if we don't have a page ID yet (new unsaved page)
    const hasUpload = project && pageId

    try {
        const builder = Editor.make()
            .config(nord)
            .config((ctx) => {
                ctx.set(rootCtx, container)
                ctx.set(defaultValueCtx, initialContent)
                if (hasUpload) {
                    ctx.set(uploadConfig.key, {
                        uploader: createUploader(project, pageId),
                        enableHtmlFileUploader: true,
                    })
                }
            })
            .use(commonmark)
            .use(gfm)
            .use(listener)

        if (hasUpload) {
            builder.use(upload)
        }

        const editor = await builder.create()

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
