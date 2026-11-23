// Number blocks: a provider hands out numbers in a block such as 020 123 4500 – 020 123 4599.
// The numbers keep the length and leading zeros (or +) of the first one.

export const MAX_BLOCK = 10000

/** Normalizes a phone number the way the server does: spaces, dashes, dots and brackets go. */
export function normalizeNumber(s: string): string {
  return s.trim().replace(/[\s\-().]/g, '')
}

const valid = (s: string) => /^\+?\d{3,20}$/.test(s)

/**
 * Expands a number block from a first number and either a last number or a count. Returns null
 * when the input is not a valid block (letters, last before first, different length, too many).
 */
export function expandBlock(first: string, lastOrCount: string): string[] | null {
  const a = normalizeNumber(first)
  const b = normalizeNumber(lastOrCount)
  if (!valid(a) || !b) return null
  const plus = a.startsWith('+') ? '+' : ''
  const digits = a.slice(plus.length)
  let count: bigint
  if (/^\d{1,5}$/.test(b) && b.length < digits.length) {
    count = BigInt(b)
  } else {
    const bd = b.startsWith('+') ? b.slice(1) : b
    if (!/^\d+$/.test(bd) || bd.length !== digits.length) return null
    count = BigInt(bd) - BigInt(digits) + 1n
  }
  if (count < 1n || count > BigInt(MAX_BLOCK)) return null
  const start = BigInt(digits)
  const out: string[] = []
  for (let i = 0n; i < count; i++) {
    const n = (start + i).toString().padStart(digits.length, '0')
    if (n.length > digits.length) break
    out.push(plus + n)
  }
  return out
}

/** Splits pasted text (one per line, or separated by commas or semicolons) into numbers. */
export function parseNumbers(text: string): string[] {
  return text.split(/[\n,;]+/).map(normalizeNumber).filter(Boolean)
}

export const isValidNumber = (s: string) => valid(normalizeNumber(s))

/** Formats a Dutch national number for reading: 0201234567 → 020 123 4567. Others stay as they are. */
export function formatNumber(n: string): string {
  if (/^0[1-9]\d{8}$/.test(n)) {
    if (/^0(6|8[0-9]|9[0-9])/.test(n)) return `${n.slice(0, 2)} ${n.slice(2, 6)} ${n.slice(6)}`
    return `${n.slice(0, 3)} ${n.slice(3, 6)} ${n.slice(6)}`
  }
  return n
}
