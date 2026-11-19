// The first screen: link the softphone to a Kiestoon server.
;(() => {
  const { t, q, invoke, err } = window.KT
  const $ = (id) => document.getElementById(id)
  document.querySelectorAll('[data-t]').forEach((el) => { el.textContent = t[el.dataset.t] })
  const server = $('server'), code = $('code'), error = $('error'), untrusted = $('untrusted'), trust = $('trust'), btn = $('connect')
  let probed = null

  const showError = (e) => {
    error.textContent = e ? err(e) : ''
    error.classList.toggle('hidden', !e)
  }
  const busy = (on, label) => {
    btn.disabled = on
    btn.innerHTML = on ? `<span class="spin"></span><span>${t.checking}</span>` : `<span>${label || (probed && !probed.trusted ? t.trustConnect : t.connect)}</span>`
  }

  invoke('setup_info').then((i) => {
    $('version').textContent = `Kiestoon Softphone ${i.version}`
    if (!server.value) server.value = q.get('server') || i.server || ''
  })
  if (q.get('server')) server.value = q.get('server')
  if (q.get('code')) code.value = q.get('code')
  if (q.get('error')) showError(q.get('error'))
  server.addEventListener('input', () => { probed = null; untrusted.classList.add('hidden'); busy(false) })

  $('form').addEventListener('submit', async (e) => {
    e.preventDefault()
    showError('')
    const url = server.value.trim()
    if (!url) return showError('empty')
    busy(true)
    try {
      if (!probed) {
        probed = await invoke('probe_server', { url })
        if (!probed.trusted) {
          $('fp').textContent = probed.fingerprint
          untrusted.classList.remove('hidden')
          trust.checked = false
          busy(false)
          return
        }
      }
      if (!probed.trusted && !trust.checked) {
        busy(false)
        return showError('untrusted')
      }
      await invoke('connect', { url, accepted: probed.trusted ? '' : probed.fingerprint, code: code.value.trim() })
    } catch (x) {
      probed = null
      untrusted.classList.add('hidden')
      showError(x)
      busy(false)
    }
  })

  $('browser').addEventListener('click', async () => {
    showError('')
    const url = server.value.trim()
    if (!url) return showError('empty')
    try {
      await invoke('browser_login', { url })
    } catch (x) {
      showError(x)
    }
  })
})()
