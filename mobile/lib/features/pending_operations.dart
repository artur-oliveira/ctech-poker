import 'package:flutter/material.dart';
import '../core/api.dart';
import 'home.dart';
import 'store.dart';
import 'widgets.dart';

class PendingOperationsScreen extends StatelessWidget {
  const PendingOperationsScreen({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Operações pendentes')),
    body: SafeArea(
      child: AsyncPanel(
        load: api.pendingOperation,
        builder: (context, value, reload) {
          if (value == null) {
            return ListView(
              padding: const EdgeInsets.all(24),
              children: const [
                Icon(Icons.check_circle_outline, size: 64),
                Text(
                  'Nenhuma operação aguardando conferência.',
                  textAlign: TextAlign.center,
                ),
              ],
            );
          }
          final pending = value as Json;
          final path = pending['path'] as String;
          final isTable = path.startsWith('/v1.0/rooms/');
          final purchasePath = purchasePaths.values
              .where((prefix) => path.startsWith('$prefix/'))
              .firstOrNull;
          return ListView(
            padding: const EdgeInsets.all(24),
            children: [
              Text(
                pending['label'] as String,
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              Text(
                DateTime.fromMillisecondsSinceEpoch(
                  pending['created_at'] as int,
                ).toLocal().toString(),
              ),
              if (pending['amount'] != null)
                Text('${chips(pending['amount'])} fichas'),
              if (pending['product'] != null)
                Text('Item: ${pending['product']}'),
              if (pending['auto_rebuy'] == true)
                const Text('Recompra automática autorizada nesta entrada.'),
              const SizedBox(height: 20),
              const Text(
                'O app não recebeu a confirmação completa. O pedido pode ter sido concluído. Confira os registros abaixo antes de iniciar outra operação. Reabrir esta tela não envia o pedido novamente.',
              ),
              const SizedBox(height: 16),
              FilledButton(
                onPressed: () => Navigator.push(
                  context,
                  MaterialPageRoute<void>(
                    builder: (_) => Scaffold(
                      appBar: AppBar(
                        title: Text(
                          isTable ? 'Minhas sessões' : 'Minhas compras',
                        ),
                      ),
                      body: PagedList(
                        api: api,
                        path: isTable
                            ? '/v1.0/players/me/sessions'
                            : '${purchasePath!}/',
                        item: (record, _) => ListTile(
                          title: Text(
                            isTable
                                ? 'Mesa ${record['table_id']}'
                                : '${record['sku'] ?? record['reaction_id'] ?? record['item_id'] ?? 'Compra'}',
                          ),
                          subtitle: Text(
                            isTable
                                ? 'Entrada: ${chips(record['buyin_amount'])} · Resultado: ${chips(record['net_pnl'])}'
                                : '${record['status']}',
                          ),
                          onTap: isTable
                              ? (record['ended_at'] == 0
                                    ? () => safely(context, () async {
                                        final profile = await api.get(
                                          '/v1.0/players/me',
                                        );
                                        if (context.mounted) {
                                          LobbyScreen(api: api).open(
                                            context,
                                            record['table_id'],
                                            profile['user_id'],
                                          );
                                        }
                                      })
                                    : null)
                              : () => Navigator.push(
                                  context,
                                  MaterialPageRoute<void>(
                                    builder: (_) => PurchaseScreen(
                                      api: api,
                                      path: purchasePath!,
                                      initial: record,
                                    ),
                                  ),
                                ),
                        ),
                      ),
                    ),
                  ),
                ),
                child: Text(
                  isTable
                      ? 'Conferir minhas sessões'
                      : 'Conferir minhas compras',
                ),
              ),
              const SizedBox(height: 16),
              OutlinedButton(
                onPressed: () => safely(context, () async {
                  if (!await confirm(
                    context,
                    'Conferência concluída',
                    'Você conferiu suas sessões/compras e o saldo? Remover este aviso não cancela, reembolsa nem repete a operação. Se ainda houver dúvida, mantenha a pendência.',
                  )) {
                    return;
                  }
                  await api.acknowledgeOperation(pending['id'] as String);
                  reload();
                }),
                child: const Text('Conferi o resultado, remover aviso'),
              ),
            ],
          );
        },
      ),
    ),
  );
}
