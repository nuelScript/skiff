import { ImageResponse } from 'next/og'
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'

export const alt = 'Skiff — Ship it to a server you own'
export const size = { width: 1200, height: 630 }
export const contentType = 'image/png'

export default async function Image() {
  const [regular, semibold] = await Promise.all([
    readFile(join(process.cwd(), 'assets/Geist-400.woff')),
    readFile(join(process.cwd(), 'assets/Geist-600.woff')),
  ])

  return new ImageResponse(
    (
      <div
        style={{
          width: '100%',
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          background: '#0a0a0a',
          color: '#fafafa',
          padding: 84,
          fontFamily: 'Geist',
        }}
      >
        <div
          style={{
            position: 'absolute',
            top: -220,
            right: -160,
            width: 760,
            height: 760,
            background: 'radial-gradient(circle, rgba(130,130,140,0.20), transparent 62%)',
          }}
        />

        <div style={{ display: 'flex', alignItems: 'center', gap: 18 }}>
          <svg width="54" height="54" viewBox="0 0 48 48">
            <rect width="48" height="48" rx="11" fill="#161616" />
            <path d="M26 8 Q38 25 40 37 H26 Z" fill="#fafafa" />
            <path d="M22 13 Q14 26 11 37 H22 Z" fill="#fafafa" fillOpacity="0.72" />
            <path
              d="M8 40.5 H40"
              stroke="#fafafa"
              strokeOpacity="0.85"
              strokeWidth="2.4"
              strokeLinecap="round"
            />
          </svg>
          <div style={{ fontSize: 36, fontWeight: 600, letterSpacing: -0.5 }}>Skiff</div>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 26 }}>
          <div
            style={{
              fontSize: 78,
              fontWeight: 600,
              lineHeight: 1.04,
              letterSpacing: -2.5,
              maxWidth: 940,
            }}
          >
            Ship it to a server you own.
          </div>
          <div
            style={{
              fontSize: 31,
              fontWeight: 400,
              color: '#a1a1aa',
              lineHeight: 1.4,
              maxWidth: 900,
            }}
          >
            Push-to-deploy with automatic HTTPS, managed databases, and preview environments — on
            infrastructure you control, not rented.
          </div>
        </div>

        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            fontSize: 25,
            color: '#71717a',
          }}
        >
          <div>useskiff.xyz</div>
          <div>Open source · self-hosted</div>
        </div>
      </div>
    ),
    {
      ...size,
      fonts: [
        { name: 'Geist', data: regular, weight: 400, style: 'normal' },
        { name: 'Geist', data: semibold, weight: 600, style: 'normal' },
      ],
    },
  )
}
