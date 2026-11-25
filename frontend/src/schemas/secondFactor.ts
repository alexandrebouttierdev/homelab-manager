import { z } from 'zod'

const totpPattern = /^\d{3}\s?\d{3}$/
const totpMessage = 'Code à 6 chiffres'

export const totpCodeSchema = z.object({
  code: z.string().trim().regex(totpPattern, totpMessage),
})

export type TotpCodeValues = z.infer<typeof totpCodeSchema>

/** Second step of the login: an app code or a recovery code. */
export const secondFactorLoginSchema = z
  .object({
    method: z.enum(['totp', 'recovery']),
    code: z.string().trim().min(1, 'Code requis').max(64, 'Code trop long'),
  })
  .superRefine((values, context) => {
    if (values.method === 'totp' && !totpPattern.test(values.code)) {
      context.addIssue({ code: 'custom', path: ['code'], message: totpMessage })
    }
  })

export type SecondFactorLoginValues = z.infer<typeof secondFactorLoginSchema>

export const passwordConfirmationSchema = z.object({
  password: z.string().min(1, 'Mot de passe requis').max(1024, 'Mot de passe trop long'),
})

export type PasswordConfirmationValues = z.infer<typeof passwordConfirmationSchema>

/** A new passkey or security key: its name and the current password. */
export const webauthnKeySchema = z.object({
  name: z.string().trim().min(1, 'Nom requis').max(64, 'Nom trop long (64 caractères au plus)'),
  password: z.string().min(1, 'Mot de passe requis').max(1024, 'Mot de passe trop long'),
})

export type WebAuthnKeyValues = z.infer<typeof webauthnKeySchema>
