import { z } from 'zod'

export const loginSchema = z.object({
  username: z.string().trim().min(1, 'Identifiant requis'),
  password: z.string().min(1, 'Mot de passe requis').max(1024, 'Mot de passe trop long'),
})

export type LoginValues = z.infer<typeof loginSchema>
