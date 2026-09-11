import 'package:flutter/material.dart';
import 'package:uuid/uuid.dart';
import '../core/api.dart';

const reportCategories = {
  'harassment': 'Assédio ou ofensas',
  'hate': 'Discurso de ódio',
  'spam': 'Spam ou propaganda',
  'cheating': 'Trapaça ou conluio',
  'inappropriate_profile': 'Nome ou avatar impróprio',
  'other': 'Outro motivo',
};

class ReportPlayerScreen extends StatefulWidget {
  const ReportPlayerScreen({
    super.key,
    required this.api,
    required this.playerId,
    required this.surface,
    this.tableId,
    this.handId,
    this.actionId,
  });
  final PokerApi api;
  final String playerId, surface;
  final String? tableId, handId, actionId;
  @override
  State<ReportPlayerScreen> createState() => _ReportPlayerScreenState();
}

class _ReportPlayerScreenState extends State<ReportPlayerScreen> {
  String category = 'harassment';
  final details = TextEditingController();
  final key = const Uuid().v4();
  Json? submitted;
  bool busy = false, sent = false;
  Object? error;
  @override
  void dispose() {
    details.dispose();
    super.dispose();
  }

  Future<void> submit() async {
    if (busy || sent) return;
    submitted ??= {
      'target_player_id': widget.playerId,
      'surface': widget.surface,
      'category': category,
      'details': details.text.trim(),
      'table_id': ?widget.tableId,
      'hand_id': ?widget.handId,
      'action_id': ?widget.actionId,
    };
    setState(() {
      busy = true;
      error = null;
    });
    try {
      await widget.api.request(
        '/v1.0/social/reports',
        method: 'POST',
        body: submitted,
        idempotencyKey: key,
      );
      if (mounted) setState(() => sent = true);
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
      appBar: AppBar(title: const Text('Denunciar jogador')),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            if (sent) ...[
              const Icon(Icons.check_circle_outline, size: 64),
              const Text('Denúncia registrada para revisão.'),
              FilledButton(
                onPressed: () => Navigator.pop(context),
                child: const Text('Fechar'),
              ),
            ] else ...[
              const Text(
                'A denúncia não avisa o jogador. Silenciar ou bloquear permite deixar de ver o conteúdo enquanto o relato é revisado.',
              ),
              const SizedBox(height: 16),
              Wrap(
                spacing: 8,
                children: [
                  for (final entry in reportCategories.entries)
                    ChoiceChip(
                      label: Text(entry.value),
                      selected: category == entry.key,
                      onSelected: submitted != null || busy
                          ? null
                          : (_) => setState(() => category = entry.key),
                    ),
                ],
              ),
              const SizedBox(height: 16),
              TextField(
                controller: details,
                enabled: submitted == null && !busy,
                maxLength: 500,
                maxLines: 4,
                decoration: const InputDecoration(
                  labelText: 'Detalhes (opcional)',
                ),
              ),
              if (error != null) Text(error.toString()),
              FilledButton(
                onPressed: busy ? null : submit,
                child: Text(
                  busy
                      ? 'Enviando…'
                      : submitted == null
                      ? 'Enviar denúncia'
                      : 'Tentar o mesmo envio',
                ),
              ),
            ],
          ],
        ),
      ),
    ),
  );
}
