import 'package:flutter/material.dart';
import '../core/api.dart';
import '../core/design.dart';
import '../core/game_mode.dart';
import '../core/labels.dart';
import '../core/achievement_examples.dart';
import 'poker_icon.dart';
import 'widgets.dart';

class AchievementsScreen extends StatelessWidget {
  const AchievementsScreen({
    super.key,
    required this.api,
    this.mode = GameMode.chips,
  });
  final PokerApi api;
  final GameMode mode;
  @override
  Widget build(BuildContext context) => AsyncPanel(
    key: ValueKey(mode),
    load: () => api.get(
      '/v1.0/players/me/achievements/summary',
      query: {'mode': mode.apiValue},
    ),
    builder: (context, summary, _) => ListView(
      padding: const EdgeInsets.all(20),
      children: [
        Row(
          children: [
            const PokerIcon(
              PokerIcons.award,
              color: PokerColors.gold,
              size: 28,
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                'Conquistas',
                style: Theme.of(context).textTheme.headlineSmall,
              ),
            ),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          '${chips(summary['totals']?['stars'])} estrelas',
          style: const TextStyle(
            color: PokerColors.gold,
            fontFamily: PokerTheme.mono,
          ),
        ),
        const SizedBox(height: 20),
        for (final achievement in rows(summary, 'achievements'))
          _AchievementRow(achievement),
      ],
    ),
  );
}

class _AchievementRow extends StatelessWidget {
  const _AchievementRow(this.data);
  final Json data;
  @override
  Widget build(BuildContext context) {
    final progress = data['progress'] as num? ?? 0;
    final target =
        data['next_target'] as num? ?? data['max_target'] as num? ?? 0;
    final stars = (data['stars'] as num? ?? 0).toInt();
    final tiers = rows(data, 'tiers');
    final total = tiers.isEmpty
        ? stars
        : tiers
              .map((t) => (t['stars'] as num? ?? 0).toInt())
              .fold<int>(0, (a, b) => a > b ? a : b);
    final example = achievementExamples[data['key']] ?? const <String>[];
    return Container(
      margin: const EdgeInsets.only(bottom: 12),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: PokerColors.seat,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: PokerColors.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (example.isNotEmpty) ...[
                PlayingCards(example, cardWidth: example.length > 2 ? 18 : 28),
                const SizedBox(width: 14),
              ],
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      achievementLabel(data['key']),
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: 6),
                    Wrap(
                      spacing: 4,
                      children: [
                        if (total > 0)
                          for (var i = 0; i < total; i++)
                            PokerIcon(
                              PokerIcons.star,
                              size: 15,
                              color: i < stars
                                  ? PokerColors.gold
                                  : PokerColors.muted,
                            )
                        else
                          const PokerIcon(
                            PokerIcons.lock,
                            size: 15,
                            color: PokerColors.muted,
                          ),
                      ],
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Text(
                '$stars',
                semanticsLabel: '$stars estrelas',
                style: const TextStyle(
                  fontFamily: PokerTheme.mono,
                  color: PokerColors.gold,
                ),
              ),
            ],
          ),
          const SizedBox(height: 14),
          LinearProgressIndicator(
            value: target > 0 ? (progress / target).clamp(0, 1) : 0,
            backgroundColor: PokerColors.control,
            color: PokerColors.gold,
            minHeight: 4,
            borderRadius: BorderRadius.circular(2),
          ),
          const SizedBox(height: 8),
          Text(
            data['completed'] == true
                ? 'Concluída'
                : '${chips(progress)} / ${chips(target)}',
            style: const TextStyle(fontSize: 12, color: PokerColors.muted),
          ),
        ],
      ),
    );
  }
}
