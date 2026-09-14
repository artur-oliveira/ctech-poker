import 'dart:convert';
import 'dart:io';
import 'package:ctech_poker/core/fairness.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('matches independent SHA-256/HMAC fixture and rejects tampering', () {
    final proof =
        jsonDecode(File('test/fixtures/fairness.json').readAsStringSync())
            as Map<String, dynamic>;
    expect(verifyDeck(proof['seed'], proof['commit']), true);
    expect(
      verifyPartial(proof['root'], proof['revealed'], proof['hidden']),
      true,
    );
    proof['revealed']['7']['card'] = 'back';
    expect(
      verifyPartial(proof['root'], proof['revealed'], proof['hidden']),
      false,
    );
  });
  test('shuffle yields exactly one of every card', () {
    final deck = shuffledDeck('00' * 32);
    expect(deck.length, 52);
    expect(deck.toSet().length, 52);
    expect(deck, shuffledDeck('00' * 32));
  });
  test('reject malformed, incomplete, overlapping and mismatching proofs', () {
    expect(verifyDeck('z' * 64, '0' * 64), false);
    expect(verifyDeck('00' * 32, '00' * 32), false);
    expect(verifyPartial('00' * 32, {}, {}), false);
    expect(
      verifyPartial(
        '00' * 32,
        {
          '0': {'card': 'Ah', 'salt_hex': '00' * 32},
        },
        {'0': '00' * 32},
      ),
      false,
    );
    expect(() => cardBytes('back'), throwsFormatException);
  });
}
