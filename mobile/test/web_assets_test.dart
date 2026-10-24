import 'dart:io';
import 'package:ctech_poker/core/game_mode.dart';
import 'package:ctech_poker/features/widgets.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('all 520 card faces and the back are the original web SVGs', () {
    final sources = Directory('../ui/public/svgs/variants')
        .listSync(recursive: true)
        .whereType<File>()
        .where((f) => f.path.endsWith('.svg'));
    expect(sources.length, 520);
    for (final source in sources) {
      final relative = source.path.split('/variants/').last;
      expect(
        File('assets/cards/variants/$relative').readAsBytesSync(),
        source.readAsBytesSync(),
        reason: relative,
      );
    }
    expect(
      File('assets/cards/back.svg').readAsBytesSync(),
      File('../ui/public/svgs/card-back-red.svg').readAsBytesSync(),
    );
  });
  test(
    'card selection normalizes case and rejects unknown or hidden codes',
    () {
      expect(
        PlayingCard.asset(' AH '),
        'assets/cards/variants/four-color/heart-ace.svg',
      );
      expect(
        PlayingCard.asset('Td', 'two-color'),
        'assets/cards/variants/two-color/diamond-10.svg',
      );
      expect(
        PlayingCard.asset('Ks', 'unknown'),
        'assets/cards/variants/four-color/spade-king.svg',
      );
      for (final code in ['back', '', '1s', 'Ax', '10h', '../As']) {
        expect(PlayingCard.asset(code), 'assets/cards/back.svg', reason: code);
      }
    },
  );
  test('game mode separates player-facing units and API identifiers', () {
    expect(GameMode.chips.apiValue, 'sandbox');
    expect(GameMode.chips.label, 'Fichas');
    expect(GameMode.chips.amount(1250), '1.250');
    expect(GameMode.real.apiValue, 'real');
    expect(GameMode.real.label, 'Dinheiro real');
    expect(GameMode.real.amount(1250), contains('12,50'));
  });
}
