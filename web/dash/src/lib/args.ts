// splitArgs splits a command line like a shell would, honouring quotes; it returns null when a quote isn't closed.
export function splitArgs(line: string): string[] | null {
  const out: string[] = []
  let cur = ''
  let inArg = false
  let quote = ''
  for (let i = 0; i < line.length; i++) {
    const c = line[i]
    if (quote) {
      if (c === quote) quote = ''
      else if (c === '\\' && quote === '"' && i + 1 < line.length) cur += line[++i]
      else cur += c
    } else if (c === '"' || c === "'") {
      quote = c
      inArg = true
    } else if (c === '\\' && i + 1 < line.length) {
      cur += line[++i]
      inArg = true
    } else if (/\s/.test(c)) {
      if (inArg) out.push(cur)
      cur = ''
      inArg = false
    } else {
      cur += c
      inArg = true
    }
  }
  if (quote) return null
  if (inArg) out.push(cur)
  return out
}

// joinArgs writes arguments back as one line, quoting those that need it.
export function joinArgs(args: string[]): string {
  return args.map((a) => (a !== '' && /^[\w@%+=:,./-]+$/.test(a) ? a : `'${a.replace(/'/g, `'"'"'`)}'`)).join(' ')
}
