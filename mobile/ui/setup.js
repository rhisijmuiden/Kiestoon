// The first screen: link the app to a Kiestoon server. The QR code in the portal opens
// kiestoon://pair?server=…&code=…, which brings the app here with auto=1.
;(() => {
  const { t, q, invoke, err } = window.KT
  const $ = (id) => document.getElementById(id)
  document.querySelectorAll('[data-t]').forEach((el) => { el.textContent = t[el.dataset.t] })
  const server = $('server'), code = $('code'), error = $('error'), btn = $('connect')

  const showError = (e) => {
    error.textContent = e ? err(e) : ''
    error.classList.toggle('hidden', !e)
  }
  const busy = (on) => {
    btn.disabled = on
    btn.innerHTML = on ? `<span class="spin"></span><span>${t.checking}</span>` : `<span>${t.connect}</span>`
  }
  const link = async () => {
    showError('')
    if (!server.value.trim()) return showError('empty')
    busy(true)
    try {
      await invoke('connect', { url: server.value, code: code.value.trim() })
    } catch (x) {
      showError(x)
      busy(false)
    }
  }

  invoke('setup_info').then((i) => {
    $('version').textContent = `Kiestoon ${i.version}`
    if (!server.value) server.value = (i.server || '').replace(/^https:\/\//, '')
  })
  if (q.get('server')) server.value = q.get('server').replace(/^https:\/\//, '')
  if (q.get('code')) code.value = q.get('code')
  $('form').addEventListener('submit', (e) => { e.preventDefault(); link() })
  if (q.get('auto') === '1' && q.get('server') && q.get('code')) link()
})()
