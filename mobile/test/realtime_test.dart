import 'dart:async';
import 'package:ctech_poker/core/realtime.dart';
import 'package:ctech_poker/core/session.dart';
import 'package:ctech_poker/generated/poker.pb.dart';
import 'package:fixnum/fixnum.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

class Session extends PokerSession {
  @override
  Future<String> token({bool force = false}) async => 'private-bearer';
}

class Sink implements WebSocketSink {
  final sent = <ClientMessage>[];
  @override
  void add(dynamic data) =>
      sent.add(ClientMessage.fromBuffer(data as List<int>));
  @override
  Future<void> close([int? code, String? reason]) async {}
  @override
  Future<void> get done async {}
  @override
  void addError(Object error, [StackTrace? stackTrace]) {}
  @override
  Future<void> addStream(Stream<dynamic> stream) async {
    await for (final message in stream) {
      add(message);
    }
  }
}

class Channel implements WebSocketChannel {
  @override
  String? closeReason;
  final incoming = StreamController<dynamic>.broadcast(sync: true);
  @override
  final Sink sink = Sink();
  @override
  Stream<dynamic> get stream => incoming.stream;
  @override
  Future<void> get ready async {}
  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
  void receive(ServerMessage message) => incoming.add(message.writeToBuffer());
}

TableSnapshot state(
  int version, {
  String hand = 'hand-1',
  String current = 'me',
}) => TableSnapshot(
  handId: hand,
  stage: 'flop',
  snapshotVersion: Int64(version),
  currentPlayerId: current,
  legalActions: LegalActions(
    actions: ['check', 'raise'],
    minRaiseTo: Int64(20),
    maxRaiseTo: Int64(100),
  ),
);
void main() {
  testWidgets('handoff stops automatic reconnect and betting', (tester) async {
    final channel = Channel(), session = Session();
    var connections = 0;
    final live = PokerRealtime(
      session,
      roomId: 'room',
      connectChannel: (_) {
        connections++;
        return channel;
      },
    )..playerId = 'me';
    await live.connect();
    channel.receive(ServerMessage(type: 'state', snapshot: state(1)));
    expect(live.canAct, true);
    channel.closeReason = 'session handoff to another device';
    unawaited(channel.incoming.close());
    await tester.pump();
    expect(live.handedOff, true);
    expect(live.canAct, false);
    await tester.pump(const Duration(seconds: 30));
    expect(connections, 1);
    live.dispose();
    session.dispose();
  });
  test(
    'binary auth first; no betting before snapshot; reject stale state and double tap',
    () async {
      final channel = Channel(), session = Session();
      final live = PokerRealtime(
        session,
        roomId: 'room',
        connectChannel: (_) => channel,
      )..playerId = 'me';
      await live.connect();
      expect(channel.sink.sent.single.type, 'auth');
      expect(channel.sink.sent.single.token, 'private-bearer');
      live.act('check');
      expect(channel.sink.sent.length, 1);
      channel.receive(ServerMessage(type: 'connected'));
      expect(channel.sink.sent.last.type, 'sync_state');
      channel.receive(ServerMessage(type: 'state', snapshot: state(5)));
      expect(live.canAct, true);
      channel.receive(
        ServerMessage(
          type: 'state',
          snapshot: state(4, current: 'other'),
        ),
      );
      expect(live.snapshot!.snapshotVersion, Int64(5));
      live.act('raise', amount: 19);
      expect(channel.sink.sent.last.type, 'sync_state');
      live.act('raise', amount: 50);
      live.act('check');
      expect(channel.sink.sent.where((m) => m.type == 'act').length, 1);
      final action = channel.sink.sent.last;
      expect(action.expectedHandId, 'hand-1');
      expect(action.expectedSnapshotVersion, Int64(5));
      channel.receive(
        ServerMessage(type: 'action_ack', actionId: action.actionId),
      );
      expect(live.pendingAction, isEmpty);
      live.suspend();
      expect(live.canAct, false);
      live.act('check');
      expect(channel.sink.sent.where((m) => m.type == 'act').length, 1);
      live.dispose();
      session.dispose();
      await channel.incoming.close();
    },
  );
  test(
    'reconnect requires state and never repeats the previous action',
    () async {
      final channels = [Channel(), Channel()], session = Session();
      var index = 0;
      final live = PokerRealtime(
        session,
        roomId: 'room',
        connectChannel: (_) => channels[index++],
      )..playerId = 'me';
      await live.connect();
      channels[0].receive(ServerMessage(type: 'state', snapshot: state(2)));
      live.act('check');
      live.suspend();
      await live.connect();
      expect(live.canAct, false);
      channels[1].receive(ServerMessage(type: 'connected'));
      expect(live.canAct, false);
      channels[0].receive(ServerMessage(type: 'state', snapshot: state(100)));
      expect(live.snapshot!.snapshotVersion, Int64(2));
      channels[1].receive(
        ServerMessage(
          type: 'state',
          snapshot: state(3, current: 'other'),
        ),
      );
      expect(live.canAct, false);
      expect(channels[1].sink.sent.where((m) => m.type == 'act'), isEmpty);
      live.dispose();
      session.dispose();
      for (final channel in channels) {
        await channel.incoming.close();
      }
    },
  );
}
