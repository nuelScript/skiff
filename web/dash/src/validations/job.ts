import { z } from 'zod'

export const jobSchema = z.object({
  name: z.string().trim(),
  schedule: z
    .string()
    .trim()
    .min(1, 'A schedule is required')
    .refine((s) => s.split(/\s+/).length === 5, 'Use 5-field cron syntax, e.g. "0 3 * * *"'),
  command: z.string().trim().min(1, 'A command is required'),
})

export type JobInput = z.infer<typeof jobSchema>
