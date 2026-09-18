import 'dart:convert';
import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/operations.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class Session extends PokerSession {
  @override
  Future<String> token({bool force = false}) async => 'token';
}

class BrokenStorage extends FlutterSecureStorage {
  @override
  Future<void> write({
    required String key,
    required String? value,
    AppleOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    AppleOptions? mOptions,
    WindowsOptions? wOptions,
  }) async => throw StateError('Storage unavailable');
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUp(() => FlutterSecureStorage.setMockInitialValues({}));
  test(
    'session change while saving prevents sending under another account',
    () async {
      var posts = 0;
      final session = Session();
      final api = PokerApi(
        session,
        client: MockClient((request) async {
          if (request.method == 'GET') {
            session.identityVersion++;
            return http.Response('{"user_id":"alice"}', 200);
          }
          posts++;
          return http.Response('{}', 200);
        }),
      );
      await expectLater(
        api.durablePost('/buy', {'idem_key': 'one'}, label: 'Compra'),
        throwsStateError,
      );
      expect(posts, 0);
      api.close();
      session.dispose();
    },
  );
  test(
    'process restart preserves uncertain operation; account isolation and explicit acknowledgment',
    () async {
      var owner = 'alice', posts = 0;
      final session = Session();
      PokerApi create() => PokerApi(
        session,
        client: MockClient((request) async {
          if (request.method == 'GET') {
            return http.Response(jsonEncode({'user_id': owner}), 200);
          }
          posts++;
          return http.Response('{"detail":"Unavailable"}', 503);
        }),
      );
      var api = create();
      await expectLater(
        api.durablePost('/v1.0/rooms/join-or-create', {
          'amount': 500,
          'idem_key': 'one',
        }, label: 'Entrada'),
        throwsA(isA<ApiFailure>()),
      );
      api.close();
      api = create();
      expect((await api.pendingOperation())!['amount'], 500);
      expect(posts, 1);
      await expectLater(
        api.durablePost('/v1.0/rooms/join-or-create', {
          'amount': 500,
          'idem_key': 'two',
        }, label: 'Entrada'),
        throwsStateError,
      );
      await expectLater(
        api.durablePost('/v1.0/rooms/join-or-create', {
          'amount': 600,
          'idem_key': 'one',
        }, label: 'Entrada'),
        throwsStateError,
      );
      expect(posts, 1);
      owner = 'bob';
      expect(await api.pendingOperation(), isNull);
      await expectLater(api.acknowledgeOperation('one'), throwsStateError);
      owner = 'alice';
      await api.acknowledgeOperation('one');
      expect(await api.pendingOperation(), isNull);
      expect(posts, 1);
      api.close();
      session.dispose();
    },
  );
  test(
    'journal is written before sending; same consent retry succeeds and clears it',
    () async {
      final session = Session(), journal = OperationJournal();
      var posts = 0;
      final api = PokerApi(
        session,
        journal: journal,
        client: MockClient((request) async {
          if (request.method == 'GET') {
            return http.Response('{"user_id":"alice"}', 200);
          }
          expect((await journal.read('alice'))!['id'], 'one');
          posts++;
          return http.Response(
            posts == 1 ? '{}' : 'OK',
            posts == 1 ? 503 : 200,
          );
        }),
      );
      const body = {'amount': 500, 'auto_rebuy': true, 'idem_key': 'one'};
      await expectLater(
        api.durablePost('/v1.0/rooms/room/join', body, label: 'Entrada'),
        throwsA(isA<ApiFailure>()),
      );
      expect(
        await api.durablePost('/v1.0/rooms/room/join', body, label: 'Entrada'),
        isEmpty,
      );
      expect(await api.pendingOperation(), isNull);
      expect(posts, 2);
      api.close();
      session.dispose();
    },
  );
  test('storage failure prevents debit request', () async {
    var posts = 0;
    final session = Session();
    final api = PokerApi(
      session,
      journal: OperationJournal(storage: BrokenStorage()),
      client: MockClient((request) async {
        if (request.method == 'POST') posts++;
        return http.Response('{"user_id":"alice"}', 200);
      }),
    );
    await expectLater(
      api.durablePost('/buy', {'idem_key': 'one'}, label: 'Compra'),
      throwsStateError,
    );
    expect(posts, 0);
    api.close();
    session.dispose();
  });
}
