// Every screenshot in the documentation: a name (used in the Markdown as
// /assets/screenshots/<lang>/<theme>/<name>.png), the page and optional steps to get there.
// Steps click by role and accessible name in both languages, so the same list works for EN and NL.
const click = (page, en, nl, role = 'button') => page.getByRole(role, { name: new RegExp(`^(${en}|${nl})`, 'i') }).first().click()
const editRow = async (page, text) => {
  const row = page.getByRole('row').filter({ hasText: text }).first()
  await row.getByRole('button', { name: /^(Edit|Bewerken)$/ }).first().click()
}
const tab = (page, en, nl) => click(page, en, nl, 'tab')
const dialog = 'dialog[open]'

export const shots = [
  { name: 'dashboard', path: '/' },
  { name: 'extensions', path: '/extensions' },
  { name: 'extension-edit', path: '/extensions', run: (p) => editRow(p, 'Sanne Bakker'), selector: dialog },
  { name: 'ring-groups', path: '/ring-groups' },
  { name: 'ring-group-edit', path: '/ring-groups', run: (p) => editRow(p, 'Sales'), selector: dialog },
  { name: 'queues', path: '/queues' },
  { name: 'queue-edit', path: '/queues', run: (p) => editRow(p, 'Support'), selector: dialog },
  { name: 'ivr-list', path: '/ivr' },
  { name: 'ivr-canvas', path: '/ivr', run: (p) => editRow(p, 'Main menu'), wait: 1200 },
  { name: 'time-conditions', path: '/time-conditions' },
  { name: 'time-condition-edit', path: '/time-conditions', run: (p) => editRow(p, 'Office hours'), selector: dialog },
  { name: 'holidays', path: '/time-conditions', run: (p) => tab(p, 'Holidays', 'Feestdagen') },
  { name: 'conferences', path: '/conferences' },
  { name: 'prompts', path: '/prompts' },
  { name: 'trunks', path: '/trunks' },
  { name: 'trunk-edit', path: '/trunks', run: (p) => editRow(p, 'Voys'), selector: dialog },
  { name: 'phones', path: '/phones', full: true },
  { name: 'phone-keys', path: '/phones', run: (p) => editRow(p, 'Reception desk'), wait: 800 },
  { name: 'switchboard', path: '/reception', wait: 1200 },
  { name: 'wallboard', path: '/wallboard', wait: 1200 },
  { name: 'fax', path: '/fax' },
  { name: 'history', path: '/history' },
  { name: 'call-journey', path: '/history?call=1731644.1', wait: 1200, full: true },
  { name: 'call-journey-voicemail', path: '/history?call=1731640.1', wait: 1200 },
  { name: 'recordings', path: '/recordings' },
  { name: 'recording-ai', path: '/recordings', run: (p) => p.getByRole('button', { name: /AI/ }).first().click(), wait: 800 },
  { name: 'phonebook', path: '/phonebook', full: true },
  { name: 'my-phone', path: '/me', full: true },
  { name: 'voicemail', path: '/me/voicemail' },
  { name: 'my-queues', path: '/me/queues', wait: 1000 },
  { name: 'colleagues', path: '/me/colleagues', wait: 1000 },
  { name: 'my-calls', path: '/me/calls' },
  { name: 'devices', path: '/me/devices', full: true },
  { name: 'sites', path: '/sites', full: true },
  { name: 'federation', path: '/federation', full: true },
  { name: 'follow-the-sun', path: '/follow-the-sun', full: true },
  { name: 'cluster', path: '/cluster', full: true },
  { name: 'users', path: '/users', full: true },
  { name: 'sso', path: '/sso', full: true },
  { name: 'features', path: '/features', full: true },
  { name: 'firewall', path: '/firewall', full: true },
  { name: 'trusted-networks', path: '/firewall', selector: 'section.card:has(h2:has-text("Trusted networks")), section.card:has(h2:has-text("Vertrouwde netwerken"))' },
  { name: 'internet', path: '/internet', full: true },
  { name: 'integrations', path: '/integrations', full: true },
  { name: 'speech', path: '/speech', full: true },
  { name: 'ai', path: '/ai', full: true },
  { name: 'backups', path: '/backups', full: true },
  { name: 'mail', path: '/mail', full: true },
  { name: 'audit', path: '/audit' },
  { name: 'settings', path: '/settings', full: true },
  { name: 'account', path: '/account', full: true },
  // The compact pages of the Windows softphone (Kiestoon Softphone shows these in its window).
  { name: 'softphone-contacts', path: '/desktop', viewport: { width: 400, height: 720 }, wait: 1200 },
  { name: 'softphone-queues', path: '/desktop/queues', viewport: { width: 400, height: 720 }, wait: 1000 },
  { name: 'softphone-history', path: '/desktop/history', viewport: { width: 400, height: 720 } },
  { name: 'softphone-voicemail', path: '/desktop/voicemail', viewport: { width: 400, height: 720 } },
  { name: 'info', path: '/', run: (p) => click(p, 'Info & updates', 'Info & updates'), selector: dialog, wait: 800 },
]
