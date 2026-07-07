import { useState } from 'react'

// useCopy is the shared state machine behind every copy button: copy(text)
// writes to the clipboard and flashes `copied` true for ~1.2s.
export function useCopy(resetMs = 1200) {
  const [copied, setCopied] = useState(false)
  const copy = (text: string) => {
    navigator.clipboard?.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), resetMs)
  }
  return { copied, copy }
}
