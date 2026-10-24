import 'package:flutter/material.dart';
import 'package:uuid/uuid.dart';
import '../core/api.dart';
import '../core/realtime.dart';
import '../core/preferences.dart';
import '../generated/poker.pb.dart';
import 'widgets.dart';

const reactionGlyphs = {
  'clap': '👏',
  'laugh': '😄',
  'wow': '😮',
  'angry': '😤',
  'cry': '😭',
  'nervous': '😰',
  'cold': '🥶',
  'fire': '🔥',
  'respect': '🫡',
  'sleepy': '🥱',
  'heartbeat': '🫀',
  'shark': '🦈',
  'pokerface': '😎',
  'chip': '🟠',
  'coffee': '☕',
  'clover': '🍀',
  'horseshoe': '🧲',
  'tear': '💧',
  'tomato': '🍅',
  'poop': '💩',
  'rofl': '🤣',
  'duck': '🦆',
  'turtle': '🐢',
  'knife': '🗡️',
  'flowers': '💐',
  'spotlight': '🔦',
  'crown': '👑',
  'bandage': '🩹',
  'cucumber': '🥒',
  'boomerang': '🪃',
};
const targetedReactions = {
  'chip',
  'coffee',
  'clover',
  'horseshoe',
  'tear',
  'tomato',
  'poop',
  'rofl',
  'duck',
  'turtle',
  'knife',
  'flowers',
  'spotlight',
  'crown',
  'bandage',
  'cucumber',
  'boomerang',
};

class ReactionsPanel extends StatelessWidget {
  const ReactionsPanel({super.key, required this.api, required this.realtime});
  final PokerApi api;
  final PokerRealtime realtime;
  @override
  Widget build(BuildContext context) => SafeArea(
    child: SizedBox(
      height: MediaQuery.sizeOf(context).height * .6,
      child: AsyncPanel(
        load: () => api.get('/v1.0/wallet/reaction-purchase/catalog'),
        builder: (context, result, _) => ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Text(
              'Reações da mesa',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 16),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final reaction in rows(result))
                  OutlinedButton(
                    onPressed: reaction['owned'] != true
                        ? null
                        : () async {
                            String? target;
                            final id = reaction['id'] as String;
                            if (targetedReactions.contains(id)) {
                              target = await showDialog<String>(
                                context: context,
                                builder: (context) => SimpleDialog(
                                  title: const Text('Enviar para quem?'),
                                  children: [
                                    for (final seat
                                        in realtime.snapshot?.seats ?? <Seat>[])
                                      if (seat.playerId != realtime.playerId)
                                        SimpleDialogOption(
                                          onPressed: () => Navigator.pop(
                                            context,
                                            seat.playerId,
                                          ),
                                          child: Text(seat.name),
                                        ),
                                  ],
                                ),
                              );
                              if (target == null) return;
                            }
                            realtime.command(
                              ClientMessage(
                                type: 'reaction',
                                reactionId: id,
                                targetPlayerId: target,
                                actionId: const Uuid().v4(),
                              ),
                            );
                            if (context.mounted) Navigator.pop(context);
                          },
                    child: Text(
                      '${PokerAppearance.favoriteReactions.contains(reaction['id']) ? '★ ' : ''}${reactionGlyphs[reaction['id']] ?? '☺'} ${reaction['owned'] == true ? '' : '🔒'}',
                      style: const TextStyle(fontSize: 28),
                    ),
                  ),
              ],
            ),
          ],
        ),
      ),
    ),
  );
}
