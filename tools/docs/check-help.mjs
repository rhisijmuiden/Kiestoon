// Checks that every Help link of the portal (web/src/components/help.tsx) points to a page and section
// that exist in the built manual, in English and in Dutch.
// Usage: node tools/docs/check-help.mjs <built site dir>
import { existsSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const site = process.argv[2]
if (!site) {
  console.error('usage: check-help.mjs <site dir>')
  process.exit(2)
}
const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '../../web/src/components/help.tsx'), 'utf8')
const re = /'([^']+)':\s*\{\s*page:\s*'([^']*)'(?:,\s*en:\s*'([^']+)')?(?:,\s*nl:\s*'([^']+)')?\s*\}/g
let count = 0
const errors = []
for (const [, path, page, en, nl] of src.matchAll(re)) {
  for (const [lang, section] of [['en', en], ['nl', nl]]) {
    count++
    const file = join(site, lang === 'nl' ? 'nl' : '', page, 'index.html')
    if (!existsSync(file)) {
      errors.push(`${path} (${lang}): page ${file} does not exist`)
      continue
    }
    if (section && !readFileSync(file, 'utf8').includes(`id="${section}"`)) errors.push(`${path} (${lang}): section #${section} not found in ${file}`)
  }
}
if (count < 40) errors.push(`only ${count} help links found; did the format of help.tsx change?`)
for (const e of errors) console.error(e)
if (errors.length) process.exit(1)
console.log(`help links ok: ${count} links`)
