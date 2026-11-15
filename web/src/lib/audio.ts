// Converts any audio the browser can decode to the format Asterisk plays without transcoding:
// WAV, 8 kHz, mono, 16-bit PCM. The server only validates the result.

export type Wav = { base64: string; seconds: number; url: string }

const RATE = 8000

function encodeWav(samples: Float32Array): ArrayBuffer {
  const buf = new ArrayBuffer(44 + samples.length * 2)
  const v = new DataView(buf)
  const str = (o: number, s: string) => [...s].forEach((c, i) => v.setUint8(o + i, c.charCodeAt(0)))
  str(0, 'RIFF')
  v.setUint32(4, 36 + samples.length * 2, true)
  str(8, 'WAVE')
  str(12, 'fmt ')
  v.setUint32(16, 16, true)
  v.setUint16(20, 1, true) // PCM
  v.setUint16(22, 1, true) // mono
  v.setUint32(24, RATE, true)
  v.setUint32(28, RATE * 2, true)
  v.setUint16(32, 2, true)
  v.setUint16(34, 16, true)
  str(36, 'data')
  v.setUint32(40, samples.length * 2, true)
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]))
    v.setInt16(44 + i * 2, s < 0 ? s * 0x8000 : s * 0x7fff, true)
  }
  return buf
}

function toBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  let s = ''
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000))
  return btoa(s)
}

/** Decodes, mixes to mono, resamples to 8 kHz and normalises the peak to -1 dBFS. */
export async function toTelephoneWav(input: Blob): Promise<Wav> {
  const AC = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
  const ctx = new AC()
  let decoded: AudioBuffer
  try {
    decoded = await ctx.decodeAudioData(await input.arrayBuffer())
  } finally {
    ctx.close()
  }
  const length = Math.max(1, Math.ceil(decoded.duration * RATE))
  const off = new OfflineAudioContext(1, length, RATE)
  const src = off.createBufferSource()
  src.buffer = decoded
  src.connect(off.destination)
  src.start()
  const out = (await off.startRendering()).getChannelData(0)
  let peak = 0
  for (const s of out) peak = Math.max(peak, Math.abs(s))
  if (peak > 0.01) {
    const gain = 0.89 / peak
    for (let i = 0; i < out.length; i++) out[i] *= gain
  }
  const wav = encodeWav(out)
  return { base64: toBase64(wav), seconds: out.length / RATE, url: URL.createObjectURL(new Blob([wav], { type: 'audio/wav' })) }
}

/** Records from the microphone until stop() is called. */
export async function startRecording(): Promise<{ stop: () => Promise<Blob> }> {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } })
  const rec = new MediaRecorder(stream)
  const chunks: Blob[] = []
  rec.ondataavailable = (e) => e.data.size && chunks.push(e.data)
  rec.start()
  return {
    stop: () =>
      new Promise((resolve) => {
        rec.onstop = () => {
          stream.getTracks().forEach((t) => t.stop())
          resolve(new Blob(chunks, { type: rec.mimeType }))
        }
        rec.stop()
      }),
  }
}
