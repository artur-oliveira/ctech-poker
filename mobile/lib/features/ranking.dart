import 'package:flutter/material.dart';
import '../core/api.dart';
import 'social_details.dart';
import 'widgets.dart';

class RankingScreen extends StatelessWidget {
  const RankingScreen({super.key, required this.api});
  final PokerApi api;
  @override
  Widget build(BuildContext context) => PagedList(
    api: api,
    path: '/v1.0/leaderboard',
    query: const {'mode': 'sandbox'},
    header: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Ranking da comunidade',
          style: Theme.of(context).textTheme.headlineSmall,
        ),
        const Text('Fichas recreativas · posição calculada pelo servidor'),
        MyRankingCard(api: api),
      ],
    ),
    indexedItem: (player, index, _) => Card(
      child: ListTile(
        leading: CircleAvatar(child: Text('${index + 1}')),
        title: Text(player['player_name'] ?? 'Jogador'),
        subtitle: Text(
          '${chips(player['hands_won'])} vitórias · ${chips(player['hands_played'])} mãos\n${((player['win_rate'] as num? ?? 0) * 100).toStringAsFixed(1)}% de aproveitamento',
        ),
        isThreeLine: true,
        trailing: const Icon(Icons.chevron_right),
        onTap: () => Navigator.push(
          context,
          MaterialPageRoute<void>(
            builder: (_) =>
                PublicProfile(api: api, playerId: player['player_id']),
          ),
        ),
      ),
    ),
  );
}

class MyRankingCard extends StatefulWidget {
  const MyRankingCard({super.key, required this.api});
  final PokerApi api;
  @override
  State<MyRankingCard> createState() => _MyRankingCardState();
}

class _MyRankingCardState extends State<MyRankingCard> {
  late Future<Json> future = load();
  Future<Json> load() =>
      widget.api.get('/v1.0/leaderboard/me', query: {'mode': 'sandbox'});
  @override
  Widget build(BuildContext context) => Card(
    child: Padding(
      padding: const EdgeInsets.all(16),
      child: FutureBuilder<Json>(
        future: future,
        builder: (context, state) {
          if (state.connectionState != ConnectionState.done) {
            return const LinearProgressIndicator(
              semanticsLabel: 'Carregando sua posição',
            );
          }
          if (state.hasError) {
            return Column(
              children: [
                const Text('Não foi possível consultar sua posição.'),
                TextButton(
                  onPressed: () {
                    setState(() {
                      future = load();
                    });
                  },
                  child: const Text('Tentar minha posição novamente'),
                ),
              ],
            );
          }
          final rank = state.data!;
          if (rank['ranked'] != true) {
            return const Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Ainda sem ranking'),
                Text('Jogue uma mão nesta modalidade para entrar no ranking.'),
              ],
            );
          }
          final entry = rank['entry'] as Map? ?? {};
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('Sua posição no ranking'),
              Text(
                '#${rank['rank']} de ${rank['total']} jogadores',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              Text(
                '${chips(entry['hands_won'])} vitórias · ${((entry['win_rate'] as num? ?? 0) * 100).toStringAsFixed(1)}% de aproveitamento',
              ),
            ],
          );
        },
      ),
    ),
  );
}
