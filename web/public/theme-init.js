// Applies the saved theme before the app loads, so the page never flashes the wrong colours.
// Kept in sync with src/lib/theme.ts.
(function () {
  var pref = 'system'
  try {
    var saved = localStorage.getItem('kiestoon.theme')
    if (saved === 'dark' || saved === 'light' || saved === 'system') pref = saved
  } catch (e) { /* storage blocked */ }
  var light = pref === 'light' || (pref === 'system' && window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches)
  document.documentElement.dataset.theme = light ? 'light' : 'dark'
})()
