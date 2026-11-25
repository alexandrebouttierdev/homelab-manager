/**
 * WebAuthn in the browser (SPEC_ADMINISTRATION.md § 3.3): the Manager sends
 * its options in JSON with base64url binaries, the browser wants buffers.
 */

export type CeremonyOptions = { publicKey: Record<string, unknown> }

type CredentialDescriptorJSON = { id: string; type: string; transports?: string[] }

export function base64urlToBuffer(value: string): ArrayBuffer {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/')
  const binary = atob(base64 + '='.repeat((4 - (base64.length % 4)) % 4))
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

export function bufferToBase64url(buffer: ArrayBuffer | ArrayBufferView): string {
  const bytes =
    buffer instanceof ArrayBuffer
      ? new Uint8Array(buffer)
      : new Uint8Array(buffer.buffer, buffer.byteOffset, buffer.byteLength)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function descriptors(list: unknown): PublicKeyCredentialDescriptor[] | undefined {
  if (!Array.isArray(list)) return undefined
  return (list as CredentialDescriptorJSON[]).map((item) => ({
    ...item,
    type: 'public-key',
    id: base64urlToBuffer(item.id),
    transports: item.transports as AuthenticatorTransport[] | undefined,
  }))
}

/** Options of navigator.credentials.create from those of the Manager. */
export function creationOptionsFromJSON(
  options: CeremonyOptions,
): PublicKeyCredentialCreationOptions {
  const publicKey = options.publicKey as Record<string, unknown> & {
    challenge: string
    user: { id: string; name: string; displayName: string }
  }
  return {
    ...(publicKey as unknown as PublicKeyCredentialCreationOptions),
    challenge: base64urlToBuffer(publicKey.challenge),
    user: { ...publicKey.user, id: base64urlToBuffer(publicKey.user.id) },
    excludeCredentials: descriptors(publicKey.excludeCredentials),
  }
}

/** Options of navigator.credentials.get from those of the Manager. */
export function requestOptionsFromJSON(
  options: CeremonyOptions,
): PublicKeyCredentialRequestOptions {
  const publicKey = options.publicKey as Record<string, unknown> & { challenge: string }
  return {
    ...(publicKey as unknown as PublicKeyCredentialRequestOptions),
    challenge: base64urlToBuffer(publicKey.challenge),
    allowCredentials: descriptors(publicKey.allowCredentials),
  }
}

type AttestationLike = {
  id: string
  rawId: ArrayBuffer
  type: string
  authenticatorAttachment?: string | null
  response: {
    clientDataJSON: ArrayBuffer
    attestationObject: ArrayBuffer
    getTransports?: () => string[]
  }
  getClientExtensionResults?: () => unknown
}

type AssertionLike = {
  id: string
  rawId: ArrayBuffer
  type: string
  authenticatorAttachment?: string | null
  response: {
    clientDataJSON: ArrayBuffer
    authenticatorData: ArrayBuffer
    signature: ArrayBuffer
    userHandle: ArrayBuffer | null
  }
  getClientExtensionResults?: () => unknown
}

/** Answer of navigator.credentials.create, in the JSON the Manager reads. */
export function attestationToJSON(credential: AttestationLike) {
  return {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment ?? undefined,
    clientExtensionResults: credential.getClientExtensionResults?.() ?? {},
    response: {
      clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
      attestationObject: bufferToBase64url(credential.response.attestationObject),
      transports: credential.response.getTransports?.() ?? [],
    },
  }
}

/** Answer of navigator.credentials.get, in the JSON the Manager reads. */
export function assertionToJSON(credential: AssertionLike) {
  return {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment ?? undefined,
    clientExtensionResults: credential.getClientExtensionResults?.() ?? {},
    response: {
      clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
      authenticatorData: bufferToBase64url(credential.response.authenticatorData),
      signature: bufferToBase64url(credential.response.signature),
      userHandle: credential.response.userHandle
        ? bufferToBase64url(credential.response.userHandle)
        : undefined,
    },
  }
}

/** The browser offers WebAuthn: a secure context with the API. */
export function webauthnSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    window.isSecureContext &&
    typeof window.PublicKeyCredential === 'function' &&
    typeof navigator.credentials?.create === 'function'
  )
}

/** French message of a refused ceremony. */
export function webauthnErrorMessage(error: unknown): string {
  const name = error instanceof DOMException || error instanceof Error ? error.name : ''
  switch (name) {
    case 'NotAllowedError':
      return 'Opération annulée ou délai dépassé.'
    case 'InvalidStateError':
      return 'Cette clé est déjà enregistrée pour ce compte.'
    case 'SecurityError':
      return "L'adresse de cette page ne correspond pas au domaine WebAuthn du Manager."
    case 'NotSupportedError':
      return 'Cette clé ne prend pas en charge les algorithmes demandés.'
    default:
      return error instanceof Error ? error.message : 'La clé de sécurité a échoué.'
  }
}

class WebAuthnError extends Error {}

async function run<T>(ceremony: () => Promise<T | null>): Promise<T> {
  if (!webauthnSupported()) {
    throw new WebAuthnError(
      'Ce navigateur ne prend pas en charge les clés de sécurité, ou la page n’est pas en HTTPS.',
    )
  }
  try {
    const result = await ceremony()
    if (!result) throw new WebAuthnError('Aucune clé n’a répondu.')
    return result
  } catch (error) {
    if (error instanceof WebAuthnError) throw error
    throw new WebAuthnError(webauthnErrorMessage(error))
  }
}

/** Registers a key with the browser and returns its answer in JSON. */
export async function createCredential(options: CeremonyOptions) {
  const credential = await run(
    () =>
      navigator.credentials.create({
        publicKey: creationOptionsFromJSON(options),
      }) as Promise<AttestationLike | null>,
  )
  return attestationToJSON(credential)
}

/** Asks a registered key to sign the challenge and returns its answer. */
export async function getCredential(options: CeremonyOptions) {
  const credential = await run(
    () =>
      navigator.credentials.get({
        publicKey: requestOptionsFromJSON(options),
      }) as Promise<AssertionLike | null>,
  )
  return assertionToJSON(credential)
}
