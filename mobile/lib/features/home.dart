import 'poker_icon.dart';
import 'poker_logo.dart';
import '../core/design.dart';
import 'pending_operations.dart';
import 'buy_in.dart';
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';
import '../core/api.dart';
import '../core/realtime.dart';
import 'widgets.dart';
import 'table.dart';
import 'create_room.dart';
import 'library.dart';
import 'store.dart';
import 'people.dart';

class PokerHome extends StatefulWidget {
  const PokerHome({super.key, required this.api});
  final PokerApi api;
  @override
  State<PokerHome> createState() => _PokerHomeState();
}

class _PokerHomeState extends State<PokerHome> with WidgetsBindingObserver {
  late final realtime = PokerRealtime(widget.api.session);
  StreamSubscription<dynamic>? subscription;
  int selected = 0, revision = 0;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    realtime.connect();
    subscription = realtime.events.stream.listen((event) {
      if ([
            'payment_received',
            'sandbox_purchase_update',
            'reaction_purchase_update',
            'cosmetic_purchase_update',
            'social_event',
            'achievement_unlocked',
          ].contains(event.type) &&
          mounted) {
        setState(() => revision++);
      }
    });
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      realtime.connect();
      setState(() => revision++);
    } else if (state == AppLifecycleState.paused) {
      realtime.suspend();
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    subscription?.cancel();
    realtime.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final screens = [
      LobbyScreen(api: widget.api),
      LibraryScreen(api: widget.api),
      PeopleScreen(api: widget.api),
      StoreScreen(api: widget.api),
      ProfileScreen(api: widget.api),
    ];
    const labels = ['Lobby', 'Mãos', 'Pessoas', 'Loja', 'Perfil'];
    const icons = [
      PokerIcons.layoutGrid,
      PokerIcons.history,
      PokerIcons.users,
      PokerIcons.shoppingBag,
      PokerIcons.userRound,
    ];
    final wide = MediaQuery.sizeOf(context).width >= 600;
    final content = KeyedSubtree(
      key: ValueKey('$selected-$revision'),
      child: screens[selected],
    );
    return Scaffold(
      appBar: AppBar(
        leading: const Center(child: PokerLogo(size: 32)),
        title: Text(labels[selected]),
        actions: [
          IconButton(
            tooltip: 'Operações pendentes',
            icon: const PokerIcon(PokerIcons.receiptText),
            onPressed: () => Navigator.push(
              context,
              MaterialPageRoute<void>(
                builder: (_) => PendingOperationsScreen(api: widget.api),
              ),
            ),
          ),
          IconButton(
            tooltip: 'Guia do Poker',
            icon: const PokerIcon(PokerIcons.circleHelp),
            onPressed: () => Navigator.push(
              context,
              MaterialPageRoute<void>(builder: (_) => const GuideScreen()),
            ),
          ),
        ],
      ),
      body: SafeArea(
        child: wide
            ? Row(
                children: [
                  NavigationRail(
                    scrollable: true,
                    selectedIndex: selected,
                    onDestinationSelected: (index) =>
                        setState(() => selected = index),
                    labelType: NavigationRailLabelType.all,
                    backgroundColor: PokerColors.seat,
                    indicatorColor: PokerColors.brand,
                    destinations: [
                      for (var i = 0; i < labels.length; i++)
                        NavigationRailDestination(
                          icon: PokerIcon(icons[i]),
                          label: Text(labels[i]),
                        ),
                    ],
                  ),
                  const VerticalDivider(width: 1),
                  Expanded(child: content),
                ],
              )
            : content,
      ),
      bottomNavigationBar: wide
          ? null
          : NavigationBar(
              selectedIndex: selected,
              onDestinationSelected: (index) =>
                  setState(() => selected = index),
              destinations: [
                for (var i = 0; i < labels.length; i++)
                  NavigationDestination(
                    icon: PokerIcon(icons[i]),
                    label: labels[i],
                  ),
              ],
            ),
    );
  }
}

class LobbyScreen extends StatelessWidget {
  const LobbyScreen({super.key, required this.api});
  final PokerApi api;
  Future<dynamic> load() async => Future.wait([
    api.get('/v1.0/players/me'),
    api.get('/v1.0/rooms/buckets', query: {'currency_mode': 'sandbox'}),
    api.get('/v1.0/players/me/sessions'),
    api.get('/v1.0/rooms/stakes', query: {'currency_mode': 'sandbox'}),
  ]);
  void open(
    BuildContext context,
    String id,
    String playerId, {
    String shareCode = '',
  }) => Navigator.push(
    context,
    MaterialPageRoute<void>(
      builder: (_) => TableScreen(
        api: api,
        roomId: id,
        playerId: playerId,
        shareCode: shareCode,
      ),
    ),
  );
  @override
  Widget build(BuildContext context) => AsyncPanel(
    load: load,
    builder: (context, result, reload) {
      final profile = result[0] as Json,
          available = rows(result[1]),
          sessions = rows(result[2]);
      final buckets = [
        for (final stake in rows(result[3], 'stakes'))
          for (final seats in [2, 6, 9])
            available
                    .where(
                      (b) =>
                          b['small_blind'] == stake['small_blind'] &&
                          b['big_blind'] == stake['big_blind'] &&
                          b['max_seats'] == seats,
                    )
                    .firstOrNull ??
                {
                  ...stake,
                  'max_seats': seats,
                  'seats_available': 0,
                  'open_rooms': 0,
                },
      ];
      final id = profile['user_id'] as String;
      return ListView(
        padding: const EdgeInsets.all(20),
        physics: const AlwaysScrollableScrollPhysics(),
        children: [
          Row(
            children: [
              const PokerIcon(
                PokerIcons.coins,
                size: 18,
                color: PokerColors.gold,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  '${chips(profile['sandbox_balance'])} fichas',
                  style: const TextStyle(
                    fontFamily: PokerTheme.mono,
                    color: PokerColors.gold,
                  ),
                ),
              ),
              TextButton.icon(
                onPressed: () => Navigator.push(
                  context,
                  MaterialPageRoute<void>(
                    builder: (_) => DailyScreen(api: api),
                  ),
                ),
                icon: const PokerIcon(PokerIcons.gift, size: 18),
                label: const Text('Recompensa'),
              ),
            ],
          ),
          const SizedBox(height: 16),
          Text(
            'Escolha os blinds e o tamanho da mesa.',
            style: Theme.of(
              context,
            ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 20),
          if (profile['poker_terms_accepted'] != true)
            Card(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      'Antes da primeira mão, leia e aceite os Termos do CTech Poker.',
                    ),
                    TextButton(
                      onPressed: () => launchUrl(
                        Uri.parse(
                          'https://accounts.aoctech.app/products/poker',
                        ),
                        mode: LaunchMode.externalApplication,
                      ),
                      child: const Text('Ler os termos'),
                    ),
                    FilledButton(
                      onPressed: () => safely(context, () async {
                        await api.post('/v1.0/players/me/terms/accept');
                        reload();
                      }),
                      child: const Text('Li e aceito os termos'),
                    ),
                  ],
                ),
              ),
            ),
          for (final session in sessions.where((s) => s['ended_at'] == 0))
            Card(
              child: ListTile(
                leading: const PokerIcon(PokerIcons.play),
                title: const Text('Voltar à minha mesa'),
                subtitle: Text(session['table_id'].toString()),
                onTap: () => open(context, session['table_id'], id),
              ),
            ),
          Row(
            children: [
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: profile['poker_terms_accepted'] != true
                      ? null
                      : () => create(context, id, reload),
                  icon: const PokerIcon(PokerIcons.lock, size: 18),
                  label: const Text('Mesa privada'),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: profile['poker_terms_accepted'] != true
                      ? null
                      : () => privateJoin(context, id),
                  icon: const PokerIcon(PokerIcons.keyRound, size: 18),
                  label: const Text('Convite'),
                ),
              ),
            ],
          ),
          const SizedBox(height: 24),
          _LobbyPicker(
            buckets: buckets,
            enabled: profile['poker_terms_accepted'] == true,
            onSelect: (bucket) => safely(context, () async {
              final bb = (bucket['big_blind'] as num).toInt();
              final room = await Navigator.push<Json>(
                context,
                MaterialPageRoute<Json>(
                  builder: (_) => BuyInScreen(
                    api: api,
                    path: '/v1.0/rooms/join-or-create',
                    minimum: bb * 40,
                    maximum: bb * 100,
                    body: {
                      'small_blind': bucket['small_blind'],
                      'big_blind': bb,
                      'max_seats': bucket['max_seats'],
                      'currency_mode': 'sandbox',
                    },
                  ),
                ),
              );
              if (room != null && context.mounted) {
                open(context, room['room_id'], id);
              }
            }),
          ),
        ],
      );
    },
  );
  Future<void> create(BuildContext context, String id, VoidCallback reload) =>
      safely(context, () async {
        final room = await Navigator.push<Json>(
          context,
          MaterialPageRoute<Json>(builder: (_) => CreateRoomScreen(api: api)),
        );
        if (room == null) return;
        reload();
        if (context.mounted) {
          await join(
            context,
            room['room_id'] ?? room['id'],
            id,
            room['share_code'] ?? '',
          );
        }
      });
  Future<void> privateJoin(BuildContext context, String id) =>
      safely(context, () async {
        final link = await input(context, 'Cole o link do convite');
        if (link == null || !context.mounted) return;
        final uri = Uri.tryParse(link);
        final roomId = uri?.queryParameters['id'];
        final code =
            uri?.queryParameters['invite'] ??
            uri?.queryParameters['code'] ??
            uri?.queryParameters['share_code'] ??
            '';
        if (roomId == null || roomId.isEmpty) {
          throw StateError('Use um link de convite válido.');
        }
        await join(context, roomId, id, code);
      });
  Future<void> join(
    BuildContext context,
    String roomId,
    String id,
    String code,
  ) async {
    final room = await api.get('/v1.0/rooms/${segment(roomId)}');
    final seat = await api.get('/v1.0/rooms/${segment(roomId)}/seated');
    if (!context.mounted) return;
    if (seat['seated'] == true && (seat['stack'] as num? ?? 0) > 0) {
      open(context, roomId, id, shareCode: code);
      return;
    }
    if (room['currency_mode'] != 'sandbox') {
      throw StateError(
        'Esta versão ainda não permite entrada em mesas de dinheiro real.',
      );
    }
    final result = await Navigator.push<Json>(
      context,
      MaterialPageRoute<Json>(
        builder: (_) => BuyInScreen(
          api: api,
          path: '/v1.0/rooms/${segment(roomId)}/join',
          minimum: (room['buy_in_min'] as num).toInt(),
          maximum: (room['buy_in_max'] as num).toInt(),
          body: {'share_code': code},
        ),
      ),
    );
    if (result == null) return;
    if (context.mounted) open(context, roomId, id, shareCode: code);
  }
}

class _LobbyPicker extends StatefulWidget {
  const _LobbyPicker({
    required this.buckets,
    required this.enabled,
    required this.onSelect,
  });
  final List<Json> buckets;
  final bool enabled;
  final void Function(Json) onSelect;
  @override
  State<_LobbyPicker> createState() => _LobbyPickerState();
}

class _LobbyPickerState extends State<_LobbyPicker> {
  String? selected;
  @override
  Widget build(BuildContext context) {
    final stakes = {
      for (final b in widget.buckets)
        '${b['small_blind']}/${b['big_blind']}': b,
    };
    if (stakes.isEmpty) {
      return const Text('Nenhum stake disponível no momento.');
    }
    final current = stakes.containsKey(selected)
        ? selected!
        : stakes.keys.first;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          'Escolha os blinds',
          style: Theme.of(context).textTheme.titleMedium,
        ),
        const SizedBox(height: 10),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final e in stakes.entries)
              ChoiceChip(
                label: Text(
                  '${chips(e.value['small_blind'])} / ${chips(e.value['big_blind'])}',
                ),
                selected: e.key == current,
                showCheckmark: false,
                onSelected: (_) => setState(() => selected = e.key),
              ),
          ],
        ),
        const SizedBox(height: 24),
        Text('Tamanho da mesa', style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 12),
        for (final bucket in widget.buckets.where(
          (b) => '${b['small_blind']}/${b['big_blind']}' == current,
        ))
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Material(
              color: PokerColors.seat,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
                side: const BorderSide(color: PokerColors.border),
              ),
              child: InkWell(
                borderRadius: BorderRadius.circular(12),
                onTap: widget.enabled ? () => widget.onSelect(bucket) : null,
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Expanded(
                            child: Text(
                              {
                                    2: 'HEADS-UP',
                                    6: '6-MAX',
                                    9: 'FULL-RING',
                                  }[bucket['max_seats']] ??
                                  'Mesa',
                              style: Theme.of(
                                context,
                              ).textTheme.titleLarge?.copyWith(fontSize: 20),
                            ),
                          ),
                          const PokerIcon(PokerIcons.arrowRight, size: 20),
                        ],
                      ),
                      const SizedBox(height: 8),
                      Row(
                        children: [
                          const PokerIcon(PokerIcons.users, size: 16),
                          const SizedBox(width: 8),
                          Expanded(
                            child: Text(
                              'Até ${bucket['max_seats']} jogadores · ${bucket['open_rooms'] ?? 0} mesas ativas',
                              style: const TextStyle(
                                color: PokerColors.secondaryText,
                              ),
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 8),
                      Text(
                        'Entrada: ${chips((bucket['big_blind'] as num) * 40)} a ${chips((bucket['big_blind'] as num) * 100)} fichas',
                        style: const TextStyle(
                          fontSize: 13,
                          color: PokerColors.muted,
                        ),
                      ),
                      const SizedBox(height: 12),
                      Text(
                        (bucket['open_rooms'] as num? ?? 0) > 0
                            ? 'Entrar agora'
                            : 'Criar mesa',
                        style: const TextStyle(fontWeight: FontWeight.w600),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
      ],
    );
  }
}

class DailyScreen extends StatelessWidget {
  const DailyScreen({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Recompensa diária')),
    body: SafeArea(
      child: AsyncPanel(
        load: () => api.get('/v1.0/sandbox-credits/'),
        builder: (context, data, reload) => ListView(
          padding: const EdgeInsets.all(24),
          children: [
            const PokerIcon(PokerIcons.gift, size: 76, color: PokerColors.gold),
            const SizedBox(height: 24),
            Text(
              '${data['current_streak']} dias de sequência',
              style: Theme.of(context).textTheme.headlineMedium,
            ),
            Text(
              'Recorde: ${data['best_streak']} dias · Dia ${data['cycle_day']} de ${data['cycle_length']}',
            ),
            if (data['protection_available'] == true)
              const Text('Proteção de sequência disponível'),
            if (data['streak_at_risk'] == true)
              const Text('Resgate hoje para proteger sua sequência.'),
            const SizedBox(height: 20),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: rows(data, 'days')
                  .map(
                    (day) => Chip(
                      avatar: PokerIcon(
                        day['claimed'] == true
                            ? PokerIcons.check
                            : PokerIcons.gift,
                        size: 18,
                      ),
                      label: Text(
                        'Dia ${day['day']} · ${chips(day['amount'])}',
                      ),
                    ),
                  )
                  .toList(),
            ),
            const SizedBox(height: 24),
            FilledButton(
              onPressed: (data['remaining_time_seconds'] as num? ?? 0) > 0
                  ? null
                  : () => safely(context, () async {
                      final result = await api.post('/v1.0/sandbox-credits/');
                      reload();
                      if (context.mounted) {
                        toast(
                          context,
                          '${chips(result['amount'])} fichas recebidas!',
                        );
                      }
                    }),
              child: Text(
                (data['remaining_time_seconds'] as num? ?? 0) > 0
                    ? 'Próximo resgate em ${((data['remaining_time_seconds'] as num) / 3600).ceil()}h'
                    : 'Resgatar fichas',
              ),
            ),
          ],
        ),
      ),
    ),
  );
}

class GuideScreen extends StatelessWidget {
  const GuideScreen({super.key});
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Guia do Poker')),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(24),
        children: const [
          Text('Sua mesa, sempre à mão', style: TextStyle(fontSize: 28)),
          SizedBox(height: 16),
          Text(
            'No Lobby, escolha os blinds e confirme a quantidade de fichas. Uma sessão aberta aparece em Voltar à minha mesa.',
          ),
          SizedBox(height: 16),
          Text(
            'Na mesa, suas cartas ficam junto às ações. Pagar, Check, Fold e Aumentar só ficam disponíveis quando o servidor confirma sua vez. O valor de aumentar é o total da aposta nesta rodada.',
          ),
          SizedBox(height: 16),
          Text(
            'Aumentar aparece em vermelho; Pagar e Check usam botões claros. O dourado destaca fichas, pote e a vez do jogador. Leia sempre o nome e o valor da ação antes de confirmar.',
          ),
          SizedBox(height: 16),
          Text(
            'Use o menu da mesa para pausar, manter seu lugar, mostrar cartas, combinar duas viradas ou pedir para sair após a mão. Voltar à navegação não encerra seu assento.',
          ),
          SizedBox(height: 16),
          Text(
            'Ao alternar de aplicativo, o jogo continua no servidor. Quando você voltar, aguarde a sincronização antes de agir. Uma ação sem confirmação não será repetida automaticamente.',
          ),
          SizedBox(height: 16),
          Text(
            'Mãos reúne mãos, replay, estatísticas, conquistas e ranking. Cartas ocultas continuam ocultas. As provas de embaralhamento são verificadas neste aparelho.',
          ),
          SizedBox(height: 16),
          Text(
            'Pessoas reúne amizades, convites e privacidade. A loja mostra preços e propriedade informados pelo servidor. Confirme compras e acompanhe seu status antes de tentar novamente.',
          ),
        ],
      ),
    ),
  );
}
