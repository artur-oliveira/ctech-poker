import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/realtime.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/features/table.dart';
import 'package:ctech_poker/generated/poker.pb.dart';
import 'package:flutter/material.dart';
import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets(
    'open chat immediately reflects moderation changes without another socket frame',
    (tester) async {
      final session = PokerSession(), revision = ValueNotifier<int>(0);
      final api = PokerApi(session),
          realtime = PokerRealtime(session)..playerId = 'me';
      realtime.snapshot = TableSnapshot(
        handId: 'hand',
        seats: [Seat(playerId: 'other', name: 'Outro')],
        reactions: [
          TableReaction(
            id: 'reaction',
            playerId: 'other',
            reactionId: 'tomato',
            targetPlayerId: 'me',
            expiresAt: Int64(9999999999999),
          ),
        ],
        chatMessages: [
          ChatMessage(
            id: 'message',
            playerId: 'other',
            message: 'Mensagem da mesa',
          ),
        ],
      );
      var ready = false, hidden = false;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ChatPanel(
              api: api,
              roomId: 'room',
              moderation: revision,
              ready: () => ready,
              retryModeration: () {},
              realtime: realtime,
              allowed: (id) => ready && !hidden,
            ),
          ),
        ),
      );
      expect(find.text('Mensagem da mesa'), findsNothing);
      expect(find.byTooltip('Denunciar reação'), findsNothing);
      expect(find.text('🍅'), findsNothing);
      ready = true;
      revision.value++;
      await tester.pump();
      expect(find.text('Mensagem da mesa'), findsOneWidget);
      expect(find.byTooltip('Denunciar mensagem'), findsOneWidget);
      expect(find.byTooltip('Denunciar reação'), findsOneWidget);
      expect(find.text('🍅'), findsOneWidget);
      hidden = true;
      revision.value++;
      await tester.pump();
      expect(find.text('Mensagem da mesa'), findsNothing);
      expect(find.byTooltip('Denunciar reação'), findsNothing);
      expect(find.text('🍅'), findsNothing);
      await tester.pumpWidget(const SizedBox());
      realtime.dispose();
      revision.dispose();
      api.close();
      session.dispose();
    },
  );
}
