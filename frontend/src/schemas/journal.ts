import { z } from 'zod'

// Bounds of API_CONTRACTS.md § 4.6; the server checks them again.
const maxSearchLength = 200

function validDate(value: string) {
  return value === '' || !Number.isNaN(new Date(value).getTime())
}

/**
 * Text search and custom range of the journal page. The dates are the
 * values of datetime-local inputs, in the browser's time zone; empty means
 * unbounded.
 */
export const journalSearchSchema = z
  .object({
    q: z.string().trim().max(maxSearchLength, `${maxSearchLength} caractères au plus`),
    from: z.string().refine(validDate, 'Date invalide'),
    to: z.string().refine(validDate, 'Date invalide'),
  })
  .refine((values) => !values.from || !values.to || new Date(values.from) < new Date(values.to), {
    path: ['to'],
    message: 'La fin doit suivre le début',
  })

export type JournalSearchValues = z.infer<typeof journalSearchSchema>

/** RFC 3339 form of a datetime-local value, or undefined when empty. */
export function rangeBound(value: string): string | undefined {
  return value ? new Date(value).toISOString() : undefined
}
