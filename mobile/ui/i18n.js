// Texts of the setup screen, in English and Dutch.
window.KT = (() => {
  const texts = {
    en: {
      title: 'Welcome to Kiestoon',
      intro: 'In the Kiestoon portal, open My devices, choose Link mobile app and scan the QR code with the camera of this phone. Or enter the address and the code here.',
      server: 'Server address', code: 'Pairing code', codeHint: 'The code under the QR code in the portal. It works for 10 minutes.',
      connect: 'Link', checking: 'Linking…',
      errors: {
        empty: 'Enter the address of your phone system.', invalid: 'This is not a valid address.',
        https_required: 'Use an https address.', unreachable: 'The server cannot be reached. Check the address and your connection.',
        not_kiestoon: 'There is no Kiestoon phone system at this address.', generic: 'Linking failed: {e}',
      },
    },
    nl: {
      title: 'Welkom bij Kiestoon',
      intro: 'Open in het Kiestoon-portaal Mijn apparaten, kies Mobiele app koppelen en scan de QR-code met de camera van deze telefoon. Of vul hier het adres en de code in.',
      server: 'Serveradres', code: 'Koppelcode', codeHint: 'De code onder de QR-code in het portaal. Hij werkt 10 minuten.',
      connect: 'Koppelen', checking: 'Koppelen…',
      errors: {
        empty: 'Vul het adres van je telefooncentrale in.', invalid: 'Dit is geen geldig adres.',
        https_required: 'Gebruik een https-adres.', unreachable: 'De server is niet bereikbaar. Controleer het adres en je verbinding.',
        not_kiestoon: 'Op dit adres staat geen Kiestoon-telefooncentrale.', generic: 'Koppelen lukte niet: {e}',
      },
    },
  }
  const q = new URLSearchParams(location.search)
  const lang = (navigator.language || 'en').toLowerCase().startsWith('nl') ? 'nl' : 'en'
  document.documentElement.lang = lang
  const t = texts[lang]
  const invoke = (cmd, args) => window.__TAURI__.core.invoke(cmd, args)
  const err = (e) => {
    const code = String(e)
    return t.errors[code] || t.errors.generic.replace('{e}', code)
  }
  return { t, lang, q, invoke, err }
})()
