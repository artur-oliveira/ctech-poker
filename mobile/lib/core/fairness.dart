import 'dart:typed_data';
import 'package:crypto/crypto.dart';

const ranks = '23456789TJQKA', suits = 'cdhs';
List<int> hexBytes(String hex) {
  if (!RegExp(r'^[0-9a-fA-F]{64}$').hasMatch(hex)) {
    throw const FormatException('Prova inválida');
  }
  return [
    for (var i = 0; i < hex.length; i += 2)
      int.parse(hex.substring(i, i + 2), radix: 16),
  ];
}

List<int> counterBytes(int index) =>
    (ByteData(4)..setUint32(0, index)).buffer.asUint8List();
List<String> shuffledDeck(String seedHex) {
  final key = hexBytes(seedHex);
  final deck = [
    for (final s in suits.split(''))
      for (final r in ranks.split('')) '$r$s',
  ];
  var counter = 0;
  for (var i = 51; i > 0; i--) {
    final max = i + 1, limit = 0x100000000 - (0x100000000 % (i + 1));
    int value;
    do {
      final hash = Hmac(sha256, key).convert(counterBytes(counter++)).bytes;
      value = ByteData.sublistView(Uint8List.fromList(hash)).getUint32(0);
    } while (value >= limit);
    final j = value % max, card = deck[i];
    deck[i] = deck[j];
    deck[j] = card;
  }
  return deck;
}

List<int> cardBytes(String card) {
  if (card.length != 2 ||
      !ranks.contains(card[0]) ||
      !suits.contains(card[1])) {
    throw const FormatException('Carta inválida');
  }
  return [ranks.indexOf(card[0]) + 2, suits.indexOf(card[1])];
}

bool verifyDeck(String seed, String commit) {
  try {
    hexBytes(commit);
    return sha256.convert([
          ...hexBytes(seed),
          for (final card in shuffledDeck(seed)) ...cardBytes(card),
        ]).toString() ==
        commit.toLowerCase();
  } on FormatException {
    return false;
  }
}

bool verifyPartial(
  String root,
  Map<String, dynamic> revealed,
  Map<String, dynamic> hidden,
) {
  try {
    hexBytes(root);
    if (revealed.keys.any(hidden.containsKey)) return false;
    if ({...revealed.keys, ...hidden.keys}.length != 52) return false;
    final hashes = <int>[];
    for (var i = 0; i < 52; i++) {
      final card = revealed['$i'];
      if (card != null) {
        hashes.addAll(
          sha256.convert([
            ...hexBytes(card['salt_hex'] as String),
            ...cardBytes(card['card'] as String),
          ]).bytes,
        );
      } else if (hidden.containsKey('$i')) {
        hashes.addAll(hexBytes(hidden['$i'] as String));
      } else {
        return false;
      }
    }
    return sha256.convert(hashes).toString() == root.toLowerCase();
  } catch (_) {
    return false;
  }
}
