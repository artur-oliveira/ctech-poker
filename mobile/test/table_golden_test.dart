import 'package:ctech_poker/features/poker_icon.dart';
import 'package:ctech_poker/core/design.dart';
import 'support/brand_fonts.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ctech_poker/features/table.dart';
import 'package:ctech_poker/core/realtime.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/generated/poker.pb.dart';
import 'package:fixnum/fixnum.dart';

void main() {
  testWidgets('portrait table keeps board and decision in the same screen', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await loadBrandFonts();
    final session = PokerSession();
    final live = PokerRealtime(session)
      ..playerId = 'me'
      ..phase = ConnectionPhase.live;
    live.snapshot = TableSnapshot(
      handId: 'hand',
      stage: 'flop',
      currentPlayerId: 'me',
      snapshotVersion: Int64(10),
      board: ['Ah', 'Td', '3c'],
      dealerPlayerId: '2',
      bigBlindPlayerId: '4',
      smallBlindPlayerId: '3',
      actionDeadlineUnixMs: Int64(30000),
      pots: [Pot(amount: Int64(1250))],
      legalActions: LegalActions(
        actions: ['fold', 'call', 'raise'],
        callAmount: Int64(200),
        minRaiseTo: Int64(400),
        maxRaiseTo: Int64(8400),
      ),
      seats: [
        Seat(
          playerId: 'me',
          name: 'Você',
          stack: Int64(8400),
          holeCards: ['As', 'Kh'],
          ready: true,
          timeBankMs: Int64(45000),
        ),
        for (var i = 0; i < 8; i++)
          Seat(
            playerId: '$i',
            name: [
              'Ana',
              'Bruno',
              'Carla',
              'Diego',
              'Elisa',
              'Fábio',
              'Gabi',
              'Hugo',
            ][i],
            stack: Int64(5000 + i * 250),
            holeCards: ['back', 'back'],
          ),
      ],
    );
    await tester.pumpWidget(
      MaterialApp(
        theme: PokerTheme.dark,
        home: RepaintBoundary(
          key: const Key('table'),
          child: Scaffold(
            appBar: AppBar(
              title: const Text('Flop'),
              leading: const PokerIcon(PokerIcons.arrowLeft),
              actions: const [
                PokerIcon(PokerIcons.messageCircle),
                SizedBox(width: 20),
                PokerIcon(PokerIcons.ellipsisVertical),
                SizedBox(width: 12),
              ],
            ),
            body: SafeArea(
              child: TablePlayArea(
                realtime: live,
                heroId: 'me',
                now: 12000,
                onSeat: (_) {},
              ),
            ),
          ),
        ),
      ),
    );
    await tester.runAsync(
      () => precacheImage(
        const AssetImage('assets/brand/logo.png'),
        tester.element(find.byType(MaterialApp)),
      ),
    );
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(find.text('Aumentar'), findsOneWidget);
    await expectLater(
      find.byKey(const Key('table')),
      matchesGoldenFile('goldens/table_portrait.png'),
    );
    live.dispose();
    session.dispose();
  });
}
