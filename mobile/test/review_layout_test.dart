import 'package:ctech_poker/core/design.dart';
import 'package:ctech_poker/features/achievements.dart';
import 'package:ctech_poker/features/hand_browser.dart';
import 'package:ctech_poker/features/statistics.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'support/brand_fonts.dart';
import 'support/review_api.dart';

void main() {
  for (final width in [320.0, 390.0]) {
    for (final scale in [1.0, 2.0]) {
      testWidgets('history, stats and achievements fit $width at $scale', (
        tester,
      ) async {
        tester.view.physicalSize = Size(width, 844);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        await loadBrandFonts();
        final api = ReviewApi();
        addTearDown(api.close);
        addTearDown(api.session.dispose);
        for (final screen in [
          HandBrowser(api: api),
          StatisticsScreen(api: api),
          AchievementsScreen(api: api),
        ]) {
          await tester.pumpWidget(
            MaterialApp(
              theme: PokerTheme.dark,
              builder: (context, child) => MediaQuery(
                data: MediaQuery.of(
                  context,
                ).copyWith(textScaler: TextScaler.linear(scale)),
                child: child!,
              ),
              home: Scaffold(body: screen),
            ),
          );
          await tester.pumpAndSettle();
          expect(
            tester.takeException(),
            isNull,
            reason: screen.runtimeType.toString(),
          );
          await tester.pumpWidget(const SizedBox());
        }
      });
    }
  }
}
