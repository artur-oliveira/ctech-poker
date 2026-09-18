import 'poker_icon.dart';
import '../core/labels.dart';
import 'package:flutter/material.dart';
import '../core/api.dart';
import 'widgets.dart';
import 'reactions.dart';
import 'social_details.dart';

class ShowcaseEditor extends StatefulWidget {
  const ShowcaseEditor({super.key, required this.api});
  final PokerApi api;
  @override
  State<ShowcaseEditor> createState() => _ShowcaseEditorState();
}

class _ShowcaseEditorState extends State<ShowcaseEditor> {
  final featured = <String>{}, favorites = <String>{}, hidden = <String>{};
  List<String> order = ['achievements', 'best_hand', 'matchup'];
  bool loaded = false, busy = false;
  String presets = 'mixed';
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Personalizar perfil')),
    body: SafeArea(
      child: AsyncPanel(
        load: () => Future.wait([
          widget.api.get('/v1.0/players/me'),
          widget.api.get(
            '/v1.0/players/me/achievements/summary',
            query: {'mode': 'sandbox'},
          ),
          widget.api.get('/v1.0/wallet/reaction-purchase/catalog'),
        ]),
        builder: (context, data, _) {
          if (!loaded) {
            final profile = data[0] as Json;
            featured.addAll(
              List<String>.from(profile['featured_achievements'] ?? []),
            );
            favorites.addAll(
              List<String>.from(profile['favorite_reactions'] ?? []),
            );
            final layout = profile['showcase_layout'] as Map?;
            order = {
              ...visibleShowcaseSections({'order': layout?['order']}),
            }.toList();
            hidden.addAll(
              (layout?['hidden'] is List ? layout!['hidden'] as List : const [])
                  .whereType<String>()
                  .where(
                    (section) =>
                        section != 'achievements' && order.contains(section),
                  ),
            );
            presets = profile['bet_preset_mode'] ?? 'mixed';
            loaded = true;
          }
          return ListView(
            padding: const EdgeInsets.all(20),
            children: [
              Text(
                'Conquistas em destaque',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              const Text('Escolha até três conquistas.'),
              for (final achievement in rows(
                data[1],
                'achievements',
              ).where((a) => a['unlocked'] == true))
                CheckboxListTile(
                  title: Text(achievementLabel(achievement['key'])),
                  value: featured.contains(achievement['key']),
                  onChanged: (value) => setState(() {
                    if (value == true && featured.length < 3) {
                      featured.add(achievement['key']);
                    } else if (value == false) {
                      featured.remove(achievement['key']);
                    }
                  }),
                ),
              const Divider(),
              Text(
                'Organização do perfil',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              for (var i = 0; i < order.length; i++)
                ListTile(
                  title: Text(
                    {
                          'achievements': 'Conquistas',
                          'best_hand': 'Melhor mão',
                          'matchup': 'Confronto',
                        }[order[i]] ??
                        order[i],
                  ),
                  leading: IconButton(
                    tooltip: 'Mover para cima',
                    onPressed: i == 0
                        ? null
                        : () => setState(() {
                            final value = order.removeAt(i);
                            order.insert(i - 1, value);
                          }),
                    icon: const PokerIcon(PokerIcons.arrowUp),
                  ),
                  trailing: order[i] == 'achievements'
                      ? null
                      : Switch(
                          value: !hidden.contains(order[i]),
                          onChanged: (value) => setState(() {
                            if (value) {
                              hidden.remove(order[i]);
                            } else {
                              hidden.add(order[i]);
                            }
                          }),
                        ),
                ),
              const Divider(),
              Text(
                'Reações favoritas',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              const Text('Escolha até três atalhos.'),
              Wrap(
                spacing: 8,
                children: [
                  for (final reaction in rows(
                    data[2],
                  ).where((r) => r['owned'] == true))
                    FilterChip(
                      label: Text(
                        reactionGlyphs[reaction['id']] ?? reaction['id'],
                      ),
                      selected: favorites.contains(reaction['id']),
                      onSelected: (value) => setState(() {
                        if (value && favorites.length < 3) {
                          favorites.add(reaction['id']);
                        } else if (!value) {
                          favorites.remove(reaction['id']);
                        }
                      }),
                    ),
                ],
              ),
              const SizedBox(height: 20),
              DropdownButtonFormField<String>(
                initialValue: presets,
                decoration: const InputDecoration(
                  labelText: 'Atalhos de aposta',
                ),
                items: [
                  for (final e in {
                    'mixed': 'Mistos',
                    'bb': 'Big blinds',
                    'pot': 'Frações do pote',
                  }.entries)
                    DropdownMenuItem(value: e.key, child: Text(e.value)),
                ],
                onChanged: (value) => presets = value ?? 'mixed',
              ),
              const SizedBox(height: 24),
              FilledButton(
                onPressed: busy
                    ? null
                    : () => safely(context, () async {
                        setState(() => busy = true);
                        try {
                          await widget.api.post('/v1.0/players/me', {
                            'featured_achievements': featured.toList(),
                            'favorite_reactions': favorites.toList(),
                            'bet_preset_mode': presets,
                            'showcase_layout': {
                              'order': order,
                              'hidden': hidden.toList(),
                            },
                          });
                          if (context.mounted) Navigator.pop(context);
                        } finally {
                          if (mounted) setState(() => busy = false);
                        }
                      }),
                child: const Text('Salvar perfil'),
              ),
            ],
          );
        },
      ),
    ),
  );
}
