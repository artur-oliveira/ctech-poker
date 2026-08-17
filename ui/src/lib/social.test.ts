import {describe, expect, test} from 'vitest';
import {ApiError} from '@/lib/api/client';
import type {SocialInboxEvent, SocialPlayer} from '@/lib/api/social';
import {
  inviteActionable, inviteExpired, lastPlayedLabel, nameResolver, presenceLabel, RELATIONSHIP_LABELS, SOCIAL_KEYS,
  socialErrorMessage, socialEventCopy, suppressedPlayerIds
} from './social';

function player(overrides: Partial<SocialPlayer>): SocialPlayer {
  return {player_id: 'p1', relationship: 'none', muted: false, blocked: false, ...overrides};
}

function event(overrides: Partial<SocialInboxEvent>): SocialInboxEvent {
  return {
    event_id: 'e1', type: 'table_invite', actor_id: 'a1', status: 'pending', unread: true,
    created_at: 1_000, ...overrides
  };
}

describe('social selectors', () => {
  test('scopes every list under one invalidation root', () => {
    expect(SOCIAL_KEYS.root).toEqual(['social']);
    expect(SOCIAL_KEYS.requests('outgoing')).toEqual(['social', 'requests', 'outgoing']);
    expect(SOCIAL_KEYS.relationships(['b', 'a'])).toEqual(['social', 'relationships', 'a,b']);
    expect(SOCIAL_KEYS.relationships([])).toEqual(['social', 'relationships', '']);
  });

  test('describes presence without ever naming a table', () => {
    expect(presenceLabel('online')).toBe('Online');
    expect(presenceLabel('in_table')).toBe('Em uma mesa');
    expect(presenceLabel()).toBe('Offline');
    expect(RELATIONSHIP_LABELS.friend).toBe('Amigo');
  });

  test('suppresses muted and blocked players and nobody else', () => {
    expect(suppressedPlayerIds([
      player({player_id: 'muted', muted: true}),
      player({player_id: 'blocked', blocked: true}),
      player({player_id: 'friend', relationship: 'friend'})
    ])).toEqual(new Set(['muted', 'blocked']));
    expect(suppressedPlayerIds()).toEqual(new Set());
  });

  test('treats an invite as actionable only while pending and unexpired', () => {
    const live = event({expires_at: 5_000});
    expect(inviteExpired(live, 1_000)).toBe(false);
    expect(inviteActionable(live, 1_000)).toBe(true);
    expect(inviteActionable(live, 9_000)).toBe(false);
    expect(inviteExpired(event({}), 9_000)).toBe(false);
    expect(inviteActionable(event({status: 'accepted'}), 1_000)).toBe(false);
    expect(inviteActionable(event({type: 'friend_request'}), 1_000)).toBe(false);
  });

  test('writes one sentence per inbox event state', () => {
    expect(socialEventCopy(event({type: 'friend_request'}), 'Bia')).toBe('Bia quer ser seu amigo.');
    expect(socialEventCopy(event({type: 'friend_accepted'}), 'Bia'))
      .toBe('Bia aceitou sua solicitação de amizade.');
    expect(socialEventCopy(event({status: 'accepted'}), 'Bia')).toBe('Você aceitou o convite de Bia.');
    expect(socialEventCopy(event({status: 'declined'}), 'Bia')).toBe('Você recusou o convite de Bia.');
    expect(socialEventCopy(event({expires_at: 2_000}), 'Bia', 9_000))
      .toBe('O convite de Bia para uma mesa expirou.');
    expect(socialEventCopy(event({}), 'Bia')).toBe('Bia te convidou para uma mesa.');
  });

  test('describes the recent-player window in days', () => {
    const now = Date.UTC(2026, 7, 17);
    expect(lastPlayedLabel(undefined, now)).toBe('');
    expect(lastPlayedLabel(now - 1_000, now)).toBe('Jogaram hoje');
    expect(lastPlayedLabel(now - 26 * 60 * 60 * 1000, now)).toBe('Jogaram ontem');
    expect(lastPlayedLabel(now - 5 * 24 * 60 * 60 * 1000, now)).toBe('Jogaram há 5 dias');
  });

  test('keeps the conflict message identical whatever the private cause was', () => {
    const conflict = new ApiError('conflict', 409, {type: '/problems/relationship-conflict'});
    expect(socialErrorMessage(conflict)).toBe('Não foi possível concluir essa ação agora.');
    expect(socialErrorMessage(new ApiError('limit', 409, {type: '/problems/friend-limit-reached'})))
      .toBe('Você atingiu o limite de amigos.');
    expect(socialErrorMessage(new ApiError('gone', 404, {type: '/problems/unknown'})))
      .toBe('Jogador não encontrado.');
    expect(socialErrorMessage(new ApiError('boom', 500)))
      .toBe('Não foi possível concluir essa ação. Tente novamente.');
    expect(socialErrorMessage(new Error('offline')))
      .toBe('Não foi possível concluir essa ação. Tente novamente.');
  });

  test('resolves inbox actor names from the lists already on screen', () => {
    const nameOf = nameResolver([player({player_id: 'a1', name: 'Bia'})], [player({player_id: 'a2'})]);
    expect(nameOf('a1')).toBe('Bia');
    expect(nameOf('a2')).toBe('Visitante');
    expect(nameOf('unknown')).toBe('Visitante');
  });
});
