import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/session.dart';

/// Offline review data. Never included in the application bundle.
class ReviewSession extends PokerSession {
  @override
  Future<String> token({bool force = false}) async =>
      throw StateError('Offline visual review');
}

class ReviewApi extends PokerApi {
  ReviewApi() : super(ReviewSession());
  bool hasPending = false;
  static const hand = <String, dynamic>{
    'hand_id': 'mao-demonstracao',
    'table_id': 'mesa-amigos',
    'outcome': 'won',
    'net_change': 1250,
    'ended_at': 1789470000000,
    'hole_cards': ['As', 'Kh'],
    'board': ['Ah', 'Td', '3c', '7s', '2h'],
  };
  static const achievements = [
    {
      'key': 'wins',
      'unlocked': true,
      'progress': 28,
      'next_target': 50,
      'stars': 2,
    },
    {
      'key': 'hands_played',
      'unlocked': true,
      'progress': 240,
      'next_target': 500,
      'stars': 3,
    },
    {
      'key': 'bluff',
      'unlocked': true,
      'progress': 8,
      'next_target': 10,
      'stars': 1,
    },
    {
      'key': 'win_category_royal_flush',
      'unlocked': false,
      'progress': 0,
      'next_target': 1,
      'stars': 0,
    },
  ];
  @override
  Future<Json?> pendingOperation() async => hasPending
      ? {
          'id': 'review',
          'path': '/v1.0/rooms/join-or-create',
          'label': 'Entrada na mesa',
          'amount': 2000,
          'created_at': 1789470000000,
        }
      : null;
  @override
  Future<dynamic> request(
    String path, {
    String method = 'GET',
    Json? body,
    Map<String, String>? query,
    String? idempotencyKey,
  }) async =>
      throw StateError('Review fixtures prohibit mutations: $method $path');
  @override
  Future<Json> get(String path, {Map<String, String>? query}) async {
    if (path.endsWith('/history')) {
      return {
        'actions': [
          {
            'action': 'call',
            'amount': 200,
            'frame': {
              'stage': 'flop',
              'board': ['Ah', 'Td', '3c'],
              'seats': [
                {'name': 'Ana', 'stack': 8400, 'state': ''},
                {'name': 'Bruno', 'stack': 6200, 'state': ''},
              ],
            },
          },
          {
            'action': 'check',
            'amount': 0,
            'frame': {
              'stage': 'turn',
              'board': ['Ah', 'Td', '3c', '7s'],
            },
          },
        ],
      };
    }
    if (path.endsWith('/meta')) {
      return {
        'street_notes': {
          'preflop': 'Aumentei em posição com AKo.',
          'flop': 'Top pair. Rever o tamanho da aposta.',
        },
        'collections': ['Mãos para estudar'],
        'review_marked': true,
      };
    }
    if (path.endsWith('/showcase')) {
      return {
        'name': 'Bruno',
        'showcase_public': true,
        'featured_achievements': [
          {'key': 'wins', 'count': 28},
        ],
        'best_hand': hand,
      };
    }
    if (path.contains('/matchups/')) {
      return {
        'hands_together': 48,
        'viewer_wins': 12,
        'opponent_wins': 10,
        'ties': 1,
        'heads_up_hands_together': 6,
        'net_change_viewer': 850,
      };
    }
    if (path.contains('/wallet/')) {
      if (path.endsWith('/skus')) {
        return {
          'data': [
            {'id': 'starter', 'total_credits': 10000, 'price_cents': 990},
            {'id': 'plus', 'total_credits': 25000, 'price_cents': 1990},
          ],
        };
      }
      if (path.endsWith('/catalog')) {
        return {
          'data': [
            {
              'id': path.contains('/deck')
                  ? 'classic'
                  : path.contains('/felt')
                  ? 'classic'
                  : 'nice_hand',
              'owned': true,
            },
            {
              'id': path.contains('/deck')
                  ? 'four_color'
                  : path.contains('/felt')
                  ? 'midnight'
                  : 'fire',
              'price_cents': 490,
              'price_fichas': 2500,
            },
          ],
        };
      }
      if (path.endsWith('/')) {
        return {
          'data': [
            {
              'purchase_id': 'review',
              'sku': 'starter',
              'status': 'confirmed',
              'price_cents': 990,
            },
          ],
        };
      }
    }
    if (path.startsWith('/v1.0/social/')) {
      if (path.endsWith('/inbox')) {
        return {
          'data': [
            {
              'event_id': 'invite',
              'actor_id': 'bruno',
              'actor_name': 'Bruno',
              'type': 'table_invite',
              'status': 'pending',
              'unread': true,
            },
            {
              'event_id': 'friend',
              'actor_name': 'Carla',
              'type': 'friend_accepted',
              'status': 'accepted',
              'unread': false,
            },
          ],
        };
      }
      return {
        'data': [
          {
            'player_id': 'bruno',
            'name': 'Bruno',
            'presence': 'in_table',
            'blocked': path.endsWith('/blocked'),
          },
          {'player_id': 'carla', 'name': 'Carla', 'presence': 'online'},
        ],
      };
    }
    return switch (path) {
      '/v1.0/players/me' => {
        'user_id': 'ana',
        'name': 'Ana',
        'sandbox_balance': 8400,
        'friend_code': 'ANA-2026',
        'poker_terms_accepted': true,
        'showcase_public': true,
        'table_public': true,
        'playstyle_public': false,
        'featured_achievements': ['wins'],
        'favorite_reactions': ['nice_hand'],
        'bet_preset_mode': 'mixed',
      },
      '/v1.0/rooms/stakes' => {
        'stakes': [
          {'small_blind': 10, 'big_blind': 20},
        ],
      },
      '/v1.0/rooms/buckets' => {
        'data': [
          {
            'small_blind': 10,
            'big_blind': 20,
            'max_seats': 6,
            'seats_available': 3, 'open_rooms': 1,
          },
        ],
      },
      '/v1.0/players/me/sessions' => {
        'data': [
          {
            'table_id': 'mesa-amigos',
            'ended_at': 1789470000000,
            'buyin_amount': 2000,
            'cashout_amount': 3250,
            'net_pnl': 1250,
          },
        ],
      },
      '/v1.0/players/me/hands' => {
        'data': [
          hand,
          {
            ...hand,
            'hand_id': 'segunda-mao',
            'outcome': 'lost',
            'net_change': -400,
            'hole_cards': ['Qs', 'Qd'],
          },
        ],
      },
      '/v1.0/players/me/hand-filters' => {'data': []},
      '/v1.0/players/me/hand-collections' => {
        'data': [
          {
            'hand_id': 'mao-demonstracao',
            'collections': ['Mãos para estudar'],
            'review_marked': true,
          },
        ],
      },
      '/v1.0/players/me/hand-shares' => {
        'data': [
          {'kind': 'brag', 'net_change': 1250, 'token': 'review'},
        ],
      },
      '/v1.0/players/me/achievements/summary' => {
        'totals': {'stars': 6},
        'achievements': achievements,
      },
      '/v1.0/players/me/poker-stats' => {
        'hands': 240,
        'vpip_hands': 60,
        'vpip_rate': .25,
        'pfr_hands': 48,
        'pfr_rate': .2,
        'three_bet_chances': 50,
        'three_bet_hands': 4,
        'three_bet_rate': .08,
      },
      '/v1.0/leaderboard/me' => {
        'ranked': true,
        'rank': 12,
        'total': 180,
        'entry': {'hands_won': 28, 'win_rate': .35},
      },
      '/v1.0/leaderboard' => {
        'data': [
          for (final name in ['Carla', 'Bruno', 'Ana'])
            {
              'player_id': name.toLowerCase(),
              'player_name': name,
              'hands_won': 28,
              'hands_played': 80,
              'win_rate': .35,
            },
        ],
      },
      '/v1.0/sandbox-credits/' => {
        'current_streak': 3,
        'best_streak': 7,
        'cycle_day': 4,
        'cycle_length': 7,
        'protection_available': true,
        'remaining_time_seconds': 0,
        'days': [
          for (var i = 1; i <= 7; i++)
            {'day': i, 'amount': i * 500, 'claimed': i < 4},
        ],
      },
      _ => throw StateError('Missing review fixture: $path'),
    };
  }
}
