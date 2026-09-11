import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;
import 'config.dart';
import 'session.dart';
import 'preferences.dart';
import 'operations.dart';

typedef Json = Map<String, dynamic>;

class ApiFailure implements Exception {
  const ApiFailure(this.status, this.message, this.requestId);
  final int status;
  final String message;
  final String? requestId;
  @override
  String toString() => message;
}

class PokerApi {
  PokerApi(this.session, {http.Client? client, OperationJournal? journal})
    : _client = client ?? http.Client(),
      journal = journal ?? OperationJournal();
  final PokerSession session;
  final OperationJournal journal;
  final http.Client _client;
  Future<dynamic> request(
    String path, {
    String method = 'GET',
    Json? body,
    Map<String, String>? query,
    String? idempotencyKey,
  }) async {
    final uri = Uri.parse(
      '${PokerConfig.api}$path',
    ).replace(queryParameters: query);
    final identity = session.identityVersion;
    var token = await session.token();
    for (var attempt = 0; attempt < 2; attempt++) {
      if (session.identityVersion != identity) {
        throw StateError('A sessão mudou. Abra a operação novamente.');
      }
      final req = http.Request(method, uri)
        ..headers.addAll({
          'Authorization': 'Bearer $token',
          'Content-Type': 'application/json',
          'Idempotency-Key': ?idempotencyKey,
        });
      if (body != null) req.body = jsonEncode(body);
      final response = await http.Response.fromStream(
        await _client.send(req).timeout(const Duration(seconds: 20)),
      ).timeout(const Duration(seconds: 20));
      if (response.statusCode == 401 && attempt == 0) {
        token = await session.token(force: true);
        continue;
      }
      dynamic data;
      try {
        data =
            (response.body.isEmpty ||
                (method == 'POST' &&
                    RegExp(r'^/v1\.0/rooms/[^/]+/join$').hasMatch(path) &&
                    response.statusCode == 200 &&
                    response.body == 'OK'))
            ? <String, dynamic>{}
            : jsonDecode(response.body);
      } on FormatException {
        if (response.statusCode < 400) {
          throw ApiFailure(
            response.statusCode,
            'O servidor retornou uma resposta inválida. Confira o estado da operação antes de tentar novamente.',
            response.headers['x-request-id'],
          );
        }
        data = <String, dynamic>{};
      }
      if (response.statusCode >= 400) {
        throw ApiFailure(
          response.statusCode,
          data is Map
              ? (data['detail'] ??
                        data['title'] ??
                        'Não foi possível concluir a operação.')
                    .toString()
              : 'Não foi possível concluir a operação.',
          response.headers['x-request-id'],
        );
      }
      if (path == '/v1.0/players/me' && data is Map<String, dynamic>) {
        PokerAppearance.update(data);
      }
      return data;
    }
    throw const ApiFailure(401, 'Entre novamente para continuar.', null);
  }

  Future<Json> get(String path, {Map<String, String>? query}) async =>
      Map<String, dynamic>.from(await request(path, query: query) as Map);
  Future<Json> post(String path, [Json? body]) async =>
      Map<String, dynamic>.from(
        await request(path, method: 'POST', body: body ?? {}) as Map,
      );
  Future<String> _operationOwner() async {
    final profile = await get('/v1.0/players/me');
    final user = profile['user_id'];
    if (user is! String || user.isEmpty) {
      throw StateError('Não foi possível identificar sua conta.');
    }
    return user;
  }

  Future<Json?> pendingOperation() =>
      journal.exclusive(() async => journal.read(await _operationOwner()));

  Future<void> acknowledgeOperation(String id) => journal.exclusive(() async {
    final user = await _operationOwner();
    final pending = await journal.read(user);
    if (pending?['id'] != id) {
      throw StateError('A operação mudou. Atualize a tela.');
    }
    await journal.clear(user);
  });

  Future<Json> durablePost(
    String path,
    Json body, {
    required String label,
  }) => journal.exclusive(() async {
    final identity = session.identityVersion;
    final user = await _operationOwner();
    final id = body['idem_key'];
    if (id is! String || id.isEmpty) {
      throw StateError('Operação sem identificação.');
    }
    final fields = body.keys.toList()..sort();
    final fingerprint = sha256
        .convert(
          utf8.encode(
            jsonEncode({for (final field in fields) field: body[field]}),
          ),
        )
        .toString();
    final pending = await journal.read(user);
    if (pending != null &&
        (pending['id'] != id ||
            pending['path'] != path ||
            pending['fingerprint'] != fingerprint)) {
      throw StateError(
        'Existe uma operação aguardando conferência. Abra “Operações pendentes” no menu superior antes de iniciar outra.',
      );
    }
    if (pending == null) {
      await journal.write(user, {
        'id': id,
        'path': path,
        'fingerprint': fingerprint,
        'label': label,
        'created_at': DateTime.now().millisecondsSinceEpoch,
        if (body['amount'] != null) 'amount': body['amount'],
        if (body['auto_rebuy'] != null) 'auto_rebuy': body['auto_rebuy'],
        if (body['sku'] ?? body['reaction_id'] ?? body['item_id']
            case final String product)
          'product': product,
      });
    }
    if (session.identityVersion != identity) {
      throw StateError(
        'A sessão mudou. Confira a pendência na conta original.',
      );
    }
    final result = await post(path, body);
    // If clearing fails, retain the journal and require reconciliation.
    await journal.clear(user);
    return result;
  });

  void close() => _client.close();
}

List<Json> rows(dynamic data, [String key = 'data']) =>
    ((data is List ? data : data[key]) as List? ?? [])
        .map((e) => Map<String, dynamic>.from(e as Map))
        .toList();
String segment(String value) => Uri.encodeComponent(value);
