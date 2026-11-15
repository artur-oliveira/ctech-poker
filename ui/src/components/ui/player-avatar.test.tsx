import {fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {avatarItemLabel} from '@/lib/avatarCosmetics';
import {PlayerAvatar} from './player-avatar';

// A fake hat keeps layer order and unknown-id handling independent of the
// art. Frames use the real FRAME_ART.
vi.mock('@/components/avatar/hats', () => ({
  HAT_ART: {'test-hat': () => <svg data-art="hat"/>},
  HAT_RISE: 22 / 64,
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
                                              frameId="chip-ring" hatId="test-hat" accessoryId="devil-horns"/>);
    const wrapper = container.firstElementChild!;
    expect(wrapper).toHaveClass('player-avatar-decorated');
    // The avatar keeps its own class (the seat CSS hooks it) and sits first.
    expect(wrapper.firstElementChild).toHaveClass('seat-avatar');
    const art = wrapper.querySelectorAll('.player-avatar-art');
    expect(art).toHaveLength(1);
    expect(art[0]).toHaveAttribute('aria-hidden', 'true');
    // Horns sit on the frame ring under the hat, which rests on top and paints last.
    const layers = [...art[0].children].map(el => el.getAttribute('data-art') ?? el.getAttribute('data-band') ?? 'frame');
    expect(layers).toEqual(['frame', 'horns', 'hat']);
    expect(art[0].firstElementChild!.getAttribute('viewBox')).toBe('0 0 64 64');
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

  test('an unknown accessory alone leaves the avatar plain', () => {
    const {container} = render(<PlayerAvatar className="seat-avatar" name="Ana Beatriz" size={36} accessoryId="horns"/>);
    expect(container.querySelector('.player-avatar-decorated')).toBeNull();
  });

  test('wings paint behind the photo and reserve room above and beside a sized avatar', () => {
    const {container} = render(<PlayerAvatar className="seat-avatar" name="Ana" size={64} accessoryId="angel-wings"/>);
    const wrapper = container.querySelector<HTMLElement>('.player-avatar-decorated')!;
    // First in tree order: the wrapper's own stacking context puts it (z-index -1) under the avatar.
    expect(wrapper.firstElementChild).toHaveClass('player-avatar-wings');
    expect(wrapper.firstElementChild).toHaveAttribute('aria-hidden', 'true');
    expect(wrapper.querySelector('.player-avatar-wings > svg')).toHaveAttribute('data-band', 'wings');
    expect(wrapper.children[1]).toHaveClass('seat-avatar');
    expect(wrapper.querySelector('.player-avatar-art')).toBeNull();
    expect(wrapper.style.getPropertyValue('--avatar-hat-room')).toBe('10px');
    expect(wrapper.style.getPropertyValue('--avatar-wing-room')).toBe('18px');
  });

  test('horns sit in the art layer and reserve the hat room; no wing room', () => {
    const {container} = render(<PlayerAvatar name="Ana" size={64} accessoryId="ram-horns"/>);
    const wrapper = container.querySelector<HTMLElement>('.player-avatar-decorated')!;
    expect([...wrapper.querySelector('.player-avatar-art')!.children].map(el => el.getAttribute('data-band'))).toEqual(['horns']);
    expect(wrapper.querySelector('.player-avatar-wings')).toBeNull();
    expect(wrapper.style.getPropertyValue('--avatar-hat-room')).toBe('22px');
    expect(wrapper.style.getPropertyValue('--avatar-wing-room')).toBe('');
  });

  test('unknown badge ids still render the fallback medal with the raw id as tooltip', () => {
    render(<PlayerAvatar name="Ana" badgeIds={['future-badge']}/>);
    expect(screen.getByTitle('future-badge').querySelector('svg')).not.toBeNull();
  });

  test('TestBadgeChipUsesRegistryLabel: a known badge renders its own glyph, tone and registry title', () => {
    render(<PlayerAvatar name="Ana" badgeIds={['rainmaker']}/>);
    const chip = screen.getByTitle(avatarItemLabel('badge', 'rainmaker'));
    expect(chip.querySelector('svg.lucide-gem')).not.toBeNull();
    expect(chip.querySelector('svg.lucide-medal')).toBeNull();
    expect(chip.style.getPropertyValue('--badge-bg')).toMatch(/^var\(--cosmetic-/);
    expect(chip.style.getPropertyValue('--badge-fg')).toMatch(/^var\(--cosmetic-/);
  });

  test('an unknown badge falls back to Medal with no tone style', () => {
    render(<PlayerAvatar name="Ana" badgeIds={['future-badge']}/>);
    const chip = screen.getByTitle('future-badge');
    expect(chip.querySelector('svg.lucide-medal')).not.toBeNull();
    expect(chip).not.toHaveAttribute('style');
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
