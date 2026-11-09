import { z } from 'zod'

export const aiProviderSchema = z
  .object({
    kind: z.enum(['openai', 'anthropic', 'custom']),
    name: z
      .string()
      .trim()
      .regex(/^[a-z0-9][a-z0-9-]{0,31}$/, 'Lowercase letters, numbers and dashes, like openai'),
    baseUrl: z
      .string()
      .trim()
      .regex(/^(https?:\/\/\S+)?$/, 'Use the full address, starting with https://'),
    key: z.string().trim().min(1, "Paste the provider's API key"),
  })
  .refine((v) => v.kind !== 'custom' || v.baseUrl, {
    message: "Give the provider's API address",
    path: ['baseUrl'],
  })

export type AIProviderInput = z.infer<typeof aiProviderSchema>
