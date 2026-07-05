import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

// A live shell inside the app's container, over a WebSocket to /api/exec.
export function ConsoleTerminal({ app }: { app: string }) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return

    const term = new Terminal({
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
      fontSize: 13,
      lineHeight: 1.4,
      cursorBlink: true,
      theme: {
        background: '#09090b',
        foreground: '#e4e4e7',
        cursor: '#e4e4e7',
        selectionBackground: '#3f3f46',
        black: '#18181b',
        brightBlack: '#52525b',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(el)
    try {
      fit.fit()
    } catch {
      /* not laid out yet */
    }

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${proto}//${location.host}/api/exec?app=${encodeURIComponent(app)}`)
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => {
      term.focus()
      ws.send(JSON.stringify({ t: 'resize', c: term.cols, r: term.rows }))
    }
    ws.onmessage = (e) => {
      if (typeof e.data === 'string') term.write(e.data)
      else term.write(new Uint8Array(e.data as ArrayBuffer))
    }
    ws.onclose = () => term.write('\r\n\x1b[90m— session ended —\x1b[0m\r\n')

    const onData = term.onData((d) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ t: 'in', d }))
    })
    const onResize = term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ t: 'resize', c: cols, r: rows }))
    })

    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
      } catch {
        /* ignore */
      }
    })
    ro.observe(el)

    return () => {
      ro.disconnect()
      onData.dispose()
      onResize.dispose()
      ws.close()
      term.dispose()
    }
  }, [app])

  return <div ref={ref} className="h-full w-full" />
}
