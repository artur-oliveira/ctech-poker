# Occupancy-balanced table seating

## Why

The table previously selected visual slots from the room capacity. A partially
filled full-ring room therefore looked unbalanced, and the ninth desktop seat
shared the lower edge with the viewer. Portrait seats were stable but did not
recompose for the number of players currently present.

## Behavior

- Seat geometry is presentation-only; server order, actions and card data are
  unchanged.
- The seated viewer is index zero and remains at the bottom.
- Desktop and tablet-landscape ovals divide the occupied perimeter evenly.
  Short phone landscape uses the compact-seat stage beside its action dock so
  full seat cards cannot collide in the shallow 320px play area. All three give
  heads-up a face-to-face axis, three players a reversed triangle and four a
  diamond.
- Portrait layouts sample a nine-point capsule path that follows the visual
  center of the walnut/gold rail. This avoids the lower pair cutting inside the
  felt, which happens when a capsule is approximated by an ellipse.
- Portrait top and lower-arc seats separate hole cards, identity and bet chips
  into distinct lanes. Bet chips remain on the felt and never share the hole
  card lane.
- The portrait felt and rail use concentric insets so the walnut band keeps a
  consistent apparent thickness instead of stretching along the sides.
- The decorative felt wordmark yields at occupancies where a top player owns
  its band.

## Local QA scenarios

Mock mode exposes `heads_up`, `layout_3`, `layout_4`, `layout_5`, `six_max`,
`layout_7`, `layout_8`, and `nine_max`. Together they exercise every supported
current occupancy from two through nine without implying additional game
formats beyond Heads-Up, 6-MAX and Full Ring.

Automated component tests assert the canonical 2/3/4 shapes, the capsule rail
anchors, all eight mock occupancies, and both desktop and portrait stage trees.
Browser geometry QA checks player seats, hole-card groups, identity labels,
bets, the board, wordmark, header and action dock for intersections and viewport
overflow at phone portrait/landscape, tablet portrait/landscape and desktop
sizes.
