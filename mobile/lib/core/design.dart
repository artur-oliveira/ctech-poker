import '../features/poker_icon.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// Brand roles mirrored from ui/src/app/base.css. See mobile/DESIGN.md.
abstract final class PokerColors {
  static const brand = Color(0xffaf2a2f);
  static const brandActive = Color(0xffd9464d);
  static const wine = Color(0xff5b1218);
  static const ink = Color(0xff120d0e);
  static const paper = Color(0xfff6f0e7);
  static const gold = Color(0xffe6b85c);
  static const goldInk = Color(0xff30230a);
  static const felt = Color(0xff0d5b45);
  static const feltLight = Color(0xff18765b);
  static const feltDark = Color(0xff084b38);
  static const feltText = Color(0xffe3f1ea);
  static const feltValue = Color(0xfff3e9c9);
  static const rail = Color(0xff7c4d2f);
  static const seat = Color(0xff161011);
  static const control = Color(0xff211416);
  static const controlHover = Color(0xff3e3133);
  static const muted = Color(0xffad9fa0);
  static const secondaryText = Color(0xffcbbfc0);
  static const success = Color(0xff48c98c);
  static const danger = Color(0xffdc2626);
  static const dangerText = Color(0xffef4444);
  static const dangerSoft = Color(0xfff5b0b3);
  static const errorSurface = Color(0xff3b0b0e);
  static const focus = Color(0xffed777c);
  static const onBrand = Color(0xffffffff);
  static const border = Color(0x24ffffff);
  static const seatBorder = Color(0x26ffffff);
}

abstract final class PokerTheme {
  static const sans = 'IBM Plex Sans';
  static const mono = 'IBM Plex Mono';

  // Explicit roles prevent Material seed generation from changing the brand.
  static const scheme = ColorScheme(
    brightness: Brightness.dark,
    primary: PokerColors.brand,
    onPrimary: PokerColors.onBrand,
    primaryContainer: PokerColors.wine,
    onPrimaryContainer: PokerColors.paper,
    secondary: PokerColors.secondaryText,
    onSecondary: PokerColors.ink,
    secondaryContainer: PokerColors.control,
    onSecondaryContainer: PokerColors.paper,
    tertiary: PokerColors.gold,
    onTertiary: PokerColors.goldInk,
    error: PokerColors.dangerText,
    onError: PokerColors.ink,
    errorContainer: PokerColors.errorSurface,
    onErrorContainer: PokerColors.dangerSoft,
    surface: PokerColors.ink,
    onSurface: PokerColors.paper,
    surfaceDim: PokerColors.ink,
    surfaceBright: PokerColors.controlHover,
    surfaceContainerLowest: PokerColors.ink,
    surfaceContainerLow: PokerColors.seat,
    surfaceContainer: PokerColors.control,
    surfaceContainerHigh: PokerColors.control,
    surfaceContainerHighest: PokerColors.controlHover,
    onSurfaceVariant: PokerColors.secondaryText,
    outline: PokerColors.muted,
    outlineVariant: PokerColors.border,
    inverseSurface: PokerColors.paper,
    onInverseSurface: PokerColors.wine,
    inversePrimary: PokerColors.brand,
    surfaceTint: Colors.transparent,
  );

  static ThemeData get dark {
    final base = ThemeData(
      useMaterial3: true,
      colorScheme: scheme,
      fontFamily: sans,
      scaffoldBackgroundColor: PokerColors.ink,
      visualDensity: VisualDensity.standard,
    );
    final shape = RoundedRectangleBorder(
      borderRadius: BorderRadius.circular(12),
    );
    return base.copyWith(
      textTheme: base.textTheme.copyWith(
        titleLarge: base.textTheme.titleLarge!.copyWith(
          fontSize: 24,
          fontWeight: FontWeight.w700,
        ),
        bodyMedium: base.textTheme.bodyMedium!.copyWith(
          fontSize: 14,
          height: 1.5,
        ),
        labelLarge: base.textTheme.labelLarge!.copyWith(
          fontSize: 14,
          fontWeight: FontWeight.w600,
        ),
      ),
      actionIconTheme: ActionIconThemeData(
        backButtonIconBuilder: (_) => const PokerIcon(PokerIcons.arrowLeft),
        closeButtonIconBuilder: (_) => const PokerIcon(PokerIcons.x),
      ),
      iconTheme: const IconThemeData(
        size: 22,
        color: PokerColors.secondaryText,
      ),
      appBarTheme: const AppBarTheme(
        backgroundColor: PokerColors.ink,
        foregroundColor: PokerColors.paper,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        titleTextStyle: TextStyle(
          fontFamily: sans,
          fontSize: 20,
          fontWeight: FontWeight.w600,
          color: PokerColors.paper,
        ),
        systemOverlayStyle: SystemUiOverlayStyle.light,
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          minimumSize: const Size(48, 52),
          shape: shape,
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: PokerColors.paper,
          minimumSize: const Size(48, 48),
          side: const BorderSide(color: PokerColors.muted),
          shape: shape,
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: PokerColors.paper,
          minimumSize: const Size(48, 48),
          shape: shape,
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: PokerColors.control,
        labelStyle: const TextStyle(color: PokerColors.secondaryText),
        hintStyle: const TextStyle(color: PokerColors.muted),
        border: OutlineInputBorder(borderRadius: BorderRadius.circular(12)),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: const BorderSide(color: PokerColors.muted),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(12),
          borderSide: const BorderSide(color: PokerColors.focus, width: 2),
        ),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: PokerColors.seat,
        indicatorColor: PokerColors.wine,
        surfaceTintColor: Colors.transparent,
        iconTheme: WidgetStateProperty.resolveWith(
          (states) => IconThemeData(
            color: states.contains(WidgetState.selected)
                ? PokerColors.onBrand
                : PokerColors.secondaryText,
          ),
        ),
        labelTextStyle: WidgetStatePropertyAll(
          TextStyle(
            fontFamily: sans,
            fontSize: 12,
            fontWeight: FontWeight.w500,
            color: PokerColors.paper,
          ),
        ),
      ),
      cardTheme: CardThemeData(
        color: PokerColors.seat,
        elevation: 0,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      ),
      dialogTheme: DialogThemeData(
        backgroundColor: PokerColors.control,
        surfaceTintColor: Colors.transparent,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      ),
      bottomSheetTheme: const BottomSheetThemeData(
        backgroundColor: PokerColors.control,
        surfaceTintColor: Colors.transparent,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
        ),
      ),
      chipTheme: base.chipTheme.copyWith(
        selectedColor: PokerColors.wine,
        labelStyle: const TextStyle(fontFamily: sans, color: PokerColors.paper),
        checkmarkColor: PokerColors.paper,
      ),
      progressIndicatorTheme: const ProgressIndicatorThemeData(
        color: PokerColors.focus,
      ),
      tabBarTheme: const TabBarThemeData(
        labelColor: PokerColors.paper,
        unselectedLabelColor: PokerColors.secondaryText,
        indicatorColor: PokerColors.focus,
      ),
      sliderTheme: base.sliderTheme.copyWith(
        thumbColor: PokerColors.focus,
        activeTrackColor: PokerColors.focus,
      ),
      textSelectionTheme: const TextSelectionThemeData(
        cursorColor: PokerColors.focus,
        selectionColor: PokerColors.wine,
        selectionHandleColor: PokerColors.focus,
      ),
    );
  }
}
