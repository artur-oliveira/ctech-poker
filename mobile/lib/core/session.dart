import 'dart:async';
import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_appauth/flutter_appauth.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
import 'config.dart';

/// Accounts returns public refresh credentials in HttpOnly cookies. Native HTTP
/// can capture that response; persist only the cookie value in Keychain/Keystore.
/// Authorization UI and PKCE/state protection are delegated to AppAuth.
class PokerSession extends ChangeNotifier {
  PokerSession({http.Client? httpClient, FlutterSecureStorage? storage})
    : _http = httpClient ?? http.Client(),
      _storage = storage ?? const FlutterSecureStorage();
  final http.Client _http;
  final FlutterSecureStorage _storage;
  String? _access;
  DateTime _expires = DateTime.fromMillisecondsSinceEpoch(0);
  Future<String>? _refreshing;
  bool _loggingOut = false;
  bool signedIn = false;
  static const _key = 'poker.mobile.refresh';

  Future<void> restore() async {
    signedIn = await _storage.read(key: _key) != null;
    notifyListeners();
  }

  Future<void> login() async {
    final auth = await const FlutterAppAuth().authorize(
      AuthorizationRequest(
        PokerConfig.clientId,
        PokerConfig.callback,
        serviceConfiguration: AuthorizationServiceConfiguration(
          authorizationEndpoint: '${PokerConfig.accounts}/v1.0/authorize',
          tokenEndpoint: '${PokerConfig.accounts}/v1.0/token',
        ),
        scopes: PokerConfig.scopes,
      ),
    );
    final code = auth.authorizationCode;
    final verifier = auth.codeVerifier;
    if (code == null || verifier == null) throw StateError('Login cancelado.');
    await _exchange({
      'grant_type': 'authorization_code',
      'code': code,
      'code_verifier': verifier,
      'redirect_uri': PokerConfig.callback,
    });
    signedIn = true;
    notifyListeners();
  }

  Future<String> token({bool force = false}) async {
    if (_loggingOut) throw StateError('Encerrando a sessão.');
    if (!force && _access != null && DateTime.now().isBefore(_expires)) {
      return _access!;
    }
    if (_refreshing != null) return _refreshing!;
    final future = _refresh();
    _refreshing = future;
    try {
      return await future;
    } finally {
      _refreshing = null;
    }
  }

  Future<String> _refresh() async {
    final refresh = await _storage.read(key: _key);
    if (refresh == null) throw StateError('Entre para continuar.');
    return _exchange({'grant_type': 'refresh_token', 'refresh_token': refresh});
  }

  Future<String> _exchange(Map<String, String> fields) async {
    final response = await _http
        .post(
          Uri.parse('${PokerConfig.accounts}/v1.0/token'),
          body: {...fields, 'client_id': PokerConfig.clientId},
        )
        .timeout(const Duration(seconds: 20));
    final body = jsonDecode(response.body) as Map<String, dynamic>;
    if (response.statusCode != 200) {
      if (body['error'] == 'invalid_grant') {
        await _storage.delete(key: _key);
        _access = null;
        signedIn = false;
        notifyListeners();
      }
      throw StateError(
        response.statusCode >= 500
            ? 'Login temporariamente indisponível. Tente novamente.'
            : 'Não foi possível renovar a sessão.',
      );
    }
    final refresh =
        body['refresh_token'] as String? ??
        refreshCookie(response.headers['set-cookie']);
    if (refresh != null && refresh.isNotEmpty) {
      await _storage.write(key: _key, value: refresh);
    }
    _access = body['access_token'] as String;
    _expires = DateTime.now().add(
      Duration(seconds: (body['expires_in'] as int? ?? 900) - 45),
    );
    return _access!;
  }

  static String? refreshCookie(String? header) {
    if (header == null) return null;
    // Ignore the retired ctech_rt cookie and unrelated account/SSO cookies.
    final suffix = sha256
        .convert(utf8.encode(PokerConfig.clientId))
        .toString()
        .substring(0, 16);
    final matches = RegExp(
      '(?:^|,\\s*)ctech_rt_$suffix=([^;,]+)',
    ).allMatches(header);
    return matches.isEmpty ? null : matches.last.group(1);
  }

  Future<void> logout() async {
    if (_loggingOut) return;
    _loggingOut = true;
    try {
      // Finish an in-flight rotation before revoking its successor.
      if (_refreshing != null) {
        try {
          await _refreshing;
        } catch (_) {
          /* Still revoke the persisted credential. */
        }
      }
      final refresh = await _storage.read(key: _key);
      if (refresh != null) {
        final response = await _http
            .post(
              Uri.parse('${PokerConfig.accounts}/v1.0/revoke'),
              body: {'token': refresh, 'client_id': PokerConfig.clientId},
            )
            .timeout(const Duration(seconds: 20));
        if (response.statusCode >= 400) {
          throw StateError(
            'Não foi possível encerrar a sessão. Tente novamente.',
          );
        }
      }
      await _storage.delete(key: _key);
      _access = null;
      signedIn = false;
      notifyListeners();
    } finally {
      _loggingOut = false;
    }
  }

  @override
  void dispose() {
    _http.close();
    super.dispose();
  }
}
