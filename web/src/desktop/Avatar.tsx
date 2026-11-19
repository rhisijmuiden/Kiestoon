import { avatarClass, initials, presenceDot, type Presence } from './presence'

export function Avatar({ name, presence, size = 'md', plain }: { name: string; presence?: Presence; size?: 'md' | 'lg'; plain?: boolean }) {
  const dim = size === 'lg' ? 'size-[72px] text-[22px]' : 'size-9 text-xs'
  return (
    <span className={`relative inline-flex flex-none items-center justify-center rounded-full font-semibold ${dim} ${plain ? 'border border-line bg-surface text-accent' : `bg-gradient-to-br text-on-accent ${avatarClass(name)}`}`} aria-hidden>
      {initials(name)}
      {presence && <span className={`absolute -bottom-0.5 -right-0.5 size-3 rounded-full border-2 border-bg ${presenceDot[presence]}`} />}
    </span>
  )
}
