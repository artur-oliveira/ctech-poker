import {act, renderHook} from '@testing-library/react';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import type {ReactNode} from 'react';
import {afterEach, beforeEach, describe, expect, test, vi} from 'vitest';
import {hasSeenOnboarding, markOnboardingSeen} from './onboarding';
import {playSound, setSoundEffectsEnabled, type SoundName} from './sound';
import {routeMetadata} from './routeMetadata';
import {OG_PREVIEWS} from './ogPreviews';
import {useCountUp} from './hooks/useCountUp';
import {useDealerVoice} from './hooks/useDealerVoice';
import {useDeckVariant} from './hooks/useDeckVariant';
import {DEFAULT_DECK_VARIANT} from './cardVariants';
import {useTablePreferences} from './tablePreferences';

const {getAccessToken, getMe} = vi.hoisted(() => ({getAccessToken: vi.fn(), getMe: vi.fn()}));
vi.mock('./api/client', () => ({getAccessToken, apiClient: {}}));
vi.mock('./api/player', () => ({getMe}));

describe('onboarding memory', () => {
  test('starts unseen and stays seen once marked', () => {
    expect(hasSeenOnboarding()).toBe(false);
    markOnboardingSeen();
    expect(hasSeenOnboarding()).toBe(true);
  });
});

describe('sound playback', () => {
  // Sound effects are preloaded and pooled per file at enable time (one
  // Audio per file, reused on every play), not allocated fresh on each
  // playSound() call — so playback is asserted on the shared `play` mock,
  // keyed by the source of the instance it was invoked on, not on
  // construction. See sound.test.ts for the pool's own lifecycle coverage.
  const play = vi.fn<(source: string) => Promise<void>>();

  beforeEach(() => {
    play.mockReset().mockResolvedValue(undefined);
    vi.stubGlobal('Audio', class {
      preload = '';
      currentTime = 0;

      constructor(public src: string) {
      }

      play = () => play(this.src);
      pause = vi.fn();
      load = vi.fn();
      removeAttribute = vi.fn();
    });
    setSoundEffectsEnabled(true);
    // Drop the pool-priming unlock play() calls fired by enabling above.
    play.mockClear();
  });

  afterEach(() => {
    setSoundEffectsEnabled(false);
    vi.unstubAllGlobals();
  });

  test.each<SoundName>(['reveal', 'showing_card', 'half_pot', 'all_in', 'bet', 'your_turn'])(
    'plays an existing mp3 for %s', name => {
      playSound(name);
      expect(play).toHaveBeenCalledOnce();
      expect(play.mock.calls[0][0]).toMatch(/^\/sounds\/[a-z0-9-]+\.mp3$/);
    });

  test('picks between the alternate chip samples', () => {
    const random = vi.spyOn(Math, 'random').mockReturnValue(0);
    playSound('bet');
    random.mockReturnValue(.99);
    playSound('bet');
    expect(new Set(play.mock.calls.map(call => call[0])).size).toBe(2);
  });

  test('swallows the autoplay rejection instead of surfacing an unhandled error', () => {
    play.mockRejectedValueOnce(new DOMException('autoplay blocked', 'NotAllowedError'));
    expect(() => playSound('your_turn')).not.toThrow();
  });
});

describe('routeMetadata', () => {
  test('builds canonical, OG and Twitter tags from a single route description', () => {
    const metadata = routeMetadata({title: 'Lobby', description: 'Escolha uma mesa.', path: '/lobby', image: 'lobby'});
    expect(metadata.alternates?.canonical).toBe('/lobby');
    expect(metadata.openGraph?.url).toBe('/lobby');
    expect(metadata.openGraph?.locale).toBe('pt_BR');
    expect((metadata.openGraph?.images as {url: string}[])[0].url).toBe('/og/lobby.webp');
    expect((metadata.openGraph?.images as {url: string}[])[1].url).toBe('/og-image.webp');
    expect((metadata.twitter as {card: string}).card).toBe('summary_large_image');
  });

  test('keeps routes out of the index unless explicitly opted in', () => {
    const base = {title: 'Mesa', description: 'Jogue.', path: '/table', image: 'table'};
    expect(routeMetadata(base).robots).toEqual({index: false, follow: false});
    expect(routeMetadata({...base, index: true}).robots).toEqual({index: true, follow: true});
  });

  test('every OG preview target is a route path', () => {
    for (const preview of OG_PREVIEWS) {
      expect(preview.route.startsWith('/')).toBe(true);
      expect(preview.title.length).toBeGreaterThan(0);
    }
    expect(new Set(OG_PREVIEWS.map(preview => preview.slug)).size).toBe(OG_PREVIEWS.length);
  });
});

describe('useCountUp', () => {
  test('animates to the target across frames and lands exactly on it', () => {
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => frames.push(callback));
    vi.stubGlobal('cancelAnimationFrame', vi.fn());
    vi.spyOn(performance, 'now').mockReturnValue(0);

    const {result} = renderHook(() => useCountUp(0, 100, 100));
    expect(result.current).toBe(0);
    act(() => frames[0](50));
    expect(result.current).toBeGreaterThan(0);
    expect(result.current).toBeLessThan(100);
    act(() => frames[1](100));
    expect(result.current).toBe(100);
    vi.unstubAllGlobals();
  });

  test('jumps straight to the target when the value did not change', () => {
    const {result} = renderHook(() => useCountUp(42, 42));
    expect(result.current).toBe(42);
  });

  test('skips the animation entirely under reduced motion', () => {
    vi.mocked(window.matchMedia).mockReturnValueOnce({matches: true} as MediaQueryList);
    const raf = vi.spyOn(window, 'requestAnimationFrame');
    const {result} = renderHook(() => useCountUp(0, 500));
    expect(result.current).toBe(500);
    expect(raf).not.toHaveBeenCalled();
  });
});

describe('useDealerVoice', () => {
  const speak = vi.fn();
  const cancel = vi.fn();

  beforeEach(() => {
    speak.mockReset();
    cancel.mockReset();
    vi.stubGlobal('SpeechSynthesisUtterance', class {
      lang = '';
      rate = 0;
      pitch = 0;

      constructor(public text: string) {
      }
    });
    Object.defineProperty(window, 'speechSynthesis', {configurable: true, value: {speak, cancel}});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    Reflect.deleteProperty(window, 'speechSynthesis');
  });

  test('announces in pt-BR and cancels whatever was still being spoken', () => {
    const {unmount} = renderHook(() => useDealerVoice('Sua vez', true));
    expect(cancel).toHaveBeenCalledOnce();
    expect(speak).toHaveBeenCalledOnce();
    expect(speak.mock.calls[0][0]).toMatchObject({text: 'Sua vez', lang: 'pt-BR'});
    unmount();
    expect(cancel).toHaveBeenCalledTimes(2);
  });

  test('stays silent when disabled or without a message', () => {
    renderHook(() => useDealerVoice('Sua vez', false));
    renderHook(() => useDealerVoice('', true));
    expect(speak).not.toHaveBeenCalled();
  });

  test('does nothing on a browser without speech synthesis', () => {
    Reflect.deleteProperty(window, 'speechSynthesis');
    expect(() => renderHook(() => useDealerVoice('Sua vez', true))).not.toThrow();
  });
});

describe('useDeckVariant', () => {
  const wrapper = ({children}: {children: ReactNode}) => {
    const client = new QueryClient({defaultOptions: {queries: {retry: false}}});
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };

  beforeEach(() => {
    getAccessToken.mockReset();
    getMe.mockReset();
  });

  test('falls back to the default variant while signed out and never fetches', () => {
    getAccessToken.mockReturnValue(null);
    const {result} = renderHook(() => useDeckVariant(), {wrapper});
    expect(result.current).toBe(DEFAULT_DECK_VARIANT);
    expect(getMe).not.toHaveBeenCalled();
  });

  test('uses the variant stored on the player profile', async () => {
    getAccessToken.mockReturnValue('token');
    getMe.mockResolvedValue({deck_variant: 'colorblind'});
    const {result} = renderHook(() => useDeckVariant(), {wrapper});
    await vi.waitFor(() => expect(result.current).toBe('colorblind'));
  });
});

describe('useTablePreferences', () => {
  test('discards a corrupted or out-of-range payload instead of applying it', () => {
    localStorage.setItem('ctech-poker:table-preferences:v1', 'not json');
    const {result: corrupted} = renderHook(() => useTablePreferences());
    expect(corrupted.current.preferences).toEqual({
      soundEffects: false, dealerVoice: false, voiceCommands: false, realityCheckMinutes: 60, equityTrainer: false
    });

    // A stale blob from before table_theme moved server-side (still carrying
    // a "theme" key) round-trips harmlessly: the extra key is ignored.
    localStorage.setItem('ctech-poker:table-preferences:v1',
      JSON.stringify({theme: 'neon', dealerVoice: 'yes', realityCheckMinutes: 7}));
    const {result} = renderHook(() => useTablePreferences());
    expect(result.current.preferences).toEqual({
      soundEffects: false, dealerVoice: false, voiceCommands: false, realityCheckMinutes: 60, equityTrainer: false
    });
  });

  test('propagates a change written by another tab', () => {
    const {result} = renderHook(() => useTablePreferences());
    act(() => {
      localStorage.setItem('ctech-poker:table-preferences:v1', JSON.stringify({realityCheckMinutes: 90}));
      window.dispatchEvent(new StorageEvent('storage', {key: 'ctech-poker:table-preferences:v1'}));
    });
    expect(result.current.preferences.realityCheckMinutes).toBe(90);
  });
});
