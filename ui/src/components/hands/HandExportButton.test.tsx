import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, test, vi} from 'vitest';
import type {HandItem} from '@/lib/api/player';
import type {HandHistoryAction} from '@/lib/api/table';
import {HandExportButton} from './HandExportButton';

const serializeHand = vi.hoisted(() => vi.fn(() => 'exported hand'));
vi.mock('@/lib/handExport', () => ({serializeHand}));

describe('HandExportButton', () => {
  test('serializes and downloads the selected hand, then releases the object URL', async () => {
    const hand = {hand_id: 'hand-42'} as HandItem;
    const actions: HandHistoryAction[] = [];
    const createObjectURL = vi.fn(() => 'blob:hand');
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, 'createObjectURL', {configurable: true, value: createObjectURL});
    Object.defineProperty(URL, 'revokeObjectURL', {configurable: true, value: revokeObjectURL});
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);
    
    render(<HandExportButton hand={hand} actions={actions} viewerId="viewer"/>);
    await userEvent.click(screen.getByRole('button', {name: /Exportar/}));
    
    expect(serializeHand).toHaveBeenCalledWith(hand, actions, 'viewer');
    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
    expect(click).toHaveBeenCalledOnce();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:hand');
  });
});
