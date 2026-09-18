import 'package:flutter/material.dart';
import '../core/api.dart';
import '../core/design.dart';
import '../core/game_mode.dart';
import 'poker_icon.dart';
import 'widgets.dart';

class StatisticsScreen extends StatelessWidget {
  const StatisticsScreen({
    super.key,
    required this.api,
    this.mode = GameMode.chips,
  });
  final PokerApi api;
  final GameMode mode;
  @override
  Widget build(BuildContext context) => AsyncPanel(
    key: ValueKey(mode),
    load: () =>
        api.get('/v1.0/players/me/poker-stats', query: {'mode': mode.apiValue}),
    builder: (context, stats, _) => ListView(
      padding: const EdgeInsets.all(20),
      children: [
        Text('Seu jogo', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 8),
        Text(
          'Amostra · ${chips(stats['hands'])} mãos',
          style: const TextStyle(color: PokerColors.muted),
        ),
        const SizedBox(height: 24),
        for (final metric in [
          (
            'vpip_rate',
            'VPIP',
            'Entrou voluntariamente',
            'vpip_hands',
            'hands',
            'Mãos em que você colocou fichas no pote pré-flop, sem contar os blinds.',
          ),
          (
            'pfr_rate',
            'PFR',
            'Aumentou pré-flop',
            'pfr_hands',
            'hands',
            'Mãos em que você fez pelo menos um raise pré-flop.',
          ),
          (
            'three_bet_rate',
            '3-bet',
            'Reaumentou',
            'three_bet_hands',
            'three_bet_chances',
            'Vezes em que você reaumentou diante de um raise, entre as oportunidades reais.',
          ),
        ])
          Padding(
            padding: const EdgeInsets.only(bottom: 24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Flexible(
                      child: Text(
                        metric.$2,
                        style: Theme.of(context).textTheme.titleLarge,
                      ),
                    ),
                    IconButton(
                      tooltip: 'Entender ${metric.$2}',
                      icon: const PokerIcon(PokerIcons.info, size: 18),
                      onPressed: () => showModalBottomSheet<void>(
                        context: context,
                        showDragHandle: true,
                        builder: (context) => SafeArea(
                          child: Padding(
                            padding: const EdgeInsets.fromLTRB(24, 0, 24, 32),
                            child: Column(
                              mainAxisSize: MainAxisSize.min,
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  metric.$2,
                                  style: Theme.of(context).textTheme.titleLarge,
                                ),
                                const SizedBox(height: 12),
                                Text(metric.$6),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ),
                    const Spacer(),
                    Flexible(
                      child: Text(
                        (stats[metric.$5] as num? ?? 0) == 0
                            ? 'Sem amostra'
                            : '${((stats[metric.$1] as num? ?? 0) * 100).toStringAsFixed(1).replaceAll('.', ',')}%',
                        textAlign: TextAlign.right,
                        style: const TextStyle(
                          fontFamily: PokerTheme.mono,
                          fontSize: 24,
                          color: PokerColors.gold,
                        ),
                      ),
                    ),
                  ],
                ),
                Text(
                  metric.$3,
                  style: const TextStyle(color: PokerColors.secondaryText),
                ),
                const SizedBox(height: 12),
                LinearProgressIndicator(
                  value: (stats[metric.$1] as num? ?? 0).toDouble().clamp(0, 1),
                  minHeight: 5,
                  borderRadius: BorderRadius.circular(3),
                  backgroundColor: PokerColors.control,
                  color: PokerColors.gold,
                ),
                const SizedBox(height: 8),
                Text(
                  '${chips(stats[metric.$4])} de ${chips(stats[metric.$5])} ${metric.$5 == 'hands' ? 'mãos' : 'oportunidades'}',
                  style: const TextStyle(
                    fontSize: 12,
                    color: PokerColors.muted,
                  ),
                ),
              ],
            ),
          ),
        if ((stats['hands'] as num? ?? 0) == 0)
          const Text('Suas tendências aparecem depois da primeira mão.'),
        if (rows(stats, 'playstyle').isNotEmpty) ...[
          const Divider(),
          Text(
            'Seu estilo pré-flop',
            style: Theme.of(context).textTheme.titleMedium,
          ),
          for (final badge in rows(stats, 'playstyle'))
            ListTile(
              contentPadding: EdgeInsets.zero,
              title: Text(
                badge['label'] ??
                    {
                      'selective': 'Seletivo',
                      'explorer': 'Explorador',
                      'initiative': 'Iniciativa',
                      'counter': 'Contra-ataque',
                      'balanced': 'Equilibrado',
                    }[badge['key']] ??
                    'Estilo de jogo',
              ),
              subtitle: badge['reason'] is String
                  ? Text(badge['reason'])
                  : null,
            ),
        ],
      ],
    ),
  );
}
