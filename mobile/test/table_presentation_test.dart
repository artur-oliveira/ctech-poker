import 'package:ctech_poker/core/realtime.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/features/poker_icon.dart';
import 'package:ctech_poker/features/table.dart';
import 'package:ctech_poker/generated/poker.pb.dart';
import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('table uses the moderated presentation snapshot', (tester) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final session = PokerSession();
    final live = PokerRealtime(session)..playerId = 'me';
    addTearDown(session.dispose);
    addTearDown(live.dispose);
    live.snapshot = TableSnapshot(stage: 'flop', board: ['Ah', 'Td', '3c'], seats: [
      Seat(playerId: 'me', name: 'Você'), Seat(playerId: 'other', name: 'Bruno'),
    ], reactions: [TableReaction(playerId: 'other', reactionId: 'fire', expiresAt: Int64(99999))]);
    final moderated = live.snapshot!.deepCopy()..reactions.clear();
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: TablePlayArea(
      realtime: live, snapshot: moderated, heroId: 'me', now: 0, onSeat: (_) {},
    ))));
    expect(find.text('Bruno'), findsOneWidget);
    expect(find.text('🔥'), findsNothing);
    expect(live.snapshot!.reactions, hasLength(1), reason: 'presentation must not mutate authoritative state');
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('Lucide icon keeps its requested size inside an app-bar slot', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: SizedBox(width: 56, height: 56,
      child: PokerIcon(PokerIcons.arrowLeft, size: 22)))));
    expect(tester.getSize(find.byType(SvgPicture)), const Size(22, 22));
  });
}
