import { useState, type FormEvent } from 'react'

export function useLoginForm(login: (password: string) => Promise<boolean>) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    const ok = await login(password)
    setBusy(false)
    if (!ok) setError('Wrong password')
  }

  return { password, setPassword, error, busy, submit }
}
