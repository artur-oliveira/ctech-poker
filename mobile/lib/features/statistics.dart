import 'package:flutter/material.dart';
import '../core/api.dart';
import 'widgets.dart';

class StatisticsScreen extends StatelessWidget {
  const StatisticsScreen({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => AsyncPanel(
    load: () =>
        api.get('/v1.0/players/me/poker-stats', query: {'mode': 'sandbox'}),
    builder: (context, stats, _) => ListView(
      padding: const EdgeInsets.all(20),
      children: [
        Text(
          '${chips(stats['hands'])} mãos analisadas',
          style: Theme.of(context).textTheme.headlineSmall,
        ),
        const Text(
          'Seu estilo em fichas recreativas. Amostras pequenas variam bastante; estes números descrevem o histórico, não garantem resultados.',
        ),
        for (final metric in [
          (
            'vpip_rate',
            'Participação voluntária (VPIP)',
            'vpip_hands',
            'hands',
            'Mãos em que você colocou fichas voluntariamente antes do flop; blinds obrigatórios não contam.',
          ),
          (
            'pfr_rate',
            'Aumento pré-flop (PFR)',
            'pfr_hands',
            'hands',
            'Mãos em que você aumentou a aposta antes do flop.',
          ),
          (
            'three_bet_rate',
            'Contra-aumento (3-bet)',
            'three_bet_hands',
            'three_bet_chances',
            'Frequência de contra-aumento nas oportunidades em que você enfrentou um aumento pré-flop.',
          ),
        ])
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    metric.$2,
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  Text(
                    (stats[metric.$4] as num? ?? 0) == 0
                        ? 'Sem amostra'
                        : '${((stats[metric.$1] as num? ?? 0) * 100).toStringAsFixed(1)}%',
                    style: Theme.of(context).textTheme.headlineSmall,
                  ),
                  Text(
                    '${chips(stats[metric.$3])} de ${chips(stats[metric.$4])} ${metric.$4 == 'hands' ? 'mãos' : 'oportunidades'}',
                  ),
                  Text(metric.$5),
                ],
              ),
            ),
          ),
        if (rows(stats, 'playstyle').isNotEmpty) ...[
          Text(
            'Seu estilo nesta amostra',
            style: Theme.of(context).textTheme.titleLarge,
          ),
          for (final badge in rows(stats, 'playstyle'))
            ListTile(
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
