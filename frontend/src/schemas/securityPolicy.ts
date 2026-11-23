import { z } from 'zod'

// Bounds of API_CONTRACTS.md § 3.3; the server checks the prefixes exactly.
const maxNetworks = 32
const cidrPattern = /^(?:\d{1,3}(?:\.\d{1,3}){3}|[0-9a-fA-F:]*:[0-9a-fA-F:.]*)\/\d{1,3}$/

/** Networks typed one per line (or separated by commas). */
export function networksFromText(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((network) => network.trim())
    .filter((network) => network !== '')
}

export const securityPolicySchema = z.object({
  idleTimeoutMinutes: z
    .number({ error: 'Durée requise' })
    .int('Nombre entier de minutes')
    .min(5, 'Au moins 5 minutes')
    .max(720, 'Au plus 720 minutes (12 h)'),
  networks: z
    .string()
    .refine((text) => networksFromText(text).length <= maxNetworks, '32 réseaux au plus')
    .refine(
      (text) => networksFromText(text).every((network) => cidrPattern.test(network)),
      'Notation CIDR attendue, par exemple 192.168.1.0/24',
    ),
})

export type SecurityPolicyValues = z.infer<typeof securityPolicySchema>
