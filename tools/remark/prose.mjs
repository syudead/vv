import path from 'node:path'
import {visit} from 'unist-util-visit'

/** Returns the file's path relative to its working directory, with `/` separators. */
export function relativePath(file) {
  const absolute = path.resolve(file.cwd, file.path)
  return path.relative(file.cwd, absolute).split(path.sep).join('/')
}

/**
 * Returns the source lines with code blocks and inline code blanked, keeping
 * line numbers. Quoted text in code is not prose, so prose rules skip it.
 */
export function proseLines(tree, file) {
  // Node offsets count UTF-16 code units, so split by code unit.
  const units = String(file).split('')
  visit(tree, ['code', 'inlineCode'], function (node) {
    const start = node.position?.start.offset
    const end = node.position?.end.offset
    if (start === undefined || end === undefined) return
    for (let i = start; i < end; i++) {
      if (units[i] !== '\n') units[i] = ' '
    }
  })
  return units.join('').split('\n')
}
