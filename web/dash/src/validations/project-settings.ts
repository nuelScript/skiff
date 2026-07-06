import { z } from 'zod'

export const projectSettingsSchema = z.object({
  branch: z.string().trim(),
  rootDir: z.string().trim(),
  port: z
    .string()
    .trim()
    .refine((v) => v === '' || /^\d+$/.test(v), 'Port must be a number'),
  auto: z.boolean(),
  previewAuto: z.boolean(),
  replicas: z.number().int().min(1).max(10),
  release: z.string().trim(),
})

export type ProjectSettingsInput = z.infer<typeof projectSettingsSchema>
