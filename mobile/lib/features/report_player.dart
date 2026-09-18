import 'poker_icon.dart';
import 'package:flutter/material.dart';
import 'package:uuid/uuid.dart';

import '../core/api.dart';
import 'widgets.dart';

/// Resolve the event against authoritative history before opening a report.
/// Socket events carry no hand ID and can survive a hand transition.
class TableEventReportButton extends StatefulWidget {
  const TableEventReportButton({
    super.key,
    required this.api,
    required this.tableId,
    required this.handId,
    required this.playerId,
    required this.actionId,
    required this.reaction,
  });
  final PokerApi api;
  final String tableId, handId, playerId, actionId;
  final bool reaction;

  @override
  State<TableEventReportButton> createState() => _TableEventReportButtonState();
}

class _TableEventReportButtonState extends State<TableEventReportButton> {
  bool busy = false;

  Future<void> open() async {
    if (busy) return;
    // Capture all evidence coordinates before the request: a new snapshot may
    // arrive while history is loading.
    final event = widget;
    setState(() => busy = true);
    await safely(context, () async {
      final history = await event.api.get(
        '/v1.0/tables/${segment(event.tableId)}/hands/${segment(event.handId)}/history',
      );
      if (!rows(history, 'actions').any(
        (action) =>
            action['action_id'] == event.actionId &&
            action['player_id'] == event.playerId &&
            action['action'] == (event.reaction ? 'reaction' : 'chat'),
      )) {
        throw StateError(
          'Não foi possível vincular este evento à mão atual. Use a denúncia de comportamento no assento do jogador.',
        );
      }
      if (!mounted) return;
      await Navigator.push(
        context,
        MaterialPageRoute<void>(
          builder: (_) => ReportPlayerScreen(
            api: event.api,
            playerId: event.playerId,
            surface: event.reaction ? 'table_reaction' : 'table_chat',
            tableId: event.tableId,
            handId: event.handId,
            actionId: event.actionId,
          ),
        ),
      );
    });
    if (mounted) setState(() => busy = false);
  }

  @override
  Widget build(BuildContext context) => IconButton(
    tooltip: busy
        ? 'Verificando evento…'
        : widget.reaction
        ? 'Denunciar reação'
        : 'Denunciar mensagem',
    onPressed: busy || widget.handId.isEmpty || widget.actionId.isEmpty
        ? null
        : open,
    icon: PokerIcon(busy ? PokerIcons.hourglass : PokerIcons.flag),
  );
}

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
              const PokerIcon(PokerIcons.circleCheck, size: 64),
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
