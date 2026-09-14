import 'dart:async';

import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/features/report_player.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class EvidenceApi extends PokerApi {
  EvidenceApi() : super(PokerSession());
  final history = Completer<Json>();
  final paths = <String>[];
  Json? report;
  @override
  Future<Json> get(String path, {Map<String, String>? query}) {
    paths.add(path);
    return history.future;
  }

  @override
  Future<dynamic> request(
    String path, {
    String method = 'GET',
    Json? body,
    Map<String, String>? query,
    String? idempotencyKey,
  }) async {
    report = body;
    return {};
  }
}

Widget screen(
  EvidenceApi api, {
  String hand = 'hand',
  String action = 'event',
}) => MaterialApp(
  home: Scaffold(
    body: TableEventReportButton(
      api: api,
      tableId: 'table',
      handId: hand,
      playerId: 'sender',
      actionId: action,
      reaction: true,
    ),
  ),
);

void main() {
  testWidgets(
    'reaction report keeps sender and original hand during rollover',
    (tester) async {
      final api = EvidenceApi();
      addTearDown(api.close);
      addTearDown(api.session.dispose);
      await tester.pumpWidget(screen(api));
      await tester.tap(find.byTooltip('Denunciar reação'));
      await tester.pump();
      expect(find.byTooltip('Verificando evento…'), findsOneWidget);
      await tester.tap(find.byType(IconButton));
      expect(api.paths, ['/v1.0/tables/table/hands/hand/history']);
      await tester.pumpWidget(
        screen(api, hand: 'next-hand', action: 'next-event'),
      );
      api.history.complete({
        'actions': [
          {'action_id': 'event', 'player_id': 'sender', 'action': 'reaction'},
        ],
      });
      await tester.pumpAndSettle();
      final report = tester.widget<ReportPlayerScreen>(
        find.byType(ReportPlayerScreen),
      );
      expect(report.playerId, 'sender');
      expect(report.handId, 'hand');
      expect(report.actionId, 'event');
      expect(report.surface, 'table_reaction');
      await tester.ensureVisible(find.text('Enviar denúncia'));
      await tester.tap(find.text('Enviar denúncia'));
      await tester.pumpAndSettle();
      expect(api.report, containsPair('target_player_id', 'sender'));
      expect(api.report, containsPair('action_id', 'event'));
      expect(api.report, containsPair('hand_id', 'hand'));
      expect(api.report, containsPair('surface', 'table_reaction'));
    },
  );

  for (final action in <Json>[
    {},
    {'action_id': 'event', 'player_id': 'someone-else', 'action': 'reaction'},
    {'action_id': 'event', 'player_id': 'sender', 'action': 'chat'},
    {'action_id': 'other-event', 'player_id': 'sender', 'action': 'reaction'},
  ]) {
    testWidgets('rejects missing or mismatched reaction evidence: $action', (
      tester,
    ) async {
      final api = EvidenceApi();
      addTearDown(api.close);
      addTearDown(api.session.dispose);
      api.history.complete({
        'actions': [action],
      });
      await tester.pumpWidget(screen(api));
      await tester.tap(find.byTooltip('Denunciar reação'));
      await tester.pumpAndSettle();
      expect(find.byType(ReportPlayerScreen), findsNothing);
      expect(
        find.textContaining('Use a denúncia de comportamento'),
        findsOneWidget,
      );
      expect(api.report, isNull);
    });
  }

  testWidgets('history failure allows retry and missing hand disables report', (
    tester,
  ) async {
    final api = EvidenceApi();
    addTearDown(api.close);
    addTearDown(api.session.dispose);
    await tester.pumpWidget(screen(api, hand: ''));
    expect(
      tester.widget<IconButton>(find.byType(IconButton)).onPressed,
      isNull,
    );
    await tester.pumpWidget(screen(api));
    await tester.tap(find.byTooltip('Denunciar reação'));
    api.history.completeError(const ApiFailure(503, 'Tente novamente', null));
    await tester.pumpAndSettle();
    expect(find.byType(ReportPlayerScreen), findsNothing);
    expect(find.text('Tente novamente'), findsOneWidget);
    expect(
      tester.widget<IconButton>(find.byType(IconButton)).onPressed,
      isNotNull,
    );
  });
}
