;(() => {
  const { t, q, invoke } = window.KT
  document.querySelectorAll('[data-t]').forEach((el) => { el.textContent = t[el.dataset.t] })
  const name = q.get('name') || '', number = q.get('number') || '', via = q.get('via') || ''
  const parts = (name || number || '?').replace(/"/g, '').trim().split(/\s+/)
  document.getElementById('av').textContent = (parts.length > 1 ? parts[0][0] + parts[parts.length - 1][0] : parts[0].slice(0, 2)).toUpperCase()
  document.getElementById('name').textContent = name || number || t.unknown
  document.getElementById('sub').textContent = [name ? number : '', via ? t.via.replace('{q}', via) : ''].filter(Boolean).join(' · ')
  document.getElementById('answer').addEventListener('click', () => invoke('incoming_action', { action: 'answer' }))
  document.getElementById('decline').addEventListener('click', () => invoke('incoming_action', { action: 'decline' }))
})()
