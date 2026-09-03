'use client';
import {useCallback} from 'react';

/** Where each seated player's card currently is on screen.
 *
 * The reaction layer is a sibling of the seats, not an ancestor, so it has no
 * ref path to them. It used to find them with
 * `document.querySelectorAll('.game-seat[data-player-id]')` — an implicit
 * contract a class rename in `Seat` would break with no type error. `Seat`
 * already holds its own element, so it publishes it here instead and the
 * layer measures on demand.
 *
 * A module singleton rather than a context: seat elements are viewport
 * geometry, nothing re-renders when one changes, and the table mounts exactly
 * one stage (the same reason `lib/api/client.ts` keeps the token this way). */
const seatElements = new Map<string, HTMLElement>();

/** Publishes `node` as the seat of `playerId` until it unmounts.
 *
 * Returns the React 19 ref cleanup, which only removes the entry when it is
 * still the element registered — a seat re-keyed onto a new node registers
 * the replacement before the old ref is torn down. */
export function registerSeatElement(playerId: string, node: HTMLElement | null) {
  if (!node) return undefined;
  seatElements.set(playerId, node);
  return () => {
    if (seatElements.get(playerId) === node) seatElements.delete(playerId);
  };
}

/** The ref callback for a seat's root element, stable per player. */
export function useSeatElementRef(playerId: string) {
  return useCallback((node: HTMLElement | null) => registerSeatElement(playerId, node), [playerId]);
}

/** Viewport centre of a seated player's card, or undefined when that player
 * has no seat on screen (not seated, or the stage swapped layouts mid-flight). */
export function seatCenter(playerId: string) {
  const node = seatElements.get(playerId);
  if (!node) return undefined;
  const rect = node.getBoundingClientRect();
  return {x: rect.left + rect.width / 2, y: rect.top + rect.height / 2};
}
