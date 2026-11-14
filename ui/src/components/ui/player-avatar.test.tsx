import {fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {PlayerAvatar} from './player-avatar';

// Hats/accessories are filled by later tasks; register fakes so layer order
// and unknown-id handling are testable now. Frames use the real FRAME_ART.
vi.mock('@/components/avatar/hats', () => ({
  HAT_ART: {'test-hat': () => <svg data-art="hat"/>},
}));
vi.mock('@/components/avatar/accessories', () => ({
  ACCESSORY_ART: {'test-accessory': () => <svg data-art="accessory"/>},
}));

describe('PlayerAvatar', () => {
  test('uses shared initials without a URL', () => {
    render(<PlayerAvatar name="Ana Beatriz"/>);
    expect(screen.getByRole('img', {name: 'Avatar de Ana Beatriz'})).toHaveTextContent('AB');
  });
  
  test('keeps the initials fallback when the image fails', () => {
    const {container} = render(<PlayerAvatar name="Ana Beatriz" avatarUrl="/missing.jpg"/>);
    const image = container.querySelector('img');
    if (image) fireEvent.error(image);
    expect(screen.getByRole('img', {name: 'Avatar de Ana Beatriz'})).toHaveTextContent('AB');
  });

  // #292: frame/badges are optional decoration. Every caller that never
  // passes them (every seat, every other avatar in the app) must render
  // byte-identical to before this field existed.
  test('renders the plain avatar with no wrapper when no frame/badges are equipped', () => {
    const {container} = render(<PlayerAvatar name="Ana Beatriz"/>);
    expect(container.querySelector('.player-avatar-decorated')).not.toBeInTheDocument();
    expect(screen.getByRole('img', {name: 'Avatar de Ana Beatriz'})).not.toHaveAttribute('data-frame');
  });

  test('renders up to three equipped badges with their labels as tooltips', () => {
    render(<PlayerAvatar name="Ana Beatriz" badgeIds={['season-2026-q3-top10', 'season-2026-q3-champion']}/>);
    expect(screen.getByTitle('Top 10 da Temporada')).toBeInTheDocument();
    expect(screen.getByTitle('Campeão da Temporada')).toBeInTheDocument();
  });

  test('the photo cannot be dragged to another tab or saved from a context menu', async () => {
    // jsdom never loads images; let the avatar's preloader succeed.
    class LoadedImage {
      onload: (() => void) | null = null;
      set src(_value: string) { queueMicrotask(() => this.onload?.()); }
    }
    vi.stubGlobal('Image', LoadedImage);
    const {container} = render(<PlayerAvatar name="Ana Beatriz" avatarUrl="/ana.jpg"/>);
    const image = await vi.waitFor(() => {
      const found = container.querySelector('img');
      if (!found) throw new Error('image not mounted');
      return found;
    });
    expect(image).toHaveAttribute('draggable', 'false');
    expect(fireEvent.contextMenu(image)).toBe(false);
  });

  test('TestNoDecorIsByteIdentical', () => {
    const {container} = render(<PlayerAvatar className="seat-avatar" name="Ana Beatriz" size={36}/>);
    expect(container.innerHTML).toMatchInlineSnapshot(`"<span role="img" aria-label="Avatar de Ana Beatriz" class="flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-full bg-[var(--brand)] text-sm font-bold text-[var(--on-brand)] seat-avatar" style="width: 36px; height: 36px;"><span class="flex size-full items-center justify-center uppercase">AB</span></span>"`);
  });

  test('TestLayerOrder', () => {
    const {container} = render(<PlayerAvatar className="seat-avatar" name="Ana Beatriz"
                                              frameId="chip-ring" hatId="test-hat" accessoryId="test-accessory"/>);
    const wrapper = container.firstElementChild!;
    expect(wrapper).toHaveClass('player-avatar-decorated');
    // The avatar keeps its own class (the seat CSS hooks it) and sits first.
    expect(wrapper.firstElementChild).toHaveClass('seat-avatar');
    const art = wrapper.querySelectorAll('.player-avatar-art');
    expect(art).toHaveLength(1);
    expect(art[0]).toHaveAttribute('aria-hidden', 'true');
    const layers = [...art[0].children].map(el => el.getAttribute('data-art') ?? 'frame');
    expect(layers).toEqual(['accessory', 'hat', 'frame']);
    expect(art[0].lastElementChild!.getAttribute('viewBox')).toBe('0 0 64 64');
    // The frame is the SVG ring now, not an outline hook on the avatar.
    expect(screen.getByRole('img', {name: 'Avatar de Ana Beatriz'})).not.toHaveAttribute('data-frame');
  });

  test('TestUnknownIdsRenderNothing', () => {
    const {container, rerender} = render(<PlayerAvatar name="Ana" frameId="nope" hatId="nope" accessoryId="nope"/>);
    expect(container.querySelector('.player-avatar-art')?.children ?? []).toHaveLength(0);
    expect(screen.getByRole('img', {name: 'Avatar de Ana'})).toBeInTheDocument();
    rerender(<PlayerAvatar name="Ana" frameId="nope" hatId="test-hat" accessoryId="nope"/>);
    const layers = [...container.querySelector('.player-avatar-art')!.children];
    expect(layers.map(el => el.getAttribute('data-art'))).toEqual(['hat']);
  });

  test('unknown badge ids still render the fallback medal with the raw id as tooltip', () => {
    render(<PlayerAvatar name="Ana" badgeIds={['future-badge']}/>);
    expect(screen.getByTitle('future-badge').querySelector('svg')).not.toBeNull();
  });

  // Seat does not forward decoration yet (Task 7); this pins the seat's state
  // hooks against the same DOM Seat renders: `.game-seat.<state> .seat-avatar`
  // must still land on the avatar element itself, never on the wrapper or art.
  test.each(['folded', 'all_in'])('TestFoldedAndAllInSeatKeepStateStyling %s', (state) => {
    const {container} = render(<div className={`game-seat ${state}`}>
      <PlayerAvatar className="seat-avatar" name="Ana" frameId="chip-ring" decorative/>
    </div>);
    const hooked = container.querySelectorAll(`.game-seat.${state} .seat-avatar`);
    expect(hooked).toHaveLength(1);
    expect(hooked[0]).not.toHaveClass('player-avatar-decorated');
    expect(hooked[0]).toHaveTextContent('A');
    expect(hooked[0].querySelector('.player-avatar-art')).toBeNull();
    expect(container.querySelector('.player-avatar-art')).toHaveAttribute('aria-hidden', 'true');
  });

  afterEach(() => vi.unstubAllGlobals());
});
