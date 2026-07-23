const SEEN_KEY = 'ctech_poker_onboarding_seen';

// A curious visitor who reads /poker-rules or /guide directly (from a link,
// not the lobby banner) has already gotten the same value the banner offers
// — so any of the three counts as "seen" for the purpose of not nagging them
// again on their next lobby visit.
export function hasSeenOnboarding(): boolean {
  if (typeof window === 'undefined') return true;
  return window.localStorage.getItem(SEEN_KEY) === 'true';
}

export function markOnboardingSeen() {
  if (typeof window !== 'undefined') window.localStorage.setItem(SEEN_KEY, 'true');
}
