import {act, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, test, vi} from 'vitest';
import {useInViewOnce} from './useInViewOnce';

function Probe({rootMargin}: {rootMargin?: string} = {}) {
  const [ref, seen] = useInViewOnce(rootMargin);
  return <section ref={ref} data-testid="section">{seen ? 'armed' : 'waiting'}</section>;
}

type Observed = {callback: IntersectionObserverCallback; options?: IntersectionObserverInit};

function stubObserver() {
  const observed: Observed[] = [];
  const disconnect = vi.fn();
  const observe = vi.fn();
  vi.stubGlobal('IntersectionObserver', class {
    constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
      observed.push({callback, options});
    }
    observe = observe;
    disconnect = disconnect;
    unobserve = vi.fn();
    takeRecords = () => [];
    root = null;
    rootMargin = '';
    thresholds = [];
  });
  return {observed, disconnect, observe};
}

afterEach(() => vi.unstubAllGlobals());

describe('useInViewOnce', () => {
  // jsdom and the static export both run without an IntersectionObserver. A
  // latch that could never open there would hide content, so it starts armed.
  test('starts armed where there is no IntersectionObserver', () => {
    expect(window.IntersectionObserver).toBeUndefined();
    render(<Probe/>);
    expect(screen.getByTestId('section')).toHaveTextContent('armed');
  });

  test('arms once the node comes into view and stops observing', () => {
    const {observed, disconnect, observe} = stubObserver();
    render(<Probe/>);
    expect(screen.getByTestId('section')).toHaveTextContent('waiting');
    expect(observe).toHaveBeenCalled();
    expect(observed[0].options?.rootMargin).toBe('400px');

    act(() => observed[0].callback([{isIntersecting: false} as IntersectionObserverEntry],
      {} as IntersectionObserver));
    expect(screen.getByTestId('section')).toHaveTextContent('waiting');

    act(() => observed[0].callback([{isIntersecting: true} as IntersectionObserverEntry],
      {} as IntersectionObserver));
    expect(screen.getByTestId('section')).toHaveTextContent('armed');
    expect(disconnect).toHaveBeenCalled();
  });

  test('takes a custom root margin and disconnects on unmount', () => {
    const {observed, disconnect} = stubObserver();
    const {unmount} = render(<Probe rootMargin="0px"/>);
    expect(observed[0].options?.rootMargin).toBe('0px');
    unmount();
    expect(disconnect).toHaveBeenCalled();
  });
});
