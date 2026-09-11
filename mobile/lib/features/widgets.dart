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
    if (mounted) setState(() => future = widget.load());
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
                const Icon(Icons.cloud_off_outlined, size: 48),
                const SizedBox(height: 16),
                Text(state.error.toString(), textAlign: TextAlign.center),
                const SizedBox(height: 12),
                FilledButton.icon(
                  onPressed: reload,
                  icon: const Icon(Icons.refresh),
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
    required this.item,
    this.query = const {},
    this.header,
    this.filter,
  });
  final PokerApi api;
  final String path;
  final Map<String, String> query;
  final Widget Function(Json, VoidCallback) item;
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
        for (final row in items.where(widget.filter ?? (_) => true))
          widget.item(row, () => fetch(reset: true)),
        if (items.isEmpty && !busy && error == null)
          const Padding(
            padding: EdgeInsets.all(32),
            child: Text('Nada por aqui ainda.', textAlign: TextAlign.center),
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
  const PlayingCards(this.cards, {super.key, this.compact = false});
  final List<String> cards;
  final bool compact;
  @override
  Widget build(BuildContext context) => Wrap(
    spacing: 4,
    runSpacing: 4,
    alignment: WrapAlignment.center,
    children: cards.map((code) {
      final hidden = code == 'back' || code.length != 2;
      final suit = hidden
          ? ''
          : {'c': '♣', 'd': '♦', 'h': '♥', 's': '♠'}[code[1]] ?? '';
      return Semantics(
        label: hidden ? 'Carta oculta' : '${code[0]} $suit',
        child: Container(
          width: compact ? 32 : 44,
          height: compact ? 43 : 60,
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: hidden ? const Color(0xff294f60) : const Color(0xfff7f1df),
            borderRadius: BorderRadius.circular(7),
            border: Border.all(color: const Color(0xffb3a887)),
          ),
          child: ExcludeSemantics(
            child: hidden
                ? CustomPaint(
                    size: Size(compact ? 18 : 26, compact ? 28 : 40),
                    painter: SuitPainter('back', Colors.white54),
                  )
                : Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Text(
                        code[0] == 'T' ? '10' : code[0],
                        textScaler: TextScaler.noScaling,
                        style: TextStyle(
                          fontWeight: FontWeight.w800,
                          fontSize: compact ? 16 : 22,
                          color: PokerAppearance.suit(code[1]),
                        ),
                      ),
                      CustomPaint(
                        size: Size(compact ? 12 : 18, compact ? 12 : 18),
                        painter: SuitPainter(
                          code[1],
                          PokerAppearance.suit(code[1]),
                        ),
                      ),
                    ],
                  ),
          ),
        ),
      );
    }).toList(),
  );
}

/// Native vector suits keep card identity independent of emoji/font fallback.
class SuitPainter extends CustomPainter {
  const SuitPainter(this.suit, this.color);
  final String suit;
  final Color color;
  @override
  void paint(Canvas canvas, Size size) {
    canvas.save();
    canvas.scale(size.width / 100, size.height / 100);
    final paint = Paint()..color = color;
    final path = Path();
    switch (suit) {
      case 'd':
        path.moveTo(50, 0);
        path.lineTo(95, 50);
        path.lineTo(50, 100);
        path.lineTo(5, 50);
        path.close();
      case 'h':
        path.moveTo(50, 95);
        path.cubicTo(-30, 35, 0, -20, 50, 22);
        path.cubicTo(100, -20, 130, 35, 50, 95);
        path.close();
      case 's':
        path.moveTo(50, 0);
        path.cubicTo(-30, 60, 0, 100, 44, 66);
        path.lineTo(32, 100);
        path.lineTo(68, 100);
        path.lineTo(56, 66);
        path.cubicTo(100, 100, 130, 60, 50, 0);
        path.close();
      case 'c':
        canvas.drawCircle(const Offset(50, 27), 26, paint);
        canvas.drawCircle(const Offset(26, 59), 25, paint);
        canvas.drawCircle(const Offset(74, 59), 25, paint);
        path.moveTo(45, 50);
        path.lineTo(32, 100);
        path.lineTo(68, 100);
        path.lineTo(55, 50);
        path.close();
      default:
        paint.style = PaintingStyle.stroke;
        paint.strokeWidth = 3;
        canvas.drawRRect(
          RRect.fromRectAndRadius(
            const Rect.fromLTWH(0, 0, 100, 100),
            const Radius.circular(10),
          ),
          paint,
        );
        for (var y = 15; y < 100; y += 25) {
          for (var x = 15; x < 100; x += 25) {
            path.moveTo(x.toDouble(), y - 8);
            path.lineTo(x + 8, y.toDouble());
            path.lineTo(x.toDouble(), y + 8);
            path.lineTo(x - 8, y.toDouble());
            path.close();
          }
        }
    }
    canvas.drawPath(path, paint);
    canvas.restore();
  }

  @override
  bool shouldRepaint(SuitPainter oldDelegate) =>
      oldDelegate.suit != suit || oldDelegate.color != color;
}
