import 'package:ctech_poker/features/poker_icon.dart';
import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;
import 'package:ctech_poker/core/design.dart';
import 'package:ctech_poker/features/buy_in.dart';
import 'package:ctech_poker/features/collections.dart';
import 'package:ctech_poker/features/create_room.dart';
import 'package:ctech_poker/features/home.dart';
import 'package:ctech_poker/features/library.dart';
import 'package:ctech_poker/features/native.dart';
import 'package:ctech_poker/features/pending_operations.dart';
import 'package:ctech_poker/features/report_player.dart';
import 'package:ctech_poker/features/share_hand.dart';
import 'package:ctech_poker/features/showcase.dart';
import 'package:ctech_poker/features/social_details.dart';
import 'package:ctech_poker/features/store.dart';
import 'package:ctech_poker/main.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'support/brand_fonts.dart';
import 'support/review_api.dart';
import 'package:ctech_poker/core/realtime.dart';
import 'package:ctech_poker/features/table.dart';
import 'package:ctech_poker/generated/poker.pb.dart';
import 'package:fixnum/fixnum.dart';

void main() {
  testWidgets('all application destinations render with offline review data', (
    tester,
  ) async {
    const export = bool.fromEnvironment('EXPORT_SCREENSHOTS');
    final manifest = <Map<String, String>>[];
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await loadBrandFonts();
    final api = ReviewApi();
    addTearDown(api.close);
    addTearDown(api.session.dispose);
    Future<void> mount(Widget screen) async {
      await tester.pumpWidget(const SizedBox());
      await tester.pumpWidget(
        RepaintBoundary(
          key: const Key('capture'),
          child: MaterialApp(
            debugShowCheckedModeBanner: false,
            theme: PokerTheme.dark,
            locale: const Locale('pt', 'BR'),
            supportedLocales: const [Locale('pt', 'BR')],
            localizationsDelegates: GlobalMaterialLocalizations.delegates,
            home: screen,
          ),
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
    }

    Future<void> capture(String title) async {
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull, reason: title);
      expect(
        find.textContaining('Missing review fixture:'),
        findsNothing,
        reason: title,
      );
      if (!export) return;
      final boundary = tester.renderObject<RenderRepaintBoundary>(
        find.byKey(const Key('capture')),
      );
      final filename =
          '${(manifest.length + 1).toString().padLeft(2, '0')}.png';
      await tester.runAsync(() async {
        final image = await boundary.toImage(pixelRatio: 2);
        final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
        final file = File('docs/screenshots/$filename');
        await file.parent.create(recursive: true);
        await file.writeAsBytes(bytes!.buffer.asUint8List());
        image.dispose();
      });
      manifest.add({'title': title, 'file': filename});
    }

    Future<void> pages(String title) async {
      await capture(title);
      final scrolls = tester
          .stateList<ScrollableState>(find.byType(Scrollable))
          .where(
            (s) =>
                s.position.axis == Axis.vertical &&
                s.position.maxScrollExtent > 0,
          )
          .toList();
      if (scrolls.isEmpty) return;
      final position = scrolls.first.position;
      var part = 2;
      while (position.pixels < position.maxScrollExtent && part <= 8) {
        position.jumpTo(
          (position.pixels + position.viewportDimension * .8).clamp(
            0,
            position.maxScrollExtent,
          ),
        );
        await capture('$title · continuação $part');
        part++;
      }
      position.jumpTo(0);
      await tester.pumpAndSettle();
    }

    Future<void> tap(String label) async {
      await tester.tap(find.text(label).last);
      await tester.pumpAndSettle();
    }

    Future<void> tab(String label) async {
      final finder = find.widgetWithText(Tab, label);
      await tester.ensureVisible(finder);
      await tester.pumpAndSettle();
      await tester.tap(finder);
      await tester.pumpAndSettle();
    }

    await mount(LoginScreen(session: api.session));
    await pages('Entrar');
    await mount(PokerHome(api: api));
    await pages('Lobby');
    await tap('Mãos');
    for (final label in [
      'Histórico',
      'Estatísticas',
      'Conquistas',
      'Ranking',
      'Sessões',
    ]) {
      await tab(label);
      await pages('Mãos · $label');
    }
    await tab('Histórico');
    await tap('Empates');
    expect(
      find.textContaining('Nenhum resultado nas páginas carregadas.'),
      findsOneWidget,
    );
    await capture('Mãos · filtro sem resultados');
    await tap('Pessoas');
    for (final label in [
      'Amigos',
      'Pedidos',
      'Recentes',
      'Bloqueados',
      'Caixa de entrada',
      'Pedidos enviados',
    ]) {
      await tab(label);
      await pages('Pessoas · $label');
    }
    await tap('Loja');
    for (final label in ['Fichas', 'Reações', 'Baralhos', 'Mesas']) {
      await tab(label);
      await pages('Loja · $label');
    }
    await tap('Histórico de compras');
    await pages('Histórico de compras');
    await mount(PokerHome(api: api));
    await tap('Perfil');
    await pages('Meu perfil');
    await tap('Links compartilhados');
    await pages('Meus links');
    for (final entry in <String, Widget>{
      'Recompensa diária': DailyScreen(api: api),
      'Criar mesa privada': CreateRoomScreen(api: api),
      'Entrada na mesa': BuyInScreen(
        api: api,
        path: '/unused',
        body: const {},
        minimum: 800,
        maximum: 2000,
      ),
      'Rever mão': HandScreen(api: api, hand: ReviewApi.hand),
      'Anotar e organizar': HandNotesEditor(
        api: api,
        handId: 'mao-demonstracao',
      ),
      'Coleções e revisão': CollectionsScreen(api: api),
      'Compartilhar mão': ShareHandScreen(api: api, handId: 'mao-demonstracao'),
      'Perfil do jogador': PublicProfile(api: api, playerId: 'bruno'),
      'Denunciar jogador': ReportPlayerScreen(
        api: api,
        playerId: 'bruno',
        surface: 'profile',
      ),
      'Personalizar perfil': ShowcaseEditor(api: api),
      'Preferências da mesa': const NativePreferences(),
      'Operações pendentes · sem pendências': PendingOperationsScreen(api: api),
      'Guia do Poker': const GuideScreen(),
    }.entries) {
      await mount(entry.value);
      await pages(entry.key);
    }
    final live = PokerRealtime(api.session)
      ..playerId = 'ana'
      ..phase = ConnectionPhase.live
      ..snapshot = TableSnapshot(
        handId: 'review',
        stage: 'flop',
        currentPlayerId: 'ana',
        snapshotVersion: Int64(10),
        board: ['Ah', 'Td', '3c'],
        dealerPlayerId: '2',
        bigBlindPlayerId: '4',
        smallBlindPlayerId: '3',
        pots: [Pot(amount: Int64(1250))],
        legalActions: LegalActions(
          actions: ['fold', 'call', 'raise'],
          callAmount: Int64(200),
          minRaiseTo: Int64(400),
          maxRaiseTo: Int64(8400),
        ),
        seats: [
          Seat(
            playerId: 'ana',
            name: 'Você',
            stack: Int64(8400),
            holeCards: ['As', 'Kh'],
            ready: true,
          ),
          for (var i = 0; i < 8; i++)
            Seat(
              playerId: '$i',
              name: [
                'Bruno',
                'Carla',
                'Diego',
                'Elisa',
                'Fábio',
                'Gabi',
                'Hugo',
                'Iara',
              ][i],
              stack: Int64(5000 + i * 250),
              holeCards: ['back', 'back'],
            ),
        ],
      );
    addTearDown(live.dispose);
    Widget table() => Scaffold(
      appBar: AppBar(
        title: const Text('Flop'),
        leading: const BackButton(),
        actions: const [
          PokerIcon(PokerIcons.messageCircle),
          SizedBox(width: 20),
          PokerIcon(PokerIcons.ellipsisVertical),
          SizedBox(width: 12),
        ],
      ),
      body: SafeArea(
        child: TablePlayArea(
          realtime: live,
          heroId: 'ana',
          now: 12000,
          onSeat: (_) {},
        ),
      ),
    );
    await mount(table());
    await capture('Mesa · nove jogadores');
    await tap('Aumentar');
    await capture('Mesa · aumentar aposta');
    tester.state<NavigatorState>(find.byType(Navigator)).pop();
    await tester.pumpAndSettle();
    await tester.pump(const Duration(seconds: 1));
    tester.view.physicalSize = const Size(844, 390);
    await mount(table());
    await pages('Mesa · horizontal');
    expect(tester.getRect(find.text('POTE 1.250')).bottom, lessThan(390));
    expect(tester.getRect(find.text('Aumentar')).bottom, lessThan(390));
    tester.view.physicalSize = const Size(390, 844);
    await mount(
      Scaffold(
        body: Align(
          alignment: Alignment.bottomCenter,
          child: ChatPanel(
            api: api,
            roomId: 'review',
            moderation: live,
            ready: () => true,
            retryModeration: () {},
            realtime: live,
            allowed: (_) => true,
          ),
        ),
      ),
    );
    await capture('Mesa · chat');
    await mount(BotChallengeScreen(realtime: live));
    await pages('Verificação de segurança · sem configuração');
    api.hasPending = true;
    await mount(PendingOperationsScreen(api: api));
    await pages('Operações pendentes · conferir entrada');
    for (final status in ['pending', 'confirmed', 'expired', 'refunded']) {
      await mount(
        PurchaseScreen(
          api: api,
          path: '/unused',
          initial: {
            'purchase_id': 'review',
            'status': status,
            'price_cents': 990,
            if (status == 'pending')
              'pix_copia_e_cola': 'DEMONSTRACAO-SEM-VALOR',
          },
        ),
      );
      await pages('Compra · $status');
    }
    tester.view.physicalSize = const Size(1024, 768);
    await mount(PokerHome(api: api));
    expect(find.byType(NavigationRail), findsOneWidget);
    expect(find.byType(NavigationBar), findsNothing);
    await capture('Tablet · Lobby');
    await tap('Mãos');
    await capture('Tablet · Mãos');
    tester.view.physicalSize = const Size(844, 390);
    await mount(PokerHome(api: api));
    await tester.ensureVisible(find.text('Perfil'));
    await tap('Perfil');
    expect(find.text('Meu código de amizade'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    if (export) {
      await tester.runAsync(
        () => File(
          'docs/screenshots/manifest.json',
        ).writeAsString(const JsonEncoder.withIndent('  ').convert(manifest)),
      );
    }
  });
}
