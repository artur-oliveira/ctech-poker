import 'package:ctech_poker/core/api.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/features/social_details.dart';
import 'package:ctech_poker/features/widgets.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

class ProfileApi extends PokerApi {
  ProfileApi({this.hidden = false, this.private = false})
    : super(PokerSession());
  final bool hidden, private;
  int matchups = 0;
  @override
  Future<Json> get(String path, {Map<String, String>? query}) async {
    if (path.endsWith('/showcase')) {
      if (private) throw const ApiFailure(404, 'Perfil indisponível', null);
      return {
        'name': 'Jogador público',
        'featured_achievements': [
          {'key': 'wins', 'count': 3},
        ],
        'showcase_layout': {
          'order': ['matchup', 'achievements', 'best_hand'],
          'hidden': hidden ? ['matchup'] : [],
        },
      };
    }
    matchups++;
    if (matchups == 1) throw const ApiFailure(503, 'Indisponível', null);
    return {
      'hands_together': 7,
      'viewer_wins': 2,
      'opponent_wins': 4,
      'ties': 1,
    };
  }
}

void main() {
  testWidgets('shared loader recovers from a failed request', (tester) async {
    var calls = 0;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: AsyncPanel(
            load: () async {
              if (++calls == 1) throw StateError('Rede indisponível');
              return 'Recuperado';
            },
            builder: (context, result, reload) =>
                ListView(children: [Text(result as String)]),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.text('Tentar novamente'));
    await tester.pumpAndSettle();
    expect(find.text('Recuperado'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
  test('showcase normalizes order and preserves mandatory achievements', () {
    expect(
      visibleShowcaseSections({
        'order': ['matchup', 'matchup', 'unknown'],
        'hidden': ['achievements', 'best_hand'],
      }),
      ['matchup', 'achievements'],
    );
    expect(visibleShowcaseSections(null), [
      'achievements',
      'best_hand',
      'matchup',
    ]);
  });
  testWidgets(
    'matchup failure does not hide profile and can retry independently',
    (tester) async {
      final api = ProfileApi();
      await tester.pumpWidget(
        MaterialApp(
          home: PublicProfile(api: api, playerId: 'other'),
        ),
      );
      await tester.pumpAndSettle();
      expect(find.text('Jogador público'), findsOneWidget);
      expect(find.text('Vitórias'), findsOneWidget);
      expect(
        tester.getTopLeft(find.text('Nosso confronto')).dy,
        lessThan(tester.getTopLeft(find.text('Conquistas em destaque')).dy),
      );
      await tester.tap(find.text('Tentar confronto novamente'));
      await tester.pumpAndSettle();
      expect(find.text('7 mãos juntos'), findsOneWidget);
      expect(api.matchups, 2);
      api.close();
      api.session.dispose();
    },
  );
  for (final private in [false, true]) {
    testWidgets(
      'does not request hidden or private matchup: private=$private',
      (tester) async {
        final api = ProfileApi(hidden: true, private: private);
        await tester.pumpWidget(
          MaterialApp(
            home: PublicProfile(api: api, playerId: 'other'),
          ),
        );
        await tester.pumpAndSettle();
        expect(api.matchups, 0);
        expect(find.text('Nosso confronto'), findsNothing);
        api.close();
        api.session.dispose();
      },
    );
  }
}
