import 'dart:async';
import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'config.dart';

/// One unresolved money/chip mutation per account. Persist before sending;
/// never replay a saved request after restart (buy-in keys are not perpetual).
class OperationJournal {
  OperationJournal({FlutterSecureStorage? storage})
    : storage = storage ?? const FlutterSecureStorage();
  final FlutterSecureStorage storage;
  Future<void> _tail = Future.value();
  Future<T> exclusive<T>(Future<T> Function() action) {
    final result = _tail.then((_) => action());
    _tail = result.then<void>((_) {}, onError: (Object _, StackTrace _) {});
    return result;
  }

  String key(String user) =>
      'poker.operations.v1.${sha256.convert(utf8.encode('${PokerConfig.api}|$user'))}';
  Future<Map<String, dynamic>?> read(String user) async {
    final value = await storage.read(key: key(user));
    if (value == null) return null;
    return Map<String, dynamic>.from(jsonDecode(value) as Map);
  }

  Future<void> write(String user, Map<String, dynamic> record) =>
      storage.write(key: key(user), value: jsonEncode(record));
  Future<void> clear(String user) => storage.delete(key: key(user));
}
