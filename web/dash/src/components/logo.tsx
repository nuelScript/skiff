export function LogoMark({ className = '' }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" aria-hidden className={'text-foreground ' + className}>
      {/* mainsail — also reads as a forward "ship it" chevron */}
      <path d="M17.5 3 Q25.5 16 27 24 H17.5 Z" fill="currentColor" />
      {/* jib */}
      <path d="M14.3 8 Q9 17.5 6 24 H14.3 Z" fill="currentColor" fillOpacity="0.55" />
      {/* waterline */}
      <path
        d="M4 26.6 H28"
        stroke="currentColor"
        strokeOpacity="0.45"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  )
}

export function Logo({ className = '' }: { className?: string }) {
  return (
    <span className={'inline-flex items-center gap-2 ' + className}>
      <LogoMark className="h-[22px] w-[22px]" />
      <span className="text-[17px] font-semibold tracking-tight">Skiff</span>
    </span>
  )
}
