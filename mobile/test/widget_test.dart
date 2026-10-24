import 'package:ctech_poker/features/widgets.dart';
import 'package:ctech_poker/features/table.dart';
import 'package:ctech_poker/generated/poker.pb.dart';
import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('hidden cards never expose a rank or suit', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(home: Scaffold(body: PlayingCards(['back', 'back']))),
    );
    expect(find.bySemanticsLabel('Carta oculta'), findsNWidgets(2));
    expect(find.text('Ah'), findsNothing);
  });
  for (final size in [
    const Size(320, 568),
    const Size(390, 844),
    const Size(844, 390),
    const Size(768, 1024),
  ]) {
    for (final scale in [1.0, 2.0]) {
      testWidgets('nine-seat felt fits $size at text scale $scale', (
        tester,
      ) async {
        tester.view.physicalSize = size;
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        final snapshot = TableSnapshot(
          board: ['Ah', 'Kd', '2c'],
          currentPlayerId: '1',
          actionDeadlineUnixMs: Int64(10000),
          seats: [
            for (var i = 0; i < 9; i++)
              Seat(
                playerId: '$i',
                name: 'Jogador com nome longo $i',
                stack: Int64(12345678),
                holeCards: ['back', 'back'],
              ),
          ],
        );
        await tester.pumpWidget(
          MaterialApp(
            home: MediaQuery(
              data: MediaQueryData(
                size: size,
                textScaler: TextScaler.linear(scale),
              ),
              child: Scaffold(
                body: TableFelt(
                  snapshot: snapshot,
                  heroId: '0',
                  now: 0,
                  onSeat: (_) {},
                ),
              ),
            ),
          ),
        );
        expect(tester.takeException(), isNull);
        expect(find.text('Jogador com nome longo 0'), findsNothing);
      });
    }
  }
}
