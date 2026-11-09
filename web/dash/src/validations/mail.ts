import { z } from 'zod'

export const splitInboxes = (v: string) =>
  v
    .split(/[\s,;]+/)
    .map((s) => s.trim())
    .filter(Boolean)

// isAgentInbox is a forward to an agent's inbox rather than an address.
export const isAgentInbox = (s: string) => /^agent:[a-z0-9][a-z0-9-]*$/i.test(s)

export const mailForwardSchema = z.object({
  alias: z
    .string()
    .trim()
    .toLowerCase()
    .min(1, 'An address is required')
    .refine(
      (v) => v === '*' || /^[a-z0-9]([a-z0-9._-]{0,62}[a-z0-9])?$/.test(v),
      'Letters, numbers, dots, hyphens — or * for everything',
    ),
  domain: z
    .string()
    .trim()
    .toLowerCase()
    .regex(
      /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/,
      'Enter a domain like example.com',
    ),
  destinations: z
    .string()
    .refine((v) => splitInboxes(v).length > 0, 'Add at least one inbox')
    .refine((v) => splitInboxes(v).length <= 10, 'Up to 10 inboxes')
    .refine(
      (v) => splitInboxes(v).every((s) => isAgentInbox(s) || z.email().safeParse(s).success),
      'Use valid email addresses, or agent:name, separated by commas',
    ),
})

export type MailForwardInput = z.infer<typeof mailForwardSchema>
