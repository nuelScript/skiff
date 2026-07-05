import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { LogoMark } from '@/components/logo'
import { errText } from '@/lib/errors'

export default function Setup({
  onSetup,
}: {
  onSetup: (
    secret: string,
    email: string,
    name: string,
    password: string,
  ) => Promise<void>
}) {
  const [secret, setSecret] = useState('')
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await onSetup(secret.trim(), email.trim(), name.trim(), password)
    } catch (err) {
      setError(errText(err, 'Setup failed'))
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <LogoMark className="mb-1 h-7 w-7" />
          <CardTitle className="text-lg tracking-tight">Set up Skiff</CardTitle>
          <CardDescription>
            Create the owner account for this instance.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="flex flex-col gap-3">
            <div className="grid gap-2">
              <Label htmlFor="secret">
                Setup secret{' '}
                <span className="text-muted-foreground font-mono text-[11px]">
                  SKIFF_PANEL_PASSWORD
                </span>
              </Label>
              <Input
                id="secret"
                type="password"
                autoFocus
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="name">Name</Label>
              <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {error && <p className="text-destructive text-xs">{error}</p>}
            <Button type="submit" disabled={busy} className="mt-1 w-full">
              {busy ? '…' : 'Create account'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
