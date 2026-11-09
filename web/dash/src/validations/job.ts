import { z } from 'zod'

export const jobSchema = z
  .object({
    name: z.string().trim(),
    trigger: z.enum(['schedule', 'webhook', 'bucket']),
    schedule: z.string().trim(),
    bucket: z.string(),
    prefix: z
      .string()
      .trim()
      .refine((s) => s === '' || !s.startsWith('/'), 'A folder in the bucket, like incoming/'),
    command: z.string().trim(),
    usesSteps: z.boolean(),
    timeoutMinutes: z
      .string()
      .trim()
      .refine((s) => s === '' || (/^\d+$/.test(s) && +s >= 1 && +s <= 10080), 'From 1 to 10080 minutes (a week)'),
    retries: z
      .string()
      .trim()
      .refine((s) => s === '' || (/^\d+$/.test(s) && +s <= 10), 'From 0 to 10'),
    catchUp: z.enum(['', 'skip']),
  })
  .superRefine((v, ctx) => {
    if (v.trigger === 'schedule' && v.schedule.split(/\s+/).length !== 5)
      ctx.addIssue({ code: 'custom', path: ['schedule'], message: 'Use 5-field cron syntax, e.g. "0 3 * * *"' })
    if (!v.usesSteps && !v.command)
      ctx.addIssue({ code: 'custom', path: ['command'], message: 'A command is required' })
    if (v.trigger === 'bucket' && !v.bucket)
      ctx.addIssue({ code: 'custom', path: ['bucket'], message: 'Pick a bucket to watch' })
  })

export type JobInput = z.infer<typeof jobSchema>
