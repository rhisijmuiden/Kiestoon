# Kiestoon house style

Kiestoon has a dark, high-tech look: a deep night background, glassy cards, cyan as the signal
colour and indigo as the second accent. Status colours are only used for status, always with a
label or icon next to them.

The source of truth for colours and type is [`tokens.json`](tokens.json). The web UI reads the same
values as CSS variables in `web/src/styles/index.css`.

## Logo

| File | Use |
| --- | --- |
| `kiestoon-mark.svg` | App icon / favicon: a dial-tone wave in a rounded tile |
| `rhis-logo-wit.svg` | "Powered by RHIS" on dark backgrounds (from the RHIS house style, do not recolour) |

## Colours

| Token | Value | Use |
| --- | --- | --- |
| `bg` | `#070B14` | Page background |
| `surface` | `#0F172A` | Cards |
| `surface-sidebar` | `#0B1120` | Left menu |
| `border` | `rgba(148,163,184,.14)` | Card and input borders |
| `text` | `#E2E8F0` | Main text |
| `text-muted` | `#94A3B8` | Secondary text |
| `text-subtle` | `#7C8AA3` | Labels |
| `accent` | `#22D3EE` | Signal colour: active menu item, links, focus |
| `accent-2` | `#818CF8` | Second accent, gradients with `accent` |
| `ok` | `#34D399` | Online, available, success |
| `warn` | `#FBBF24` | Waiting, attention |
| `danger` | `#F87171` | Offline, busy, errors |

## Type

- Headings: **Space Grotesk** 600/700
- Body: **Inter** 400/500/600
- Numbers, versions, codes: **JetBrains Mono**

All fonts are bundled with the app (no external font requests), so the portal works without internet.

## Layout rules

- Admin menu always on the left; on phones it collapses into a top bar.
- One primary action per screen, as a cyan-to-indigo gradient button.
- "Powered by RHIS" with the RHIS logo bottom-right on every screen.
