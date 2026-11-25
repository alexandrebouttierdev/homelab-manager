import { z } from 'zod'
import { repositoryIssues } from './backupRepository'

// Mirror of the server rules for the copy repository (API_CONTRACTS.md
// § 4.10): it guides the input, the server decides.
export const backupCopySchema = z
  .object({
    type: z.enum(['local', 'rest', 's3', 'sftp']),
    location: z.string().trim().min(1, 'Emplacement requis').max(2048, 'Au plus 2 048 caractères'),
    hostKey: z.string().max(8192, 'Au plus 8 192 caractères'),
    username: z.string().max(1024, 'Au plus 1 024 caractères'),
    secret: z.string().max(1024, 'Au plus 1 024 caractères'),
    afterBackup: z.boolean(),
  })
  .superRefine((values, context) => {
    for (const issue of repositoryIssues(values.type, values.location, values.hostKey))
      context.addIssue({ code: 'custom', path: [issue.path], message: issue.message })
  })

export type BackupCopyValues = z.infer<typeof backupCopySchema>
