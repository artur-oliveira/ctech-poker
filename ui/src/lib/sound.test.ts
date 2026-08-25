import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {playSound, setSoundEffectsEnabled, soundEffectsEnabled} from './sound';

const play = vi.fn<() => Promise<void>>();
const audio = vi.fn(function AudioMock(this: {src: string}, src: string) {
  this.src = src;
  return {src, play};
});

describe('table sound effects', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    play.mockResolvedValue();
    vi.stubGlobal('Audio', audio);
    setSoundEffectsEnabled(false);
  });

  afterEach(() => {
    setSoundEffectsEnabled(false);
    vi.unstubAllGlobals();
  });

  test('starts silent and never constructs audio before explicit opt-in', () => {
    expect(soundEffectsEnabled()).toBe(false);
    expect(playSound('your_turn')).toBe(false);
    expect(audio).not.toHaveBeenCalled();
  });

  test('plays the requested effect after opt-in', () => {
    setSoundEffectsEnabled(true);
    expect(playSound('reveal')).toBe(true);
    expect(audio).toHaveBeenCalledWith('/sounds/revealing-card-table.mp3');
    expect(play).toHaveBeenCalledOnce();
  });

  test('swallows autoplay rejection while keeping the preference enabled', async () => {
    play.mockRejectedValueOnce(new DOMException('blocked', 'NotAllowedError'));
    setSoundEffectsEnabled(true);
    expect(playSound('showing_card')).toBe(true);
    await Promise.resolve();
    expect(soundEffectsEnabled()).toBe(true);
  });
});
