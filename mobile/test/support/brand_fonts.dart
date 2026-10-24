import 'package:flutter/services.dart';
import 'package:ctech_poker/core/design.dart';

Future<void> loadBrandFonts() async {
  for (final family in [PokerTheme.sans, PokerTheme.mono]) {
    final loader = FontLoader(family);
    final prefix = family == PokerTheme.sans ? 'IBMPlexSans' : 'IBMPlexMono';
    for (final weight in [
      'Regular',
      if (family == PokerTheme.sans) 'Medium',
      'SemiBold',
      'Bold',
    ]) {
      loader.addFont(rootBundle.load('assets/fonts/$prefix-$weight.ttf'));
    }
    await loader.load();
  }
  await (FontLoader(
    'MaterialIcons',
  )..addFont(rootBundle.load('fonts/MaterialIcons-Regular.otf'))).load();
}
