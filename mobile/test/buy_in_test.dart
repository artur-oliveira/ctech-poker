import 'dart:async';
import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/features/buy_in.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class BuyApi extends PokerApi {
  BuyApi() : super(PokerSession());
  final calls = <Json>[];
  Completer<Json>? result;
  @override
  Future<Json> durablePost(String path, Json body, {required String label}) {
    calls.add(Map.of(body));
    return (result = Completer<Json>()).future;
  }
}

void main() {
  testWidgets(
    'buy-in validates range, prevents double tap and retries identical consent',
    (tester) async {
      final api = BuyApi();
      await tester.pumpWidget(
        MaterialApp(
          home: BuyInScreen(
            api: api,
            path: '/join',
            body: const {'share_code': 'invite'},
            minimum: 400,
            maximum: 1000,
          ),
        ),
      );
      await tester.enterText(find.byType(TextField), '1');
      await tester.tap(find.text('Confirmar entrada'));
      await tester.pump();
      expect(api.calls, isEmpty);
      await tester.enterText(find.byType(TextField), '800');
      await tester.tap(find.byType(SwitchListTile));
      await tester.pump();
      await tester.tap(find.text('Confirmar entrada'));
      await tester.tap(find.text('Confirmar entrada'));
      await tester.pump();
      expect(api.calls, hasLength(1));
      expect(api.calls.single['auto_rebuy'], true);
      api.result!.completeError(TimeoutException('Confirmação pendente'));
      await tester.pumpAndSettle();
      expect(tester.widget<TextField>(find.byType(TextField)).enabled, false);
      await tester.drag(find.byType(ListView), const Offset(0, -350));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Tentar a mesma entrada'));
      await tester.pump();
      expect(api.calls, hasLength(2));
      expect(api.calls[1], api.calls[0]);
      api.result!.completeError(TimeoutException('Confirmação pendente'));
      await tester.pumpAndSettle();
      api.close();
      api.session.dispose();
    },
  );
}
