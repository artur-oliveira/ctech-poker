import '../core/labels.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../core/api.dart';
import '../core/fairness.dart';
import 'widgets.dart';
import 'native.dart';
import 'collections.dart';
import 'showcase.dart';
import 'hand_browser.dart';

class LibraryScreen extends StatelessWidget {
  const LibraryScreen({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => DefaultTabController(
    length: 5,
    child: Column(
      children: [
        const TabBar(
          isScrollable: true,
          tabs: [
            Tab(text: 'Mãos'),
            Tab(text: 'Estatísticas'),
            Tab(text: 'Conquistas'),
            Tab(text: 'Ranking'),
            Tab(text: 'Sessões'),
          ],
        ),
        Expanded(
          child: TabBarView(
            children: [
              HandBrowser(api: api),
              AsyncPanel(
                load: () => api.get(
                  '/v1.0/players/me/poker-stats',
                  query: {'mode': 'sandbox'},
                ),
                builder: (context, stats, _) => ListView(
                  padding: const EdgeInsets.all(20),
                  children: [
                    Text(
                      '${chips(stats['hands'])} mãos jogadas',
                      style: Theme.of(context).textTheme.headlineMedium,
                    ),
                    for (final entry in {
                      'vpip_rate': 'Participação voluntária (VPIP)',
                      'pfr_rate': 'Aumento pré-flop (PFR)',
                      'three_bet_rate': '3-bet',
                    }.entries)
                      Card(
                        child: ListTile(
                          title: Text(entry.value),
                          trailing: Text(
                            '${((stats[entry.key] as num? ?? 0) * 100).toStringAsFixed(1)}%',
                          ),
                        ),
                      ),
                  ],
                ),
              ),
              AsyncPanel(
                load: () => api.get(
                  '/v1.0/players/me/achievements/summary',
                  query: {'mode': 'sandbox'},
                ),
                builder: (context, summary, _) => ListView(
                  padding: const EdgeInsets.all(16),
                  children: [
                    Text(
                      '${summary['totals']['stars']} estrelas conquistadas',
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    for (final achievement in rows(summary, 'achievements'))
                      Card(
                        child: ListTile(
                          leading: Icon(
                            achievement['unlocked'] == true
                                ? Icons.workspace_premium
                                : Icons.lock_outline,
                          ),
                          title: Text(achievementLabel(achievement['key'])),
                          subtitle: Text(
                            '${achievement['progress']} / ${achievement['next_target'] ?? achievement['max_target']}',
                          ),
                          trailing: Text('★ ${achievement['stars']}'),
                        ),
                      ),
                  ],
                ),
              ),
              PagedList(
                api: api,
                path: '/v1.0/leaderboard',
                query: const {'mode': 'sandbox'},
                item: (player, _) => Card(
                  child: ListTile(
                    leading: const Icon(Icons.emoji_events_outlined),
                    title: Text(player['player_name'] ?? 'Jogador'),
                    subtitle: Text(
                      '${chips(player['hands_won'])} vitórias · ${chips(player['hands_played'])} mãos',
                    ),
                    trailing: Text(
                      '${((player['win_rate'] as num? ?? 0) * 100).toStringAsFixed(1)}%',
                    ),
                  ),
                ),
              ),
              PagedList(
                api: api,
                path: '/v1.0/players/me/sessions',
                item: (session, _) => Card(
                  child: ListTile(
                    title: Text(
                      'Resultado: ${chips(session['net_pnl'])} fichas',
                    ),
                    subtitle: Text(
                      'Entrada ${chips(session['buyin_amount'])} · Saída ${chips(session['cashout_amount'])}',
                    ),
                    trailing: session['ended_at'] == 0
                        ? const Chip(label: Text('Aberta'))
                        : null,
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    ),
  );
}

class HandScreen extends StatefulWidget {
  const HandScreen({super.key, required this.api, required this.hand});
  final PokerApi api;
  final Json hand;
  @override
  State<HandScreen> createState() => _HandScreenState();
}

class _HandScreenState extends State<HandScreen> {
  int frame = 0;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(
      title: const Text('Rever mão'),
      actions: [
        IconButton(
          tooltip: 'Compartilhar mão',
          icon: const Icon(Icons.share),
          onPressed: share,
        ),
        IconButton(
          tooltip: 'Anotar e organizar',
          icon: const Icon(Icons.edit_note),
          onPressed: notes,
        ),
      ],
    ),
    body: SafeArea(
      child: AsyncPanel(
        load: () => widget.api.get(
          '/v1.0/tables/${segment(widget.hand['table_id'])}/hands/${segment(widget.hand['hand_id'])}/history',
        ),
        builder: (context, data, _) {
          final actions = rows(data, 'actions');
          final index = frame.clamp(
            0,
            actions.isEmpty ? 0 : actions.length - 1,
          );
          final current = actions.isEmpty
              ? <String, dynamic>{}
              : actions[index];
          final replay = current['frame'] as Map?;
          final board = List<String>.from(
            replay?['board'] ??
                (actions.isEmpty ? widget.hand['board'] ?? [] : []),
          );
          return ListView(
            padding: const EdgeInsets.all(20),
            children: [
              PlayingCards(board),
              if (replay?['board_two'] != null)
                PlayingCards(List<String>.from(replay!['board_two'])),
              const SizedBox(height: 16),
              Text('Suas cartas', textAlign: TextAlign.center),
              PlayingCards(List<String>.from(widget.hand['hole_cards'] ?? [])),
              const SizedBox(height: 20),
              Text(
                actions.isEmpty
                    ? 'Replay indisponível para esta mão.'
                    : '${index + 1} / ${actions.length} · ${current['action']} · ${chips(current['amount'])}',
                textAlign: TextAlign.center,
              ),
              if (actions.isNotEmpty)
                Slider(
                  value: index.toDouble(),
                  min: 0,
                  max: (actions.length - 1).clamp(1, 99999).toDouble(),
                  divisions: (actions.length - 1).clamp(1, 99999),
                  onChanged: (value) => setState(() => frame = value.round()),
                ),
              Wrap(
                alignment: WrapAlignment.center,
                children: [
                  IconButton(
                    tooltip: 'Ação anterior',
                    onPressed: index > 0 ? () => setState(() => frame--) : null,
                    icon: const Icon(Icons.skip_previous),
                  ),
                  IconButton(
                    tooltip: 'Próxima ação',
                    onPressed: index < actions.length - 1
                        ? () => setState(() => frame++)
                        : null,
                    icon: const Icon(Icons.skip_next),
                  ),
                ],
              ),
              for (final seat in (replay?['seats'] as List? ?? []))
                ListTile(
                  title: Text(seat['name'] ?? 'Jogador'),
                  subtitle: Text(seat['state'] ?? ''),
                  trailing: Text(chips(seat['stack'])),
                ),
              const Divider(),
              Text(
                'Resultado da mão',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              for (final opponent in rows(widget.hand, 'opponents'))
                ListTile(
                  title: Text(opponent['name'] ?? 'Jogador'),
                  subtitle: PlayingCards(
                    List<String>.from(opponent['hole_cards'] ?? []),
                    compact: true,
                  ),
                  trailing: opponent['won'] == true
                      ? const Icon(Icons.emoji_events)
                      : null,
                ),
              const Divider(),
              FilledButton.tonalIcon(
                onPressed: fairness,
                icon: const Icon(Icons.verified_user_outlined),
                label: const Text('Verificar embaralhamento neste aparelho'),
              ),
            ],
          );
        },
      ),
    ),
  );
  void fairness() {
    final hand = widget.hand;
    final seed = hand['server_seed'] as String? ?? '';
    final root = hand['root_commit_hash'] as String? ?? '';
    if (seed.isEmpty && root.isEmpty) {
      toast(context, 'Esta mão não possui prova disponível.');
      return;
    }
    final valid = seed.isNotEmpty
        ? verifyDeck(seed, hand['commit_hash'] ?? '')
        : verifyPartial(
            root,
            Map<String, dynamic>.from(hand['revealed_card_salts'] ?? {}),
            Map<String, dynamic>.from(hand['unrevealed_card_hashes'] ?? {}),
          );
    toast(
      context,
      valid
          ? 'Prova verificada neste aparelho. Cartas ocultas permanecem privadas.'
          : 'A prova não confere. Não considere este embaralhamento verificado.',
    );
  }

  Future<void> notes() => Navigator.push(
    context,
    MaterialPageRoute<void>(
      builder: (_) =>
          HandNotesEditor(api: widget.api, handId: widget.hand['hand_id']),
    ),
  );
  Future<void> share() => safely(context, () async {
    if (!await confirm(
      context,
      'Criar link público',
      'Compartilhar esta mão por 7 dias, sem suas cartas privadas?',
    )) {
      return;
    }
    final result = await widget.api.post(
      '/v1.0/players/me/hand/${segment(widget.hand['hand_id'])}/share',
      {
        'kind': 'brag',
        'include_hero_cards': false,
        'expiry_days': 7,
        'mode': 'sandbox',
      },
    );
    await Clipboard.setData(
      ClipboardData(
        text:
            'https://poker.aoctech.app/share?token=${Uri.encodeComponent(result['token'])}',
      ),
    );
    if (mounted) toast(context, 'Link copiado. Você pode revogá-lo no perfil.');
  });
}

class ProfileScreen extends StatelessWidget {
  const ProfileScreen({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => AsyncPanel(
    load: () => api.get('/v1.0/players/me'),
    builder: (context, profile, reload) => ListView(
      padding: const EdgeInsets.all(20),
      children: [
        Center(
          child: ClipOval(
            child:
                profile['avatar_url'] is String &&
                    (profile['avatar_url'] as String).isNotEmpty
                ? Image.network(
                    profile['avatar_url'],
                    width: 80,
                    height: 80,
                    fit: BoxFit.cover,
                    errorBuilder: (_, error, stack) =>
                        const Icon(Icons.person, size: 80),
                  )
                : const CircleAvatar(
                    radius: 40,
                    child: Icon(Icons.person, size: 42),
                  ),
          ),
        ),
        const SizedBox(height: 16),
        Text(
          profile['name'] ?? 'Jogador',
          textAlign: TextAlign.center,
          style: Theme.of(context).textTheme.headlineMedium,
        ),
        ListTile(
          title: const Text('Meu código de amizade'),
          subtitle: Text(profile['friend_code'] ?? 'Indisponível'),
          trailing: const Icon(Icons.copy),
          onTap: () async {
            await Clipboard.setData(
              ClipboardData(text: profile['friend_code'] ?? ''),
            );
            if (context.mounted) toast(context, 'Código copiado.');
          },
        ),
        ListTile(
          title: const Text('Apelido nas mesas'),
          trailing: const Icon(Icons.edit),
          onTap: () => safely(context, () async {
            final name = await input(
              context,
              'Apelido',
              initial: profile['name'] ?? '',
            );
            if (name == null || name.isEmpty) return;
            await api.post('/v1.0/players/me', {'name': name});
            reload();
          }),
        ),
        for (final entry in {
          'showcase_public': 'Perfil público',
          'table_public': 'Amigos podem ver minha mesa',
          'playstyle_public': 'Mostrar meu estilo de jogo',
        }.entries)
          SwitchListTile(
            title: Text(entry.value),
            value: profile[entry.key] == true,
            onChanged: (value) => safely(context, () async {
              await api.post('/v1.0/players/me', {entry.key: value});
              reload();
            }),
          ),
        ListTile(
          title: const Text('Links compartilhados'),
          trailing: const Icon(Icons.chevron_right),
          onTap: () => Navigator.push(
            context,
            MaterialPageRoute<void>(
              builder: (_) => Scaffold(
                appBar: AppBar(title: const Text('Meus links')),
                body: AsyncPanel(
                  load: () => api.get('/v1.0/players/me/hand-shares'),
                  builder: (context, result, refresh) => ListView(
                    children: [
                      for (final share in rows(result))
                        ListTile(
                          title: Text(
                            '${share['kind']} · ${chips(share['net_change'])} fichas',
                          ),
                          trailing: TextButton(
                            onPressed: () => safely(context, () async {
                              if (!await confirm(
                                context,
                                'Revogar link',
                                'Quem recebeu este link não poderá mais acessar a mão.',
                              )) {
                                return;
                              }
                              await api.request(
                                '/v1.0/players/me/hand-shares/${segment(share['token'])}',
                                method: 'DELETE',
                              );
                              refresh();
                            }),
                            child: const Text('Revogar'),
                          ),
                        ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
        ListTile(
          title: const Text('Foto de perfil'),
          leading: const Icon(Icons.add_a_photo),
          onTap: () => safely(context, () async {
            await updateAvatar(api);
            reload();
          }),
        ),
        ListTile(
          title: const Text('Remover foto'),
          onTap: () => safely(context, () async {
            await api.request('/v1.0/players/me/avatar', method: 'DELETE');
            reload();
          }),
        ),
        ListTile(
          title: const Text('Preferências da mesa'),
          onTap: () => Navigator.push(
            context,
            MaterialPageRoute<void>(builder: (_) => const NativePreferences()),
          ),
        ),
        ListTile(
          title: const Text('Personalizar perfil'),
          onTap: () => Navigator.push(
            context,
            MaterialPageRoute<void>(builder: (_) => ShowcaseEditor(api: api)),
          ),
        ),
        const SizedBox(height: 24),
        OutlinedButton.icon(
          onPressed: () => safely(context, api.session.logout),
          icon: const Icon(Icons.logout),
          label: const Text('Sair da conta'),
        ),
      ],
    ),
  );
}
