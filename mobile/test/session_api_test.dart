import 'dart:convert';
import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(
    () => FlutterSecureStorage.setMockInitialValues({
      'poker.mobile.refresh': 'refresh-old',
    }),
  );
  test(
    'concurrent callers rotate one refresh credential and persist the cookie',
    () async {
      var requests = 0;
      final session = PokerSession(
        httpClient: MockClient((request) async {
          requests++;
          expect(request.bodyFields['refresh_token'], 'refresh-old');
          await Future<void>.delayed(const Duration(milliseconds: 5));
          return http.Response(
            jsonEncode({'access_token': 'access-new', 'expires_in': 900}),
            200,
            headers: {
              'set-cookie':
                  'ctech_rt_eebd010226fb99a2=refresh-new; HttpOnly; Secure; Path=/, ctech_rt=; Max-Age=0',
            },
          );
        }),
      );
      final tokens = await Future.wait([
        session.token(),
        session.token(),
        session.token(),
      ]);
      expect(tokens.toSet(), {'access-new'});
      expect(requests, 1);
      expect(
        await const FlutterSecureStorage().read(key: 'poker.mobile.refresh'),
        'refresh-new',
      );
      session.dispose();
    },
  );
  test(
    'transient refresh failure preserves credential; invalid grant clears it',
    () async {
      var invalid = false;
      final session = PokerSession(
        httpClient: MockClient(
          (_) async => http.Response(
            jsonEncode({
              'error': invalid ? 'invalid_grant' : 'temporarily_unavailable',
            }),
            invalid ? 400 : 503,
          ),
        ),
      );
      await session.restore();
      await expectLater(session.token(), throwsStateError);
      expect(session.signedIn, true);
      expect(
        await const FlutterSecureStorage().read(key: 'poker.mobile.refresh'),
        'refresh-old',
      );
      invalid = true;
      await expectLater(session.token(), throwsStateError);
      expect(session.signedIn, false);
      expect(
        await const FlutterSecureStorage().read(key: 'poker.mobile.refresh'),
        isNull,
      );
      session.dispose();
    },
  );
  test(
    '401 retries once and preserves the mutation idempotency key and body',
    () async {
      var refreshes = 0, calls = 0;
      final session = PokerSession(
        httpClient: MockClient(
          (_) async => http.Response(
            jsonEncode({
              'access_token': 'access-${++refreshes}',
              'expires_in': 900,
            }),
            200,
          ),
        ),
      );
      final api = PokerApi(
        session,
        client: MockClient((request) async {
          calls++;
          expect(request.headers['Idempotency-Key'], 'operation-1');
          expect(jsonDecode(request.body), {
            'amount': 100,
            'idem_key': 'buyin-1',
          });
          return http.Response(
            calls == 1 ? '{}' : '{"room_id":"room-1"}',
            calls == 1 ? 401 : 200,
          );
        }),
      );
      final result = await api.request(
        '/v1.0/rooms/join-or-create',
        method: 'POST',
        body: {'amount': 100, 'idem_key': 'buyin-1'},
        idempotencyKey: 'operation-1',
      );
      expect(result['room_id'], 'room-1');
      expect(calls, 2);
      expect(refreshes, 2);
      api.close();
      session.dispose();
    },
  );
  test(
    'malformed successful mutation response remains an error without retry',
    () async {
      var calls = 0;
      final session = PokerSession(
        httpClient: MockClient(
          (_) async =>
              http.Response('{"access_token":"token","expires_in":900}', 200),
        ),
      );
      final api = PokerApi(
        session,
        client: MockClient((_) async {
          calls++;
          return http.Response('<html>proxy</html>', 200);
        }),
      );
      await expectLater(
        api.post('/join', {'idem_key': 'same'}),
        throwsA(isA<ApiFailure>()),
      );
      expect(calls, 1);
      api.close();
      session.dispose();
    },
  );
  test('a failed purchase is not automatically retried on 503', () async {
    var calls = 0;
    final session = PokerSession(
      httpClient: MockClient(
        (_) async =>
            http.Response('{"access_token":"token","expires_in":900}', 200),
      ),
    );
    final api = PokerApi(
      session,
      client: MockClient((_) async {
        calls++;
        return http.Response('{"detail":"Indisponível"}', 503);
      }),
    );
    await expectLater(
      api.post('/v1.0/wallet/sandbox-purchase/', {'idem_key': 'same'}),
      throwsA(isA<ApiFailure>()),
    );
    expect(calls, 1);
    api.close();
    session.dispose();
  });
}
