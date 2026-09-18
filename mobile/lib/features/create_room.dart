import 'package:flutter/material.dart';
import '../core/api.dart';
import 'widgets.dart';

class CreateRoomScreen extends StatefulWidget {
  const CreateRoomScreen({super.key, required this.api});
  final PokerApi api;
  @override
  State<CreateRoomScreen> createState() => _CreateRoomScreenState();
}

class _CreateRoomScreenState extends State<CreateRoomScreen> {
  final form = GlobalKey<FormState>();
  final fields = {
    'small_blind': TextEditingController(text: '50'),
    'big_blind': TextEditingController(text: '100'),
    'buy_in_min': TextEditingController(text: '2000'),
    'buy_in_max': TextEditingController(text: '20000'),
    'interval_minutes': TextEditingController(text: '15'),
    'multiplier': TextEditingController(text: '200'),
    'max': TextEditingController(text: '10000'),
  };
  int seats = 6, timeout = 30;
  bool equity = true, twice = true, escalation = false, busy = false;
  @override
  void dispose() {
    for (final field in fields.values) {
      field.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Criar mesa privada')),
    body: SafeArea(
      child: Form(
        key: form,
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            const Text('Escolha os blinds e as regras da sua mesa.'),
            const SizedBox(height: 20),
            for (final e in {
              'small_blind': 'Small blind',
              'big_blind': 'Big blind',
              'buy_in_min': 'Entrada mínima',
              'buy_in_max': 'Entrada máxima',
            }.entries)
              field(e.key, e.value),
            DropdownButtonFormField<int>(
              initialValue: seats,
              decoration: const InputDecoration(labelText: 'Lugares'),
              items: [
                for (final value in [2, 6, 9])
                  DropdownMenuItem(
                    value: value,
                    child: Text('$value jogadores'),
                  ),
              ],
              onChanged: (value) => setState(() => seats = value!),
            ),
            const SizedBox(height: 16),
            Text('Tempo de decisão: $timeout segundos'),
            Slider(
              value: timeout.toDouble(),
              min: 5,
              max: 60,
              divisions: 11,
              onChanged: (value) => setState(() => timeout = value.round()),
            ),
            SwitchListTile(
              title: const Text('Exibir equidade'),
              value: equity,
              onChanged: (value) => setState(() => equity = value),
            ),
            SwitchListTile(
              title: const Text('Permitir duas viradas'),
              value: twice,
              onChanged: (value) => setState(() => twice = value),
            ),
            SwitchListTile(
              title: const Text('Aumentar blinds periodicamente'),
              value: escalation,
              onChanged: (value) => setState(() => escalation = value),
            ),
            if (escalation) ...[
              field('interval_minutes', 'Intervalo em minutos'),
              field('multiplier', 'Multiplicador percentual (200 = dobrar)'),
              field('max', 'Big blind máximo'),
            ],
            FilledButton(
              onPressed: busy
                  ? null
                  : () => safely(context, () async {
                      if (!form.currentState!.validate()) return;
                      final values = fields.map(
                        (key, value) => MapEntry(key, int.parse(value.text)),
                      );
                      if (values['big_blind']! <= values['small_blind']!) {
                        throw StateError(
                          'Big blind precisa ser maior que o small blind.',
                        );
                      }
                      if (values['buy_in_max']! < values['buy_in_min']! ||
                          values['buy_in_min']! % values['big_blind']! != 0 ||
                          values['buy_in_max']! % values['big_blind']! != 0) {
                        throw StateError(
                          'Entradas devem ser múltiplos do big blind, com máximo maior ou igual ao mínimo.',
                        );
                      }
                      if (escalation &&
                          (values['multiplier']! <= 100 ||
                              values['max']! < values['big_blind']!)) {
                        throw StateError(
                          'Revise o multiplicador e o limite dos blinds.',
                        );
                      }
                      setState(() => busy = true);
                      try {
                        final result = await widget.api.post('/v1.0/rooms', {
                          'visibility': 'private',
                          'currency_mode': 'sandbox',
                          for (final key in [
                            'small_blind',
                            'big_blind',
                            'buy_in_min',
                            'buy_in_max',
                          ])
                            key: values[key],
                          'max_seats': seats,
                          'turn_timeout_seconds': timeout,
                          'equity_display_enabled': equity,
                          'run_it_twice_enabled': twice,
                          if (escalation)
                            'blind_escalation': {
                              for (final key in [
                                'interval_minutes',
                                'multiplier',
                                'max',
                              ])
                                key: values[key],
                            },
                        });
                        if (context.mounted) Navigator.pop(context, result);
                      } finally {
                        if (mounted) setState(() => busy = false);
                      }
                    }),
              child: Text(busy ? 'Criando…' : 'Criar mesa'),
            ),
          ],
        ),
      ),
    ),
  );
  Widget field(String key, String label) => Padding(
    padding: const EdgeInsets.only(bottom: 16),
    child: TextFormField(
      controller: fields[key],
      keyboardType: TextInputType.number,
      decoration: InputDecoration(labelText: label),
      validator: (value) => (int.tryParse(value ?? '') ?? 0) <= 0
          ? 'Informe um inteiro positivo.'
          : null,
    ),
  );
}
