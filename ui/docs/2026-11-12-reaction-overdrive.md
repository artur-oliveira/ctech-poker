# 2026-11-12 — Every reaction has its own choreography

Presentation only: `src/app/(app)/table-reactions.css`, the `REACTION_THEATER` accents in `TableReactions.tsx` and the
captions in `lib/reactions.ts`. Labels, ids, the wire, scopes, cooldown, mute and the catalogue are unchanged. The
projectile is the animated Noto WebP (see `2026-11-12-noto-emoji-reactions.md`); each impact is CSS around it.

## Contract every reaction keeps

- CSS only (no library, canvas, SVG filter or sound); `transform`/`opacity` only; colours from `:root` tokens.
- Impact visuals live inside `.reaction-impact`, so the `prefers-reduced-motion` block (impact hidden, projectile landed)
  covers all of them. No OS emoji text inside an impact except the deliberate suit/note glyphs listed below.
- An impact that travels in px (anything on `--reaction-dx/dy`) opts out of the compact `.reaction-impact { scale(.64) }`
  and scales its own sizes (`--u`, `--knife-k`, `--chip-k`). Travel is `calc()` over the two vars so it re-aims on resize.
- Self emotes: punchlines land before ~2.3 s (the emote wrapper is opaque until ~74% of 3.2 s).
- `renderer.css` still has legacy `.reaction-impact-<id>` rules; the new blocks beat them by specificity
  (`.reaction-impact.reaction-impact-<id>`). Those legacy rules and a few keyframes are now dead (see "Left over").

## The catalogue

| id | caption | accent | idea |
|---|---|---|---|
| clap | Palmas. Bem devagar. | OVAÇÃO | three sarcastic slow claps that turn into a standing ovation |
| laugh | Kkkkk... tô sem ar | KKKKK | an escalating laughing fit that runs out of air |
| wow | Você jogou isso mesmo? | UAU?! | double take, jaw drop, two shock rings |
| angry | Tô a um bad beat de surtar | PSSSHH! | pressure cooker venting steam |
| cry | Chora, mas paga o blind | SNIF | tears from the eyes, a puddle forms where the stack was |
| nervous | Tá tudo sob controle | TUDO BEM | escalating tremble and sweat, the calm banner insists |
| cold | Mesa virou geladeira | BRRR! | frost creeps in, the face freezes in a cube, cracks |
| fire | Chama o bombeiro | QUENTE | shake, ignition ring, six flames of six heights |
| respect | F pelo stack caído | F | parade-ground salute, memorial light, one falling "F" |
| sleepy | Acorda quando subir o blind | BLIND! | three nods, z's as a blind clock, jolt awake |
| heartbeat | Reza que o coração aguenta | ALL IN | ECG blips, flatline, slam and shock disc |
| shark | Tubarão não dorme, blefa | NHAC! | Jaws fin in two steps, the bite lands on the banner |
| pokerface | Blefe ou nuts? Adivinha. | BLEFE? | the face never moves while the suits vibrate, then freeze |
| chip | Toma, vai comprar talento | GORJETA | eleven chips flicked into a pile, a twelfth tossed like a coin |
| coffee | Café pra quem dormiu | ACORDOU? | cup slides in, leaves a ring on the felt, caffeine jolt |
| clover | Te abençoo. Vai precisar | AMÉM | clover assembles leaf by leaf, the late leaf goes limp |
| horseshoe | Ímã de ficha, hein? | SÓ SORTE | a magnet: the field ring closes in, chips snap on and are pulled back |
| tear | Chora mais, eu aguento | VIOLINO | drop breaks, a second drop falls in the crater, sad notes |
| tomato | Esse blefe pede molho | MOLHO! | lands flat, slumps, long drip, seeds |
| poop | Chegou o delivery | PLOFT | plop, squash, bounce, wavy fumes and flies |
| rofl | Sua jogada é piada pronta | HA HA HA | rolls on the floor, "HA"/"HÁ" bubbles ricochet between seats |
| duck | Pato confirmado | QUÁ! | rubber duck squeaks, floats lame in a puddle |
| turtle | Tá no timebank desde ontem | TIMEBANK | the turtle crawls in 8 steps while a clock fast-forwards on the target |
| knife | Nada pessoal, parça | ET TU? | a second dagger from behind, quivers, the eight-drip bleed |
| flowers | Foi bonito enquanto durou | R.I.P. | funeral bouquet delivered straight-faced, black sash |
| spotlight | Tá escrito na sua testa | FLAGRA! | interrogation lamp cone, generic card-back gag (never real cards) |
| crown | Agora o problema é seu, rei | SE VIRA! | a crown one size too big slips over one eye |
| bandage | Passa com beijinho | TALVEZ | band-aid closes a hairline crack, then peels off |
| cucumber | Olha atrás, gatinho | SUSTO! | the whole cucumber appears behind the target, jump scare |
| boomerang | Vai, mas volta com juros | KARMA | hits the target, returns to the sender's seat |

Deliberate glyph text kept: `respect` ✦, `sleepy` z/Z, `heartbeat` ♥ ·, `shark` ≈, `pokerface` ♠♦♣♥ (shape carries the
suit, never colour alone), `horseshoe` ★, `boomerang` ★, `tear` ♪ ♫.

## Left over (not deleted on purpose)

Legacy `renderer.css` reaction rules and keyframes that no reaction uses any more (for example `reaction-shiver`,
`reaction-ember-fan`, `reaction-chip-scatter`, `reaction-splat`/`-word`/`-drop`, `.reaction-clover-glow`). Grep before
removing: some are still referenced by the reduced-motion lists.

## Verification limits (read before trusting it)

Each reaction was checked by freezing the animation (`getAnimations()` + `currentTime`) on hand-injected markup, in
Chromium only, at 1400 px and 390 px (the browser window would not go narrower than ~500 px, so 390 was emulated).
Premium reactions open the purchase dialog in the mock, so none was fired through the real thrown flow, and the
projectile flight was not observed. No real-time 60 fps run, no Firefox/WebKit, no handset. Per-reaction notes on
frames that were not seen live in the PR description.
