import { z } from 'zod'

// Mirror of the server rules for the copy repository (API_CONTRACTS.md
// § 4.10): it guides the input, the server decides.
const absolutePath = /^\/(?!.*\/\/)(?!.*\/$)(?!.*(^|\/)\.\.(\/|$)).*$/

export const backupCopySchema = z
  .object({
    type: z.enum(['local', 'rest', 's3']),
    location: z.string().trim().min(1, 'Emplacement requis').max(2048, 'Au plus 2 048 caractères'),
    username: z.string().max(1024, 'Au plus 1 024 caractères'),
    secret: z.string().max(1024, 'Au plus 1 024 caractères'),
    afterBackup: z.boolean(),
  })
  .superRefine((values, context) => {
    if (values.type === 'local' && (!absolutePath.test(values.location) || values.location === '/'))
      context.addIssue({
        code: 'custom',
        path: ['location'],
        message: 'Chemin absolu, sans « .. » ni « / » final',
      })
    if (values.type !== 'local' && !/^https:\/\/[^/@?#\s]+/.test(values.location))
      context.addIssue({
        code: 'custom',
        path: ['location'],
        message: 'Adresse https://, sans identifiants',
      })
    if (values.type === 's3' && !/^https:\/\/[^/]+\/[^/]+/.test(values.location))
      context.addIssue({
        code: 'custom',
        path: ['location'],
        message: 'Indiquez le bucket : https://hôte/bucket[/préfixe]',
      })
  })

export type BackupCopyValues = z.infer<typeof backupCopySchema>
