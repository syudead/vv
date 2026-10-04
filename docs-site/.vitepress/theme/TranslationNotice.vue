<script setup lang="ts">
import { computed } from 'vue'
import { useData, withBase } from 'vitepress'

const { frontmatter } = useData()

const status = computed(() => frontmatter.value.translation as string | undefined)
const english = computed(() => {
  const source = (frontmatter.value.sourcePath as string | undefined) ?? ''
  return withBase('/' + source.replace(/(^|\/)README\.md$/, '$1').replace(/\.md$/, ''))
})
</script>

<template>
  <div v-if="status === 'stale'" class="custom-block warning translation-notice">
    <p>
      この和訳は英語版の最新の内容に追いついていません。次の翻訳で更新されます。最新の内容は
      <a :href="english">英語版</a>を読んでください。
    </p>
  </div>
  <div v-else-if="status === 'untranslated'" class="custom-block info translation-notice">
    <p>
      この文書はまだ翻訳されていないため、英語のまま表示しています。<a :href="english">英語版</a>
    </p>
  </div>
</template>

<style scoped>
.translation-notice {
  margin-bottom: 24px;
}
</style>
