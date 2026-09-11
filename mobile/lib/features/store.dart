import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:uuid/uuid.dart';
import '../core/api.dart';
import 'widgets.dart';

const purchasePaths = {
  'Fichas': '/v1.0/wallet/sandbox-purchase',
  'Reações': '/v1.0/wallet/reaction-purchase',
  'Baralhos': '/v1.0/wallet/cosmetic-purchase/deck',
  'Mesas': '/v1.0/wallet/cosmetic-purchase/felt',
};

class StoreScreen extends StatelessWidget {
  const StoreScreen({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => DefaultTabController(
    length: 4,
    child: Column(
      children: [
        TabBar(
          isScrollable: true,
          tabs: [for (final title in purchasePaths.keys) Tab(text: title)],
        ),
        Expanded(
          child: TabBarView(
            children: [
              for (final entry in purchasePaths.entries)
                catalog(context, entry.key, entry.value),
            ],
          ),
        ),
      ],
    ),
  );
  Widget catalog(BuildContext context, String title, String path) => AsyncPanel(
    load: () => api.get('$path/${title == 'Fichas' ? 'skus' : 'catalog'}'),
    builder: (context, result, reload) => ListView(
      padding: const EdgeInsets.all(16),
      children: [
        OutlinedButton.icon(
          onPressed: () => Navigator.push(
            context,
            MaterialPageRoute<void>(
              builder: (_) => Scaffold(
                appBar: AppBar(title: Text('Compras · $title')),
                body: PagedList(
                  api: api,
                  path: '$path/',
                  item: (purchase, refresh) => Card(
                    child: ListTile(
                      title: Text(
                        purchase['sku'] ??
                            purchase['reaction_id'] ??
                            purchase['item_id'] ??
                            'Compra',
                      ),
                      subtitle: Text(purchase['status'] ?? ''),
                      onTap: () => Navigator.push(
                        context,
                        MaterialPageRoute<void>(
                          builder: (_) => PurchaseScreen(
                            api: api,
                            path: path,
                            initial: purchase,
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
          icon: const Icon(Icons.receipt_long),
          label: const Text('Histórico de compras'),
        ),
        if (rows(result).isEmpty)
          const Padding(
            padding: EdgeInsets.all(24),
            child: Text('Nenhum item disponível agora.'),
          ),
        for (final item in rows(result))
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Icon(
                    title == 'Fichas' ? Icons.paid_outlined : Icons.style,
                    size: 40,
                  ),
                  const SizedBox(height: 12),
                  Text(
                    title == 'Fichas'
                        ? '${chips(item['total_credits'])} fichas'
                        : item['id'].toString().replaceAll('_', ' '),
                    style: Theme.of(context).textTheme.titleLarge,
                  ),
                  const SizedBox(height: 12),
                  if (item['owned'] == true)
                    Wrap(
                      children: [
                        const Chip(label: Text('Disponível na sua conta')),
                        if (title == 'Baralhos' || title == 'Mesas')
                          TextButton(
                            onPressed: () => safely(context, () async {
                              await api.post('/v1.0/players/me', {
                                title == 'Baralhos'
                                        ? 'deck_variant'
                                        : 'table_theme':
                                    item['id'],
                              });
                              if (context.mounted) {
                                toast(context, 'Aparência atualizada.');
                              }
                            }),
                            child: const Text('Usar'),
                          ),
                      ],
                    )
                  else
                    Wrap(
                      spacing: 8,
                      children: [
                        if ((item['price_cents'] as num? ?? 0) > 0)
                          FilledButton(
                            onPressed: () =>
                                buy(context, path, title, item, 'pix', reload),
                            child: Text('${money(item['price_cents'])} · PIX'),
                          ),
                        if ((item['price_fichas'] as num? ?? 0) > 0)
                          OutlinedButton(
                            onPressed: () => buy(
                              context,
                              path,
                              title,
                              item,
                              'fichas',
                              reload,
                            ),
                            child: Text(
                              '${chips(item['price_fichas'])} fichas',
                            ),
                          ),
                      ],
                    ),
                ],
              ),
            ),
          ),
      ],
    ),
  );
  Future<void> buy(
    BuildContext context,
    String path,
    String title,
    Json item,
    String method,
    VoidCallback reload,
  ) => safely(context, () async {
    final price = method == 'pix'
        ? money(item['price_cents'])
        : '${chips(item['price_fichas'])} fichas';
    if (!await confirm(
      context,
      'Confirmar compra',
      'Comprar ${item['id']} por $price?',
    )) {
      return;
    }
    final body = {
      'idem_key': const Uuid().v4(),
      if (title == 'Fichas')
        'sku': item['id']
      else if (title == 'Reações')
        'reaction_id': item['id']
      else
        'item_id': item['id'],
      if (title != 'Fichas') 'method': method,
    };
    if (!context.mounted) return;
    await Navigator.push(
      context,
      MaterialPageRoute<void>(
        builder: (_) => Scaffold(
          appBar: AppBar(title: const Text('Confirmando compra')),
          body: AsyncPanel(
            load: () =>
                api.durablePost('$path/', body, label: 'Compra de $title'),
            builder: (context, result, _) => PurchaseScreen(
              api: api,
              path: path,
              initial: result,
              embedded: true,
            ),
          ),
        ),
      ),
    );
    reload();
  });
}

class PurchaseScreen extends StatefulWidget {
  const PurchaseScreen({
    super.key,
    required this.api,
    required this.path,
    required this.initial,
    this.embedded = false,
  });
  final PokerApi api;
  final String path;
  final Json initial;
  final bool embedded;
  @override
  State<PurchaseScreen> createState() => _PurchaseScreenState();
}

class _PurchaseScreenState extends State<PurchaseScreen>
    with WidgetsBindingObserver {
  late Json purchase = widget.initial;
  Timer? timer;
  bool busy = false, active = true;
  Object? error;
  String? refundKey;
  bool get pending =>
      ['pending', 'processing', 'refunding'].contains(purchase['status']);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    schedule();
  }

  void schedule() {
    timer?.cancel();
    if (pending && active) timer = Timer(const Duration(seconds: 4), refresh);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    active = state == AppLifecycleState.resumed;
    if (active) {
      refresh();
    } else {
      timer?.cancel();
    }
  }

  Future<void> refresh() async {
    if (busy) return;
    busy = true;
    try {
      final next = await widget.api.get(
        '${widget.path}/${segment(purchase['purchase_id'])}',
      );
      if (mounted) {
        setState(() {
          purchase = next;
          error = null;
        });
      }
    } catch (e) {
      if (mounted) setState(() => error = e);
    } finally {
      busy = false;
      if (mounted) schedule();
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final content = SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(24),
        children: [
          Icon(
            purchase['status'] == 'confirmed'
                ? Icons.check_circle
                : Icons.receipt_long,
            size: 72,
          ),
          const SizedBox(height: 20),
          Text(
            {
                  'pending': 'Aguardando pagamento',
                  'processing': 'Processando compra',
                  'confirmed': 'Compra confirmada',
                  'refunded': 'Compra reembolsada',
                  'refunding': 'Processando reembolso',
                  'failed': 'Compra não concluída',
                  'expired': 'Pagamento expirado',
                }[purchase['status']] ??
                'Atualizando compra',
            style: Theme.of(context).textTheme.headlineMedium,
          ),
          if (purchase['price_cents'] != null)
            Text(money(purchase['price_cents'])),
          if (purchase['expires_at'] != null && pending)
            Text('Válido até ${purchase['expires_at']}'),
          if (purchase['qr_code_base64'] is String && pending)
            Builder(
              builder: (context) {
                try {
                  return Padding(
                    padding: const EdgeInsets.all(16),
                    child: Image.memory(
                      base64Decode(
                        (purchase['qr_code_base64'] as String).split(',').last,
                      ),
                      height: 220,
                      semanticLabel: 'QR Code PIX',
                      errorBuilder: (_, _, _) =>
                          const Text('Use o código PIX abaixo.'),
                    ),
                  );
                } on FormatException {
                  return const Text('Use o código PIX abaixo.');
                }
              },
            ),
          if (purchase['pix_copia_e_cola'] != null && pending)
            FilledButton.icon(
              onPressed: () async {
                await Clipboard.setData(
                  ClipboardData(text: purchase['pix_copia_e_cola']),
                );
                if (context.mounted) toast(context, 'Código PIX copiado.');
              },
              icon: const Icon(Icons.copy),
              label: const Text('Copiar código PIX'),
            ),
          if (error != null)
            const Text(
              'Não foi possível atualizar. Consulte o status antes de comprar novamente.',
            ),
          TextButton(onPressed: refresh, child: const Text('Atualizar status')),
          if (purchase['status'] == 'confirmed')
            OutlinedButton(
              onPressed: busy
                  ? null
                  : () => safely(context, () async {
                      if (!await confirm(
                        context,
                        'Solicitar reembolso',
                        'O servidor verificará a disponibilidade do reembolso.',
                      )) {
                        return;
                      }
                      refundKey ??= const Uuid().v4();
                      setState(() => busy = true);
                      try {
                        final next = await widget.api.durablePost(
                          '${widget.path}/${segment(purchase['purchase_id'])}/refund',
                          {'idem_key': refundKey},
                          label: 'Reembolso de compra',
                        );
                        if (mounted) setState(() => purchase = next);
                      } finally {
                        if (mounted) {
                          setState(() => busy = false);
                          schedule();
                        }
                      }
                    }),
              child: const Text('Solicitar reembolso'),
            ),
        ],
      ),
    );
    return widget.embedded
        ? content
        : Scaffold(
            appBar: AppBar(title: const Text('Detalhes da compra')),
            body: content,
          );
  }
}
