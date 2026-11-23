import { z } from 'zod'

// Bounds of API_CONTRACTS.md § 3.1. The server counts bytes; these counts
// are characters, so the server remains the reference.
export const accountSchema = z.object({
  username: z
    .string()
    .regex(/^[a-z0-9._-]{1,64}$/, 'Minuscules, chiffres, « . », « _ » ou « - », 64 au plus'),
  displayName: z.string().max(128, 'Maximum 128 caractères'),
  role: z.enum(['viewer', 'operator', 'admin']),
  password: z.string().min(12, 'Au moins 12 caractères').max(1024, 'Maximum 1 024 caractères'),
})

export type AccountValues = z.infer<typeof accountSchema>
