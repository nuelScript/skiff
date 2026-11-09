import { z } from 'zod'

export const jobSchema = z.object({
  name: z.string().trim(),
  schedule: z
    .string()
    .trim()
    .min(1, 'A schedule is required')
    .refine((s) => s.split(/\s+/).length === 5, 'Use 5-field cron syntax, e.g. "0 3 * * *"'),
  command: z.string().trim().min(1, 'A command is required'),
  timeoutMinutes: z
    .string()
    .trim()
    .refine((s) => s === '' || (/^\d+$/.test(s) && +s >= 1 && +s <= 1440), 'From 1 to 1440 minutes'),
  retries: z
    .string()
    .trim()
    .refine((s) => s === '' || (/^\d+$/.test(s) && +s <= 10), 'From 0 to 10'),
  catchUp: z.enum(['', 'skip']),
})

export type JobInput = z.infer<typeof jobSchema>
