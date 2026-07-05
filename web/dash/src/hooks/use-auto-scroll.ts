import { useEffect, useRef } from 'react'

// Keeps a scrollable element pinned to the bottom as `dep` changes.
export function useAutoScroll<T extends HTMLElement>(dep: unknown) {
  const ref = useRef<T>(null)
  useEffect(() => {
    if (ref.current) ref.current.scrollTop = ref.current.scrollHeight
  }, [dep])
  return ref
}
