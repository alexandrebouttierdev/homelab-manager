import { describe, expect, it } from 'vitest'
import {
  assertionToJSON,
  attestationToJSON,
  base64urlToBuffer,
  bufferToBase64url,
  creationOptionsFromJSON,
  requestOptionsFromJSON,
  webauthnErrorMessage,
} from './webauthn'

const bytes = (...values: number[]) => new Uint8Array(values).buffer

describe('webauthn', () => {
  it('round-trips base64url without padding', () => {
    const buffer = bytes(0xfb, 0xff, 0x00, 0x3e)
    const encoded = bufferToBase64url(buffer)
    expect(encoded).toBe('-_8APg')
    expect(new Uint8Array(base64urlToBuffer(encoded))).toEqual(new Uint8Array(buffer))
    expect(bufferToBase64url(new Uint8Array([1, 2, 3]).subarray(1))).toBe('AgM')
  })

  it('decodes the creation options of the Manager', () => {
    const options = creationOptionsFromJSON({
      publicKey: {
        challenge: 'AQID',
        rp: { id: 'homelab.example', name: 'Homelab Manager' },
        user: { id: 'BAU', name: 'alice', displayName: 'Alice' },
        pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
        excludeCredentials: [{ type: 'public-key', id: 'Bg', transports: ['usb'] }],
      },
    })
    expect(new Uint8Array(options.challenge as ArrayBuffer)).toEqual(new Uint8Array([1, 2, 3]))
    expect(new Uint8Array(options.user.id as ArrayBuffer)).toEqual(new Uint8Array([4, 5]))
    expect(options.rp.id).toBe('homelab.example')
    expect(new Uint8Array(options.excludeCredentials?.[0].id as ArrayBuffer)).toEqual(
      new Uint8Array([6]),
    )
    expect(options.excludeCredentials?.[0].transports).toEqual(['usb'])
  })

  it('decodes the request options without allowed credentials', () => {
    const options = requestOptionsFromJSON({
      publicKey: { challenge: 'AQID', userVerification: 'required' },
    })
    expect(options.allowCredentials).toBeUndefined()
    expect(options.userVerification).toBe('required')
  })

  it('encodes the answers of the browser', () => {
    const attestation = attestationToJSON({
      id: 'Bg',
      rawId: bytes(6),
      type: 'public-key',
      authenticatorAttachment: 'cross-platform',
      response: {
        clientDataJSON: bytes(1),
        attestationObject: bytes(2),
        getTransports: () => ['usb'],
      },
      getClientExtensionResults: () => ({}),
    })
    expect(attestation).toEqual({
      id: 'Bg',
      rawId: 'Bg',
      type: 'public-key',
      authenticatorAttachment: 'cross-platform',
      clientExtensionResults: {},
      response: { clientDataJSON: 'AQ', attestationObject: 'Ag', transports: ['usb'] },
    })
    const assertion = assertionToJSON({
      id: 'Bg',
      rawId: bytes(6),
      type: 'public-key',
      response: {
        clientDataJSON: bytes(1),
        authenticatorData: bytes(2),
        signature: bytes(3),
        userHandle: null,
      },
    })
    expect(assertion.response).toEqual({
      clientDataJSON: 'AQ',
      authenticatorData: 'Ag',
      signature: 'Aw',
      userHandle: undefined,
    })
  })

  it('explains the refusals in French', () => {
    expect(webauthnErrorMessage(new DOMException('x', 'NotAllowedError'))).toBe(
      'Opération annulée ou délai dépassé.',
    )
    expect(webauthnErrorMessage(new DOMException('x', 'InvalidStateError'))).toContain(
      'déjà enregistrée',
    )
  })
})
