import 'dart:convert';
import 'package:http/http.dart' as http;
import 'config.dart';
import 'session.dart';
import 'preferences.dart';

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
  PokerApi(this.session, {http.Client? client})
    : _client = client ?? http.Client();
  final PokerSession session;
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
    var token = await session.token();
    for (var attempt = 0; attempt < 2; attempt++) {
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
        data = response.body.isEmpty
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
  void close() => _client.close();
}

List<Json> rows(dynamic data, [String key = 'data']) =>
    ((data is List ? data : data[key]) as List? ?? [])
        .map((e) => Map<String, dynamic>.from(e as Map))
        .toList();
String segment(String value) => Uri.encodeComponent(value);
