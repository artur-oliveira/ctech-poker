import 'poker_logo.dart';
import '../core/design.dart';
import 'report_player.dart';
import 'buy_in.dart';

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:fixnum/fixnum.dart';
import 'package:uuid/uuid.dart';
import 'package:flutter_tts/flutter_tts.dart';

import '../core/api.dart';
import '../core/fairness.dart';
import '../core/realtime.dart';
import '../generated/poker.pb.dart';
import 'widgets.dart';
import 'native.dart';
import 'reactions.dart';
import 'social_details.dart';
import '../core/preferences.dart';

class TableScreen extends StatefulWidget {
  const TableScreen({
    super.key,
    required this.api,
    required this.roomId,
    required this.playerId,
    this.shareCode = '',
  });
  final PokerApi api;
  final String roomId, playerId, shareCode;
  @override
  State<TableScreen> createState() => _TableScreenState();
}

class _TableScreenState extends State<TableScreen> with WidgetsBindingObserver {
  late final realtime = PokerRealtime(
    widget.api.session,
    roomId: widget.roomId,
    shareCode: widget.shareCode,
  )..playerId = widget.playerId;
  Timer? ticker, socialRetry;
  final tts = FlutterTts();
  String narratedTurn = '';
  String rabbitFailureReported = '';
  Json? room;
  String socialRoster = '';
  Set<String> hiddenPlayers = {};
  bool socialReady = false;
  final Map<String, Json> relationships = {};
  final socialRevision = ValueNotifier<int>(0);
  int lastReminder = DateTime.now().millisecondsSinceEpoch;
  int now = DateTime.now().millisecondsSinceEpoch;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    tts.setLanguage('pt-BR');
    realtime.addListener(changed);
    realtime.connect();
    widget.api
        .get('/v1.0/rooms/${segment(widget.roomId)}')
        .then((value) {
          if (mounted) setState(() => room = value);
        })
        .catchError((Object error) {
          if (mounted) toast(context, error);
        });
    ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (!mounted) return;
      setState(() => now = DateTime.now().millisecondsSinceEpoch);
      final interval = PokerPreferences.instance.reminderMinutes * 60000;
      if (interval > 0 &&
          now - lastReminder >= interval &&
          WidgetsBinding.instance.lifecycleState == AppLifecycleState.resumed) {
        lastReminder = now;
        toast(
          context,
          'Você está jogando há ${PokerPreferences.instance.reminderMinutes} minutos. Que tal uma pausa?',
        );
      }
    });
  }

  void changed() {
    final roster =
        (realtime.snapshot?.seats.map((seat) => seat.playerId).toList() ?? [])
          ..sort();
    final signature = roster.join(',');
    if (signature.isNotEmpty && signature != socialRoster) {
      socialRetry?.cancel();
      socialRoster = signature;
      socialReady = false;
      socialRevision.value++;
      widget.api
          .get('/v1.0/social/relationships', query: {'player_ids': signature})
          .then((response) {
            if (!mounted || socialRoster != signature) return;
            setState(() {
              relationships
                ..clear()
                ..addEntries(
                  rows(
                    response,
                  ).map((p) => MapEntry(p['player_id'] as String, p)),
                );
              hiddenPlayers = rows(response)
                  .where((p) => p['muted'] == true || p['blocked'] == true)
                  .map((p) => p['player_id'] as String)
                  .toSet();
              socialReady = true;
              socialRevision.value++;
            });
          })
          .catchError((Object error) {
            if (mounted && socialRoster == signature) {
              socialRetry?.cancel();
              socialRetry = Timer(
                const Duration(seconds: 10),
                refreshModeration,
              );
            }
          });
    }
    final snapshot = realtime.snapshot;
    if (snapshot != null &&
        snapshot.runoutCards.isNotEmpty &&
        rabbitFailureReported != snapshot.handId &&
        !verifyPartial(
          snapshot.rootCommitHash,
          {
            for (final e in snapshot.revealedCardSalts.entries)
              '${e.key}': {'card': e.value.card, 'salt_hex': e.value.saltHex},
          },
          {
            for (final e in snapshot.unrevealedCardHashes.entries)
              '${e.key}': e.value,
          },
        )) {
      rabbitFailureReported = snapshot.handId;
      command('rabbit_hunt_verify_failed');
    }
    final turn =
        '${realtime.snapshot?.handId}:${realtime.snapshot?.currentPlayerId}';
    if (realtime.canAct && turn != narratedTurn) {
      narratedTurn = turn;
      if (PokerPreferences.instance.sound) {
        SystemSound.play(SystemSoundType.click);
      }
      if (PokerPreferences.instance.narration) tts.speak('Sua vez de jogar.');
    }
    if (mounted) setState(() {});
  }

  void refreshModeration() {
    if (!mounted) return;
    socialRoster = '';
    changed();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      refreshModeration();
      if (!realtime.handedOff) realtime.connect();
    } else if (state == AppLifecycleState.paused) {
      realtime.suspend();
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    ticker?.cancel();
    socialRetry?.cancel();
    socialRevision.dispose();
    tts.stop();
    realtime.removeListener(changed);
    realtime.dispose();
    super.dispose();
  }

  Seat? get hero => realtime.snapshot?.seats
      .where((seat) => seat.playerId == widget.playerId)
      .firstOrNull;
  void command(
    String type, {
    bool? ready,
    bool? twice,
    int? card,
    String? action,
    int? amount,
  }) => realtime.command(
    ClientMessage(
      type: type,
      ready: ready,
      runItTwice: twice,
      cardIndex: card,
      action: action,
      amount: amount == null ? null : Int64(amount),
      expectedHandId: realtime.snapshot?.handId,
      expectedStage: realtime.snapshot?.stage,
      actionId: const Uuid().v4(),
    ),
  );
  @override
  Widget build(BuildContext context) {
    final snapshot = realtime.snapshot?.deepCopy();
    snapshot?.reactions.removeWhere(
      (reaction) => !socialReady || hiddenPlayers.contains(reaction.playerId),
    );
    final connected = realtime.phase == ConnectionPhase.live;
    return Scaffold(
      appBar: AppBar(
        title: Text(snapshot == null ? 'Sua mesa' : stageLabel(snapshot.stage)),
        actions: [
          IconButton(
            tooltip: 'Copiar convite',
            icon: const Icon(Icons.ios_share),
            onPressed: () async {
              final uri = Uri.https('poker.aoctech.app', '/table', {
                'id': widget.roomId,
                if (widget.shareCode.isNotEmpty) 'invite': widget.shareCode,
              });
              await Clipboard.setData(ClipboardData(text: uri.toString()));
              if (context.mounted) toast(context, 'Link copiado.');
            },
          ),
          IconButton(
            tooltip: 'Reações',
            icon: const Icon(Icons.emoji_emotions_outlined),
            onPressed: () => showModalBottomSheet<void>(
              context: context,
              isScrollControlled: true,
              builder: (_) =>
                  ReactionsPanel(api: widget.api, realtime: realtime),
            ),
          ),
          IconButton(
            tooltip: 'Chat da mesa',
            icon: const Icon(Icons.chat_bubble_outline),
            onPressed: chat,
          ),
          IconButton(
            tooltip: 'Opções da mesa',
            icon: const Icon(Icons.more_vert),
            onPressed: options,
          ),
        ],
      ),
      body: SafeArea(
        child: Column(
          children: [
            if (realtime.challengeRequired)
              ListTile(
                title: const Text('Verificação necessária para continuar'),
                trailing: FilledButton(
                  onPressed: () => Navigator.push(
                    context,
                    MaterialPageRoute<void>(
                      builder: (_) => BotChallengeScreen(realtime: realtime),
                    ),
                  ),
                  child: const Text('Verificar'),
                ),
              ),
            if (!connected)
              Material(
                color: PokerColors.control,
                child: ListTile(
                  dense: true,
                  leading: const Icon(Icons.sync),
                  title: Text(
                    realtime.handedOff
                        ? 'Sessão transferida para outro aparelho'
                        : realtime.phase == ConnectionPhase.removed
                        ? 'Você saiu da mesa'
                        : 'Reconectando e sincronizando a mesa…',
                  ),
                  trailing: TextButton(
                    onPressed: realtime.connect,
                    child: const Text('Reconectar'),
                  ),
                ),
              ),
            if (realtime.error.isNotEmpty)
              Padding(
                padding: const EdgeInsets.all(8),
                child: Text(realtime.error, textAlign: TextAlign.center),
              ),
            if (snapshot == null)
              const Expanded(child: Center(child: CircularProgressIndicator()))
            else
              Expanded(
                child: LayoutBuilder(
                  builder: (context, constraints) {
                    final landscape =
                        constraints.maxWidth > 650 &&
                        constraints.maxWidth > constraints.maxHeight;
                    final board = TableFelt(
                      snapshot: snapshot,
                      heroId: widget.playerId,
                      now: now,
                      onSeat: seatMenu,
                    );
                    final actions = ActionDock(
                      realtime: realtime,
                      bigBlind: (room?['big_blind'] as num?)?.toInt() ?? 0,
                    );
                    return landscape
                        ? Row(
                            children: [
                              Expanded(child: board),
                              SizedBox(
                                width: 300,
                                child: SingleChildScrollView(child: actions),
                              ),
                            ],
                          )
                        : Column(
                            children: [
                              Expanded(child: board),
                              ConstrainedBox(
                                constraints: BoxConstraints(
                                  maxHeight: constraints.maxHeight * .45,
                                ),
                                child: SingleChildScrollView(child: actions),
                              ),
                            ],
                          );
                  },
                ),
              ),
            if (snapshot?.pendingWinnerCards.winnerId == widget.playerId)
              Wrap(
                children: [
                  TextButton(
                    onPressed: connected
                        ? () => command('accept_winner_cards')
                        : null,
                    child: const Text('Aceitar mostrar cartas'),
                  ),
                  TextButton(
                    onPressed: connected
                        ? () => command('decline_winner_cards')
                        : null,
                    child: const Text('Recusar pedido'),
                  ),
                ],
              ),
          ],
        ),
      ),
    );
  }

  void options() => showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (context) => SafeArea(
      child: ListenableBuilder(
        listenable: realtime,
        builder: (context, _) => SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(16, 0, 16, 20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                'Sua mesa',
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              ListTile(
                title: const Text('Comando de voz'),
                leading: const Icon(Icons.mic),
                onTap: () => showModalBottomSheet<void>(
                  context: context,
                  builder: (_) => VoiceCommandSheet(realtime: realtime),
                ),
              ),
              SwitchListTile(
                title: const Text('Participar das próximas mãos'),
                value: hero?.ready ?? false,
                onChanged: realtime.phase == ConnectionPhase.live
                    ? (value) => command('ready', ready: value)
                    : null,
              ),
              ListTile(
                title: const Text('Recomprar fichas'),
                enabled: hero?.stack == Int64.ZERO,
                onTap: hero?.stack == Int64.ZERO
                    ? () => safely(context, () async {
                        final currentRoom = await widget.api.get(
                          '/v1.0/rooms/${segment(widget.roomId)}',
                        );
                        if (!context.mounted) return;
                        if (currentRoom['currency_mode'] != 'sandbox') {
                          throw StateError(
                            'Recompra disponível apenas em fichas recreativas nesta versão.',
                          );
                        }
                        final result = await Navigator.push<Json>(
                          context,
                          MaterialPageRoute<Json>(
                            builder: (_) => BuyInScreen(
                              api: widget.api,
                              path:
                                  '/v1.0/rooms/${segment(widget.roomId)}/join',
                              minimum: (currentRoom['buy_in_min'] as num)
                                  .toInt(),
                              maximum: (currentRoom['buy_in_max'] as num)
                                  .toInt(),
                              autoRebuy: hero?.autoRebuy ?? false,
                              body: {'share_code': widget.shareCode},
                            ),
                          ),
                        );
                        if (result == null) return;
                        if (context.mounted) Navigator.pop(context);
                      })
                    : null,
              ),
              SwitchListTile(
                title: const Text('Duas viradas (Run it twice)'),
                value: hero?.runItTwice ?? false,
                onChanged:
                    realtime.phase == ConnectionPhase.live &&
                        room?['run_it_twice_enabled'] == true
                    ? (value) => command('set_run_it_twice', twice: value)
                    : null,
              ),
              for (final entry in {
                'keep_seat': 'Manter meu lugar',
                'post_big_blind': 'Pagar big blind para entrar',
                'show_cards': 'Mostrar minhas duas cartas',
                'peek_cards': 'Espiar minhas cartas',
                'request_rabbit_hunt': 'Rabbit hunt',
                'request_winner_cards': 'Pedir cartas do vencedor',
                'request_handoff': 'Trazer sessão para este aparelho',
              }.entries)
                ListTile(
                  title: Text(entry.value),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: realtime.phase != ConnectionPhase.live
                      ? null
                      : () async {
                          if ([
                            'request_rabbit_hunt',
                            'request_winner_cards',
                          ].contains(entry.key)) {
                            await safely(context, () async {
                              final handId = realtime.snapshot?.handId;
                              final room = await widget.api.get(
                                '/v1.0/rooms/${segment(widget.roomId)}',
                              );
                              if (!context.mounted) return;
                              if (room['currency_mode'] != 'sandbox' ||
                                  realtime.snapshot?.stage != 'complete' ||
                                  realtime.snapshot?.wonWithoutShowdown !=
                                      true) {
                                toast(
                                  context,
                                  'Disponível em mãos sandbox encerradas sem showdown.',
                                );
                                return;
                              }
                              if (!await confirm(
                                context,
                                entry.value,
                                'Custo: ${chips(room['big_blind'])} fichas. O pedido de cartas depende do consentimento do vencedor.',
                              )) {
                                return;
                              }
                              if (handId == realtime.snapshot?.handId) {
                                command(entry.key);
                              }
                            });
                            return;
                          }
                          if (entry.key == 'show_cards' &&
                              !await confirm(
                                context,
                                'Mostrar cartas',
                                'Suas cartas ficarão visíveis aos demais jogadores.',
                              )) {
                            return;
                          }
                          command(entry.key);
                        },
                ),
              ListTile(
                title: Text(
                  hero?.pendingExit == true
                      ? 'Cancelar saída após a mão'
                      : 'Sair após esta mão',
                ),
                onTap: () {
                  command(
                    hero?.pendingExit == true ? 'cancel_exit' : 'request_exit',
                  );
                  Navigator.pop(context);
                },
              ),
              ListTile(
                title: const Text('Sair e devolver fichas ao saldo'),
                leading: const Icon(Icons.logout),
                onTap: () => safely(context, () async {
                  if (!await confirm(
                    context,
                    'Sair da mesa',
                    'O servidor vai confirmar a saída e devolver seu saldo disponível.',
                  )) {
                    return;
                  }
                  await widget.api.post(
                    '/v1.0/rooms/${segment(widget.roomId)}/leave',
                    {'idem_key': const Uuid().v4()},
                  );
                  if (mounted && context.mounted) {
                    Navigator.pop(context);
                    Navigator.pop(this.context);
                  }
                }),
              ),
            ],
          ),
        ),
      ),
    ),
  );
  void chat() => showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (_) => ChatPanel(
      api: widget.api,
      roomId: widget.roomId,
      moderation: socialRevision,
      ready: () => socialReady,
      retryModeration: refreshModeration,
      realtime: realtime,
      allowed: (id) => socialReady && !hiddenPlayers.contains(id),
    ),
  );
  void seatMenu(Seat seat) => showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    builder: (context) => SafeArea(
      child: ListView(
        shrinkWrap: true,
        children: [
          ListTile(
            onTap: () => Navigator.push(
              context,
              MaterialPageRoute<void>(
                builder: (_) =>
                    PublicProfile(api: widget.api, playerId: seat.playerId),
              ),
            ),
            title: Text(seat.name),
            subtitle: Text(
              '${chips(seat.stack.toInt())} fichas · ${seat.state}',
            ),
          ),
          ListTile(
            title: const Text('Nota privada sobre jogador'),
            onTap: () => safely(context, () async {
              final existing = await widget.api.get(
                '/v1.0/players/me/notes/',
                query: {'opponent_ids': seat.playerId},
              );
              if (!context.mounted) return;
              final notes = rows(existing);
              final note = await input(
                context,
                'Nota privada',
                initial: notes.isEmpty ? '' : notes.first['note'] ?? '',
              );
              if (note == null) return;
              await widget.api.post(
                '/v1.0/players/me/notes/${segment(seat.playerId)}',
                {'note': note},
              );
              if (context.mounted) Navigator.pop(context);
            }),
          ),
          if (seat.playerId != widget.playerId && socialReady)
            for (final action in ['mute', 'block'])
              ListTile(
                title: Text(
                  action == 'mute'
                      ? (relationships[seat.playerId]?['muted'] == true
                            ? 'Reativar mensagens'
                            : 'Silenciar jogador')
                      : (relationships[seat.playerId]?['blocked'] == true
                            ? 'Desbloquear jogador'
                            : 'Bloquear jogador'),
                ),
                onTap: () => safely(context, () async {
                  final enabled =
                      relationships[seat.playerId]?[action == 'mute'
                          ? 'muted'
                          : 'blocked'] ==
                      true;
                  if (action == 'block' &&
                      !enabled &&
                      !await confirm(
                        context,
                        'Bloquear jogador',
                        'Bloquear ${seat.name}?',
                      )) {
                    return;
                  }
                  await socialMutation(
                    widget.api,
                    '/${action == 'mute' ? 'mutes' : 'blocks'}/${segment(seat.playerId)}',
                    method: enabled ? 'DELETE' : 'PUT',
                  );
                  refreshModeration();
                  if (context.mounted) Navigator.pop(context);
                }),
              ),
          if (seat.playerId != widget.playerId)
            ListTile(
              title: const Text('Denunciar comportamento'),
              leading: const Icon(Icons.flag_outlined),
              onTap: () => Navigator.push(
                context,
                MaterialPageRoute<void>(
                  builder: (_) => ReportPlayerScreen(
                    api: widget.api,
                    playerId: seat.playerId,
                    surface: 'table_behavior',
                    tableId: widget.roomId,
                    handId: realtime.snapshot?.handId,
                  ),
                ),
              ),
            ),
          if (seat.playerId == widget.playerId)
            for (var i = 0; i < 2; i++)
              ListTile(
                title: Text('Mostrar carta ${i + 1}'),
                onTap: () async {
                  if (await confirm(
                    context,
                    'Mostrar carta',
                    'Esta carta ficará visível a todos.',
                  )) {
                    command('show_cards', card: i);
                  }
                },
              ),
        ],
      ),
    ),
  );
}

String stageLabel(String stage) =>
    {
      'preflop': 'Pré-flop',
      'flop': 'Flop',
      'turn': 'Turn',
      'river': 'River',
      'showdown': 'Showdown',
      'waiting': 'Aguardando jogadores',
    }[stage] ??
    'Mesa';

class TableFelt extends StatelessWidget {
  const TableFelt({
    super.key,
    required this.snapshot,
    required this.heroId,
    required this.now,
    required this.onSeat,
  });
  final TableSnapshot snapshot;
  final String heroId;
  final int now;
  final void Function(Seat) onSeat;
  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, constraints) {
      final largeText = MediaQuery.textScalerOf(context).scale(14) > 19;
      if (largeText ||
          constraints.maxHeight < 430 ||
          constraints.maxWidth < 350) {
        return expandedLayout(context);
      }
      final seats = snapshot.seats;
      final heroIndex = seats.indexWhere((seat) => seat.playerId == heroId);
      final ordered = heroIndex < 0
          ? seats.toList()
          : [...seats.skip(heroIndex + 1), ...seats.take(heroIndex)];
      final width = constraints.maxWidth;
      final height = constraints.maxHeight;
      const slots = [
        Offset(.03, .85),
        Offset(.03, .56),
        Offset(.03, .27),
        Offset(.27, .01),
        Offset(.73, .01),
        Offset(.97, .27),
        Offset(.97, .56),
        Offset(.97, .85),
      ];
      return Container(
        margin: const EdgeInsets.symmetric(horizontal: 8),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(100),
          gradient: RadialGradient(colors: PokerAppearance.feltColors),
          border: Border.all(color: PokerAppearance.railColor, width: 3),
        ),
        child: Stack(
          children: [
            Positioned(
              left: 90,
              right: 90,
              top: height * .32,
              bottom: height * .12,
              child: SingleChildScrollView(
                child: Column(
                  children: [
                    const PokerLogo(size: 26),
                    const SizedBox(height: 16),
                    Text(
                      'POTE ${chips(snapshot.pots.fold<int>(0, (sum, pot) => sum + pot.amount.toInt()))}',
                      style: TextStyle(
                        color: PokerAppearance.feltValueColor,
                        fontFamily: PokerTheme.mono,
                        fontWeight: FontWeight.bold,
                      ),
                    ),
                    const SizedBox(height: 12),
                    PlayingCards(snapshot.board, compact: true),
                    if (snapshot.boardTwo.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(top: 8),
                        child: PlayingCards(snapshot.boardTwo, compact: true),
                      ),
                    if (snapshot.stage == 'complete')
                      Padding(
                        padding: const EdgeInsets.all(8),
                        child: Text(
                          snapshot.winners
                              .map(
                                (id) =>
                                    seats
                                        .where((s) => s.playerId == id)
                                        .firstOrNull
                                        ?.name ??
                                    'Jogador',
                              )
                              .join(' e '),
                          textAlign: TextAlign.center,
                        ),
                      ),
                    if (snapshot.nextHandUnixMs.toInt() > now)
                      Text(
                        'Próxima mão em ${((snapshot.nextHandUnixMs.toInt() - now) / 1000).ceil()}s',
                        textAlign: TextAlign.center,
                      ),
                    if (snapshot.runoutCards.isNotEmpty)
                      TextButton(
                        onPressed: () => showModalBottomSheet<void>(
                          context: context,
                          isScrollControlled: true,
                          builder: (_) => SafeArea(
                            child: SizedBox(
                              height: MediaQuery.sizeOf(context).height * .7,
                              child: expandedLayout(context),
                            ),
                          ),
                        ),
                        child: const Text('Ver rabbit hunt'),
                      ),
                  ],
                ),
              ),
            ),
            for (var i = 0; i < ordered.length && i < 8; i++)
              Positioned(
                left: (slots[i].dx * (width - 106)).clamp(0, width - 106),
                top: slots[i].dy * (height - 100),
                width: 90,
                child: CompactSeat(
                  seat: ordered[i],
                  snapshot: snapshot,
                  now: now,
                  onTap: () => onSeat(ordered[i]),
                ),
              ),
          ],
        ),
      );
    },
  );

  Widget expandedLayout(BuildContext context) {
    final opponents = snapshot.seats
        .where((s) => s.playerId != heroId)
        .toList();
    return DecoratedBox(
      decoration: BoxDecoration(
        gradient: RadialGradient(
          colors: PokerAppearance.feltColors,
          radius: .9,
        ),
      ),
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(12),
        child: Column(
          children: [
            Wrap(
              alignment: WrapAlignment.center,
              spacing: 8,
              runSpacing: 8,
              children: opponents
                  .map(
                    (seat) => SizedBox(
                      width: 112,
                      child: SeatTile(
                        seat: seat,
                        snapshot: snapshot,
                        now: now,
                        onTap: () => onSeat(seat),
                      ),
                    ),
                  )
                  .toList(),
            ),
            const SizedBox(height: 24),
            Text(
              'POTE ${chips(snapshot.pots.fold<int>(0, (sum, p) => sum + p.amount.toInt()))}',
              style: TextStyle(
                fontFamily: PokerTheme.mono,
                fontWeight: FontWeight.bold,
                color: PokerAppearance.feltValueColor,
              ),
            ),
            const SizedBox(height: 12),
            PlayingCards(snapshot.board),
            if (snapshot.runoutCards.isNotEmpty) ...[
              const Text('Rabbit hunt'),
              if (verifyPartial(
                snapshot.rootCommitHash,
                {
                  for (final e in snapshot.revealedCardSalts.entries)
                    '${e.key}': {
                      'card': e.value.card,
                      'salt_hex': e.value.saltHex,
                    },
                },
                {
                  for (final e in snapshot.unrevealedCardHashes.entries)
                    '${e.key}': e.value,
                },
              ))
                PlayingCards(snapshot.runoutCards)
              else
                const Text('Não foi possível verificar o runout.'),
            ],
            if (snapshot.boardTwo.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: PlayingCards(snapshot.boardTwo),
              ),
            if (snapshot.winners.isNotEmpty)
              Padding(
                padding: const EdgeInsets.all(12),
                child: Text(
                  '${snapshot.winners.map((id) => snapshot.seats.where((s) => s.playerId == id).firstOrNull?.name ?? 'Jogador').join(' e ')} venceu',
                  textAlign: TextAlign.center,
                ),
              ),
            for (final pot in snapshot.potResults)
              Text(
                '${pot.refund ? 'Devolução' : 'Premiação'}: ${chips(pot.payoutAmount.toInt())}',
              ),
            if (snapshot.nextHandUnixMs.toInt() > now)
              Text(
                'Próxima mão em ${((snapshot.nextHandUnixMs.toInt() - now) / 1000).ceil()}s',
              ),
            const SizedBox(height: 16),
          ],
        ),
      ),
    );
  }
}

class CompactSeat extends StatelessWidget {
  const CompactSeat({
    super.key,
    required this.seat,
    required this.snapshot,
    required this.now,
    required this.onTap,
  });
  final Seat seat;
  final TableSnapshot snapshot;
  final int now;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final active = snapshot.currentPlayerId == seat.playerId;
    final role = snapshot.dealerPlayerId == seat.playerId
        ? 'D'
        : snapshot.bigBlindPlayerId == seat.playerId
        ? 'BB'
        : snapshot.smallBlindPlayerId == seat.playerId
        ? 'SB'
        : '';
    return Semantics(
      button: true,
      label: '${seat.name}, ${chips(seat.stack.toInt())} fichas, ${seat.state}',
      child: InkWell(
        onTap: onTap,
        child: AnimatedContainer(
          duration: MediaQuery.disableAnimationsOf(context)
              ? Duration.zero
              : const Duration(milliseconds: 180),
          padding: const EdgeInsets.all(6),
          decoration: BoxDecoration(
            color: PokerColors.seat,
            borderRadius: BorderRadius.circular(14),
            border: Border.all(
              color: active ? PokerColors.gold : PokerColors.seatBorder,
              width: 2,
            ),
          ),
          child: Column(
            children: [
              Text(
                seat.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w700,
                ),
              ),
              Text(
                '${chips(seat.stack.toInt())} $role',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 11,
                  fontFamily: PokerTheme.mono,
                  color: PokerColors.gold,
                ),
              ),
              PlayingCards(seat.holeCards, compact: true),
              if (active)
                Text(
                  '${((snapshot.actionDeadlineUnixMs.toInt() - now) / 1000).ceil().clamp(0, 999)}s',
                  style: const TextStyle(fontSize: 11),
                ),
              if (seat.state == 'folded')
                const Text('Desistiu', style: TextStyle(fontSize: 10)),
              for (final reaction in snapshot.reactions.where(
                (r) =>
                    (r.targetPlayerId.isEmpty
                            ? r.playerId
                            : r.targetPlayerId) ==
                        seat.playerId &&
                    r.expiresAt.toInt() > now,
              ))
                Text(reactionGlyphs[reaction.reactionId] ?? ''),
            ],
          ),
        ),
      ),
    );
  }
}

class SeatTile extends StatelessWidget {
  const SeatTile({
    super.key,
    required this.seat,
    required this.snapshot,
    required this.now,
    required this.onTap,
  });
  final Seat seat;
  final TableSnapshot snapshot;
  final int now;
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    final active = snapshot.currentPlayerId == seat.playerId;
    final seconds = ((snapshot.actionDeadlineUnixMs.toInt() - now) / 1000)
        .ceil()
        .clamp(0, 999);
    final role = [
      if (snapshot.dealerPlayerId == seat.playerId) 'D',
      if (snapshot.smallBlindPlayerId == seat.playerId) 'SB',
      if (snapshot.bigBlindPlayerId == seat.playerId) 'BB',
    ].join(' · ');
    return Semantics(
      button: true,
      label:
          '${seat.name}, ${chips(seat.stack.toInt())} fichas${active ? ', sua vez' : ''}',
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(16),
        child: Container(
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: PokerColors.seat,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(
              color: active ? PokerColors.gold : PokerColors.seatBorder,
              width: active ? 2 : 1,
            ),
          ),
          child: Column(
            children: [
              Text(
                seat.name.isEmpty ? 'Jogador' : seat.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
              Text(chips(seat.stack.toInt())),
              if (role.isNotEmpty)
                Text(
                  role,
                  style: const TextStyle(color: PokerColors.gold, fontSize: 11),
                ),
              PlayingCards(seat.holeCards, compact: true),
              if (seat.contributed > Int64.ZERO)
                Text(
                  'Aposta ${chips(seat.contributed.toInt())}',
                  style: const TextStyle(fontSize: 11),
                ),
              if (active)
                Text(
                  '${seconds}s',
                  style: const TextStyle(color: PokerColors.gold),
                ),
              if (seat.connectionState == 'disconnected')
                const Text('Reconectando', style: TextStyle(fontSize: 10)),
              if (seat.hasEquity())
                Text('${(seat.equity * 100).toStringAsFixed(1)}%'),
              if (seat.handCategory.isNotEmpty)
                Text(seat.handCategory, style: const TextStyle(fontSize: 11)),
              if (seat.pendingExit)
                const Text('Saindo após a mão', style: TextStyle(fontSize: 10)),
            ],
          ),
        ),
      ),
    );
  }
}

class ActionDock extends StatelessWidget {
  const ActionDock({super.key, required this.realtime, this.bigBlind = 0});
  final int bigBlind;
  final PokerRealtime realtime;
  @override
  Widget build(BuildContext context) {
    final snapshot = realtime.snapshot!;
    final hero = snapshot.seats
        .where((s) => s.playerId == realtime.playerId)
        .firstOrNull;
    final legal = snapshot.legalActions;
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (hero != null)
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                PlayingCards(hero.holeCards),
                const SizedBox(width: 16),
                Flexible(
                  child: Column(
                    children: [
                      Text(
                        'Você · ${chips(hero.stack.toInt())}',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                      if (hero.handCategory.isNotEmpty) Text(hero.handCategory),
                      Text(
                        'Reserva ${(hero.timeBankMs.toInt() / 1000).floor()}s',
                      ),
                    ],
                  ),
                ),
              ],
            ),
          if (PokerPreferences.instance.trainer &&
              hero != null &&
              snapshot.currentPlayerId != realtime.playerId)
            ExpansionTile(
              title: const Text('Treinador de equidade'),
              children: [
                Padding(
                  padding: const EdgeInsets.all(12),
                  child: Text(
                    hero.hasEquity()
                        ? '${hero.handCategory}: ${(hero.equity * 100).toStringAsFixed(1)}% de equidade informada pelo servidor. Equidade é a participação esperada no pote, incluindo empates; não garante vitória.'
                        : 'Equidade indisponível nesta etapa. Nenhuma carta oculta é estimada pelo aplicativo.',
                  ),
                ),
              ],
            ),
          const SizedBox(height: 12),
          if (!realtime.canAct)
            Text(
              realtime.pendingAction.isNotEmpty
                  ? 'Confirmando ação…'
                  : 'Aguardando sua vez',
            ),
          Wrap(
            alignment: WrapAlignment.center,
            spacing: 8,
            runSpacing: 8,
            children: [
              for (final action in ['fold', 'check', 'call'])
                if (legal.actions.contains(action))
                  FilledButton.tonal(
                    style: action == 'fold'
                        ? null
                        : FilledButton.styleFrom(
                            backgroundColor: PokerColors.paper,
                            foregroundColor: PokerColors.wine,
                          ),
                    onPressed: realtime.canAct
                        ? () => realtime.act(action)
                        : null,
                    child: Text(
                      {
                        'fold': 'Desistir',
                        'check': 'Passar',
                        'call': 'Pagar ${chips(legal.callAmount.toInt())}',
                      }[action]!,
                    ),
                  ),
              if (legal.actions.contains('raise'))
                FilledButton(
                  onPressed: realtime.canAct ? () => raise(context) : null,
                  child: const Text('Aumentar'),
                ),
            ],
          ),
          if (hero != null && !hero.ready)
            TextButton(
              onPressed: () =>
                  realtime.command(ClientMessage(type: 'ready', ready: true)),
              child: const Text('Participar da próxima mão'),
            ),
          if (!realtime.canAct && hero != null)
            PopupMenuButton<String>(
              tooltip: 'Pré-selecionar ação',
              onSelected: (action) => realtime.command(
                ClientMessage(
                  type: 'preselect_action',
                  action: action,
                  expectedHandId: snapshot.handId,
                  expectedStage: snapshot.stage,
                  amount: snapshot.prospectiveCallAmount,
                ),
              ),
              itemBuilder: (_) => [
                for (final e in {
                  '': 'Limpar pré-seleção',
                  'check_fold': 'Passar ou desistir',
                  'fold': 'Desistir',
                  'call': 'Pagar valor atual',
                  'call_any': 'Pagar qualquer valor',
                  'all_in': 'All-in',
                }.entries)
                  PopupMenuItem(value: e.key, child: Text(e.value)),
              ],
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Text(
                  snapshot.actionPreselection.isEmpty
                      ? 'Pré-selecionar ação'
                      : 'Pré-seleção: ${snapshot.actionPreselection}',
                ),
              ),
            ),
        ],
      ),
    );
  }

  Future<void> raise(BuildContext context) async {
    final snapshot = realtime.snapshot!;
    final legal = snapshot.legalActions;
    final controller = TextEditingController(text: legal.minRaiseTo.toString());
    final amount = await showModalBottomSheet<int>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (context) => SafeArea(
        child: Padding(
          padding: EdgeInsets.fromLTRB(
            20,
            0,
            20,
            MediaQuery.viewInsetsOf(context).bottom + 20,
          ),
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Text('Aumentar para', style: TextStyle(fontSize: 24)),
                const SizedBox(height: 16),
                TextField(
                  controller: controller,
                  keyboardType: TextInputType.number,
                  decoration: InputDecoration(
                    labelText: 'Total nesta rodada',
                    helperText:
                        '${legal.minRaiseTo} a ${legal.maxRaiseTo} fichas',
                  ),
                ),
                const SizedBox(height: 12),
                Wrap(
                  spacing: 8,
                  children: [
                    for (final e in {
                      'Mín': legal.minRaiseTo,
                      '½ pote': legal.halfPotRaiseTo,
                      'Pote': legal.potRaiseTo,
                      'All-in': legal.maxRaiseTo,
                    }.entries)
                      if (e.value >= legal.minRaiseTo &&
                          e.value <= legal.maxRaiseTo)
                        ActionChip(
                          label: Text(e.key),
                          onPressed: () => controller.text = e.value.toString(),
                        ),
                  ],
                ),
                const SizedBox(height: 16),
                FilledButton(
                  onPressed: () {
                    final parsed = int.tryParse(controller.text);
                    if (parsed == null ||
                        parsed < legal.minRaiseTo.toInt() ||
                        parsed > legal.maxRaiseTo.toInt()) {
                      toast(context, 'Informe um valor dentro dos limites.');
                      return;
                    }
                    Navigator.pop(context, parsed);
                  },
                  child: const Text('Confirmar aposta'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
    Future<void>.delayed(const Duration(seconds: 1), controller.dispose);
    // A sheet may remain open across another action/hand. Never apply its
    // value to a later decision, even if the later min/max happen to match.
    if (amount != null &&
        realtime.snapshot?.snapshotVersion == snapshot.snapshotVersion &&
        realtime.snapshot?.handId == snapshot.handId) {
      realtime.act('raise', amount: amount);
    }
  }
}

class ChatPanel extends StatefulWidget {
  const ChatPanel({
    super.key,
    required this.api,
    required this.roomId,
    required this.moderation,
    required this.ready,
    required this.retryModeration,
    required this.realtime,
    required this.allowed,
  });
  final PokerApi api;
  final String roomId;
  final Listenable moderation;
  final bool Function() ready;
  final VoidCallback retryModeration;
  final bool Function(String) allowed;
  final PokerRealtime realtime;
  @override
  State<ChatPanel> createState() => _ChatPanelState();
}

class _ChatPanelState extends State<ChatPanel> {
  final text = TextEditingController();
  @override
  void dispose() {
    text.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => SafeArea(
    child: SizedBox(
      height: MediaQuery.sizeOf(context).height * .7,
      child: Padding(
        padding: EdgeInsets.fromLTRB(
          16,
          0,
          16,
          MediaQuery.viewInsetsOf(context).bottom + 12,
        ),
        child: ListenableBuilder(
          listenable: Listenable.merge([widget.realtime, widget.moderation]),
          builder: (context, _) => Column(
            children: [
              const Text('Chat da mesa', style: TextStyle(fontSize: 24)),
              if (!widget.ready())
                TextButton(
                  onPressed: widget.retryModeration,
                  child: const Text('Atualizar preferências de moderação'),
                ),
              Expanded(
                child: ListView(
                  children: [
                    for (final reaction
                        in widget.realtime.snapshot?.reactions ??
                            <TableReaction>[])
                      if (widget.allowed(reaction.playerId))
                        ListTile(
                          leading: Text(
                            reactionGlyphs[reaction.reactionId] ?? '☺',
                            style: const TextStyle(fontSize: 28),
                          ),
                          title: Text(
                            widget.realtime.snapshot?.seats
                                    .where(
                                      (s) => s.playerId == reaction.playerId,
                                    )
                                    .firstOrNull
                                    ?.name ??
                                'Jogador',
                          ),
                          subtitle: Text(
                            reaction.targetPlayerId.isEmpty
                                ? 'Reação na mesa'
                                : 'Reação para ${widget.realtime.snapshot?.seats.where((s) => s.playerId == reaction.targetPlayerId).firstOrNull?.name ?? "jogador"}',
                          ),
                          trailing:
                              reaction.playerId != widget.realtime.playerId
                              ? TableEventReportButton(
                                  api: widget.api,
                                  tableId: widget.roomId,
                                  handId:
                                      widget.realtime.snapshot?.handId ?? '',
                                  playerId: reaction.playerId,
                                  actionId: reaction.id,
                                  reaction: true,
                                )
                              : null,
                        ),
                    for (final message
                        in widget.realtime.snapshot?.chatMessages ??
                            <ChatMessage>[])
                      if (widget.allowed(message.playerId))
                        ListTile(
                          title: Text(
                            widget.realtime.snapshot?.seats
                                    .where(
                                      (s) => s.playerId == message.playerId,
                                    )
                                    .firstOrNull
                                    ?.name ??
                                'Jogador',
                          ),
                          subtitle: Text(message.message),
                          trailing: message.playerId != widget.realtime.playerId
                              ? TableEventReportButton(
                                  api: widget.api,
                                  tableId: widget.roomId,
                                  handId:
                                      widget.realtime.snapshot?.handId ?? '',
                                  playerId: message.playerId,
                                  actionId: message.id,
                                  reaction: false,
                                )
                              : null,
                        ),
                  ],
                ),
              ),
              TextField(
                controller: text,
                maxLength: 280,
                decoration: InputDecoration(
                  labelText: 'Mensagem',
                  suffixIcon: IconButton(
                    tooltip: 'Enviar mensagem',
                    onPressed: widget.realtime.phase == ConnectionPhase.live
                        ? () {
                            if (text.text.trim().isEmpty) return;
                            widget.realtime.command(
                              ClientMessage(
                                type: 'chat',
                                message: text.text.trim(),
                              ),
                            );
                            text.clear();
                          }
                        : null,
                    icon: const Icon(Icons.send),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    ),
  );
}
