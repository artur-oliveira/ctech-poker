import 'dart:io';
import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/design.dart';
import 'package:ctech_poker/core/preferences.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/features/buy_in.dart';
import 'package:ctech_poker/features/home.dart';
import 'package:ctech_poker/features/poker_logo.dart';
import 'package:ctech_poker/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:image/image.dart' as img;
import 'support/brand_fonts.dart';

class BrandSession extends PokerSession {
  @override
  Future<String> token({bool force = false}) async =>
      throw StateError('Visual fixture: no live socket');
}

class BrandApi extends PokerApi {
  BrandApi(super.session);
  @override
  Future<Json> get(String path, {Map<String, String>? query}) async =>
      switch (path) {
        '/v1.0/players/me' => {
          'user_id': 'fixture',
          'name': 'Ana',
          'sandbox_balance': 8400,
          'poker_terms_accepted': true,
        },
        '/v1.0/rooms/stakes' => {
          'stakes': [
            {'small_blind': 10, 'big_blind': 20},
          ],
        },
        '/v1.0/rooms/buckets' => {
          'data': [
            {
              'small_blind': 10,
              'big_blind': 20,
              'max_seats': 6,
              'seats_available': 3,
            },
          ],
        },
        _ => {'data': []},
      };
}

void main() {
  test('mobile brand colors and source logo match the web', () {
    final css = File('../ui/src/app/base.css').readAsStringSync();
    for (final entry in {
      'brand': PokerColors.brand,
      'ink': PokerColors.ink,
      'wine': PokerColors.wine,
      'paper': PokerColors.paper,
      'gold': PokerColors.gold,
      'muted': PokerColors.muted,
      'surface-seat': PokerColors.seat,
      'surface-control': PokerColors.control,
      'text-secondary': PokerColors.secondaryText,
      'table-rail': PokerColors.rail,
      'table-felt-light': PokerColors.feltLight,
      'table-felt-dark': PokerColors.feltDark,
      'focus-ring': PokerColors.focus,
    }.entries) {
      final match = RegExp(
        '--${entry.key}:\\s*#([0-9a-f]{6});',
      ).firstMatch(css)!;
      expect(
        entry.value.toARGB32(),
        0xff000000 | int.parse(match[1]!, radix: 16),
        reason: entry.key,
      );
    }
    expect(
      File('assets/brand/logo.svg').readAsStringSync(),
      File('../ui/public/svgs/logo.svg').readAsStringSync(),
    );
    final icon = img.decodePng(
      File(
        'ios/Runner/Assets.xcassets/AppIcon.appiconset/Icon-App-1024x1024@1x.png',
      ).readAsBytesSync(),
    )!;
    expect(icon.width, 1024);
    expect(icon.numChannels, 3, reason: 'iOS app icons must be opaque');
  });

  test('brand text pairs retain AA contrast including all felts', () {
    double ratio(Color a, Color b) {
      final values = [a.computeLuminance(), b.computeLuminance()]..sort();
      return (values.last + .05) / (values.first + .05);
    }

    for (final pair in [
      [PokerColors.onBrand, PokerColors.brand],
      [PokerColors.paper, PokerColors.wine],
      [PokerColors.wine, PokerColors.paper],
      [PokerColors.muted, PokerColors.control],
      [PokerColors.secondaryText, PokerColors.control],
      [PokerColors.gold, PokerColors.seat],
      [PokerColors.dangerText, PokerColors.control],
    ]) {
      expect(ratio(pair[0], pair[1]), greaterThanOrEqualTo(4.5));
    }
    final previous = PokerAppearance.felt;
    addTearDown(() => PokerAppearance.felt = previous);
    for (final name in ['classic', 'midnight', 'burgundy', 'ocean']) {
      PokerAppearance.felt = name;
      for (final color in PokerAppearance.feltColors) {
        expect(
          ratio(PokerAppearance.feltValueColor, color),
          greaterThanOrEqualTo(4.5),
          reason: name,
        );
      }
    }
  });

  for (final name in ['login', 'lobby', 'buy_in', 'guide']) {
    for (final scale in [1.0, 2.0]) {
      testWidgets('$name uses the real theme at mobile text scale $scale', (
        tester,
      ) async {
        tester.view.physicalSize = const Size(390, 844);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        await loadBrandFonts();
        final session = BrandSession();
        final api = BrandApi(session);
        addTearDown(session.dispose);
        addTearDown(api.close);
        final screen = switch (name) {
          'login' => LoginScreen(session: session),
          'lobby' => PokerHome(api: api),
          'buy_in' => BuyInScreen(
            api: api,
            path: '/unused',
            body: const {},
            minimum: 1000,
            maximum: 5000,
          ),
          _ => const GuideScreen(),
        };
        await tester.pumpWidget(
          MaterialApp(
            theme: PokerTheme.dark,
            builder: (context, child) => MediaQuery(
              data: MediaQuery.of(
                context,
              ).copyWith(textScaler: TextScaler.linear(scale)),
              child: child!,
            ),
            home: RepaintBoundary(key: const Key('screen'), child: screen),
          ),
        );
        await tester.runAsync(
          () => precacheImage(
            const AssetImage('assets/brand/logo.png'),
            tester.element(find.byType(MaterialApp)),
          ),
        );
        await tester.pumpAndSettle();
        expect(tester.takeException(), isNull);
        if (name != 'guide') expect(find.byType(PokerLogo), findsOneWidget);
        if (scale == 1) {
          await expectLater(
            find.byKey(const Key('screen')),
            matchesGoldenFile('goldens/brand_$name.png'),
          );
        }
        await tester.pumpWidget(const SizedBox());
      });
    }
  }
}
