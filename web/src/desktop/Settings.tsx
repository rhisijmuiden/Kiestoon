import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BellRing, Headphones, LogOut, Unlink } from 'lucide-react'
import { LanguageSwitch, ThemeSwitch } from '../components/brand'
import { Select } from '../components/kit'
import { Button } from '../components/ui'
import { useSession } from '../lib/session'
import { audioKeys, audioPref, findHeadset, refreshHeadset } from '../lib/softphone'
import { inShell, shell, type Headset } from './bridge'
import { Title } from './Contacts'
import { Switch } from './Switch'

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-3 rounded-2xl border border-line bg-surface p-3.5">
      <h2 className="text-[10.5px] uppercase tracking-[0.08em] text-subtle">{title}</h2>
      {children}
    </section>
  )
}

/** Whether a HID name and a sound device label belong to the same headset ("Logitech Zone Wired" and "Headset (Zone Wired)"). */
function sameDevice(hid: string, label: string) {
  const words = (x: string) => x.toLowerCase().replace(/\(.*?\)/g, (m) => ` ${m} `).split(/[^a-z0-9]+/).filter((w) => w.length > 2 && !['usb', 'headset', 'logitech', 'microphone', 'speakers', 'hands', 'free'].includes(w))
  const a = words(hid)
  return a.length > 0 && a.some((w) => words(label).includes(w))
}

/** Plays a short ring on the chosen device. */
async function testRing(sink: string) {
  const ctx = new AudioContext()
  if (sink) await (ctx as unknown as { setSinkId?: (id: string) => Promise<void> }).setSinkId?.(sink).catch(() => undefined)
  const o = ctx.createOscillator()
  const g = ctx.createGain()
  o.frequency.value = 440
  o.connect(g)
  g.connect(ctx.destination)
  g.gain.setValueAtTime(0.08, ctx.currentTime)
  o.start()
  o.stop(ctx.currentTime + 0.9)
  window.setTimeout(() => ctx.close(), 1200)
}

export function Settings() {
  const { t } = useTranslation()
  const { signOut } = useSession()
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([])
  const [prefs, setPrefs] = useState({ mic: audioPref('mic'), speaker: audioPref('speaker'), ringer: audioPref('ringer') })
  const [autostart, setAutostart] = useState<boolean | null>(null)
  const [headsets, setHeadsets] = useState<Headset[] | null>(null)

  useEffect(() => {
    const load = async () => {
      try {
        // Labels are only shown after the microphone was allowed once.
        const list = await navigator.mediaDevices.enumerateDevices()
        if (!list.some((d) => d.label)) {
          const s = await navigator.mediaDevices.getUserMedia({ audio: true }).catch(() => null)
          s?.getTracks().forEach((tr) => tr.stop())
          setDevices(await navigator.mediaDevices.enumerateDevices())
        } else setDevices(list)
        refreshHeadset()
      } catch {
        setDevices([])
      }
    }
    load()
    navigator.mediaDevices?.addEventListener('devicechange', load)
    shell.info().then((i) => setAutostart(i ? i.autostart : null)).catch(() => undefined)
    const scan = () => shell.headsets().then(setHeadsets).catch(() => undefined)
    scan()
    // The shell looks for headset buttons every few seconds; follow it when one is plugged in.
    const timer = window.setInterval(scan, 3000)
    return () => {
      window.clearInterval(timer)
      navigator.mediaDevices?.removeEventListener('devicechange', load)
    }
  }, [])

  const set = (k: keyof typeof audioKeys, v: string) => {
    setPrefs((p) => ({ ...p, [k]: v }))
    try {
      if (v) localStorage.setItem(audioKeys[k], v)
      else localStorage.removeItem(audioKeys[k])
    } catch {
      /* ignore */
    }
  }
  const inputs = devices.filter((d) => d.kind === 'audioinput' && d.deviceId !== 'default' && d.deviceId !== 'communications')
  const outputs = devices.filter((d) => d.kind === 'audiooutput' && d.deviceId !== 'default' && d.deviceId !== 'communications')
  // A headset Windows only knows as a sound device (no call buttons, or Bluetooth).
  const audioHeadset = findHeadset(devices)
  const hidList = headsets ?? []
  const soundOnly = audioHeadset && !hidList.some((h) => sameDevice(h.name, audioHeadset.mic.label)) ? audioHeadset.mic.label : ''
  const usesHeadset = !!audioHeadset && !prefs.mic && !prefs.speaker
  const opts = (list: MediaDeviceInfo[]) => [
    <option key="" value="">{t('desktop.settings.systemDefault')}</option>,
    ...list.map((d, i) => <option key={d.deviceId} value={d.deviceId}>{d.label || `${t('desktop.settings.device')} ${i + 1}`}</option>),
  ]

  return (
    <div className="pb-4">
      <Title>{t('desktop.nav.settings')}</Title>
      <div className="flex flex-col gap-3 px-3.5">
        <Section title={t('desktop.settings.audio')}>
          <Select label={t('desktop.settings.mic')} value={prefs.mic} onChange={(e) => set('mic', e.target.value)}>{opts(inputs)}</Select>
          <Select label={t('desktop.settings.speaker')} value={prefs.speaker} onChange={(e) => set('speaker', e.target.value)}>{opts(outputs)}</Select>
          <div className="flex items-end gap-2">
            <div className="min-w-0 flex-1"><Select label={t('desktop.settings.ringer')} value={prefs.ringer} onChange={(e) => set('ringer', e.target.value)}>{opts(outputs)}</Select></div>
            <Button variant="ghost" onClick={() => testRing(prefs.ringer)} aria-label={t('desktop.settings.testRing')} title={t('desktop.settings.testRing')}><BellRing className="size-4" aria-hidden /></Button>
          </div>
          <p className="text-xs text-subtle">{t('desktop.settings.audioHint')}</p>
        </Section>

        {inShell() && (
          <Section title={t('desktop.settings.headset')}>
            {hidList.length === 0 && !soundOnly
              ? <p className="text-sm text-muted">{t('desktop.settings.noHeadset')}</p>
              : [...hidList, ...(soundOnly ? [{ name: soundOnly, vendor: '', buttons: false }] : [])].map((h) => (
                <div key={h.name} className="flex items-center gap-2.5 text-sm">
                  <Headphones className="size-4 text-accent" aria-hidden />
                  <span className="min-w-0 flex-1 truncate text-ink">{h.name}</span>
                  <span className={`text-xs ${h.buttons ? 'text-ok' : 'text-subtle'}`}>{h.buttons ? t('desktop.settings.buttonsOn') : t('desktop.settings.buttonsOff')}</span>
                </div>
              ))}
            {usesHeadset && <p className="text-xs text-ok">{t('desktop.settings.headsetAuto')}</p>}
            <p className="text-xs text-subtle">{t('desktop.settings.headsetHint')}</p>
          </Section>
        )}

        <Section title={t('desktop.settings.app')}>
          {autostart !== null && (
            <div className="flex items-center gap-3">
              <span className="flex-1 text-sm text-ink">{t('desktop.settings.autostart')}</span>
              <Switch checked={autostart} label={t('desktop.settings.autostart')} onChange={(v) => { setAutostart(v); shell.setAutostart(v) }} />
            </div>
          )}
          <div className="flex flex-wrap items-center gap-2"><ThemeSwitch persist /><LanguageSwitch persist /></div>
        </Section>

        <div className="grid grid-cols-2 gap-2">
          <Button variant="ghost" className="min-h-9 text-[12.5px]" onClick={signOut}><LogOut className="size-4" aria-hidden />{t('common.signOut')}</Button>
          {inShell() && <Button variant="danger" className="min-h-9 text-[12.5px]" onClick={async () => { await signOut(); shell.unpair() }}><Unlink className="size-4" aria-hidden />{t('desktop.settings.unlink')}</Button>}
        </div>
      </div>
    </div>
  )
}
