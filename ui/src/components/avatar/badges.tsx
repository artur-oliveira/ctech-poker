import {Gem, Flame, Medal, ShieldCheck, Spade, Trophy, Users, type LucideIcon} from 'lucide-react';

/** Badge glyph by id; an id missing here falls back to `Medal` in PlayerAvatar. */
export const BADGE_ART: Record<string, LucideIcon> = {
  'season-2026-q3-top10': Medal,
  'season-2026-q3-champion': Trophy,
  'all-in-hero': Flame,
  'iron-nerve': ShieldCheck,
  'rainmaker': Gem,
  'veteran': Spade,
  'social-butterfly': Users,
};

type Token = 'gold' | 'gold-pale' | 'oxblood' | 'felt' | 'walnut' | 'paper' | 'ink' | 'wine';

/** Chip colours by id, `--cosmetic-*` tokens; every pair is >= 5.8:1. Glyph
 * differs per id too, so tone is never the only cue. */
export const BADGE_TONE: Record<string, { bg: Token; fg: Token }> = {
  'season-2026-q3-top10': {bg: 'gold', fg: 'ink'},
  'season-2026-q3-champion': {bg: 'gold', fg: 'ink'},
  'all-in-hero': {bg: 'oxblood', fg: 'paper'},
  'iron-nerve': {bg: 'felt', fg: 'paper'},
  'rainmaker': {bg: 'gold-pale', fg: 'wine'},
  'veteran': {bg: 'walnut', fg: 'paper'},
  'social-butterfly': {bg: 'wine', fg: 'gold-pale'},
};
