// The default theme, plus a notice above Japanese pages whose translation is
// out of date or missing, and Mermaid diagrams drawn in the browser.
import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import { h } from 'vue'
import MermaidDiagram from './MermaidDiagram.vue'
import TranslationNotice from './TranslationNotice.vue'

export default {
  extends: DefaultTheme,
  Layout: () => h(DefaultTheme.Layout, null, { 'doc-before': () => h(TranslationNotice) }),
  enhanceApp({ app }) {
    app.component('MermaidDiagram', MermaidDiagram)
  },
} satisfies Theme
