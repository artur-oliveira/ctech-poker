import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/core/replay.dart';
import 'package:ctech_poker/features/ranking.dart';
import 'package:ctech_poker/features/library.dart';
import 'package:ctech_poker/features/report_player.dart';
import 'package:ctech_poker/features/statistics.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class JourneyApi extends PokerApi {
  JourneyApi() : super(PokerSession());
  int rankRequests = 0;
  final pages = <String?>[];
  @override
  Future<Json> get(String path, {Map<String, String>? query}) async {
    expect(query?['mode'], 'sandbox');
    if (path.endsWith('/me')) {
      if (++rankRequests == 1) throw const ApiFailure(503, 'Unavailable', null);
      return {
        'ranked': true,
        'rank': 103,
        'total': 200,
        'entry': {'hands_won': 5, 'win_rate': .2},
      };
    }
    if (path.endsWith('poker-stats')) {
      return {
        'hands': 10,
        'vpip_hands': 3,
        'vpip_rate': .3,
        'pfr_hands': 2,
        'pfr_rate': .2,
        'three_bet_chances': 0,
        'three_bet_hands': 0,
        'three_bet_rate': 0,
      };
    }
    pages.add(query?['cursor']);
    final second = query?['cursor'] != null;
    return {
      'data': [
        {
          'player_id': second ? 'two' : 'one',
          'player_name': second ? 'Bia' : 'Ana',
          'hands_won': 4,
          'hands_played': 10,
          'win_rate': .4,
        },
      ],
      'has_next': !second,
      if (!second) 'next_cursor': 'page2',
    };
  }
}

class HistoryApi extends PokerApi {
  HistoryApi() : super(PokerSession());
  @override
  Future<Json> get(String path, {Map<String, String>? query}) async => {
    'actions': [
      {
        'action': 'check',
        'frame': {
          'stage': 'flop',
          'board': ['Ah', '2d', '3c'],
        },
      },
      {
        'action': 'end',
        'frame': {
          'stage': 'complete',
          'board': ['Ah', '2d', '3c', '4s', '5h'],
        },
      },
    ],
  };
}

class ReportApi extends PokerApi {
  ReportApi() : super(PokerSession());
  final keys = <String?>[];
  final bodies = <Json?>[];
  @override
  Future<dynamic> request(
    String path, {
    String method = 'GET',
    Json? body,
    Map<String, String>? query,
    String? idempotencyKey,
  }) async {
    keys.add(idempotencyKey);
    bodies.add(body);
    if (keys.length == 1) throw const ApiFailure(503, 'Indisponível', null);
    return {};
  }
}

void main() {
  testWidgets(
    'replay hides final opponent information and pauses in background',
    (tester) async {
      final api = HistoryApi();
      await tester.pumpWidget(
        MaterialApp(
          home: HandScreen(
            api: api,
            hand: const {
              'hand_id': 'h',
              'table_id': 't',
              'opponents': [
                {
                  'name': 'Vencedor final',
                  'hole_cards': ['Ks', 'Kd'],
                },
              ],
            },
          ),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Vencedor final'), findsNothing);
      await tester.tap(find.byTooltip('Reproduzir replay'));
      await tester.pump();
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await tester.pump(const Duration(seconds: 3));
      expect(find.text('Vencedor final'), findsNothing);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.tap(find.text('Resultado'));
      await tester.pumpAndSettle();
      expect(find.text('Vencedor final'), findsOneWidget);
      await tester.tap(find.byTooltip('Primeira ação'));
      await tester.pumpAndSettle();
      expect(find.text('Vencedor final'), findsNothing);
      api.close();
      api.session.dispose();
    },
  );
  testWidgets('report preserves category context and operation key on retry', (
    tester,
  ) async {
    final api = ReportApi();
    await tester.pumpWidget(
      MaterialApp(
        home: ReportPlayerScreen(
          api: api,
          playerId: 'other',
          surface: 'table_behavior',
          tableId: 'table',
          handId: 'hand',
          actionId: 'message-1',
        ),
      ),
    );
    await tester.tap(find.text('Trapaça ou conluio'));
    await tester.enterText(find.byType(TextField), 'Relato da mesa');
    await tester.ensureVisible(find.text('Enviar denúncia'));
    await tester.tap(find.text('Enviar denúncia'));
    await tester.pumpAndSettle();
    await tester.drag(find.byType(ListView), const Offset(0, -300));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Tentar o mesmo envio'));
    await tester.pumpAndSettle();
    expect(api.keys[0], isNotEmpty);
    expect(api.keys[0], api.keys[1]);
    expect(api.bodies[0], api.bodies[1]);
    expect(api.bodies[0]!['category'], 'cheating');
    expect(api.bodies[0]!['table_id'], 'table');
    expect(api.bodies[0]!['hand_id'], 'hand');
    expect(api.bodies[0]!['action_id'], 'message-1');
    expect(find.text('Denúncia registrada para revisão.'), findsOneWidget);
    api.close();
    api.session.dispose();
  });

  testWidgets(
    'global personal rank is independent and positions continue across pages',
    (tester) async {
      final api = JourneyApi();
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(body: RankingScreen(api: api)),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Ana'), findsOneWidget);
      await tester.tap(find.text('Tentar minha posição novamente'));
      await tester.pumpAndSettle();
      expect(find.text('#103 de 200 jogadores'), findsOneWidget);
      await tester.tap(find.text('Carregar mais'));
      await tester.pumpAndSettle();
      expect(find.text('Bia'), findsOneWidget);
      expect(find.text('2'), findsOneWidget);
      expect(api.pages, [null, 'page2']);
      api.close();
      api.session.dispose();
    },
  );
  testWidgets('statistics distinguish no opportunity from zero success', (
    tester,
  ) async {
    final api = JourneyApi();
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: StatisticsScreen(api: api)),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('3 de 10 mãos'), findsOneWidget);
    await tester.drag(find.byType(ListView), const Offset(0, -400));
    await tester.pumpAndSettle();
    expect(find.text('0 de 0 oportunidades'), findsOneWidget);
    expect(find.text('Sem amostra'), findsOneWidget);
    api.close();
    api.session.dispose();
  });
  testWidgets('replay supports pause, speed, seeking and stops at last frame', (
    tester,
  ) async {
    final replay = ReplayController()..configure(4);
    replay.toggle();
    await tester.pump(const Duration(seconds: 1));
    expect(replay.index, 1);
    replay.pause();
    await tester.pump(const Duration(seconds: 3));
    expect(replay.index, 1);
    replay.cycleSpeed();
    expect(replay.speed, 2);
    replay.toggle();
    await tester.pump(const Duration(milliseconds: 500));
    expect(replay.index, 2);
    await tester.pump(const Duration(milliseconds: 500));
    expect(replay.index, 3);
    expect(replay.playing, false);
    replay.toggle();
    expect(replay.index, 0);
    replay.seek(2);
    expect(replay.playing, false);
    replay.seek(100);
    expect(replay.index, 3);
    replay.configure(0);
    replay.toggle();
    expect(replay.playing, false);
    replay.dispose();
  });
}
