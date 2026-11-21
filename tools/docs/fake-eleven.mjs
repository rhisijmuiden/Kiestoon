// A small stand-in for the ElevenLabs API, so the demo office can show the ElevenLabs settings and the
// voice picker without a real account. Usage: node fake-eleven.mjs <port>
// Voices and numbers are made up. Text-to-speech returns half a second of a soft tone (16 kHz PCM).
import http from 'node:http'

const port = Number(process.argv[2] ?? 9300)
const base = `http://127.0.0.1:${port}`
const voice = (id, name, language, gender, age, accent, useCase, descriptive, description) =>
  ({ voice_id: id, public_owner_id: 'owner' + id, name, language, gender, age, accent, use_case: useCase, descriptive, description, preview_url: `${base}/p/${id}.mp3` })
const library = [
  voice('ruthNL01', 'Ruth', 'nl', 'female', 'middle_aged', 'standard', 'conversational', 'warm', 'Warm and professional, made for customer contact.'),
  voice('daanNL02', 'Daan', 'nl', 'male', 'middle_aged', 'standard', 'informative_educational', 'calm', 'Calm, clear business voice.'),
  voice('sanneNL3', 'Sanne', 'nl', 'female', 'young', 'standard', 'conversational', 'friendly', 'Friendly and bright.'),
  voice('pieterB4', 'Pieter', 'nl', 'male', 'middle_aged', 'flemish', 'narrative_story', 'confident', 'Flemish narrator with a steady pace.'),
  voice('lotteBE5', 'Lotte', 'nl', 'female', 'young', 'flemish', 'conversational', 'soft', 'Soft Flemish voice.'),
  voice('jorisNL6', 'Joris', 'nl', 'male', 'young', 'standard', 'advertisement', 'casual', 'Energetic and informal.'),
  voice('graceEN7', 'Grace', 'en', 'female', 'middle_aged', 'british', 'conversational', 'professional', 'Polished British voice.'),
  voice('oliverE8', 'Oliver', 'en', 'male', 'middle_aged', 'american', 'narrative_story', 'deep', 'Deep and steady.'),
]
const mine = new Set(['ruthNL01', 'graceEN7'])

function tone() {
  const n = 8000, b = Buffer.alloc(n * 2)
  for (let i = 0; i < n; i++) b.writeInt16LE(Math.round(Math.sin(i / 8) * 3000 * Math.sin((Math.PI * i) / n)), i * 2)
  return b
}

http.createServer((req, res) => {
  const u = new URL(req.url, base)
  const json = (v, code = 200) => { res.writeHead(code, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(v)) }
  if (u.pathname.startsWith('/p/')) { res.writeHead(404); return res.end() }
  if (!req.headers['xi-api-key']) return json({ detail: { status: 'invalid_api_key', message: 'Invalid API key' } }, 401)
  if (u.pathname === '/v1/user/subscription') return json({ tier: 'creator', status: 'active', character_count: 18420, character_limit: 100000, next_character_count_reset_unix: Math.floor(new Date('2026-12-01T00:00:00Z') / 1000) })
  if (u.pathname === '/v2/voices') {
    return json({ has_more: false, voices: library.filter((v) => mine.has(v.voice_id)).map((v) => ({ voice_id: v.voice_id, name: v.name, category: 'professional', description: v.description, preview_url: v.preview_url,
      labels: { gender: v.gender, age: v.age, accent: v.accent, use_case: v.use_case, descriptive: v.descriptive, language: v.language } })) })
  }
  if (u.pathname === '/v1/shared-voices') {
    const lang = u.searchParams.get('language'), gender = u.searchParams.get('gender'), q = (u.searchParams.get('search') ?? '').toLowerCase()
    return json({ has_more: false, voices: library.filter((v) => (!lang || v.language === lang) && (!gender || v.gender === gender) && (!q || (v.name + v.description).toLowerCase().includes(q)))
      .map((v) => ({ ...v, is_added_by_user: mine.has(v.voice_id) })) })
  }
  if (u.pathname.startsWith('/v1/voices/add/')) { const id = u.pathname.split('/').pop(); mine.add(id); return json({ voice_id: id }) }
  if (u.pathname.startsWith('/v1/voices/')) { const v = library.find((x) => x.voice_id === u.pathname.split('/').pop()); return v ? json({ voice_id: v.voice_id, name: v.name, preview_url: v.preview_url, labels: {} }) : json({ detail: 'not found' }, 404) }
  if (u.pathname.startsWith('/v1/text-to-speech/')) { res.writeHead(200, { 'Content-Type': 'application/octet-stream' }); return res.end(tone()) }
  json({ detail: 'not found' }, 404)
}).listen(port, '127.0.0.1')
