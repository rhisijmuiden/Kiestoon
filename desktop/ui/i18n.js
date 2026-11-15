// Texts of the app's own pages (setup screen and incoming-call popup), in English and Dutch.
window.KT = (() => {
  const texts = {
    en: {
      title: 'Welcome to Kiestoon', intro: 'In the Kiestoon portal, open My devices and click "Link this computer". Or enter the address of your phone system: your phone line and settings are fetched automatically.',
      server: 'Server address', code: 'Pairing code (optional)', codeHint: 'From the portal or your welcome e-mail.', connect: 'Connect', or: 'or',
      browser: 'Sign in with your browser (SSO)', checking: 'Checking the server…',
      untrustedTitle: 'Check the certificate of this phone system', untrustedText: 'This server uses its own (self-signed) certificate. Compare this fingerprint with the one in the portal under My devices, Softphone for Windows.',
      trust: 'The fingerprint matches, I trust this phone system', trustConnect: 'Trust and connect',
      errors: {
        empty: 'Enter the address of your phone system.', invalid: 'This is not a valid address.', https_required: 'Use an https address.',
        unreachable: 'The server cannot be reached. Check the address and your network.', not_kiestoon: 'There is no Kiestoon phone system at this address.',
        fingerprint_mismatch: 'The certificate does not match. Do not connect, and ask your administrator.', untrusted: 'Confirm the certificate first.',
        generic: 'Connecting failed: {e}',
      },
      incoming: 'Incoming call', via: 'via {q}', answer: 'Answer', decline: 'Decline', unknown: 'Unknown number',
    },
    nl: {
      title: 'Welkom bij Kiestoon', intro: 'Open in het Kiestoon-portaal Mijn toestellen en klik op "Deze computer koppelen". Of vul het adres van je telefooncentrale in: je lijn en instellingen worden automatisch opgehaald.',
      server: 'Serveradres', code: 'Koppelcode (optioneel)', codeHint: 'Uit het portaal of je welkomstmail.', connect: 'Verbinden', or: 'of',
      browser: 'Inloggen via je browser (SSO)', checking: 'Server controleren…',
      untrustedTitle: 'Controleer het certificaat van deze telefooncentrale', untrustedText: 'Deze server gebruikt een eigen (zelfondertekend) certificaat. Vergelijk deze vingerafdruk met die in het portaal onder Mijn toestellen, Softphone voor Windows.',
      trust: 'De vingerafdruk klopt, ik vertrouw deze telefooncentrale', trustConnect: 'Vertrouwen en verbinden',
      errors: {
        empty: 'Vul het adres van je telefooncentrale in.', invalid: 'Dit is geen geldig adres.', https_required: 'Gebruik een https-adres.',
        unreachable: 'De server is niet bereikbaar. Controleer het adres en je netwerk.', not_kiestoon: 'Op dit adres staat geen Kiestoon-telefooncentrale.',
        fingerprint_mismatch: 'Het certificaat klopt niet. Maak geen verbinding en vraag het je beheerder.', untrusted: 'Bevestig eerst het certificaat.',
        generic: 'Verbinden lukte niet: {e}',
      },
      incoming: 'Inkomend gesprek', via: 'via {q}', answer: 'Opnemen', decline: 'Weigeren', unknown: 'Onbekend nummer',
    },
  }
  const q = new URLSearchParams(location.search)
  const lang = (q.get('lang') || navigator.language || 'en').toLowerCase().startsWith('nl') ? 'nl' : 'en'
  document.documentElement.lang = lang
  const t = texts[lang]
  const invoke = (cmd, args) => window.__TAURI__.core.invoke(cmd, args)
  const err = (e) => {
    const code = String(e)
    return t.errors[code] || t.errors.generic.replace('{e}', code)
  }
  return { t, lang, q, invoke, err }
})()
