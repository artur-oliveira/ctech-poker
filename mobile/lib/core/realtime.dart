import 'dart:async';
import 'dart:math';
import 'package:flutter/foundation.dart';
import 'package:fixnum/fixnum.dart';
import 'package:uuid/uuid.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import '../generated/poker.pb.dart';
import 'config.dart';
import 'session.dart';

enum ConnectionPhase { offline, connecting, syncing, live, removed }

/// Owns one gateway. Commands are never buffered across a disconnect. A new
/// table snapshot is required after every reconnect before betting is enabled.
class PokerRealtime extends ChangeNotifier {
  PokerRealtime(
    this.session, {
    this.roomId,
    this.shareCode = '',
    WebSocketChannel Function(Uri)? connectChannel,
  }) : _connectChannel = connectChannel ?? WebSocketChannel.connect;
  final WebSocketChannel Function(Uri) _connectChannel;
  final PokerSession session;
  final String? roomId;
  final String shareCode;
  final events = StreamController<ServerMessage>.broadcast();
  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _subscription;
  Timer? _heartbeat, _retry, _ackTimer;
  int _generation = 0, _attempt = 0;
  bool _stopped = true;
  bool _forceRefresh = false;
  DateTime _lastFrame = DateTime.now();
  ConnectionPhase phase = ConnectionPhase.offline;
  TableSnapshot? snapshot;
  String playerId = '', error = '', pendingAction = '';
  bool challengeRequired = false;
  bool handedOff = false;
  bool get canAct =>
      phase == ConnectionPhase.live &&
      !challengeRequired &&
      pendingAction.isEmpty &&
      snapshot != null &&
      playerId.isNotEmpty &&
      snapshot!.currentPlayerId == playerId;

  Future<void> connect() async {
    handedOff = false;
    _stopped = false;
    final generation = ++_generation;
    _retry?.cancel();
    _heartbeat?.cancel();
    phase = ConnectionPhase.connecting;
    notifyListeners();
    await _subscription?.cancel();
    await _channel?.sink.close();
    if (_stopped || generation != _generation) return;
    try {
      final token = await session.token(force: _forceRefresh);
      _forceRefresh = false;
      if (_stopped || generation != _generation) return;
      final origin = Uri.parse(PokerConfig.api);
      final uri = origin.replace(
        scheme: origin.scheme == 'https' ? 'wss' : 'ws',
        path: roomId == null
            ? '/v1.0/ws'
            : '/v1.0/tables/${Uri.encodeComponent(roomId!)}/ws',
      );
      final channel = _connectChannel(uri);
      _channel = channel;
      await channel.ready.timeout(const Duration(seconds: 15));
      if (_stopped || generation != _generation) {
        await channel.sink.close();
        return;
      }
      phase = ConnectionPhase.syncing;
      _lastFrame = DateTime.now();
      _subscription = channel.stream.listen(
        (data) {
          if (generation != _generation || _stopped) return;
          try {
            if (data is! List<int>) {
              throw const FormatException('Frame não binário');
            }
            _receive(ServerMessage.fromBuffer(data));
          } catch (_) {
            error = 'Não foi possível sincronizar a mesa.';
            _lost(generation);
          }
        },
        onError: (Object _) => _lost(generation),
        onDone: () {
          if (_stopped || generation != _generation) return;
          if (channel.closeReason == 'session handoff to another device') {
            handedOff = true;
            _stopped = true;
            _heartbeat?.cancel();
            _retry?.cancel();
            _ackTimer?.cancel();
            pendingAction = '';
            phase = ConnectionPhase.removed;
            error =
                'A sessão continua em outro aparelho. Reconecte somente se quiser voltar a esta mesa aqui.';
            notifyListeners();
          } else {
            _lost(generation);
          }
        },
      );
      channel.sink.add(
        ClientMessage(
          type: 'auth',
          token: token,
          shareCode: shareCode,
        ).writeToBuffer(),
      );
      _heartbeat = Timer.periodic(const Duration(seconds: 10), (_) {
        if (phase == ConnectionPhase.syncing ||
            DateTime.now().difference(_lastFrame) >
                const Duration(seconds: 30)) {
          _lost(generation);
        } else {
          _send(ClientMessage(type: 'ping'));
        }
      });
      notifyListeners();
    } catch (_) {
      _lost(generation);
    }
  }

  void _receive(ServerMessage message) {
    _lastFrame = DateTime.now();
    if (message.type == 'bot_challenge') {
      challengeRequired = true;
    } else if (message.type == 'bot_challenge_passed') {
      challengeRequired = false;
    } else if (message.type == 'connected') {
      _attempt = 0;
      if (message.playerId.isNotEmpty) playerId = message.playerId;
      if (roomId == null) {
        phase = ConnectionPhase.live;
      } else {
        _send(ClientMessage(type: 'sync_state'));
      }
    } else if (message.type == 'state' && message.hasSnapshot()) {
      final next = message.snapshot;
      if (snapshot == null ||
          next.snapshotVersion >= snapshot!.snapshotVersion) {
        snapshot = next;
        phase = ConnectionPhase.live;
        error = '';
      }
    } else if (message.type == 'equity' &&
        snapshot != null &&
        message.snapshotVersion == snapshot!.snapshotVersion) {
      final updated = snapshot!.deepCopy();
      for (final seat in updated.seats) {
        if (seat.playerId == message.playerId && message.hasEquity()) {
          seat.equity = message.equity;
        }
      }
      snapshot = updated;
    } else if (message.type == 'removed') {
      error = message.message;
      phase = ConnectionPhase.removed;
      _stopped = true;
      _heartbeat?.cancel();
      _channel?.sink.close();
    } else if (message.type == 'table_migrating') {
      _lost(_generation);
    } else if (message.type == 'error') {
      error = message.message.isEmpty ? message.code : message.message;
      if (message.code == 'unauthorized') {
        _forceRefresh = true;
        _lost(_generation);
      } else {
        _send(ClientMessage(type: 'sync_state'));
      }
    }
    if (message.actionId.isNotEmpty &&
        message.actionId == pendingAction &&
        (message.type == 'action_ack' || message.type == 'error')) {
      pendingAction = '';
      _ackTimer?.cancel();
    }
    events.add(message);
    notifyListeners();
  }

  void _lost(int generation) {
    if (_stopped || generation != _generation || _retry?.isActive == true) {
      return;
    }
    phase = ConnectionPhase.offline;
    _heartbeat?.cancel();
    _channel?.sink.close();
    notifyListeners();
    final delay = min(20, 1 << min(_attempt++, 4));
    _retry = Timer(
      Duration(milliseconds: delay * 1000 + Random().nextInt(500)),
      connect,
    );
  }

  bool _send(ClientMessage message) {
    if (_channel == null ||
        phase == ConnectionPhase.offline ||
        phase == ConnectionPhase.removed) {
      return false;
    }
    _channel!.sink.add(message.writeToBuffer());
    return true;
  }

  void command(ClientMessage message) {
    if (phase != ConnectionPhase.live) return;
    if (message.actionId.isEmpty) message.actionId = const Uuid().v4();
    _send(message);
  }

  void act(String action, {int amount = 0}) {
    if (!canAct || !snapshot!.legalActions.actions.contains(action)) return;
    final legal = snapshot!.legalActions;
    if (action == 'raise' &&
        (amount < legal.minRaiseTo.toInt() ||
            amount > legal.maxRaiseTo.toInt())) {
      return;
    }
    pendingAction = const Uuid().v4();
    _send(
      ClientMessage(
        type: 'act',
        action: action,
        amount: Int64(amount),
        actionId: pendingAction,
        expectedHandId: snapshot!.handId,
        expectedSnapshotVersion: snapshot!.snapshotVersion,
      ),
    );
    _ackTimer?.cancel();
    _ackTimer = Timer(const Duration(seconds: 8), () {
      // An uncertain action is never replayed; recover authoritative state.
      error = 'Confirmando a última ação com o servidor…';
      pendingAction = '';
      phase = ConnectionPhase.syncing;
      _send(ClientMessage(type: 'sync_state'));
      notifyListeners();
    });
    notifyListeners();
  }

  void suspend() {
    _stopped = true;
    _generation++;
    _retry?.cancel();
    _heartbeat?.cancel();
    _ackTimer?.cancel();
    pendingAction = '';
    phase = ConnectionPhase.offline;
    _subscription?.cancel();
    _channel?.sink.close();
    notifyListeners();
  }

  @override
  void dispose() {
    suspend();
    events.close();
    super.dispose();
  }
}
