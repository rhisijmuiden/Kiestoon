// Browser side of passkeys (WebAuthn). The server sends the options as JSON with base64url fields.

const b64urlToBuf = (s: string) => {
  const b = atob(s.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(s.length / 4) * 4, '='))
  return Uint8Array.from(b, (c) => c.charCodeAt(0)).buffer
}
const bufToB64url = (b: ArrayBuffer | null | undefined) => {
  if (!b) return undefined
  let s = ''
  new Uint8Array(b).forEach((x) => (s += String.fromCharCode(x)))
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

type Json = Record<string, unknown>

export const passkeysSupported = () => typeof window !== 'undefined' && !!window.PublicKeyCredential && window.isSecureContext

function credentialJSON(c: PublicKeyCredential): Json {
  const withJSON = c as unknown as { toJSON?: () => Json }
  if (typeof withJSON.toJSON === 'function') return withJSON.toJSON()
  const r = c.response as AuthenticatorAttestationResponse & AuthenticatorAssertionResponse
  const response: Json = { clientDataJSON: bufToB64url(r.clientDataJSON) }
  if ('attestationObject' in r && r.attestationObject) {
    response.attestationObject = bufToB64url(r.attestationObject)
    response.transports = r.getTransports?.() ?? []
  } else {
    response.authenticatorData = bufToB64url(r.authenticatorData)
    response.signature = bufToB64url(r.signature)
    response.userHandle = bufToB64url(r.userHandle)
  }
  return { id: c.id, rawId: bufToB64url(c.rawId), type: c.type, response, clientExtensionResults: c.getClientExtensionResults(), authenticatorAttachment: c.authenticatorAttachment }
}

/** Creates a passkey with the options of /account/passkeys/begin. */
export async function createPasskey(options: { publicKey: Json }) {
  const pk = options.publicKey as Json & { challenge: string; user: Json & { id: string }; excludeCredentials?: (Json & { id: string })[] }
  const publicKey = {
    ...pk,
    challenge: b64urlToBuf(pk.challenge),
    user: { ...pk.user, id: b64urlToBuf(pk.user.id) },
    excludeCredentials: (pk.excludeCredentials ?? []).map((c) => ({ ...c, id: b64urlToBuf(c.id) })),
  } as unknown as PublicKeyCredentialCreationOptions
  const c = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential | null
  if (!c) throw new Error('cancelled')
  return credentialJSON(c)
}

/** Signs in with a passkey with the options of /auth/passkey/begin. */
export async function getPasskey(options: { publicKey: Json }) {
  const pk = options.publicKey as Json & { challenge: string; allowCredentials?: (Json & { id: string })[] }
  const publicKey = {
    ...pk,
    challenge: b64urlToBuf(pk.challenge),
    allowCredentials: (pk.allowCredentials ?? []).map((c) => ({ ...c, id: b64urlToBuf(c.id) })),
  } as unknown as PublicKeyCredentialRequestOptions
  const c = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null
  if (!c) throw new Error('cancelled')
  return credentialJSON(c)
}
