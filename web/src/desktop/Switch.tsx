export function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <button type="button" role="switch" aria-checked={checked} aria-label={label} title={label} disabled={disabled} onClick={() => onChange(!checked)}
      className={`relative h-[22px] w-10 flex-none rounded-full transition disabled:opacity-40 ${checked ? 'bg-accent' : 'bg-line'}`}>
      <span className={`absolute top-[3px] size-4 rounded-full transition-all ${checked ? 'left-[21px] bg-on-accent' : 'left-[3px] bg-muted'}`} />
    </button>
  )
}
