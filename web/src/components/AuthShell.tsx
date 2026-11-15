import type { ReactNode } from 'react'
import { Brand, LanguageSwitch, PoweredBy } from './brand'

/** Centered layout for setup, sign-in and MFA screens. */
export function AuthShell({ children, wide = false }: { children: ReactNode; wide?: boolean }) {
  return (
    <div className="relative flex min-h-screen flex-col">
      <div aria-hidden className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute -top-40 left-1/2 h-[480px] w-[900px] -translate-x-1/2 rounded-full bg-accent-2/10 blur-3xl" />
        <svg className="absolute inset-x-0 top-0 h-full w-full opacity-[0.07]" aria-hidden>
          <defs>
            <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
              <path d="M40 0H0V40" fill="none" stroke="#94A3B8" strokeWidth=".6" />
            </pattern>
          </defs>
          <rect width="100%" height="100%" fill="url(#grid)" />
        </svg>
      </div>
      <header className="relative flex items-center justify-between px-4 py-5 sm:px-8">
        <Brand />
        <LanguageSwitch />
      </header>
      <main className="relative flex flex-1 items-start justify-center px-4 pb-24 pt-6 sm:items-center sm:pt-0">
        <div className={`card w-full p-6 sm:p-8 ${wide ? 'max-w-2xl' : 'max-w-md'}`}>{children}</div>
      </main>
      <PoweredBy className="fixed bottom-4 right-4 flex sm:right-6" />
    </div>
  )
}
