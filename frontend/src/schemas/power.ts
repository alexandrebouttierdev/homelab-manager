import { z } from 'zod'

/** Confirmation of a power action: the exact machine name (API_CONTRACTS.md § 4.5). */
export function powerConfirmationSchema(hostname: string) {
  return z.object({
    confirmHostname: z
      .string()
      .trim()
      .refine(
        (value) => hostname !== '' && value === hostname,
        'Saisissez exactement le nom de la machine',
      ),
  })
}

export type PowerConfirmationValues = { confirmHostname: string }
