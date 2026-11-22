import { z } from 'zod'

export const passwordSchema = z.object({
  currentPassword: z.string().min(1, 'Mot de passe actuel requis'),
  newPassword: z
    .string()
    .min(12, 'Au moins 12 caractères')
    .max(1024, 'Maximum 1 024 caractères'),
  confirmPassword: z.string().min(1, 'Confirmation requise'),
}).refine((values) => values.newPassword === values.confirmPassword, {
  path: ['confirmPassword'],
  message: 'Les mots de passe ne correspondent pas',
})

export type PasswordValues = z.infer<typeof passwordSchema>
