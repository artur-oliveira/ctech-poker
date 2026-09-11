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
    const labels = ['Jogar', 'Minha jornada', 'Pessoas', 'Loja', 'Perfil'];
    const icons = [
      Icons.casino_outlined,
      Icons.insights_outlined,
      Icons.people_outline,
      Icons.storefront_outlined,
      Icons.person_outline,
    ];
    return Scaffold(
      appBar: AppBar(
        title: Text(labels[selected]),
        actions: [
          IconButton(
            tooltip: 'Operações pendentes',
            icon: const Icon(Icons.receipt_long_outlined),
            onPressed: () => Navigator.push(
              context,
              MaterialPageRoute<void>(
                builder: (_) => PendingOperationsScreen(api: widget.api),
              ),
            ),
          ),
          IconButton(
            tooltip: 'Guia do Poker',
            icon: const Icon(Icons.help_outline),
            onPressed: () => Navigator.push(
              context,
              MaterialPageRoute<void>(builder: (_) => const GuideScreen()),
            ),
          ),
        ],
      ),
      body: SafeArea(
        child: KeyedSubtree(
          key: ValueKey('$selected-$revision'),
          child: screens[selected],
        ),
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: selected,
        onDestinationSelected: (index) => setState(() => selected = index),
        destinations: [
          for (var i = 0; i < labels.length; i++)
            NavigationDestination(icon: Icon(icons[i]), label: labels[i]),
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
                {...stake, 'max_seats': seats, 'seats_available': 0},
      ];
      final name = profile['name'] ?? 'Jogador';
      final id = profile['user_id'] as String;
      return ListView(
        padding: const EdgeInsets.all(20),
        physics: const AlwaysScrollableScrollPhysics(),
        children: [
          Text(
            'Boa mesa, $name.',
            style: Theme.of(context).textTheme.headlineMedium,
          ),
          const SizedBox(height: 8),
          Text(
            '${chips(profile['sandbox_balance'])} fichas disponíveis',
            style: Theme.of(context).textTheme.titleLarge,
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
                leading: const Icon(Icons.play_circle_outline),
                title: const Text('Voltar à minha mesa'),
                subtitle: Text(session['table_id'].toString()),
                onTap: () => open(context, session['table_id'], id),
              ),
            ),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              OutlinedButton.icon(
                onPressed: () => Navigator.push(
                  context,
                  MaterialPageRoute<void>(
                    builder: (_) => DailyScreen(api: api),
                  ),
                ),
                icon: const Icon(Icons.redeem),
                label: const Text('Recompensa diária'),
              ),
              OutlinedButton.icon(
                onPressed: profile['poker_terms_accepted'] != true
                    ? null
                    : () => create(context, id, reload),
                icon: const Icon(Icons.add),
                label: const Text('Criar mesa privada'),
              ),
              OutlinedButton.icon(
                onPressed: profile['poker_terms_accepted'] != true
                    ? null
                    : () => privateJoin(context, id),
                icon: const Icon(Icons.key),
                label: const Text('Entrar por convite'),
              ),
            ],
          ),
          const SizedBox(height: 24),
          Text(
            'Escolha o seu ritmo',
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const Text('Fichas recreativas · sem conversão em dinheiro'),
          const SizedBox(height: 12),
          if (buckets.isEmpty)
            const Padding(
              padding: EdgeInsets.all(24),
              child: Text(
                'Nenhuma mesa disponível agora. Puxe para atualizar.',
              ),
            ),
          for (final bucket in buckets)
            Card(
              child: ListTile(
                contentPadding: const EdgeInsets.all(16),
                leading: const CircleAvatar(child: Icon(Icons.style)),
                title: Text(
                  '${chips(bucket['small_blind'])} / ${chips(bucket['big_blind'])}',
                ),
                subtitle: Text(
                  '${bucket['max_seats']} lugares · ${bucket['seats_available']} assentos livres',
                ),
                trailing: const Icon(Icons.chevron_right),
                enabled: profile['poker_terms_accepted'] == true,
                onTap: () => safely(context, () async {
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
                  if (room == null) return;
                  if (context.mounted) open(context, room['room_id'], id);
                }),
              ),
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
            const Icon(Icons.redeem, size: 76, color: Color(0xffdfbc71)),
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
                      avatar: Icon(
                        day['claimed'] == true ? Icons.check : Icons.redeem,
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
            'Em Jogar, escolha os blinds e confirme a quantidade de fichas. Uma sessão aberta aparece em Voltar à minha mesa.',
          ),
          SizedBox(height: 16),
          Text(
            'Na mesa, suas cartas ficam junto às ações. Pagar, passar, desistir e aumentar só ficam disponíveis quando o servidor confirma sua vez. O valor de aumentar é o total da aposta nesta rodada.',
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
            'Minha jornada reúne mãos, replay, estatísticas, conquistas e ranking. Cartas ocultas continuam ocultas. As provas de embaralhamento são verificadas neste aparelho.',
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
