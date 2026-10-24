import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/features/share_hand.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class ShareApi extends PokerApi {
  ShareApi() : super(PokerSession());
  Json? body;
  String? deleted;
  @override
  Future<Json> post(String path, [Json? body]) async {
    this.body = body;
    return {'token': 'public token'};
  }

  @override
  Future<dynamic> request(
    String path, {
    String method = 'GET',
    Json? body,
    Map<String, String>? query,
    String? idempotencyKey,
  }) async {
    expect(method, 'DELETE');
    deleted = path;
    return {};
  }
}

void main() {
  testWidgets(
    'share defaults keep cards private; chosen consent and expiry are sent; revoke works',
    (tester) async {
      final api = ShareApi();
      await tester.pumpWidget(
        MaterialApp(
          home: ShareHandScreen(api: api, handId: 'hand'),
        ),
      );
      expect(
        tester.widget<SwitchListTile>(find.byType(SwitchListTile)).value,
        false,
      );
      expect(api.body, isNull);
      await tester.tap(find.text('Bad beat'));
      await tester.tap(find.byType(SwitchListTile));
      await tester.tap(find.text('30 dias'));
      await tester.pump();
      await tester.ensureVisible(find.text('Criar link público'));
      await tester.tap(find.text('Criar link público'));
      await tester.pumpAndSettle();
      expect(api.body, {
        'kind': 'bad_beat',
        'include_hero_cards': true,
        'expiry_days': 30,
        'mode': 'sandbox',
      });
      expect(
        find.text('https://poker.aoctech.app/share?token=public%20token'),
        findsOneWidget,
      );
      await tester.tap(find.text('Revogar link'));
      await tester.pumpAndSettle();
      expect(api.deleted, '/v1.0/players/me/hand-shares/public%20token');
      expect(find.text('Link criado'), findsNothing);
      api.close();
      api.session.dispose();
    },
  );
}
