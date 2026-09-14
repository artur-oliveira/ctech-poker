import 'package:flutter/material.dart';
import 'design.dart';
import 'package:shared_preferences/shared_preferences.dart';

class PokerPreferences extends ChangeNotifier {
  static final instance = PokerPreferences();
  bool sound = false, narration = false, trainer = false;
  int reminderMinutes = 60;
  Future<void> load() async {
    final prefs = SharedPreferencesAsync();
    sound = await prefs.getBool('poker.sound') ?? false;
    narration = await prefs.getBool('poker.narration') ?? false;
    trainer = await prefs.getBool('poker.trainer') ?? false;
    reminderMinutes = await prefs.getInt('poker.reminder') ?? 60;
    notifyListeners();
  }

  Future<void> save() async {
    final prefs = SharedPreferencesAsync();
    await prefs.setBool('poker.sound', sound);
    await prefs.setBool('poker.narration', narration);
    await prefs.setBool('poker.trainer', trainer);
    await prefs.setInt('poker.reminder', reminderMinutes);
    notifyListeners();
  }
}

class PokerAppearance {
  static String deck = 'four-color', felt = 'classic', betPresets = 'mixed';
  static List<String> favoriteReactions = [];
  static void update(Map<String, dynamic> profile) {
    betPresets = profile['bet_preset_mode'] as String? ?? 'mixed';
    favoriteReactions = List<String>.from(profile['favorite_reactions'] ?? []);
    deck = profile['deck_variant'] as String? ?? 'four-color';
    felt = profile['table_theme'] as String? ?? 'classic';
  }

  static const decks = {
    'four-color': [0xff1a1a1a, 0xffe60000, 0xff0066cc, 0xff008a00],
    'two-color': [0xff1a1a1a, 0xffcc0000, 0xffcc0000, 0xff1a1a1a],
    'colorblind': [0xff000000, 0xffe69f00, 0xff56b4e9, 0xff009e73],
    'high-constrast': [0xff000000, 0xffff0000, 0xff0055ff, 0xff00aa00],
    'casino': [0xff1a1a2e, 0xff8b0000, 0xff8b0000, 0xff1a1a2e],
    'bicycle': [0xff1a1a1a, 0xffb22222, 0xffb22222, 0xff1a1a1a],
    'vintage': [0xff3d2b1f, 0xff8b3a3a, 0xff2c5282, 0xff276749],
    'golden': [0xff141414, 0xff8b0000, 0xffc9a227, 0xff0e3b2e],
    'pink': [0xff2e1a2e, 0xffff4d6d, 0xffff8fb1, 0xff6b2d5c],
    'alt': [0xff3a3a3c, 0xffd63447, 0xffd4af37, 0xff2e5fa3],
  };
  static Color suit(String suit) => Color(
    (decks[deck] ?? decks['four-color']!)['shdc'.indexOf(suit).clamp(0, 3)],
  );
  static Color get railColor => switch (felt) {
    'midnight' => const Color(0xff3e4d59),
    'burgundy' => const Color(0xff63392f),
    'ocean' => const Color(0xff574538),
    _ => PokerColors.rail,
  };
  static Color get feltValueColor => felt == 'midnight' || felt == 'burgundy'
      ? PokerColors.gold
      : PokerColors.feltValue;
  static List<Color> get feltColors => switch (felt) {
    'midnight' => const [Color(0xff244b65), Color(0xff102b3d)],
    'burgundy' => const [Color(0xff71323b), Color(0xff35151b)],
    'ocean' => const [Color(0xff14717a), Color(0xff073f49)],
    _ => const [PokerColors.feltLight, PokerColors.feltDark],
  };
}
