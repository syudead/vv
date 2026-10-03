// The default theme, plus a notice above Japanese pages whose translation is
// out of date or missing.
import DefaultTheme from 'vitepress/theme'
import { h } from 'vue'
import TranslationNotice from './TranslationNotice.vue'

export default {
  extends: DefaultTheme,
  Layout: () => h(DefaultTheme.Layout, null, { 'doc-before': () => h(TranslationNotice) }),
}
