import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../core/api.dart';
import 'widgets.dart';

class ShareHandScreen extends StatefulWidget {
  const ShareHandScreen({super.key, required this.api, required this.handId});
  final PokerApi api;
  final String handId;
  @override
  State<ShareHandScreen> createState() => _ShareHandScreenState();
}

class _ShareHandScreenState extends State<ShareHandScreen> {
  String kind = 'brag';
  int days = 7;
  bool includeCards = false, busy = false;
  String? token;
  Object? error;
  Future<void> create() async {
    if (busy) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      final result = await widget.api
          .post('/v1.0/players/me/hand/${segment(widget.handId)}/share', {
            'kind': kind,
            'include_hero_cards': includeCards,
            'expiry_days': days,
            'mode': 'sandbox',
          });
      if (result['token'] is! String || (result['token'] as String).isEmpty) {
        throw StateError(
          'Não foi possível confirmar o link. Confira seus links no perfil.',
        );
      }
      if (mounted) setState(() => token = result['token'] as String);
    } catch (failure) {
      if (mounted) setState(() => error = failure);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  Future<void> revoke() async {
    if (busy || token == null) return;
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await widget.api.request(
        '/v1.0/players/me/hand-shares/${segment(token!)}',
        method: 'DELETE',
      );
      if (mounted) {
        setState(() => token = null);
        toast(context, 'Link revogado.');
      }
    } catch (failure) {
      if (mounted) setState(() => error = failure);
    } finally {
      if (mounted) setState(() => busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final url = token == null
        ? null
        : 'https://poker.aoctech.app/share?token=${Uri.encodeComponent(token!)}';
    return PopScope(
      canPop: !busy,
      child: Scaffold(
        appBar: AppBar(title: const Text('Compartilhar mão')),
        body: SafeArea(
          child: ListView(
            padding: const EdgeInsets.all(24),
            children: [
              if (url == null) ...[
                Text(
                  'Como contar essa mão?',
                  style: Theme.of(context).textTheme.titleLarge,
                ),
                const SizedBox(height: 16),
                Wrap(
                  spacing: 8,
                  children: [
                    ChoiceChip(
                      label: const Text('Boa jogada'),
                      selected: kind == 'brag',
                      onSelected: busy
                          ? null
                          : (_) => setState(() => kind = 'brag'),
                    ),
                    ChoiceChip(
                      label: const Text('Bad beat'),
                      selected: kind == 'bad_beat',
                      onSelected: busy
                          ? null
                          : (_) => setState(() => kind = 'bad_beat'),
                    ),
                  ],
                ),
                SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('Mostrar minhas cartas'),
                  subtitle: const Text(
                    'Se ativado, suas cartas também ficarão disponíveis a quem tiver o link.',
                  ),
                  value: includeCards,
                  onChanged: busy
                      ? null
                      : (value) => setState(() => includeCards = value),
                ),
                const Text('Link disponível por'),
                Wrap(
                  spacing: 8,
                  children: [
                    for (final day in [1, 7, 30])
                      ChoiceChip(
                        label: Text(day == 1 ? '24 horas' : '$day dias'),
                        selected: days == day,
                        onSelected: busy
                            ? null
                            : (_) => setState(() => days = day),
                      ),
                  ],
                ),
                const SizedBox(height: 20),
                const Text(
                  'O link é público e expira automaticamente. Cartas não reveladas dos outros jogadores permanecem privadas. Você pode revogar o link aqui ou no perfil.',
                ),
                const SizedBox(height: 24),
                FilledButton(
                  onPressed: busy ? null : create,
                  child: Text(busy ? 'Criando…' : 'Criar link público'),
                ),
              ] else ...[
                const Icon(Icons.check_circle_outline, size: 64),
                const Text('Link criado', textAlign: TextAlign.center),
                const SizedBox(height: 16),
                SelectableText(url),
                FilledButton.icon(
                  onPressed: () => safely(context, () async {
                    await Clipboard.setData(ClipboardData(text: url));
                    if (context.mounted) toast(context, 'Link copiado.');
                  }),
                  icon: const Icon(Icons.copy),
                  label: const Text('Copiar link'),
                ),
                OutlinedButton(
                  onPressed: busy ? null : revoke,
                  child: Text(busy ? 'Revogando…' : 'Revogar link'),
                ),
              ],
              if (error != null)
                Padding(
                  padding: const EdgeInsets.only(top: 16),
                  child: Text(error.toString().replaceFirst('Bad state: ', '')),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
