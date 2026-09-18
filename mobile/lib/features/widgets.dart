import 'package:flutter_svg/flutter_svg.dart';
import 'poker_icon.dart';
import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import '../core/api.dart';
import '../core/preferences.dart';

String chips(dynamic value) =>
    NumberFormat.decimalPattern('pt_BR').format(value is num ? value : 0);
String money(dynamic cents) => NumberFormat.currency(
  locale: 'pt_BR',
  symbol: 'R\$',
).format((cents is num ? cents : 0) / 100);
void toast(BuildContext context, Object error) =>
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(error.toString().replaceFirst('Bad state: ', ''))),
    );
Future<void> safely(BuildContext context, Future<void> Function() task) async {
  try {
    await task();
  } catch (error) {
    if (context.mounted) toast(context, error);
  }
}

Future<String?> input(
  BuildContext context,
  String title, {
  String initial = '',
  bool numeric = false,
}) async {
  final controller = TextEditingController(text: initial);
  final value = await showDialog<String>(
    context: context,
    builder: (context) => AlertDialog(
      title: Text(title),
      content: TextField(
        controller: controller,
        autofocus: true,
        keyboardType: numeric ? TextInputType.number : TextInputType.text,
        onSubmitted: (value) => Navigator.pop(context, value),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Cancelar'),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context, controller.text.trim()),
          child: const Text('Continuar'),
        ),
      ],
    ),
  );
  // The dialog may still animate while the controller is being read.
  Future<void>.delayed(const Duration(seconds: 1), controller.dispose);
  return value;
}

Future<bool> confirm(
  BuildContext context,
  String title,
  String message,
) async =>
    await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(title),
        content: Text(message),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancelar'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('Confirmar'),
          ),
        ],
      ),
    ) ??
    false;

class AsyncPanel extends StatefulWidget {
  const AsyncPanel({super.key, required this.load, required this.builder});
  final Future<dynamic> Function() load;
  final Widget Function(BuildContext, dynamic, VoidCallback) builder;
  @override
  State<AsyncPanel> createState() => _AsyncPanelState();
}

class _AsyncPanelState extends State<AsyncPanel> {
  late Future<dynamic> future = widget.load();
  void reload() {
    if (mounted) {
      setState(() {
        future = widget.load();
      });
    }
  }

  @override
  Widget build(BuildContext context) => FutureBuilder<dynamic>(
    future: future,
    builder: (context, state) {
      if (state.connectionState != ConnectionState.done) {
        return const Center(child: CircularProgressIndicator());
      }
      if (state.hasError) {
        return Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const PokerIcon(PokerIcons.cloudOff, size: 48),
                const SizedBox(height: 16),
                Text(state.error.toString(), textAlign: TextAlign.center),
                const SizedBox(height: 12),
                FilledButton.icon(
                  onPressed: reload,
                  icon: const PokerIcon(PokerIcons.refreshCw),
                  label: const Text('Tentar novamente'),
                ),
              ],
            ),
          ),
        );
      }
      return RefreshIndicator(
        onRefresh: () async {
          reload();
          await future;
        },
        child: widget.builder(context, state.data, reload),
      );
    },
  );
}

class PagedList extends StatefulWidget {
  const PagedList({
    super.key,
    required this.api,
    required this.path,
    this.item,
    this.indexedItem,
    this.query = const {},
    this.header,
    this.filter,
  }) : assert(item != null || indexedItem != null);
  final PokerApi api;
  final String path;
  final Map<String, String> query;
  final Widget Function(Json, VoidCallback)? item;
  final Widget Function(Json, int, VoidCallback)? indexedItem;
  final Widget? header;
  final bool Function(Json)? filter;
  @override
  State<PagedList> createState() => _PagedListState();
}

class _PagedListState extends State<PagedList> {
  final items = <Json>[];
  String? cursor;
  bool busy = false, hasNext = true;
  Object? error;
  int generation = 0;
  @override
  void initState() {
    super.initState();
    fetch();
  }

  Future<void> fetch({bool reset = false}) async {
    if (busy && !reset) return;
    final revision = ++generation;
    setState(() {
      busy = true;
      error = null;
      if (reset) {
        items.clear();
        cursor = null;
        hasNext = true;
      }
    });
    try {
      final page = await widget.api.get(
        widget.path,
        query: {...widget.query, 'cursor': ?cursor},
      );
      if (!mounted || revision != generation) return;
      setState(() {
        items.addAll(rows(page));
        cursor = page['next_cursor'] as String?;
        hasNext =
            page['has_next'] == true && cursor != null && cursor!.isNotEmpty;
      });
    } catch (e) {
      if (mounted && revision == generation) setState(() => error = e);
    } finally {
      if (mounted && revision == generation) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => RefreshIndicator(
    onRefresh: () => fetch(reset: true),
    child: ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.all(16),
      children: [
        ?widget.header,
        for (var index = 0; index < items.length; index++)
          if (widget.filter?.call(items[index]) ?? true)
            widget.indexedItem?.call(
                  items[index],
                  index,
                  () => fetch(reset: true),
                ) ??
                widget.item!(items[index], () => fetch(reset: true)),
        if (!items.any((item) => widget.filter?.call(item) ?? true) &&
            !busy &&
            error == null)
          Padding(
            padding: const EdgeInsets.all(32),
            child: Text(
              items.isEmpty
                  ? 'Nada por aqui ainda.'
                  : 'Nenhum resultado nas páginas carregadas. Altere os filtros ou carregue mais.',
              textAlign: TextAlign.center,
            ),
          ),
        if (error != null)
          Padding(
            padding: const EdgeInsets.all(12),
            child: Text(error.toString()),
          ),
        if (busy)
          const Center(child: CircularProgressIndicator())
        else if (hasNext || error != null)
          TextButton(
            onPressed: fetch,
            child: Text(error == null ? 'Carregar mais' : 'Tentar novamente'),
          ),
      ],
    ),
  );
}

class PlayingCards extends StatelessWidget {
  const PlayingCards(
    this.cards, {
    super.key,
    this.compact = false,
    this.cardWidth,
  });
  final List<String> cards;
  final bool compact;
  final double? cardWidth;
  @override
  Widget build(BuildContext context) => Wrap(
    spacing: 4,
    runSpacing: 4,
    alignment: WrapAlignment.center,
    children: [
      for (final card in cards)
        PlayingCard(card, width: cardWidth ?? (compact ? 28 : 46)),
    ],
  );
}

class PlayingCard extends StatelessWidget {
  const PlayingCard(this.code, {super.key, this.width = 46});
  final String code;
  final double width;
  static String asset(String code, [String? variant]) {
    final normalized = code.trim();
    if (!RegExp(
      r'^[2-9TJQKA][cdhs]$',
      caseSensitive: false,
    ).hasMatch(normalized)) {
      return 'assets/cards/back.svg';
    }
    final rank = normalized[0].toUpperCase();
    final suit = {
      's': 'spade',
      'h': 'heart',
      'd': 'diamond',
      'c': 'club',
    }[normalized[1].toLowerCase()];
    final name =
        {'T': '10', 'J': 'jack', 'Q': 'queen', 'K': 'king', 'A': 'ace'}[rank] ??
        rank;
    final requested = variant ?? PokerAppearance.deck;
    final deck = PokerAppearance.decks.containsKey(requested)
        ? requested
        : 'four-color';
    return 'assets/cards/variants/$deck/$suit-$name.svg';
  }

  @override
  Widget build(BuildContext context) {
    final path = asset(code);
    final hidden = path == 'assets/cards/back.svg';
    final rank = hidden ? '' : code.trim()[0].toUpperCase();
    final suit = hidden
        ? ''
        : {'s': 'espadas', 'h': 'copas', 'd': 'ouros', 'c': 'paus'}[code
              .trim()[1]
              .toLowerCase()];
    return Semantics(
      label: hidden
          ? 'Carta oculta'
          : '${{'A': 'Ás', 'K': 'Rei', 'Q': 'Dama', 'J': 'Valete', 'T': '10'}[rank] ?? rank} de $suit',
      image: true,
      child: SizedBox(
        width: width,
        height: width * 512 / 370.76,
        child: ClipRRect(
          borderRadius: BorderRadius.circular(3),
          child: SvgPicture.asset(
            path,
            width: width,
            height: width * 512 / 370.76,
            fit: BoxFit.cover,
            excludeFromSemantics: true,
          ),
        ),
      ),
    );
  }
}
