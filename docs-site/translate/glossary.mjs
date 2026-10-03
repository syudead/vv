// The glossary maps an English term to its one Japanese rendering. UI labels
// map to themselves: the screen is English, and the translation uses the word
// the screen shows.
import fs from 'node:fs'

export function loadGlossary(file) {
  const entries = []
  for (const line of fs.readFileSync(file, 'utf8').split('\n')) {
    if (!line.trim() || line.startsWith('#')) continue
    const [en, ja] = line.split('\t')
    if (en && ja) entries.push({ en: en.trim(), ja: ja.trim() })
  }
  // Longer terms first, so "tentative tag" wins over "tag".
  return entries.sort((x, y) => y.en.length - x.en.length)
}

// A term that maps to itself is a UI label ("Versions"): it matches only with
// the screen's capitalisation, so the ordinary word stays translatable.
function pattern(entry) {
  const flags = entry.en === entry.ja ? '' : 'i'
  return new RegExp(`(?<![A-Za-z])${entry.en.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}(?![A-Za-z])`, flags)
}

// termsFor returns the glossary entries whose English term occurs in text.
export function termsFor(glossary, text) {
  const found = []
  let rest = text
  for (const entry of glossary) {
    const re = pattern(entry)
    if (re.test(rest)) {
      found.push(entry)
      rest = rest.replace(new RegExp(re.source, re.flags + 'g'), ' ')
    }
  }
  return found
}

// missedTerms returns the entries whose Japanese rendering is absent from a
// translation.
export function missedTerms(terms, translation) {
  return terms.filter((t) => !translation.includes(t.ja))
}
