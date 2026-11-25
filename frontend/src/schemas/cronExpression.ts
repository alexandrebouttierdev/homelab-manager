import { z } from 'zod'

// The inspector checks the size only: the server parses the expression.
export const cronExpressionSchema = z.object({
  expression: z.string().trim().min(1, 'Expression requise').max(200, 'Au plus 200 caractères'),
})

export type CronExpressionValues = z.infer<typeof cronExpressionSchema>
