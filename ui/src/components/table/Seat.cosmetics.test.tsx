import {render} from '@testing-library/react';
import {describe, expect, test, vi} from 'vitest';
import {Seat} from './Seat';
import type {SeatView} from '@/lib/api/table';

vi.mock('@/lib/hooks/useDeckVariant', () => ({useDeckVariant: () => 'four-color'}));

/** Every protobuf decode hands the seat a fresh `badge_ids` array, so the
 * prop PlayerAvatar receives must be keyed on content, not identity: the
 * same badges in a new frame keep the same reference. */
const avatarProps: Array<Record<string, unknown>> = [];
vi.mock('@/components/ui/player-avatar', () => ({
  PlayerAvatar: (props: Record<string, unknown>) => {
    avatarProps.push(props);
    return <span/>;
  },
}));

const seat = (badges: string[]): SeatView => ({
  player_id: 'p1', name: 'Bia', stack: 1000, state: 'active', contributed: 0,
  frame_id: 'chip-ring', hat_id: 'crown', accessory_id: 'monocle', badge_ids: badges,
});

describe('Seat cosmetics forwarding', () => {
  test('forwards the proto cosmetics and keeps badgeIds stable while the content is unchanged', () => {
    const {rerender} = render(<Seat seat={seat(['veteran', 'all-in-hero'])} isViewer={false} isTurn={false} index={0}/>);
    rerender(<Seat seat={seat(['veteran', 'all-in-hero'])} isViewer={false} isTurn={false} index={0}/>);
    rerender(<Seat seat={seat(['veteran'])} isViewer={false} isTurn={false} index={0}/>);
    const [first, second, third] = avatarProps;
    expect(first).toMatchObject({frameId: 'chip-ring', hatId: 'crown', accessoryId: 'monocle',
      badgeIds: ['veteran', 'all-in-hero']});
    expect(second.badgeIds).toBe(first.badgeIds);
    expect(third.badgeIds).toEqual(['veteran']);
  });
});
