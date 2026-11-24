import { z } from 'zod'

// Bounds of API_CONTRACTS.md § 4.8; the server remains the reference. The
// numbers are typed as text (a decimal comma is accepted) and converted by
// alertRuleInput in src/lib/alerts.ts.
const unitPattern = /^[A-Za-z0-9@._:\-*?[\]]{1,255}$/

export function parseDecimal(value: string): number | null {
  const normalized = value.trim().replace(',', '.')
  if (normalized === '' || !/^-?\d+(\.\d+)?$/.test(normalized)) return null
  const parsed = Number(normalized)
  return Number.isFinite(parsed) ? parsed : null
}

export const alertRuleSchema = z
  .object({
    name: z.string().trim().min(1, 'Nom requis').max(100, 'Maximum 100 caractères'),
    type: z.enum([
      'host_metric',
      'container_metric',
      'container_state',
      'unit_failed',
      'collector',
      'vulnerabilities',
      'backup',
    ]),
    severity: z.enum(['info', 'warning', 'critical']),
    enabled: z.boolean(),
    metric: z.string(),
    labelKey: z.string(),
    labelValue: z.string().max(255, 'Maximum 255 caractères'),
    operator: z.enum(['>', '>=', '<', '<=']),
    threshold: z.string(),
    containerId: z.string(),
    states: z.array(z.string()),
    unitPattern: z.string(),
    collector: z.string(),
    forMinutes: z.string(),
    vulnSeverity: z.enum(['critical', 'high', 'medium', 'low']),
    fixableOnly: z.boolean(),
    maxAgeHours: z.string(),
  })
  .superRefine((values, ctx) => {
    const metricRule = values.type === 'host_metric' || values.type === 'container_metric'
    if (metricRule) {
      if (!values.metric)
        ctx.addIssue({ code: 'custom', path: ['metric'], message: 'Métrique requise' })
      if (parseDecimal(values.threshold) === null)
        ctx.addIssue({ code: 'custom', path: ['threshold'], message: 'Seuil numérique requis' })
      if (values.labelKey && !values.labelValue.trim())
        ctx.addIssue({ code: 'custom', path: ['labelValue'], message: 'Valeur du label requise' })
    }
    if (values.type === 'container_state' && values.states.length === 0)
      ctx.addIssue({ code: 'custom', path: ['states'], message: 'Choisissez au moins un état' })
    if (values.type === 'unit_failed' && !unitPattern.test(values.unitPattern))
      ctx.addIssue({
        code: 'custom',
        path: ['unitPattern'],
        message: 'Nom ou motif d’unité : lettres, chiffres, @ . _ : - * ? [ ]',
      })
    if (values.type === 'collector' && !values.collector)
      ctx.addIssue({ code: 'custom', path: ['collector'], message: 'Collecteur requis' })
    if (values.type === 'backup') {
      const hours = parseDecimal(values.maxAgeHours)
      if (hours === null || !Number.isInteger(hours) || hours < 1 || hours > 720)
        ctx.addIssue({
          code: 'custom',
          path: ['maxAgeHours'],
          message: 'Nombre entier d’heures, de 1 à 720',
        })
    }
    if (
      values.type !== 'unit_failed' &&
      values.type !== 'vulnerabilities' &&
      values.type !== 'backup'
    ) {
      const minutes = parseDecimal(values.forMinutes)
      if (minutes === null || minutes < 0 || minutes > 1440)
        ctx.addIssue({
          code: 'custom',
          path: ['forMinutes'],
          message: 'Durée de 0 à 1 440 minutes',
        })
    }
  })

export type AlertRuleValues = z.infer<typeof alertRuleSchema>
