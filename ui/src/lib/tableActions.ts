import type {TableSnapshot} from '@/lib/api/table';
import type {ActionAvailability} from '@/components/table/ActionBar';

/** Everything the action bar needs, derived from the server's own
 * `legal_actions` — never re-decided locally. */
export function actionState(snapshot: TableSnapshot, viewer?: string) {
  const seat = snapshot.seats.find(item => item.player_id === viewer);
  const serverActions = snapshot.legal_actions;
  const callAmount = Math.min(seat?.stack || 0, Math.max(0, serverActions?.call_amount || 0));
  // An empty current_player_id is a deliberate server state (street
  // transition, runout, showdown), never permission for the viewer to act.
  const isTurn = Boolean(viewer && snapshot.current_player_id === viewer);
  const actions = new Set(serverActions?.actions || []);
  const available: ActionAvailability = {
    fold: actions.has('fold'), check: actions.has('check'), call: actions.has('call'), raise: actions.has('raise')
  };
  const maxRaise = Math.max(0, serverActions?.max_raise_to || 0);
  const minRaise = Math.min(maxRaise, Math.max(0, serverActions?.min_raise_to || 0));
  return {
    available, callAmount, isTurn, minRaise, maxRaise, raiseStep: Math.max(1, serverActions?.step || 1),
    effectiveStack: Math.min(seat?.stack || 0, Math.max(0, ...snapshot.seats
      .filter(item => item.player_id !== viewer && item.state !== 'folded' && item.state !== 'sitting_out')
      .map(item => item.stack))),
    // The ⅓/½/⅔/Pote entries are only present when the server actually sent
    // that raise-to figure: `stageBetPresets` reads them by label and drops the
    // fraction it cannot price, rather than pricing it at the minimum and
    // claiming a bigger fraction costs less than a smaller one.
    raisePresets: [
      {label: 'Mín', value: minRaise},
      ...(serverActions?.one_third_pot_raise_to ? [{label: '⅓ pote', value: serverActions.one_third_pot_raise_to}] : []),
      ...(serverActions?.half_pot_raise_to ? [{label: '½ pote', value: serverActions.half_pot_raise_to}] : []),
      ...(serverActions?.two_thirds_pot_raise_to ? [{label: '⅔ pote', value: serverActions.two_thirds_pot_raise_to}] : []),
      ...(serverActions?.pot_raise_to ? [{label: 'Pote', value: serverActions.pot_raise_to}] : []),
      {label: 'Máx', value: maxRaise}
    ]
  };
}
