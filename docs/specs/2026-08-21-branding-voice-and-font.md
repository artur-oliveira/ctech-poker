# Branding Voice Copy + Font Change — Design

## Summary

Two independent, low-risk changes, both scoped to `ui/`:

1. Rewrite four flagged landing-page phrases in `ui/src/app/page.tsx` to plainer language.
2. Replace the Geist/Geist Mono font pairing with a less "every AI-scaffolded app uses this"
   choice, without going "fancy."

No backend change either part.

## Part 1 — Copy audit: landing page (full) + guide (spot-check)

The user's original four examples were illustrative, not exhaustive: "essas são só exemplos, deve ser
verificadas todas as expressões textuais da landing page e preferencialmente do guia." This part
covers every user-facing string in `ui/src/app/page.tsx` (all ~230 lines were read and each heading/
paragraph individually assessed against the same "does this sound like something a person would
actually say" test the user applied) and a full read of the 8-page guide (`ui/src/app/guide/**`,
`~530` lines).

### Landing page (`ui/src/app/page.tsx`) — full audit result

Six strings need a rewrite (the original four, plus two more found by applying the same test to the
rest of the file). Everything else in the file — nav labels, feature-grid copy, the CTA, the footer —
already reads as plain description, not marketing voice, and is left alone; see the "left unchanged"
note after the table for why each was judged fine.

| Line | Current | Problem | Proposed |
|------|---------|---------|----------|
| 85 | `A noite de poker <em>começa aqui.</em>` | "Então quer dizer que eu não posso jogar de dia?" — literally implies night-only play | `Chame seus amigos para jogar poker.` |
| 86-87 | `leia a mesa e guarde as histórias de cada mão` | Same failure mode as the "leitura dos rivais/resenha" phrase below — figurative poker-culture phrasing ("leia a mesa", "guarde as histórias") standing in for a plain feature description, one sentence before the H1 even gets to plain language | `acompanhe seus adversários e reveja o histórico de cada mão` |
| 97 | `Baralho auditável` | "Estranho a primeira vista" — "auditável" reads as accounting/compliance jargon, not a poker trust signal | `Baralho verificável` |
| 105-106 | `a leitura dos rivais, a resenha com os amigos e aquela mão que merece ser vista mais uma vez` | "Leitura dos rivais, resenha entre amigos: parece meio cringe" | `entender os adversários, bater papo com os amigos e rever aquela mão que merece ser vista de novo` |
| 112 | `A mesa ficou mais divertida.` | flagged directly, vague marketing-speak that doesn't say what changed | `Mais formas de interagir na mesa.` (the paragraph right below it already lists the concrete additions — reactions, notes, run it twice — so the heading should point at that list, not restate a mood) |
| 171 | `Algumas mãos terminam. Outras viram estrela.` | Same "trying to sound clever" register as the night-framing H1 and the old "mesa ficou mais divertida" heading — a poetic contrast line where the paragraph right after it already states the real content plainly | `Conquistas para os momentos que valem a pena lembrar.` |

Left unchanged, with reasoning:

- **Nav labels** (`Novidades`, `Por que jogar`, `Conquistas`, `Regras`, `Guia`, `Ranking`, `Entrar`,
  line 73-79) — single-word/short functional labels, no room for a voice problem to hide in.
- **Feature-grid titles/bodies** (`features` array, lines 32-63) — "Cada mão pode ser conferida"
  already uses the plain "conferir" verb (the same fix being applied to "auditável"), and the rest
  ("Convide seus amigos", "Reviva qualquer mão") state the action directly.
- Line 61, `"...sem pegadinhas ou pressão"` — flagged and considered, but "pegadinha" is common,
  understood Portuguese (not slang trying to sound cool), and states a real property (no dark
  patterns) rather than a mood; left as-is.
- **CTA and footer** (lines 216-231) — "Abra sua mesa em segundos", "Jogue com responsabilidade" are
  plain imperative statements, same register the rest of the rewrites are moving toward.
- **`experience` section** (`"sem transformar poker em um painel de controle"`, line 157) — an idiom,
  but a clear, common one (not poker-culture slang), and it states a real design intent; left as-is.

### Guide (`ui/src/app/guide/**`, 8 pages) — audit result: no changes needed

Read in full: `guide/page.tsx` (index) and all seven topic pages (`basics`, `table`, `hands`,
`achievements`, `profile`, `store`, `community`). This is documentation register throughout — short
declarative sentences describing what a control does and what happens after you click it — not
marketing copy, and it doesn't share the landing page's failure mode (no figurative "poker-culture
slang" phrasing, no vague mood-statements standing in for features). Specific things checked and
found fine:

- `Provably Fair` and `Rabbit hunt` (English terms, `guide/hands/page.tsx`, `guide/table/page.tsx`)
  are established feature names elsewhere in the product (see `docs/specs/2026-08-21-paid-rabbit-hunt.md`
  for Rabbit Hunt), not copy trying to sound clever — left as-is, consistent with treating "Baralho
  verificável" as the plain-language front door to the same underlying "Provably Fair" mechanism.
- No use of "auditável"/"auditoria" anywhere in the guide (the two other non-landing "auditável"
  mentions are `table/layout.tsx`'s meta description and `leaderboard/page.tsx`, both already noted
  below as out of this scope).
- No equivalent of "resenha"/"leitura dos rivais" style phrasing — terms like "leitura pessoal" (a
  player note, `guide/community/page.tsx:71`) and "Radar e badges" (`guide/profile/page.tsx:43`) are
  literal descriptions of on-screen elements, not attempts at poker-culture voice.

No guide file needs an edit for this spec.

Notes on choices:

- The H1 drops the day/night framing entirely instead of patching it (e.g. "a qualquer hora" would
  just add words to deny an implication nobody should have to read past). The existing kicker just
  above it (`Sua próxima mesa já está pronta`, line 84) already carries the "ready now" idea, so the
  H1 doesn't need to re-earn it — it states the actual action (play poker with friends).
- "Baralho verificável" keeps the exact same claim (the shuffle is cryptographically provable, see
  `lib/deckVerify.ts`) with a word people actually use for that ("verificar" vs. "auditar") — a
  one-word swap, no new concept introduced.
- "resenha" and "leitura dos rivais" were both trying to sound like poker-culture slang and landed as
  try-hard instead; the replacement says the same two things (reading opponents, chatting with
  friends) in plain verbs.
- "leia a mesa e guarde as histórias" (hero paragraph) has the same problem one sentence earlier than
  the "novidades" heading that was originally flagged — fixed the same way, plain verbs for the same
  two claims (opponent-reading, hand history).
- "Algumas mãos terminam. Outras viram estrela." reaches for a poetic turn where the line right below
  it already says the real thing (bluffs, all-ins, rare hands earn achievements) — replaced with a
  sentence that names what the section actually does, matching the "Mais formas de interagir na mesa"
  fix applied to the same pattern above it.
- Two other "auditável" mentions exist (`ui/src/app/table/layout.tsx:5`'s meta description, and
  `leaderboard/page.tsx:33`'s "Desempenho auditável") — left alone. Both are describing a *process*
  in a technical/analytics register (a page `<meta description>` and a leaderboard methodology blurb,
  neither is a landing-page trust badge trying to sound approachable), which is a different context
  than the hero chip that was flagged. Worth a follow-up look once someone reads them with fresh eyes,
  but not changed here to keep this a scoped, verifiable diff.

Test update: `ui/src/app/page.test.tsx:26` asserts
`screen.getByRole('heading', {name: /A noite de poker/})` — updates to match the new H1 text.

## Part 2 — Font

Current: `Geist`/`Geist_Mono` via `next/font/google` (`ui/src/app/layout.tsx:2,9-10`), bound to CSS
custom properties `--font-sans`/`--font-mono`. Every consumer in `globals.css` already reads through
those two variable names (`font-family: var(--font-sans)`, `font: ... var(--font-mono)` — dozens of
call sites, e.g. `globals.css:195,261,266,572...`), never the font name directly. That indirection
means **the swap is exactly two lines in `layout.tsx`** — no other file changes, because nothing else
ever named "Geist" to begin with.

The complaint (Inter/Geist read as "generated by an AI coding tool" at this point, but wants
"original," explicitly not "fancy") is a real, common observation — both are Vercel/Google-promoted
defaults that ship in nearly every AI-scaffolded starter today, which is exactly why they read as a
tell rather than a choice.

**Recommendation: IBM Plex Sans + IBM Plex Mono**, both on `next/font/google` (self-hosted at build
time, same as Geist today — no new runtime dependency, no external font-loading network call, honors
`ui/CLAUDE.md`'s "no server at runtime" constraint identically to the current setup). Reasoning:
- Distinct silhouette from Inter/Geist (squarer terminals, more character personality) without being
  a display/"fancy" face — it's a workhorse UI font used in production products (IBM's own design
  system, Red Hat, and others), not a novelty pick.
- Has a matched mono sibling (IBM Plex Mono) for the many `--font-mono` numeric/timer call sites,
  same pairing convenience Geist/Geist Mono currently provides.
- Free (SIL Open Font License), already on Google Fonts, zero licensing overhead — reuse over
  build, same as everything else in this codebase's dependency choices.

**Alternative, if a warmer/rounder voice is preferred over Plex's slightly technical feel**:
`Manrope` (sans) + keep `Geist_Mono` or pair with `IBM Plex Mono` for numerics — Manrope reads
friendlier, still uncommon enough not to read as an AI-tool default.

Implementation, once a choice is made:

```ts
// ui/src/app/layout.tsx
import {IBM_Plex_Sans, IBM_Plex_Mono} from 'next/font/google';
const sans = IBM_Plex_Sans({subsets: ['latin'], weight: ['400', '500', '600', '700'], variable: '--font-sans'});
const mono = IBM_Plex_Mono({subsets: ['latin'], weight: ['400', '500', '600', '700'], variable: '--font-mono'});
```

(`IBM_Plex_Sans`/`IBM_Plex_Mono` are variable in weight only via fixed cuts on Google Fonts, not a
true variable-font axis the way Geist is — hence the explicit `weight` array; check
`globals.css`'s existing `font: <weight> ...` shorthand call sites all land on a weight present in
that array — they currently use 400/600/700/750, so `750` needs to fall back to `700`, a one-line
CSS change where it appears, e.g. `globals.css:2407`.)

## Testing

- `page.test.tsx`: heading assertion updated to the new H1 text; no other test in the suite currently
  string-matches any of the other five rewritten phrases (checked via grep) so no further test
  changes are needed for Part 1.
- Font swap has no unit-testable behavior — visual check only: load the app, confirm `--font-sans`/
  `--font-mono` resolve to the new families in devtools, and manually re-check any `font: 750 ...`
  call site that assumed a Geist-only weight cut.

## Out of scope

- A copy pass on pages other than the landing page and the guide (e.g. `/poker-rules`,
  `/leaderboard`, `/achievements`) — the user's scope was explicitly "a landing page e
  preferencialmente do guia"; those other pages weren't asked for and weren't audited here.
- The two non-landing "auditável" mentions (`table/layout.tsx` meta description,
  `leaderboard/page.tsx`) — different register (technical/methodology, not a landing trust badge),
  left unchanged per the Part 1 notes above.
- A custom/self-hosted (non-Google-Fonts) typeface — adds licensing and asset-hosting work for a
  "not fancy" goal that Plex/Manrope already satisfy for free.
- Changing `font-size`/spacing scale — only the family changes, not the type scale.
