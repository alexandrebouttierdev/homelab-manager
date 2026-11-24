import { z } from 'zod'

// Bounds of API_CONTRACTS.md § 4.8 (silences); the server remains the
// reference.
export const silenceDurations = ['15', '60', '240', '1440', '4320', '10080'] as const

export function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const code = character.charCodeAt(0)
    if (code < 0x20 || code === 0x7f) return true
  }
  return false
}

export const silenceSchema = z.object({
  scope: z.enum(['rule', 'target', 'all']),
  durationMinutes: z.enum(silenceDurations),
  reason: z
    .string()
    .trim()
    .min(1, 'Motif requis')
    .max(200, 'Maximum 200 caractères')
    .refine((value) => !hasControlCharacter(value), 'Une seule ligne, sans caractère spécial'),
})

export type SilenceValues = z.infer<typeof silenceSchema>
