import '../core/labels.dart';
import 'package:flutter/material.dart';
import 'package:uuid/uuid.dart';
import '../core/api.dart';
import 'home.dart';
import 'widgets.dart';

Future<void> socialMutation(
  PokerApi api,
  String path, {
  String method = 'POST',
  Json body = const {},
}) async {
  await api.request(
    '/v1.0/social$path',
    method: method,
    body: body,
    idempotencyKey: const Uuid().v4(),
  );
}

class SocialInbox extends StatelessWidget {
  const SocialInbox({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => PagedList(
    api: api,
    path: '/v1.0/social/inbox',
    item: (event, reload) => Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            ListTile(
              leading: Icon(
                event['unread'] == true
                    ? Icons.mark_email_unread
                    : Icons.drafts_outlined,
              ),
              title: Text(event['actor_name'] ?? 'Jogador'),
              subtitle: Text(
                {
                      'table_invite': 'Convite para jogar',
                      'friend_request': 'Pedido de amizade',
                      'friend_accepted': 'Amizade aceita',
                    }[event['type']] ??
                    'Notificação',
              ),
              onTap: () => safely(context, () async {
                await socialMutation(
                  api,
                  '/inbox/read',
                  body: {
                    'event_ids': [event['event_id']],
                  },
                );
                reload();
              }),
            ),
            if (event['status'] == 'pending' &&
                ['table_invite', 'friend_request'].contains(event['type']))
              Wrap(
                spacing: 8,
                children: [
                  FilledButton(
                    onPressed: () => safely(context, () async {
                      if (event['type'] == 'friend_request') {
                        await socialMutation(
                          api,
                          '/friend-requests/${segment(event['actor_id'])}/accept',
                        );
                        reload();
                        return;
                      }
                      final result = await api.request(
                        '/v1.0/social/table-invites/${segment(event['event_id'])}/accept',
                        method: 'POST',
                        body: {},
                        idempotencyKey: const Uuid().v4(),
                      );
                      final profile = await api.get('/v1.0/players/me');
                      if (!context.mounted) return;
                      final room = result['room'] as Json;
                      await LobbyScreen(api: api).join(
                        context,
                        room['room_id'] ?? room['id'],
                        profile['user_id'],
                        room['share_code'] ?? '',
                      );
                      reload();
                    }),
                    child: const Text('Aceitar'),
                  ),
                  TextButton(
                    onPressed: () => safely(context, () async {
                      final path = event['type'] == 'table_invite'
                          ? '/table-invites/${segment(event['event_id'])}/decline'
                          : '/friend-requests/${segment(event['actor_id'])}/decline';
                      await socialMutation(api, path);
                      reload();
                    }),
                    child: const Text('Recusar'),
                  ),
                ],
              ),
          ],
        ),
      ),
    ),
  );
}

class PublicProfile extends StatelessWidget {
  const PublicProfile({super.key, required this.api, required this.playerId});
  final PokerApi api;
  final String playerId;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Perfil do jogador')),
    body: SafeArea(
      child: AsyncPanel(
        load: () => api.get('/v1.0/players/${segment(playerId)}/showcase'),
        builder: (context, result, _) {
          final profile = result as Json;
          return ListView(
            padding: const EdgeInsets.all(24),
            children: [
              CircleAvatar(
                radius: 42,
                backgroundImage: profile['avatar_url'] is String
                    ? NetworkImage(profile['avatar_url'])
                    : null,
                child: profile['avatar_url'] == null
                    ? const Icon(Icons.person, size: 40)
                    : null,
              ),
              Text(
                profile['name'] ?? 'Jogador',
                style: Theme.of(context).textTheme.headlineMedium,
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 20),
              for (final section in visibleShowcaseSections(
                profile['showcase_layout'],
              ))
                switch (section) {
                  'achievements' => Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Conquistas em destaque',
                        style: Theme.of(context).textTheme.titleLarge,
                      ),
                      if (rows(profile, 'featured_achievements').isEmpty)
                        const Padding(
                          padding: EdgeInsets.symmetric(vertical: 12),
                          child: Text('Nenhuma conquista em destaque.'),
                        ),
                      for (final achievement in rows(
                        profile,
                        'featured_achievements',
                      ))
                        ListTile(
                          leading: const Icon(Icons.workspace_premium),
                          title: Text(achievementLabel(achievement['key'])),
                          trailing: Text(chips(achievement['count'])),
                        ),
                    ],
                  ),
                  'best_hand' =>
                    profile['best_hand'] is Map
                        ? Card(
                            child: Padding(
                              padding: const EdgeInsets.all(16),
                              child: Column(
                                children: [
                                  const Text('Melhor mão'),
                                  PlayingCards(
                                    List<String>.from(
                                      profile['best_hand']['board'] ?? [],
                                    ),
                                  ),
                                  Text(
                                    '${chips(profile['best_hand']['net_change'])} fichas',
                                  ),
                                ],
                              ),
                            ),
                          )
                        : const SizedBox.shrink(),
                  'matchup' => MatchupPanel(api: api, playerId: playerId),
                  _ => const SizedBox.shrink(),
                },
              FilledButton.icon(
                onPressed: () => safely(context, () async {
                  await socialMutation(
                    api,
                    '/friend-requests',
                    body: {'target_player_id': playerId},
                  );
                  if (context.mounted) toast(context, 'Pedido enviado.');
                }),
                icon: const Icon(Icons.person_add),
                label: const Text('Adicionar amigo'),
              ),
              OutlinedButton.icon(
                onPressed: () => invite(context),
                icon: const Icon(Icons.casino),
                label: const Text('Convidar para minha mesa'),
              ),
            ],
          );
        },
      ),
    ),
  );
  Future<void> invite(BuildContext context) => safely(context, () async {
    final sessions = rows(
      await api.get('/v1.0/players/me/sessions'),
    ).where((s) => s['ended_at'] == 0).toList();
    if (!context.mounted) return;
    if (sessions.isEmpty) {
      toast(context, 'Entre em uma mesa antes de convidar.');
      return;
    }
    final id = await showDialog<String>(
      context: context,
      builder: (context) => SimpleDialog(
        title: const Text('Convidar para qual mesa?'),
        children: [
          for (final session in sessions)
            SimpleDialogOption(
              onPressed: () => Navigator.pop(context, session['table_id']),
              child: Text(session['table_id']),
            ),
        ],
      ),
    );
    if (id == null) return;
    await socialMutation(
      api,
      '/table-invites',
      body: {'target_player_id': playerId, 'room_id': id},
    );
    if (context.mounted) toast(context, 'Convite enviado.');
  });
}

List<String> visibleShowcaseSections(dynamic layout) {
  const defaults = ['achievements', 'best_hand', 'matchup'];
  final value = layout is Map ? layout : const {};
  final order = value['order'] is List ? value['order'] as List : const [];
  final hidden = value['hidden'] is List ? value['hidden'] as List : const [];
  return {
    ...order.whereType<String>().where(defaults.contains),
    ...defaults,
  }.where((id) => id == 'achievements' || !hidden.contains(id)).toList();
}

class MatchupPanel extends StatefulWidget {
  const MatchupPanel({super.key, required this.api, required this.playerId});
  final PokerApi api;
  final String playerId;
  @override
  State<MatchupPanel> createState() => _MatchupPanelState();
}

class _MatchupPanelState extends State<MatchupPanel> {
  late Future<Json> future = load();
  Future<Json> load() =>
      widget.api.get('/v1.0/players/me/matchups/${segment(widget.playerId)}');
  @override
  void didUpdateWidget(MatchupPanel oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.playerId != widget.playerId || oldWidget.api != widget.api) {
      future = load();
    }
  }

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 20),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('Nosso confronto', style: Theme.of(context).textTheme.titleLarge),
        FutureBuilder<Json>(
          future: future,
          builder: (context, state) {
            if (state.connectionState != ConnectionState.done) {
              return const LinearProgressIndicator();
            }
            if (state.hasError) {
              return Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('Não foi possível carregar o confronto.'),
                  TextButton(
                    onPressed: () => setState(() {
                      future = load();
                    }),
                    child: const Text('Tentar confronto novamente'),
                  ),
                ],
              );
            }
            final matchup = state.data!;
            if (matchup['hands_together'] == 0) {
              return const Text('Vocês ainda não jogaram juntos.');
            }
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('${chips(matchup['hands_together'])} mãos juntos'),
                Text(
                  'Você: ${chips(matchup['viewer_wins'])} vitórias · Oponente: ${chips(matchup['opponent_wins'])} · Empates: ${chips(matchup['ties'])}',
                ),
                Text(
                  '${chips(matchup['heads_up_hands_together'])} mãos em duelo',
                ),
                Text(
                  'Resultado: ${chips(matchup['net_change_viewer'])} fichas',
                ),
              ],
            );
          },
        ),
      ],
    ),
  );
}
