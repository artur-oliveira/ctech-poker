import {describe, expect, test} from 'vitest';
import {registerSeatElement, seatCenter} from './seatRects';

function seatNode(rect: {left: number; top: number; width: number; height: number}) {
  const node = document.createElement('div');
  node.getBoundingClientRect = () => ({...rect, right: rect.left + rect.width,
    bottom: rect.top + rect.height, x: rect.left, y: rect.top, toJSON: () => undefined}) as DOMRect;
  return node;
}

describe('seatRects', () => {
  test('reports the live centre of a registered seat', () => {
    const stop = registerSeatElement('alice', seatNode({left: 100, top: 40, width: 60, height: 20}));

    expect(seatCenter('alice')).toEqual({x: 130, y: 50});

    stop?.();
    expect(seatCenter('alice')).toBeUndefined();
  });

  test('re-measures instead of caching, so a moved seat reports its new centre', () => {
    let left = 0;
    const node = document.createElement('div');
    node.getBoundingClientRect = () => ({left, top: 0, width: 40, height: 40, right: left + 40,
      bottom: 40, x: left, y: 0, toJSON: () => undefined}) as DOMRect;
    const stop = registerSeatElement('bob', node);

    expect(seatCenter('bob')).toEqual({x: 20, y: 20});
    left = 300;
    expect(seatCenter('bob')).toEqual({x: 320, y: 20});

    stop?.();
  });

  test('has nothing to publish for a null node and nothing to report for an unseated player', () => {
    expect(registerSeatElement('nobody', null)).toBeUndefined();
    expect(seatCenter('nobody')).toBeUndefined();
  });

  test('a seat re-keyed onto a new element keeps the replacement when the old ref is torn down', () => {
    const stopOld = registerSeatElement('carol', seatNode({left: 0, top: 0, width: 10, height: 10}));
    const stopNew = registerSeatElement('carol', seatNode({left: 90, top: 90, width: 20, height: 20}));

    stopOld?.();
    expect(seatCenter('carol')).toEqual({x: 100, y: 100});

    stopNew?.();
    expect(seatCenter('carol')).toBeUndefined();
  });
});
