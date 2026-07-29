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
});
