/** Base32 secret in groups of four, easier to type in an authenticator app. */
export function formatTotpSecret(secret: string): string {
  return secret.replace(/(.{4})(?=.)/g, '$1 ')
}
