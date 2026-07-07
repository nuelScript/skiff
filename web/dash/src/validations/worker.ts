import { z } from 'zod'

export const workerSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'A name is required')
    .regex(/^[a-z0-9-]+$/, 'Use lowercase letters, numbers, and hyphens'),
  command: z.string().trim().min(1, 'A command is required'),
  replicas: z.number().int().min(1).max(10),
})

export type WorkerInput = z.infer<typeof workerSchema>
