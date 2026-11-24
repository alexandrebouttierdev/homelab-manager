import type { SecondFactorMethod } from './api/auth'

/** Base32 secret in groups of four, easier to type in an authenticator app. */
export function formatTotpSecret(secret: string): string {
  return secret.replace(/(.{4})(?=.)/g, '$1 ')
}

/** Order in which the login offers the second factors. */
const methodPreference: SecondFactorMethod[] = ['webauthn', 'totp', 'recovery']

/** The method the login starts with: a key, else the app, else a code. */
export function preferredMethod(methods: SecondFactorMethod[]): SecondFactorMethod {
  return methodPreference.find((method) => methods.includes(method)) ?? 'recovery'
}

/** The other methods the account can switch to, in preference order. */
export function otherMethods(
  methods: SecondFactorMethod[],
  current: SecondFactorMethod,
): SecondFactorMethod[] {
  return methodPreference.filter((method) => method !== current && methods.includes(method))
}
