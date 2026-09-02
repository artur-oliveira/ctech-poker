import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {playSound, setSoundEffectsEnabled, soundEffectsEnabled} from './sound';

type AudioMock = {
  src: string;
  preload: string;
  currentTime: number;
  play: ReturnType<typeof vi.fn>;
  pause: ReturnType<typeof vi.fn>;
  load: ReturnType<typeof vi.fn>;
  removeAttribute: ReturnType<typeof vi.fn>;
};

const instances: AudioMock[] = [];
const play = vi.fn<() => Promise<void>>();

const audio = vi.fn(function AudioCtor(this: AudioMock, src: string) {
  this.src = src;
  this.preload = '';
  this.currentTime = 0;
  this.play = play;
  this.pause = vi.fn();
  this.load = vi.fn();
  this.removeAttribute = vi.fn(() => {
    this.src = '';
  });
  instances.push(this);
  return this;
});

const ALL_FILES = [
  '/sounds/revealing-card-table.mp3',
  '/sounds/player-showing-card.mp3',
  '/sounds/half-pot-chips.mp3',
  '/sounds/all-in-chips.mp3',
  '/sounds/basic-chips-1.mp3',
  '/sounds/basic-chips-2.mp3',
  '/sounds/your-turn.mp3'
];

describe('table sound effects', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    instances.length = 0;
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

  test('preloads one pooled element per sound file at enable time, not first play', () => {
    setSoundEffectsEnabled(true);
    const constructed = new Set(audio.mock.calls.map(call => call[0]));
    expect(constructed).toEqual(new Set(ALL_FILES));
    for (const instance of instances) {
      expect(instance.preload).toBe('auto');
      expect(instance.load).toHaveBeenCalled();
    }
    // The unlock play() happens once per element inside the enabling gesture.
    expect(play).toHaveBeenCalledTimes(ALL_FILES.length);
  });

  test('re-enabling while already enabled does not rebuild the pool', () => {
    setSoundEffectsEnabled(true);
    audio.mockClear();
    setSoundEffectsEnabled(true);
    expect(audio).not.toHaveBeenCalled();
  });

  test('plays the requested effect from the pool without allocating a new Audio', () => {
    setSoundEffectsEnabled(true);
    audio.mockClear();
    play.mockClear();
    expect(playSound('reveal')).toBe(true);
    expect(audio).not.toHaveBeenCalled();
    expect(play).toHaveBeenCalledOnce();
    const revealElement = instances.find(instance => instance.src === '/sounds/revealing-card-table.mp3');
    expect(revealElement?.currentTime).toBe(0);
  });

  test('rewinds a pooled element that is mid-play', () => {
    setSoundEffectsEnabled(true);
    const turnElement = instances.find(instance => instance.src === '/sounds/your-turn.mp3');
    turnElement!.currentTime = 4;
    playSound('your_turn');
    expect(turnElement!.currentTime).toBe(0);
  });

  test('disabling releases the pool so later plays fall back to a fresh Audio', () => {
    setSoundEffectsEnabled(true);
    const pooled = [...instances];
    setSoundEffectsEnabled(false);
    for (const instance of pooled) {
      expect(instance.pause).toHaveBeenCalled();
      expect(instance.removeAttribute).toHaveBeenCalledWith('src');
    }
    setSoundEffectsEnabled(true);
    audio.mockClear();
    playSound('bet');
    // Pool was rebuilt on the second enable, so playback still comes from the pool.
    expect(audio).not.toHaveBeenCalled();
  });

  test('swallows autoplay rejection while keeping the preference enabled', async () => {
    play.mockRejectedValue(new DOMException('blocked', 'NotAllowedError'));
    setSoundEffectsEnabled(true);
    expect(playSound('showing_card')).toBe(true);
    await Promise.resolve();
    expect(soundEffectsEnabled()).toBe(true);
  });

  test('falls back to a constructed Audio when the pool has no entry (no Audio at enable)', () => {
    setSoundEffectsEnabled(false);
    vi.stubGlobal('Audio', undefined);
    setSoundEffectsEnabled(true);
    vi.stubGlobal('Audio', audio);
    expect(playSound('all_in')).toBe(true);
    expect(audio).toHaveBeenCalledWith('/sounds/all-in-chips.mp3');
  });
});
