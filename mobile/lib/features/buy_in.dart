import 'poker_logo.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:uuid/uuid.dart';
import '../core/api.dart';
import 'widgets.dart';

/// Keeps an uncertain buy-in on screen with the same operation key for retries.
class BuyInScreen extends StatefulWidget {
  const BuyInScreen({
    super.key,
    required this.api,
    required this.path,
    required this.body,
    required this.minimum,
    required this.maximum,
    this.autoRebuy = false,
  });
  final PokerApi api;
  final String path;
  final Json body;
  final int minimum, maximum;
  final bool autoRebuy;
  @override
  State<BuyInScreen> createState() => _BuyInScreenState();
}

class _BuyInScreenState extends State<BuyInScreen> {
  late final amount = TextEditingController(text: '${widget.minimum}');
  late bool autoRebuy = widget.autoRebuy;
  String? operationKey;
  Json? submitted;
  Object? error;
  bool busy = false;
  @override
  void dispose() {
    amount.dispose();
    super.dispose();
  }

  Future<void> submit() async {
    if (busy) return;
    final value = int.tryParse(amount.text);
    if (value == null || value < widget.minimum || value > widget.maximum) {
      setState(
        () => error =
            'Escolha um valor entre ${chips(widget.minimum)} e ${chips(widget.maximum)} fichas.',
      );
      return;
    }
    operationKey ??= const Uuid().v4();
    submitted ??= {
      ...widget.body,
      'amount': value,
      'auto_rebuy': autoRebuy,
      'idem_key': operationKey,
    };
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final result = await widget.api.durablePost(
        widget.path,
        submitted!,
        label: 'Entrada ou recompra na mesa',
      );
      if (mounted) Navigator.pop(context, result);
    } catch (failure) {
      if (mounted) setState(() => error = failure);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: !busy,
    child: Scaffold(
      appBar: AppBar(title: const Text('Sua entrada na mesa')),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            const PokerLogo(size: 64),
            const SizedBox(height: 24),
            Text(
              'De ${chips(widget.minimum)} a ${chips(widget.maximum)} fichas',
              style: Theme.of(context).textTheme.titleLarge,
            ),
            const SizedBox(height: 16),
            TextField(
              controller: amount,
              enabled: submitted == null && !busy,
              keyboardType: TextInputType.number,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(
                labelText: 'Fichas para levar à mesa',
              ),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('Recompra automática'),
              subtitle: const Text(
                'Autoriza novas entradas com seu saldo quando suas fichas na mesa acabarem, conforme as regras da mesa.',
              ),
              value: autoRebuy,
              onChanged: submitted == null && !busy
                  ? (value) => setState(() => autoRebuy = value)
                  : null,
            ),
            const Text(
              'Ao confirmar, este valor será reservado do seu saldo. Fichas não têm saque nem conversão em dinheiro.',
            ),
            if (error != null)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 16),
                child: Text(error.toString().replaceFirst('Bad state: ', '')),
              ),
            if (submitted != null && !busy)
              const Padding(
                padding: EdgeInsets.only(top: 16),
                child: Text(
                  'A confirmação ainda não chegou. Tentar novamente consulta a mesma operação. Antes de iniciar outra entrada, confira suas mesas e seu saldo no lobby.',
                ),
              ),
            const SizedBox(height: 24),
            FilledButton(
              onPressed: busy ? null : submit,
              child: Text(
                busy
                    ? 'Confirmando…'
                    : submitted == null
                    ? 'Confirmar entrada'
                    : 'Tentar a mesma entrada',
              ),
            ),
          ],
        ),
      ),
    ),
  );
}
