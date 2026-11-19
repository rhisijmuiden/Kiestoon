// A voice prompt as an 8 kHz mono WAV: a soft tone with a speech-like envelope, so the waveform looks real.
export function wav(seconds, seed) {
  const rate = 8000, n = rate * seconds
  const data = Buffer.alloc(n * 2)
  let x = seed
  const rnd = () => ((x = (x * 1103515245 + 12345) % 2147483648) / 2147483648)
  let env = 0
  for (let i = 0; i < n; i++) {
    const syll = Math.sin((i / rate) * Math.PI * (3 + (seed % 3))) ** 2
    const pause = Math.sin((i / rate) * Math.PI * 0.45) > -0.3 ? 1 : 0.05
    env += ((syll * pause) - env) * 0.002
    const v = env * (0.55 * Math.sin(i * 0.09) + 0.3 * Math.sin(i * 0.23) + 0.25 * (rnd() - 0.5))
    data.writeInt16LE(Math.max(-32767, Math.min(32767, Math.round(v * 22000))), i * 2)
  }
  const h = Buffer.alloc(44)
  h.write('RIFF', 0); h.writeUInt32LE(36 + data.length, 4); h.write('WAVE', 8); h.write('fmt ', 12)
  h.writeUInt32LE(16, 16); h.writeUInt16LE(1, 20); h.writeUInt16LE(1, 22); h.writeUInt32LE(rate, 24); h.writeUInt32LE(rate * 2, 28)
  h.writeUInt16LE(2, 32); h.writeUInt16LE(16, 34); h.write('data', 36); h.writeUInt32LE(data.length, 40)
  return Buffer.concat([h, data]).toString('base64')
}
