import 'poker_icon.dart';
import 'report_player.dart';
import 'package:flutter/material.dart';
import 'package:uuid/uuid.dart';
import '../core/api.dart';
import 'widgets.dart';
import 'social_details.dart';

class PeopleScreen extends StatefulWidget {
  const PeopleScreen({super.key, required this.api});
  final PokerApi api;
  @override
  State<PeopleScreen> createState() => _PeopleScreenState();
}

class _PeopleScreenState extends State<PeopleScreen> {
  int revision = 0;
  Future<void> mutation(
    BuildContext context,
    String path, {
    String method = 'POST',
    Json body = const {},
  }) => safely(context, () async {
    await widget.api.request(
      '/v1.0/social$path',
      method: method,
      body: body,
      idempotencyKey: const Uuid().v4(),
    );
    if (mounted) setState(() => revision++);
  });
  @override
  Widget build(BuildContext context) => DefaultTabController(
    length: 6,
    child: Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(12),
          child: FilledButton.icon(
            onPressed: () => safely(context, () async {
              final code = await input(context, 'Código de amizade');
              if (code == null || !context.mounted) return;
              final player = await widget.api.get(
                '/v1.0/social/lookup/${segment(code)}',
              );
              if (!context.mounted ||
                  !await confirm(
                    context,
                    'Adicionar amigo',
                    'Enviar pedido para ${player['name'] ?? 'este jogador'}?',
                  )) {
                return;
              }
              if (context.mounted) {
                await mutation(
                  context,
                  '/friend-requests',
                  body: {'friend_code': code},
                );
              }
            }),
            icon: const PokerIcon(PokerIcons.userRoundPlus),
            label: const Text('Adicionar por código'),
          ),
        ),
        const TabBar(
          isScrollable: true,
          tabs: [
            Tab(text: 'Amigos'),
            Tab(text: 'Pedidos'),
            Tab(text: 'Recentes'),
            Tab(text: 'Bloqueados'),
            Tab(text: 'Caixa de entrada'),
            Tab(text: 'Pedidos enviados'),
          ],
        ),
        Expanded(
          child: TabBarView(
            key: ValueKey(revision),
            children: [
              for (final path in [
                '/friends',
                '/friend-requests',
                '/recent',
                '/blocked',
              ])
                PagedList(
                  api: widget.api,
                  path: '/v1.0/social$path',
                  query: path == '/friend-requests'
                      ? const {'direction': 'incoming'}
                      : const {},
                  item: (player, reload) => Card(
                    child: ListTile(
                      leading: const CircleAvatar(
                        child: PokerIcon(PokerIcons.userRound),
                      ),
                      onTap: () => Navigator.push(
                        context,
                        MaterialPageRoute<void>(
                          builder: (_) => PublicProfile(
                            api: widget.api,
                            playerId: player['player_id'],
                          ),
                        ),
                      ),
                      title: Text(player['name'] ?? 'Jogador'),
                      subtitle: Text(
                        player['presence'] == 'in_table'
                            ? 'Jogando'
                            : player['presence'] == 'online'
                            ? 'Online'
                            : 'Offline',
                      ),
                      trailing: PopupMenuButton<String>(
                        tooltip: 'Opções do jogador',
                        itemBuilder: (_) => [
                          if (path == '/friend-requests')
                            const PopupMenuItem(
                              value: 'accept',
                              child: Text('Aceitar amizade'),
                            ),
                          if (path == '/friend-requests')
                            const PopupMenuItem(
                              value: 'decline',
                              child: Text('Recusar pedido'),
                            ),
                          if (path == '/friends')
                            const PopupMenuItem(
                              value: 'remove',
                              child: Text('Remover amizade'),
                            ),
                          PopupMenuItem(
                            value: player['muted'] == true ? 'unmute' : 'mute',
                            child: Text(
                              player['muted'] == true
                                  ? 'Reativar mensagens'
                                  : 'Silenciar',
                            ),
                          ),
                          PopupMenuItem(
                            value: player['blocked'] == true
                                ? 'unblock'
                                : 'block',
                            child: Text(
                              player['blocked'] == true
                                  ? 'Desbloquear'
                                  : 'Bloquear',
                            ),
                          ),
                          const PopupMenuItem(
                            value: 'report',
                            child: Text('Denunciar'),
                          ),
                        ],
                        onSelected: (action) =>
                            actionFor(context, player, action),
                      ),
                    ),
                  ),
                ),
              SocialInbox(api: widget.api),
              PagedList(
                api: widget.api,
                path: '/v1.0/social/friend-requests',
                query: const {'direction': 'outgoing'},
                item: (player, reload) => Card(
                  child: ListTile(
                    title: Text(player['name'] ?? 'Jogador'),
                    subtitle: const Text('Aguardando resposta'),
                    trailing: TextButton(
                      onPressed: () => mutation(
                        context,
                        '/friend-requests/${segment(player['player_id'])}',
                        method: 'DELETE',
                      ),
                      child: const Text('Cancelar pedido'),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    ),
  );
  Future<void> actionFor(
    BuildContext context,
    Json player,
    String action,
  ) async {
    final id = segment(player['player_id']);
    if (action == 'report') {
      await Navigator.push(
        context,
        MaterialPageRoute<void>(
          builder: (_) => ReportPlayerScreen(
            api: widget.api,
            playerId: player['player_id'],
            surface: 'recent_player',
          ),
        ),
      );
      return;
    }
    if (['block', 'remove'].contains(action) &&
        !await confirm(
          context,
          action == 'block' ? 'Bloquear jogador' : 'Remover amizade',
          'Confirmar alteração para ${player['name'] ?? 'este jogador'}?',
        )) {
      return;
    }
    final operations = {
      'accept': ('/friend-requests/$id/accept', 'POST'),
      'decline': ('/friend-requests/$id/decline', 'POST'),
      'remove': ('/friends/$id', 'DELETE'),
      'mute': ('/mutes/$id', 'PUT'),
      'unmute': ('/mutes/$id', 'DELETE'),
      'block': ('/blocks/$id', 'PUT'),
      'unblock': ('/blocks/$id', 'DELETE'),
    };
    final op = operations[action];
    if (op != null && context.mounted) {
      await mutation(context, op.$1, method: op.$2);
    }
  }
}
