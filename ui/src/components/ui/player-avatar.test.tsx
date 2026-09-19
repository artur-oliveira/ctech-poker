import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {PlayerAvatar} from './player-avatar';

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

  test('marks the avatar element with the equipped frame id', () => {
    render(<PlayerAvatar name="Ana Beatriz" frameId="season-2026-q3"/>);
    expect(screen.getByRole('img', {name: 'Avatar de Ana Beatriz'})).toHaveAttribute('data-frame', 'season-2026-q3');
  });

  test('renders up to three equipped badges with their labels as tooltips', () => {
    render(<PlayerAvatar name="Ana Beatriz" badgeIds={['season-2026-q3-top10', 'season-2026-q3-champion']}/>);
    expect(screen.getByTitle('Top 10 da Temporada')).toBeInTheDocument();
    expect(screen.getByTitle('Campeão da Temporada')).toBeInTheDocument();
  });
});
