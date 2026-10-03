// Engines turn one segment into Japanese. The llama engine talks to a running
// llama-server and uses the prompt format of the model family named in
// model.json. The fake engine is for tests.

const hasTags = /<s\d+>/

function hyPrompt(text, terms) {
  const ref = terms.length
    ? `参考下面的翻译：\n${terms.map((t) => `${t.en} 翻译成 ${t.ja}`).join('\n')}\n\n`
    : ''
  if (hasTags.test(text)) {
    return (
      ref +
      '将以下<source></source>之间的文本翻译为日语，注意只需要输出翻译后的结果，不要额外解释，' +
      '原文中的<sn></sn>标签表示标签内文本包含格式信息，需要在译文中相应的位置尽量保留该标签。' +
      `输出格式为：<target>str</target>\n\n<source>${text}</source>`
    )
  }
  if (terms.length) return `${ref}将以下文本翻译为日语，注意只需要输出翻译后的结果，不要额外解释：\n${text}`
  return `Translate the following segment into Japanese, without additional explanation.\n\n${text}`
}

function genericPrompt(text, terms) {
  const rules = []
  if (terms.length) rules.push(`Use these term translations: ${terms.map((t) => `${t.en} -> ${t.ja}`).join('; ')}.`)
  if (hasTags.test(text)) rules.push('Keep every <sN>...</sN> tag around the corresponding words.')
  return `Translate the following text from English to Japanese. Output only the translation.\n${rules.join('\n')}\n\n${text}`
}

function unwrapTarget(out) {
  const m = out.match(/<target>([\s\S]*?)(<\/target>|$)/)
  return (m ? m[1] : out).trim()
}

export function llamaEngine({ server, family }) {
  async function post(path, body) {
    const res = await fetch(server + path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error(`${path}: HTTP ${res.status} ${await res.text()}`)
    return res.json()
  }
  return {
    // attempt > 0 is a retry after a rejected output: sample greedily.
    async translate(text, terms, { attempt = 0 } = {}) {
      if (family === 'plamo') {
        const prompt =
          '<|plamo:op|>dataset\ntranslation\n' +
          `<|plamo:op|>input lang=English\n${text}\n<|plamo:op|>output lang=Japanese\n`
        const r = await post('/completion', { prompt, n_predict: 1024, temperature: 0, stop: ['<|plamo:op|>'] })
        return { text: r.content.trim(), tokens: r.tokens_predicted ?? 0 }
      }
      const prompt = family === 'hy' ? hyPrompt(text, terms) : genericPrompt(text, terms)
      const params =
        family === 'hy'
          ? { temperature: attempt > 0 ? 0 : 0.7, top_p: 0.6, top_k: 20, repeat_penalty: 1.05 }
          : { temperature: 0 }
      const r = await post('/v1/chat/completions', {
        messages: [{ role: 'user', content: prompt }],
        max_tokens: 1024,
        ...params,
      })
      return { text: unwrapTarget(r.choices[0].message.content), tokens: r.usage?.completion_tokens ?? 0 }
    },
  }
}

// fakeEngine "translates" by prefixing each word run with a marker and
// keeping the tags. Options let a test drop tags or fail.
export function fakeEngine({ dropTags = false, fail = false, addCode = false } = {}) {
  let calls = 0
  return {
    get calls() {
      return calls
    },
    async translate(text, terms) {
      calls++
      if (fail) throw new Error('engine unavailable')
      let out = text.replace(/(^|>)([^<]+)/g, (_, a, b) => `${a}訳:${b}`)
      for (const t of terms) out += ` ${t.ja}`
      if (dropTags) out = out.replace(/<\/?s\d+>/g, '')
      if (addCode) out += ' `extra`'
      return { text: out, tokens: out.length }
    },
  }
}
