import { useState } from 'react'

export function useDeployForm(
  onDeploy: (git: string, name: string, port: string, token: string, agent: boolean) => void,
) {
  const [git, setGit] = useState('')
  const [name, setName] = useState('')
  const [port, setPort] = useState('3000')
  const [token, setToken] = useState('')
  const [agent, setAgent] = useState(false)

  const submit = () => {
    if (!git.trim() || !name.trim()) return
    onDeploy(git.trim(), name.trim(), port.trim() || '3000', token.trim(), agent)
  }

  return { git, setGit, name, setName, port, setPort, token, setToken, agent, setAgent, submit }
}
