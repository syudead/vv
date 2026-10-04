<!-- Renders a ```mermaid block in the browser. The block arrives URI-encoded
     from the markdown fence rule in config.mts; mermaid loads only on pages that
     have a diagram, and the diagram is redrawn when the colour scheme changes. -->
<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useData } from 'vitepress'

const props = defineProps<{ code: string }>()
const { isDark } = useData()
const host = ref<HTMLElement>()
let count = 0

async function draw() {
  const { default: mermaid } = await import('mermaid')
  mermaid.initialize({ startOnLoad: false, theme: isDark.value ? 'dark' : 'default', securityLevel: 'strict' })
  const id = `mermaid-${Math.random().toString(36).slice(2)}-${count++}`
  try {
    const { svg } = await mermaid.render(id, decodeURIComponent(props.code))
    if (host.value) host.value.innerHTML = svg
  } catch (e) {
    if (host.value) host.value.textContent = String(e)
  }
}

onMounted(draw)
watch(isDark, draw)
</script>

<template>
  <div ref="host" class="mermaid-diagram"></div>
</template>

<style scoped>
.mermaid-diagram {
  margin: 16px 0;
  overflow-x: auto;
  text-align: center;
}
</style>
