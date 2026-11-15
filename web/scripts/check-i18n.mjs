// Fails when English and Dutch translations do not have exactly the same keys.
import { readFileSync } from 'node:fs'
const load = (l) => JSON.parse(readFileSync(new URL(`../src/i18n/${l}.json`, import.meta.url)))
const keys = (o, p = '') => Object.entries(o).flatMap(([k, v]) => (v && typeof v === 'object' ? keys(v, `${p}${k}.`) : [`${p}${k}`]))
const en = new Set(keys(load('en'))), nl = new Set(keys(load('nl')))
const missing = [...en].filter((k) => !nl.has(k)).map((k) => `nl is missing ${k}`)
  .concat([...nl].filter((k) => !en.has(k)).map((k) => `en is missing ${k}`))
if (missing.length) { console.error(missing.join('\n')); process.exit(1) }
console.log(`i18n ok: ${en.size} keys in en and nl`)
