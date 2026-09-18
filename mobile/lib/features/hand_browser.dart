import 'package:intl/intl.dart';
import '../core/design.dart';
import '../core/game_mode.dart';
import 'poker_icon.dart';
import 'package:flutter/material.dart';
import '../core/api.dart';
import 'collections.dart';
import 'library.dart';
import 'widgets.dart';

class HandBrowser extends StatefulWidget {
  const HandBrowser({super.key, required this.api, this.mode = GameMode.chips});
  final PokerApi api;
  final GameMode mode;
  @override
  State<HandBrowser> createState() => _HandBrowserState();
}

class _HandBrowserState extends State<HandBrowser> {
  String outcome = 'all', table = '';
  List<Json> saved = [];
  Object? savedError;
  bool filtersLoaded = false;
  bool saving = false;
  @override
  void initState() {
    super.initState();
    loadFilters();
  }

  Future<void> loadFilters() async {
    try {
      final data = await widget.api.get('/v1.0/players/me/hand-filters');
      if (mounted) {
        setState(() {
          saved = rows(data);
          savedError = null;
          filtersLoaded = true;
        });
      }
    } catch (error) {
      if (mounted) setState(() => savedError = error);
    }
  }

  @override
  Widget build(BuildContext context) => PagedList(
    key: ValueKey(widget.mode),
    api: widget.api,
    path: '/v1.0/players/me/hands',
    query: {'mode': widget.mode.apiValue},
    filter: (hand) =>
        (outcome == 'all' || hand['outcome'] == outcome) &&
        (table.isEmpty || hand['table_id'] == table),
    header: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Expanded(
              child: OutlinedButton.icon(
                onPressed: () => Navigator.push(
                  context,
                  MaterialPageRoute<void>(
                    builder: (_) =>
                        CollectionsScreen(api: widget.api, mode: widget.mode),
                  ),
                ),
                icon: const PokerIcon(PokerIcons.folder),
                label: const Text('Coleções'),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: OutlinedButton.icon(
                onPressed: () async {
                  final value = await input(
                    context,
                    'Filtrar pela mesa (vazio = todas)',
                    initial: table,
                  );
                  if (value != null && mounted) setState(() => table = value);
                },
                icon: const PokerIcon(PokerIcons.listFilter),
                label: Text(
                  table.isEmpty ? 'Todas as mesas' : 'Mesa selecionada',
                ),
              ),
            ),
          ],
        ),
        SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Row(
            children: [
              for (final e in {
                'all': 'Todas',
                'won': 'Vitórias',
                'lost': 'Derrotas',
                'tied': 'Empates',
              }.entries)
                Padding(
                  padding: const EdgeInsets.only(right: 6, top: 12, bottom: 8),
                  child: ChoiceChip(
                    label: Text(e.value),
                    showCheckmark: false,
                    selected: outcome == e.key,
                    onSelected: (_) => setState(() => outcome = e.key),
                  ),
                ),
            ],
          ),
        ),
        Wrap(
          spacing: 8,
          children: [
            for (final filter in saved)
              InputChip(
                label: Text(filter['name'] ?? 'Filtro'),
                onPressed: () => setState(() {
                  outcome = filter['outcome'] ?? 'all';
                  table = filter['table_id'] == 'all'
                      ? ''
                      : filter['table_id'] ?? '';
                }),
                onDeleted: saving
                    ? null
                    : () => safely(context, () async {
                        final next = saved.where((f) => f != filter).toList();
                        await save(next);
                      }),
              ),
            if (outcome != 'all' || table.isNotEmpty)
              ActionChip(
                label: const Text('Salvar filtro'),
                avatar: const PokerIcon(PokerIcons.bookmarkPlus),
                onPressed: !filtersLoaded || saving
                    ? null
                    : () => safely(context, () async {
                        final name = await input(context, 'Nome do filtro');
                        if (name == null || name.isEmpty) return;
                        await save([
                          ...saved,
                          {
                            'name': name,
                            'outcome': outcome,
                            'table_id': table.isEmpty ? 'all' : table,
                          },
                        ]);
                      }),
              ),
          ],
        ),
        if (outcome != 'all' || table.isNotEmpty)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 8),
            child: Text(
              'Filtro nas mãos carregadas',
              style: TextStyle(fontSize: 12, color: PokerColors.muted),
            ),
          ),
        if (savedError != null)
          TextButton(
            onPressed: loadFilters,
            child: const Text('Tentar carregar filtros salvos'),
          ),
      ],
    ),
    item: (hand, _) => Card(
      margin: const EdgeInsets.symmetric(vertical: 6),
      child: ListTile(
        contentPadding: const EdgeInsets.symmetric(
          horizontal: 14,
          vertical: 10,
        ),
        leading: PlayingCards(
          List<String>.from(hand['hole_cards'] ?? []),
          compact: true,
        ),
        title: Text(
          '${{'won': 'Vitória', 'lost': 'Derrota', 'tied': 'Empate'}[hand['outcome']] ?? 'Mão'} · ${widget.mode.amount(hand['net_change'] as num? ?? 0)}${widget.mode == GameMode.chips ? ' fichas' : ''}',
        ),
        subtitle: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              DateFormat('dd/MM/yyyy · HH:mm').format(
                DateTime.fromMillisecondsSinceEpoch(
                  hand['ended_at'] ?? 0,
                ).toLocal(),
              ),
              style: const TextStyle(fontSize: 12, color: PokerColors.muted),
            ),
          ],
        ),
        trailing: const PokerIcon(PokerIcons.chevronRight),
        onTap: () => Navigator.push(
          context,
          MaterialPageRoute<void>(
            builder: (_) =>
                HandScreen(api: widget.api, hand: hand, mode: widget.mode),
          ),
        ),
      ),
    ),
  );
  Future<void> save(List<Json> filters) async {
    if (!filtersLoaded || saving) return;
    setState(() => saving = true);
    try {
      await widget.api.request(
        '/v1.0/players/me/hand-filters',
        method: 'PUT',
        body: {'filters': filters},
      );
      if (mounted) {
        setState(() {
          saved = filters;
          savedError = null;
        });
      }
    } finally {
      if (mounted) setState(() => saving = false);
    }
  }
}
