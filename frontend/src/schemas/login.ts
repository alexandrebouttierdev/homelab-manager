import { z } from 'zod'

export const loginSchema = z.object({
  username: z.string().trim().min(1, 'Identifiant requis'),
  password: z.string().min(8, 'Au moins 8 caractères'),
})

export type LoginValues = z.infer<typeof loginSchema>
