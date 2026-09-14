// This is a generated file - do not edit.
//
// Generated from poker.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports

import 'dart:core' as $core;

import 'package:fixnum/fixnum.dart' as $fixnum;
import 'package:protobuf/protobuf.dart' as $pb;

export 'package:protobuf/protobuf.dart' show GeneratedMessageGenericExtensions;

class Card extends $pb.GeneratedMessage {
  factory Card({
    $core.String? rank,
    $core.String? suit,
  }) {
    final result = create();
    if (rank != null) result.rank = rank;
    if (suit != null) result.suit = suit;
    return result;
  }

  Card._();

  factory Card.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Card.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Card',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'rank')
    ..aOS(2, _omitFieldNames ? '' : 'suit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Card clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Card copyWith(void Function(Card) updates) =>
      super.copyWith((message) => updates(message as Card)) as Card;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Card create() => Card._();
  @$core.override
  Card createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Card getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Card>(create);
  static Card? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get rank => $_getSZ(0);
  @$pb.TagNumber(1)
  set rank($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRank() => $_has(0);
  @$pb.TagNumber(1)
  void clearRank() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get suit => $_getSZ(1);
  @$pb.TagNumber(2)
  set suit($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSuit() => $_has(1);
  @$pb.TagNumber(2)
  void clearSuit() => $_clearField(2);
}

class Seat extends $pb.GeneratedMessage {
  factory Seat({
    $core.String? playerId,
    $core.String? name,
    $fixnum.Int64? stack,
    $core.String? state,
    $fixnum.Int64? contributed,
    $core.Iterable<$core.String>? holeCards,
    $core.double? equity,
    $core.String? handCategory,
    $core.String? connectionState,
    $core.bool? dealtIn,
    $core.bool? ready,
    $core.Iterable<$core.bool>? holeCardsRevealed,
    $fixnum.Int64? stackAtHandStart,
    $fixnum.Int64? timeBankMs,
    $core.int? handScore,
    $core.String? avatarUrl,
    $core.String? playstyleBadge,
    $core.bool? runItTwice,
    $core.bool? autoRebuy,
    $core.int? currentStreak,
    $core.bool? pendingExit,
  }) {
    final result = create();
    if (playerId != null) result.playerId = playerId;
    if (name != null) result.name = name;
    if (stack != null) result.stack = stack;
    if (state != null) result.state = state;
    if (contributed != null) result.contributed = contributed;
    if (holeCards != null) result.holeCards.addAll(holeCards);
    if (equity != null) result.equity = equity;
    if (handCategory != null) result.handCategory = handCategory;
    if (connectionState != null) result.connectionState = connectionState;
    if (dealtIn != null) result.dealtIn = dealtIn;
    if (ready != null) result.ready = ready;
    if (holeCardsRevealed != null)
      result.holeCardsRevealed.addAll(holeCardsRevealed);
    if (stackAtHandStart != null) result.stackAtHandStart = stackAtHandStart;
    if (timeBankMs != null) result.timeBankMs = timeBankMs;
    if (handScore != null) result.handScore = handScore;
    if (avatarUrl != null) result.avatarUrl = avatarUrl;
    if (playstyleBadge != null) result.playstyleBadge = playstyleBadge;
    if (runItTwice != null) result.runItTwice = runItTwice;
    if (autoRebuy != null) result.autoRebuy = autoRebuy;
    if (currentStreak != null) result.currentStreak = currentStreak;
    if (pendingExit != null) result.pendingExit = pendingExit;
    return result;
  }

  Seat._();

  factory Seat.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Seat.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Seat',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'playerId')
    ..aOS(2, _omitFieldNames ? '' : 'name')
    ..aInt64(3, _omitFieldNames ? '' : 'stack')
    ..aOS(4, _omitFieldNames ? '' : 'state')
    ..aInt64(5, _omitFieldNames ? '' : 'contributed')
    ..pPS(6, _omitFieldNames ? '' : 'holeCards')
    ..aD(7, _omitFieldNames ? '' : 'equity')
    ..aOS(8, _omitFieldNames ? '' : 'handCategory')
    ..aOS(9, _omitFieldNames ? '' : 'connectionState')
    ..aOB(10, _omitFieldNames ? '' : 'dealtIn')
    ..aOB(11, _omitFieldNames ? '' : 'ready')
    ..p<$core.bool>(
        12, _omitFieldNames ? '' : 'holeCardsRevealed', $pb.PbFieldType.KB)
    ..aInt64(13, _omitFieldNames ? '' : 'stackAtHandStart')
    ..aInt64(14, _omitFieldNames ? '' : 'timeBankMs')
    ..aI(15, _omitFieldNames ? '' : 'handScore', fieldType: $pb.PbFieldType.OU3)
    ..aOS(16, _omitFieldNames ? '' : 'avatarUrl')
    ..aOS(17, _omitFieldNames ? '' : 'playstyleBadge')
    ..aOB(18, _omitFieldNames ? '' : 'runItTwice')
    ..aOB(19, _omitFieldNames ? '' : 'autoRebuy')
    ..aI(20, _omitFieldNames ? '' : 'currentStreak')
    ..aOB(21, _omitFieldNames ? '' : 'pendingExit')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Seat clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Seat copyWith(void Function(Seat) updates) =>
      super.copyWith((message) => updates(message as Seat)) as Seat;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Seat create() => Seat._();
  @$core.override
  Seat createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Seat getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Seat>(create);
  static Seat? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get playerId => $_getSZ(0);
  @$pb.TagNumber(1)
  set playerId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPlayerId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPlayerId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get name => $_getSZ(1);
  @$pb.TagNumber(2)
  set name($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasName() => $_has(1);
  @$pb.TagNumber(2)
  void clearName() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get stack => $_getI64(2);
  @$pb.TagNumber(3)
  set stack($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasStack() => $_has(2);
  @$pb.TagNumber(3)
  void clearStack() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get state => $_getSZ(3);
  @$pb.TagNumber(4)
  set state($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasState() => $_has(3);
  @$pb.TagNumber(4)
  void clearState() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get contributed => $_getI64(4);
  @$pb.TagNumber(5)
  set contributed($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasContributed() => $_has(4);
  @$pb.TagNumber(5)
  void clearContributed() => $_clearField(5);

  @$pb.TagNumber(6)
  $pb.PbList<$core.String> get holeCards => $_getList(5);

  @$pb.TagNumber(7)
  $core.double get equity => $_getN(6);
  @$pb.TagNumber(7)
  set equity($core.double value) => $_setDouble(6, value);
  @$pb.TagNumber(7)
  $core.bool hasEquity() => $_has(6);
  @$pb.TagNumber(7)
  void clearEquity() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get handCategory => $_getSZ(7);
  @$pb.TagNumber(8)
  set handCategory($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasHandCategory() => $_has(7);
  @$pb.TagNumber(8)
  void clearHandCategory() => $_clearField(8);

  @$pb.TagNumber(9)
  $core.String get connectionState => $_getSZ(8);
  @$pb.TagNumber(9)
  set connectionState($core.String value) => $_setString(8, value);
  @$pb.TagNumber(9)
  $core.bool hasConnectionState() => $_has(8);
  @$pb.TagNumber(9)
  void clearConnectionState() => $_clearField(9);

  /// True when this seat belongs to the hand identified by TableSnapshot.hand_id.
  /// A seat can be active while false when it returns mid-hand for the next deal.
  /// Optional so a new client can distinguish an old server (field absent)
  /// from an explicit false for a player waiting for the next hand.
  @$pb.TagNumber(10)
  $core.bool get dealtIn => $_getBF(9);
  @$pb.TagNumber(10)
  set dealtIn($core.bool value) => $_setBool(9, value);
  @$pb.TagNumber(10)
  $core.bool hasDealtIn() => $_has(9);
  @$pb.TagNumber(10)
  void clearDealtIn() => $_clearField(10);

  /// The next-hand opt-in. During a live hand a player can be folded from the
  /// current round while ready=false already communicates "paused afterwards".
  @$pb.TagNumber(11)
  $core.bool get ready => $_getBF(10);
  @$pb.TagNumber(11)
  set ready($core.bool value) => $_setBool(10, value);
  @$pb.TagNumber(11)
  $core.bool hasReady() => $_has(10);
  @$pb.TagNumber(11)
  void clearReady() => $_clearField(11);

  /// Public reveal mask for the two hole-card positions. A player's own cards
  /// are always visible to themselves; this mask says what everyone may see.
  @$pb.TagNumber(12)
  $pb.PbList<$core.bool> get holeCardsRevealed => $_getList(11);

  /// Stack before this hand posted blinds or accepted any wager. Optional so
  /// clients can fall back safely while API instances are rolling.
  @$pb.TagNumber(13)
  $fixnum.Int64 get stackAtHandStart => $_getI64(12);
  @$pb.TagNumber(13)
  set stackAtHandStart($fixnum.Int64 value) => $_setInt64(12, value);
  @$pb.TagNumber(13)
  $core.bool hasStackAtHandStart() => $_has(12);
  @$pb.TagNumber(13)
  void clearStackAtHandStart() => $_clearField(13);

  /// Durable decision reserve in milliseconds. Zero is a valid exhausted
  /// balance, so clients must not replace it with a default.
  @$pb.TagNumber(14)
  $fixnum.Int64 get timeBankMs => $_getI64(13);
  @$pb.TagNumber(14)
  set timeBankMs($fixnum.Int64 value) => $_setInt64(13, value);
  @$pb.TagNumber(14)
  $core.bool hasTimeBankMs() => $_has(13);
  @$pb.TagNumber(14)
  void clearTimeBankMs() => $_clearField(14);

  /// Canonical server-side evaluator score. Higher wins; equal is a split.
  /// Only present when the same visibility rules allow hand_category.
  @$pb.TagNumber(15)
  $core.int get handScore => $_getIZ(14);
  @$pb.TagNumber(15)
  set handScore($core.int value) => $_setUnsignedInt32(14, value);
  @$pb.TagNumber(15)
  $core.bool hasHandScore() => $_has(14);
  @$pb.TagNumber(15)
  void clearHandScore() => $_clearField(15);

  @$pb.TagNumber(16)
  $core.String get avatarUrl => $_getSZ(15);
  @$pb.TagNumber(16)
  set avatarUrl($core.String value) => $_setString(15, value);
  @$pb.TagNumber(16)
  $core.bool hasAvatarUrl() => $_has(15);
  @$pb.TagNumber(16)
  void clearAvatarUrl() => $_clearField(16);

  @$pb.TagNumber(17)
  $core.String get playstyleBadge => $_getSZ(16);
  @$pb.TagNumber(17)
  set playstyleBadge($core.String value) => $_setString(16, value);
  @$pb.TagNumber(17)
  $core.bool hasPlaystyleBadge() => $_has(16);
  @$pb.TagNumber(17)
  void clearPlaystyleBadge() => $_clearField(17);

  @$pb.TagNumber(18)
  $core.bool get runItTwice => $_getBF(17);
  @$pb.TagNumber(18)
  set runItTwice($core.bool value) => $_setBool(17, value);
  @$pb.TagNumber(18)
  $core.bool hasRunItTwice() => $_has(17);
  @$pb.TagNumber(18)
  void clearRunItTwice() => $_clearField(18);

  @$pb.TagNumber(19)
  $core.bool get autoRebuy => $_getBF(18);
  @$pb.TagNumber(19)
  set autoRebuy($core.bool value) => $_setBool(18, value);
  @$pb.TagNumber(19)
  $core.bool hasAutoRebuy() => $_has(18);
  @$pb.TagNumber(19)
  void clearAutoRebuy() => $_clearField(19);

  /// Running per-table win/loss streak: positive counts consecutive hand
  /// wins, negative counts consecutive losses, zero means no streak (no
  /// badge). Public, like the D/SB/BB role badge — every viewer sees it.
  @$pb.TagNumber(20)
  $core.int get currentStreak => $_getIZ(19);
  @$pb.TagNumber(20)
  set currentStreak($core.int value) => $_setSignedInt32(19, value);
  @$pb.TagNumber(20)
  $core.bool hasCurrentStreak() => $_has(19);
  @$pb.TagNumber(20)
  void clearCurrentStreak() => $_clearField(20);

  /// The player has asked to leave. They are paused (no future hands) and,
  /// once no longer dealt into the current hand, will be removed and cashed
  /// out automatically. Cancelable via cancel_exit until that removal commits.
  @$pb.TagNumber(21)
  $core.bool get pendingExit => $_getBF(20);
  @$pb.TagNumber(21)
  set pendingExit($core.bool value) => $_setBool(20, value);
  @$pb.TagNumber(21)
  $core.bool hasPendingExit() => $_has(20);
  @$pb.TagNumber(21)
  void clearPendingExit() => $_clearField(21);
}

class BlindEscalation extends $pb.GeneratedMessage {
  factory BlindEscalation({
    $core.int? intervalMinutes,
    $core.int? multiplier,
    $fixnum.Int64? max,
  }) {
    final result = create();
    if (intervalMinutes != null) result.intervalMinutes = intervalMinutes;
    if (multiplier != null) result.multiplier = multiplier;
    if (max != null) result.max = max;
    return result;
  }

  BlindEscalation._();

  factory BlindEscalation.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory BlindEscalation.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'BlindEscalation',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aI(1, _omitFieldNames ? '' : 'intervalMinutes')
    ..aI(2, _omitFieldNames ? '' : 'multiplier')
    ..aInt64(3, _omitFieldNames ? '' : 'max')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BlindEscalation clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  BlindEscalation copyWith(void Function(BlindEscalation) updates) =>
      super.copyWith((message) => updates(message as BlindEscalation))
          as BlindEscalation;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static BlindEscalation create() => BlindEscalation._();
  @$core.override
  BlindEscalation createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static BlindEscalation getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<BlindEscalation>(create);
  static BlindEscalation? _defaultInstance;

  @$pb.TagNumber(1)
  $core.int get intervalMinutes => $_getIZ(0);
  @$pb.TagNumber(1)
  set intervalMinutes($core.int value) => $_setSignedInt32(0, value);
  @$pb.TagNumber(1)
  $core.bool hasIntervalMinutes() => $_has(0);
  @$pb.TagNumber(1)
  void clearIntervalMinutes() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.int get multiplier => $_getIZ(1);
  @$pb.TagNumber(2)
  set multiplier($core.int value) => $_setSignedInt32(1, value);
  @$pb.TagNumber(2)
  $core.bool hasMultiplier() => $_has(1);
  @$pb.TagNumber(2)
  void clearMultiplier() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get max => $_getI64(2);
  @$pb.TagNumber(3)
  set max($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMax() => $_has(2);
  @$pb.TagNumber(3)
  void clearMax() => $_clearField(3);
}

class Room extends $pb.GeneratedMessage {
  factory Room({
    $core.String? roomId,
    $core.String? visibility,
    $core.String? currencyMode,
    $fixnum.Int64? smallBlind,
    $fixnum.Int64? bigBlind,
    $core.int? maxSeats,
    $fixnum.Int64? buyInMin,
    $fixnum.Int64? buyInMax,
    $fixnum.Int64? entryFeeCents,
    $core.String? shareCode,
    BlindEscalation? blindEscalation,
    $core.int? turnTimeoutSeconds,
    $core.bool? equityDisplayEnabled,
    $core.String? status,
    $core.int? seatsTaken,
    $core.String? createdBy,
    $core.String? createdAt,
    $core.bool? runItTwiceEnabled,
  }) {
    final result = create();
    if (roomId != null) result.roomId = roomId;
    if (visibility != null) result.visibility = visibility;
    if (currencyMode != null) result.currencyMode = currencyMode;
    if (smallBlind != null) result.smallBlind = smallBlind;
    if (bigBlind != null) result.bigBlind = bigBlind;
    if (maxSeats != null) result.maxSeats = maxSeats;
    if (buyInMin != null) result.buyInMin = buyInMin;
    if (buyInMax != null) result.buyInMax = buyInMax;
    if (entryFeeCents != null) result.entryFeeCents = entryFeeCents;
    if (shareCode != null) result.shareCode = shareCode;
    if (blindEscalation != null) result.blindEscalation = blindEscalation;
    if (turnTimeoutSeconds != null)
      result.turnTimeoutSeconds = turnTimeoutSeconds;
    if (equityDisplayEnabled != null)
      result.equityDisplayEnabled = equityDisplayEnabled;
    if (status != null) result.status = status;
    if (seatsTaken != null) result.seatsTaken = seatsTaken;
    if (createdBy != null) result.createdBy = createdBy;
    if (createdAt != null) result.createdAt = createdAt;
    if (runItTwiceEnabled != null) result.runItTwiceEnabled = runItTwiceEnabled;
    return result;
  }

  Room._();

  factory Room.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Room.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Room',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'roomId')
    ..aOS(2, _omitFieldNames ? '' : 'visibility')
    ..aOS(3, _omitFieldNames ? '' : 'currencyMode')
    ..aInt64(4, _omitFieldNames ? '' : 'smallBlind')
    ..aInt64(5, _omitFieldNames ? '' : 'bigBlind')
    ..aI(6, _omitFieldNames ? '' : 'maxSeats')
    ..aInt64(7, _omitFieldNames ? '' : 'buyInMin')
    ..aInt64(8, _omitFieldNames ? '' : 'buyInMax')
    ..aInt64(9, _omitFieldNames ? '' : 'entryFeeCents')
    ..aOS(10, _omitFieldNames ? '' : 'shareCode')
    ..aOM<BlindEscalation>(11, _omitFieldNames ? '' : 'blindEscalation',
        subBuilder: BlindEscalation.create)
    ..aI(12, _omitFieldNames ? '' : 'turnTimeoutSeconds')
    ..aOB(13, _omitFieldNames ? '' : 'equityDisplayEnabled')
    ..aOS(14, _omitFieldNames ? '' : 'status')
    ..aI(15, _omitFieldNames ? '' : 'seatsTaken')
    ..aOS(16, _omitFieldNames ? '' : 'createdBy')
    ..aOS(17, _omitFieldNames ? '' : 'createdAt')
    ..aOB(18, _omitFieldNames ? '' : 'runItTwiceEnabled')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Room clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Room copyWith(void Function(Room) updates) =>
      super.copyWith((message) => updates(message as Room)) as Room;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Room create() => Room._();
  @$core.override
  Room createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Room getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Room>(create);
  static Room? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get roomId => $_getSZ(0);
  @$pb.TagNumber(1)
  set roomId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRoomId() => $_has(0);
  @$pb.TagNumber(1)
  void clearRoomId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get visibility => $_getSZ(1);
  @$pb.TagNumber(2)
  set visibility($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasVisibility() => $_has(1);
  @$pb.TagNumber(2)
  void clearVisibility() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get currencyMode => $_getSZ(2);
  @$pb.TagNumber(3)
  set currencyMode($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasCurrencyMode() => $_has(2);
  @$pb.TagNumber(3)
  void clearCurrencyMode() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get smallBlind => $_getI64(3);
  @$pb.TagNumber(4)
  set smallBlind($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasSmallBlind() => $_has(3);
  @$pb.TagNumber(4)
  void clearSmallBlind() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get bigBlind => $_getI64(4);
  @$pb.TagNumber(5)
  set bigBlind($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasBigBlind() => $_has(4);
  @$pb.TagNumber(5)
  void clearBigBlind() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.int get maxSeats => $_getIZ(5);
  @$pb.TagNumber(6)
  set maxSeats($core.int value) => $_setSignedInt32(5, value);
  @$pb.TagNumber(6)
  $core.bool hasMaxSeats() => $_has(5);
  @$pb.TagNumber(6)
  void clearMaxSeats() => $_clearField(6);

  @$pb.TagNumber(7)
  $fixnum.Int64 get buyInMin => $_getI64(6);
  @$pb.TagNumber(7)
  set buyInMin($fixnum.Int64 value) => $_setInt64(6, value);
  @$pb.TagNumber(7)
  $core.bool hasBuyInMin() => $_has(6);
  @$pb.TagNumber(7)
  void clearBuyInMin() => $_clearField(7);

  @$pb.TagNumber(8)
  $fixnum.Int64 get buyInMax => $_getI64(7);
  @$pb.TagNumber(8)
  set buyInMax($fixnum.Int64 value) => $_setInt64(7, value);
  @$pb.TagNumber(8)
  $core.bool hasBuyInMax() => $_has(7);
  @$pb.TagNumber(8)
  void clearBuyInMax() => $_clearField(8);

  @$pb.TagNumber(9)
  $fixnum.Int64 get entryFeeCents => $_getI64(8);
  @$pb.TagNumber(9)
  set entryFeeCents($fixnum.Int64 value) => $_setInt64(8, value);
  @$pb.TagNumber(9)
  $core.bool hasEntryFeeCents() => $_has(8);
  @$pb.TagNumber(9)
  void clearEntryFeeCents() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get shareCode => $_getSZ(9);
  @$pb.TagNumber(10)
  set shareCode($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasShareCode() => $_has(9);
  @$pb.TagNumber(10)
  void clearShareCode() => $_clearField(10);

  @$pb.TagNumber(11)
  BlindEscalation get blindEscalation => $_getN(10);
  @$pb.TagNumber(11)
  set blindEscalation(BlindEscalation value) => $_setField(11, value);
  @$pb.TagNumber(11)
  $core.bool hasBlindEscalation() => $_has(10);
  @$pb.TagNumber(11)
  void clearBlindEscalation() => $_clearField(11);
  @$pb.TagNumber(11)
  BlindEscalation ensureBlindEscalation() => $_ensure(10);

  @$pb.TagNumber(12)
  $core.int get turnTimeoutSeconds => $_getIZ(11);
  @$pb.TagNumber(12)
  set turnTimeoutSeconds($core.int value) => $_setSignedInt32(11, value);
  @$pb.TagNumber(12)
  $core.bool hasTurnTimeoutSeconds() => $_has(11);
  @$pb.TagNumber(12)
  void clearTurnTimeoutSeconds() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.bool get equityDisplayEnabled => $_getBF(12);
  @$pb.TagNumber(13)
  set equityDisplayEnabled($core.bool value) => $_setBool(12, value);
  @$pb.TagNumber(13)
  $core.bool hasEquityDisplayEnabled() => $_has(12);
  @$pb.TagNumber(13)
  void clearEquityDisplayEnabled() => $_clearField(13);

  @$pb.TagNumber(14)
  $core.String get status => $_getSZ(13);
  @$pb.TagNumber(14)
  set status($core.String value) => $_setString(13, value);
  @$pb.TagNumber(14)
  $core.bool hasStatus() => $_has(13);
  @$pb.TagNumber(14)
  void clearStatus() => $_clearField(14);

  @$pb.TagNumber(15)
  $core.int get seatsTaken => $_getIZ(14);
  @$pb.TagNumber(15)
  set seatsTaken($core.int value) => $_setSignedInt32(14, value);
  @$pb.TagNumber(15)
  $core.bool hasSeatsTaken() => $_has(14);
  @$pb.TagNumber(15)
  void clearSeatsTaken() => $_clearField(15);

  @$pb.TagNumber(16)
  $core.String get createdBy => $_getSZ(15);
  @$pb.TagNumber(16)
  set createdBy($core.String value) => $_setString(15, value);
  @$pb.TagNumber(16)
  $core.bool hasCreatedBy() => $_has(15);
  @$pb.TagNumber(16)
  void clearCreatedBy() => $_clearField(16);

  @$pb.TagNumber(17)
  $core.String get createdAt => $_getSZ(16);
  @$pb.TagNumber(17)
  set createdAt($core.String value) => $_setString(16, value);
  @$pb.TagNumber(17)
  $core.bool hasCreatedAt() => $_has(16);
  @$pb.TagNumber(17)
  void clearCreatedAt() => $_clearField(17);

  @$pb.TagNumber(18)
  $core.bool get runItTwiceEnabled => $_getBF(17);
  @$pb.TagNumber(18)
  set runItTwiceEnabled($core.bool value) => $_setBool(17, value);
  @$pb.TagNumber(18)
  $core.bool hasRunItTwiceEnabled() => $_has(17);
  @$pb.TagNumber(18)
  void clearRunItTwiceEnabled() => $_clearField(18);
}

class LegalActions extends $pb.GeneratedMessage {
  factory LegalActions({
    $core.Iterable<$core.String>? actions,
    $fixnum.Int64? callAmount,
    $fixnum.Int64? minRaiseTo,
    $fixnum.Int64? maxRaiseTo,
    $fixnum.Int64? step,
    $fixnum.Int64? currentContribution,
    $fixnum.Int64? currentBet,
    $fixnum.Int64? oneThirdPotRaiseTo,
    $fixnum.Int64? halfPotRaiseTo,
    $fixnum.Int64? twoThirdsPotRaiseTo,
    $fixnum.Int64? potRaiseTo,
  }) {
    final result = create();
    if (actions != null) result.actions.addAll(actions);
    if (callAmount != null) result.callAmount = callAmount;
    if (minRaiseTo != null) result.minRaiseTo = minRaiseTo;
    if (maxRaiseTo != null) result.maxRaiseTo = maxRaiseTo;
    if (step != null) result.step = step;
    if (currentContribution != null)
      result.currentContribution = currentContribution;
    if (currentBet != null) result.currentBet = currentBet;
    if (oneThirdPotRaiseTo != null)
      result.oneThirdPotRaiseTo = oneThirdPotRaiseTo;
    if (halfPotRaiseTo != null) result.halfPotRaiseTo = halfPotRaiseTo;
    if (twoThirdsPotRaiseTo != null)
      result.twoThirdsPotRaiseTo = twoThirdsPotRaiseTo;
    if (potRaiseTo != null) result.potRaiseTo = potRaiseTo;
    return result;
  }

  LegalActions._();

  factory LegalActions.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory LegalActions.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'LegalActions',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..pPS(1, _omitFieldNames ? '' : 'actions')
    ..aInt64(2, _omitFieldNames ? '' : 'callAmount')
    ..aInt64(3, _omitFieldNames ? '' : 'minRaiseTo')
    ..aInt64(4, _omitFieldNames ? '' : 'maxRaiseTo')
    ..aInt64(5, _omitFieldNames ? '' : 'step')
    ..aInt64(6, _omitFieldNames ? '' : 'currentContribution')
    ..aInt64(7, _omitFieldNames ? '' : 'currentBet')
    ..aInt64(8, _omitFieldNames ? '' : 'oneThirdPotRaiseTo')
    ..aInt64(9, _omitFieldNames ? '' : 'halfPotRaiseTo')
    ..aInt64(10, _omitFieldNames ? '' : 'twoThirdsPotRaiseTo')
    ..aInt64(11, _omitFieldNames ? '' : 'potRaiseTo')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LegalActions clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  LegalActions copyWith(void Function(LegalActions) updates) =>
      super.copyWith((message) => updates(message as LegalActions))
          as LegalActions;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static LegalActions create() => LegalActions._();
  @$core.override
  LegalActions createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static LegalActions getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<LegalActions>(create);
  static LegalActions? _defaultInstance;

  @$pb.TagNumber(1)
  $pb.PbList<$core.String> get actions => $_getList(0);

  @$pb.TagNumber(2)
  $fixnum.Int64 get callAmount => $_getI64(1);
  @$pb.TagNumber(2)
  set callAmount($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasCallAmount() => $_has(1);
  @$pb.TagNumber(2)
  void clearCallAmount() => $_clearField(2);

  @$pb.TagNumber(3)
  $fixnum.Int64 get minRaiseTo => $_getI64(2);
  @$pb.TagNumber(3)
  set minRaiseTo($fixnum.Int64 value) => $_setInt64(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMinRaiseTo() => $_has(2);
  @$pb.TagNumber(3)
  void clearMinRaiseTo() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get maxRaiseTo => $_getI64(3);
  @$pb.TagNumber(4)
  set maxRaiseTo($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasMaxRaiseTo() => $_has(3);
  @$pb.TagNumber(4)
  void clearMaxRaiseTo() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get step => $_getI64(4);
  @$pb.TagNumber(5)
  set step($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasStep() => $_has(4);
  @$pb.TagNumber(5)
  void clearStep() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get currentContribution => $_getI64(5);
  @$pb.TagNumber(6)
  set currentContribution($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasCurrentContribution() => $_has(5);
  @$pb.TagNumber(6)
  void clearCurrentContribution() => $_clearField(6);

  @$pb.TagNumber(7)
  $fixnum.Int64 get currentBet => $_getI64(6);
  @$pb.TagNumber(7)
  set currentBet($fixnum.Int64 value) => $_setInt64(6, value);
  @$pb.TagNumber(7)
  $core.bool hasCurrentBet() => $_has(6);
  @$pb.TagNumber(7)
  void clearCurrentBet() => $_clearField(7);

  @$pb.TagNumber(8)
  $fixnum.Int64 get oneThirdPotRaiseTo => $_getI64(7);
  @$pb.TagNumber(8)
  set oneThirdPotRaiseTo($fixnum.Int64 value) => $_setInt64(7, value);
  @$pb.TagNumber(8)
  $core.bool hasOneThirdPotRaiseTo() => $_has(7);
  @$pb.TagNumber(8)
  void clearOneThirdPotRaiseTo() => $_clearField(8);

  @$pb.TagNumber(9)
  $fixnum.Int64 get halfPotRaiseTo => $_getI64(8);
  @$pb.TagNumber(9)
  set halfPotRaiseTo($fixnum.Int64 value) => $_setInt64(8, value);
  @$pb.TagNumber(9)
  $core.bool hasHalfPotRaiseTo() => $_has(8);
  @$pb.TagNumber(9)
  void clearHalfPotRaiseTo() => $_clearField(9);

  @$pb.TagNumber(10)
  $fixnum.Int64 get twoThirdsPotRaiseTo => $_getI64(9);
  @$pb.TagNumber(10)
  set twoThirdsPotRaiseTo($fixnum.Int64 value) => $_setInt64(9, value);
  @$pb.TagNumber(10)
  $core.bool hasTwoThirdsPotRaiseTo() => $_has(9);
  @$pb.TagNumber(10)
  void clearTwoThirdsPotRaiseTo() => $_clearField(10);

  @$pb.TagNumber(11)
  $fixnum.Int64 get potRaiseTo => $_getI64(10);
  @$pb.TagNumber(11)
  set potRaiseTo($fixnum.Int64 value) => $_setInt64(10, value);
  @$pb.TagNumber(11)
  $core.bool hasPotRaiseTo() => $_has(10);
  @$pb.TagNumber(11)
  void clearPotRaiseTo() => $_clearField(11);
}

class Pot extends $pb.GeneratedMessage {
  factory Pot({
    $fixnum.Int64? amount,
    $core.Iterable<$core.String>? eligiblePlayerIds,
  }) {
    final result = create();
    if (amount != null) result.amount = amount;
    if (eligiblePlayerIds != null)
      result.eligiblePlayerIds.addAll(eligiblePlayerIds);
    return result;
  }

  Pot._();

  factory Pot.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory Pot.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'Pot',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'amount')
    ..pPS(2, _omitFieldNames ? '' : 'eligiblePlayerIds')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Pot clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  Pot copyWith(void Function(Pot) updates) =>
      super.copyWith((message) => updates(message as Pot)) as Pot;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static Pot create() => Pot._();
  @$core.override
  Pot createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static Pot getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<Pot>(create);
  static Pot? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get amount => $_getI64(0);
  @$pb.TagNumber(1)
  set amount($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAmount() => $_has(0);
  @$pb.TagNumber(1)
  void clearAmount() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<$core.String> get eligiblePlayerIds => $_getList(1);
}

class PotResult extends $pb.GeneratedMessage {
  factory PotResult({
    $fixnum.Int64? amount,
    $fixnum.Int64? payoutAmount,
    $core.Iterable<$core.String>? eligiblePlayerIds,
    $core.Iterable<$core.String>? winnerPlayerIds,
    $core.Iterable<$core.MapEntry<$core.String, $fixnum.Int64>>? payouts,
    $core.bool? refund,
    $core.int? runout,
  }) {
    final result = create();
    if (amount != null) result.amount = amount;
    if (payoutAmount != null) result.payoutAmount = payoutAmount;
    if (eligiblePlayerIds != null)
      result.eligiblePlayerIds.addAll(eligiblePlayerIds);
    if (winnerPlayerIds != null) result.winnerPlayerIds.addAll(winnerPlayerIds);
    if (payouts != null) result.payouts.addEntries(payouts);
    if (refund != null) result.refund = refund;
    if (runout != null) result.runout = runout;
    return result;
  }

  PotResult._();

  factory PotResult.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PotResult.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PotResult',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aInt64(1, _omitFieldNames ? '' : 'amount')
    ..aInt64(2, _omitFieldNames ? '' : 'payoutAmount')
    ..pPS(3, _omitFieldNames ? '' : 'eligiblePlayerIds')
    ..pPS(4, _omitFieldNames ? '' : 'winnerPlayerIds')
    ..m<$core.String, $fixnum.Int64>(5, _omitFieldNames ? '' : 'payouts',
        entryClassName: 'PotResult.PayoutsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.O6,
        packageName: const $pb.PackageName('poker'))
    ..aOB(6, _omitFieldNames ? '' : 'refund')
    ..aI(7, _omitFieldNames ? '' : 'runout')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PotResult clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PotResult copyWith(void Function(PotResult) updates) =>
      super.copyWith((message) => updates(message as PotResult)) as PotResult;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PotResult create() => PotResult._();
  @$core.override
  PotResult createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PotResult getDefault() =>
      _defaultInstance ??= $pb.GeneratedMessage.$_defaultFor<PotResult>(create);
  static PotResult? _defaultInstance;

  @$pb.TagNumber(1)
  $fixnum.Int64 get amount => $_getI64(0);
  @$pb.TagNumber(1)
  set amount($fixnum.Int64 value) => $_setInt64(0, value);
  @$pb.TagNumber(1)
  $core.bool hasAmount() => $_has(0);
  @$pb.TagNumber(1)
  void clearAmount() => $_clearField(1);

  @$pb.TagNumber(2)
  $fixnum.Int64 get payoutAmount => $_getI64(1);
  @$pb.TagNumber(2)
  set payoutAmount($fixnum.Int64 value) => $_setInt64(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPayoutAmount() => $_has(1);
  @$pb.TagNumber(2)
  void clearPayoutAmount() => $_clearField(2);

  @$pb.TagNumber(3)
  $pb.PbList<$core.String> get eligiblePlayerIds => $_getList(2);

  @$pb.TagNumber(4)
  $pb.PbList<$core.String> get winnerPlayerIds => $_getList(3);

  @$pb.TagNumber(5)
  $pb.PbMap<$core.String, $fixnum.Int64> get payouts => $_getMap(4);

  @$pb.TagNumber(6)
  $core.bool get refund => $_getBF(5);
  @$pb.TagNumber(6)
  set refund($core.bool value) => $_setBool(5, value);
  @$pb.TagNumber(6)
  $core.bool hasRefund() => $_has(5);
  @$pb.TagNumber(6)
  void clearRefund() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.int get runout => $_getIZ(6);
  @$pb.TagNumber(7)
  set runout($core.int value) => $_setSignedInt32(6, value);
  @$pb.TagNumber(7)
  $core.bool hasRunout() => $_has(6);
  @$pb.TagNumber(7)
  void clearRunout() => $_clearField(7);
}

class ChatMessage extends $pb.GeneratedMessage {
  factory ChatMessage({
    $core.String? id,
    $core.String? playerId,
    $core.String? message,
    $fixnum.Int64? timestamp,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (playerId != null) result.playerId = playerId;
    if (message != null) result.message = message;
    if (timestamp != null) result.timestamp = timestamp;
    return result;
  }

  ChatMessage._();

  factory ChatMessage.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ChatMessage.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ChatMessage',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'playerId')
    ..aOS(3, _omitFieldNames ? '' : 'message')
    ..aInt64(4, _omitFieldNames ? '' : 'timestamp')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatMessage clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ChatMessage copyWith(void Function(ChatMessage) updates) =>
      super.copyWith((message) => updates(message as ChatMessage))
          as ChatMessage;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ChatMessage create() => ChatMessage._();
  @$core.override
  ChatMessage createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ChatMessage getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ChatMessage>(create);
  static ChatMessage? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get playerId => $_getSZ(1);
  @$pb.TagNumber(2)
  set playerId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPlayerId() => $_has(1);
  @$pb.TagNumber(2)
  void clearPlayerId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get message => $_getSZ(2);
  @$pb.TagNumber(3)
  set message($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasMessage() => $_has(2);
  @$pb.TagNumber(3)
  void clearMessage() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get timestamp => $_getI64(3);
  @$pb.TagNumber(4)
  set timestamp($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasTimestamp() => $_has(3);
  @$pb.TagNumber(4)
  void clearTimestamp() => $_clearField(4);
}

class TableReaction extends $pb.GeneratedMessage {
  factory TableReaction({
    $core.String? id,
    $core.String? playerId,
    $core.String? reactionId,
    $core.String? targetPlayerId,
    $fixnum.Int64? timestamp,
    $fixnum.Int64? expiresAt,
  }) {
    final result = create();
    if (id != null) result.id = id;
    if (playerId != null) result.playerId = playerId;
    if (reactionId != null) result.reactionId = reactionId;
    if (targetPlayerId != null) result.targetPlayerId = targetPlayerId;
    if (timestamp != null) result.timestamp = timestamp;
    if (expiresAt != null) result.expiresAt = expiresAt;
    return result;
  }

  TableReaction._();

  factory TableReaction.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory TableReaction.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'TableReaction',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'id')
    ..aOS(2, _omitFieldNames ? '' : 'playerId')
    ..aOS(3, _omitFieldNames ? '' : 'reactionId')
    ..aOS(4, _omitFieldNames ? '' : 'targetPlayerId')
    ..aInt64(5, _omitFieldNames ? '' : 'timestamp')
    ..aInt64(6, _omitFieldNames ? '' : 'expiresAt')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TableReaction clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TableReaction copyWith(void Function(TableReaction) updates) =>
      super.copyWith((message) => updates(message as TableReaction))
          as TableReaction;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static TableReaction create() => TableReaction._();
  @$core.override
  TableReaction createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static TableReaction getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<TableReaction>(create);
  static TableReaction? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get id => $_getSZ(0);
  @$pb.TagNumber(1)
  set id($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasId() => $_has(0);
  @$pb.TagNumber(1)
  void clearId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get playerId => $_getSZ(1);
  @$pb.TagNumber(2)
  set playerId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasPlayerId() => $_has(1);
  @$pb.TagNumber(2)
  void clearPlayerId() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get reactionId => $_getSZ(2);
  @$pb.TagNumber(3)
  set reactionId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasReactionId() => $_has(2);
  @$pb.TagNumber(3)
  void clearReactionId() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get targetPlayerId => $_getSZ(3);
  @$pb.TagNumber(4)
  set targetPlayerId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasTargetPlayerId() => $_has(3);
  @$pb.TagNumber(4)
  void clearTargetPlayerId() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get timestamp => $_getI64(4);
  @$pb.TagNumber(5)
  set timestamp($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasTimestamp() => $_has(4);
  @$pb.TagNumber(5)
  void clearTimestamp() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get expiresAt => $_getI64(5);
  @$pb.TagNumber(6)
  set expiresAt($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasExpiresAt() => $_has(5);
  @$pb.TagNumber(6)
  void clearExpiresAt() => $_clearField(6);
}

class TableSnapshot extends $pb.GeneratedMessage {
  factory TableSnapshot({
    $core.String? stage,
    $core.Iterable<$core.String>? board,
    $core.Iterable<Seat>? seats,
    $core.Iterable<$core.MapEntry<$core.String, $fixnum.Int64>>? payouts,
    $core.Iterable<$core.String>? winners,
    $fixnum.Int64? rake,
    $core.String? currentPlayerId,
    LegalActions? legalActions,
    $fixnum.Int64? actionDeadlineUnixMs,
    $fixnum.Int64? nextHandUnixMs,
    $core.bool? wonWithoutShowdown,
    $core.String? shuffleCommitHash,
    $core.String? shuffleServerSeedHex,
    $core.String? smallBlindPlayerId,
    $core.String? bigBlindPlayerId,
    $core.String? dealerPlayerId,
    $fixnum.Int64? snapshotVersion,
    $core.Iterable<Pot>? pots,
    $core.String? handId,
    $core.Iterable<PotResult>? potResults,
    $core.int? protocolVersion,
    $fixnum.Int64? idleRemovalUnixMs,
    $fixnum.Int64? actionBaseDeadlineUnixMs,
    $core.Iterable<ChatMessage>? chatMessages,
    $core.Iterable<TableReaction>? reactions,
    $core.String? actionPreselection,
    $fixnum.Int64? actionPreselectionAmount,
    $fixnum.Int64? prospectiveCallAmount,
    $core.String? rootCommitHash,
    $core.Iterable<$core.MapEntry<$core.int, RevealedSalt>>? revealedCardSalts,
    $core.Iterable<$core.MapEntry<$core.int, $core.String>>?
        unrevealedCardHashes,
    $core.Iterable<$core.String>? runoutCards,
    $core.Iterable<$core.String>? boardTwo,
    $core.int? boardSplitAt,
    WinnerCardsRequest? pendingWinnerCards,
  }) {
    final result = create();
    if (stage != null) result.stage = stage;
    if (board != null) result.board.addAll(board);
    if (seats != null) result.seats.addAll(seats);
    if (payouts != null) result.payouts.addEntries(payouts);
    if (winners != null) result.winners.addAll(winners);
    if (rake != null) result.rake = rake;
    if (currentPlayerId != null) result.currentPlayerId = currentPlayerId;
    if (legalActions != null) result.legalActions = legalActions;
    if (actionDeadlineUnixMs != null)
      result.actionDeadlineUnixMs = actionDeadlineUnixMs;
    if (nextHandUnixMs != null) result.nextHandUnixMs = nextHandUnixMs;
    if (wonWithoutShowdown != null)
      result.wonWithoutShowdown = wonWithoutShowdown;
    if (shuffleCommitHash != null) result.shuffleCommitHash = shuffleCommitHash;
    if (shuffleServerSeedHex != null)
      result.shuffleServerSeedHex = shuffleServerSeedHex;
    if (smallBlindPlayerId != null)
      result.smallBlindPlayerId = smallBlindPlayerId;
    if (bigBlindPlayerId != null) result.bigBlindPlayerId = bigBlindPlayerId;
    if (dealerPlayerId != null) result.dealerPlayerId = dealerPlayerId;
    if (snapshotVersion != null) result.snapshotVersion = snapshotVersion;
    if (pots != null) result.pots.addAll(pots);
    if (handId != null) result.handId = handId;
    if (potResults != null) result.potResults.addAll(potResults);
    if (protocolVersion != null) result.protocolVersion = protocolVersion;
    if (idleRemovalUnixMs != null) result.idleRemovalUnixMs = idleRemovalUnixMs;
    if (actionBaseDeadlineUnixMs != null)
      result.actionBaseDeadlineUnixMs = actionBaseDeadlineUnixMs;
    if (chatMessages != null) result.chatMessages.addAll(chatMessages);
    if (reactions != null) result.reactions.addAll(reactions);
    if (actionPreselection != null)
      result.actionPreselection = actionPreselection;
    if (actionPreselectionAmount != null)
      result.actionPreselectionAmount = actionPreselectionAmount;
    if (prospectiveCallAmount != null)
      result.prospectiveCallAmount = prospectiveCallAmount;
    if (rootCommitHash != null) result.rootCommitHash = rootCommitHash;
    if (revealedCardSalts != null)
      result.revealedCardSalts.addEntries(revealedCardSalts);
    if (unrevealedCardHashes != null)
      result.unrevealedCardHashes.addEntries(unrevealedCardHashes);
    if (runoutCards != null) result.runoutCards.addAll(runoutCards);
    if (boardTwo != null) result.boardTwo.addAll(boardTwo);
    if (boardSplitAt != null) result.boardSplitAt = boardSplitAt;
    if (pendingWinnerCards != null)
      result.pendingWinnerCards = pendingWinnerCards;
    return result;
  }

  TableSnapshot._();

  factory TableSnapshot.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory TableSnapshot.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'TableSnapshot',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'stage')
    ..pPS(2, _omitFieldNames ? '' : 'board')
    ..pPM<Seat>(3, _omitFieldNames ? '' : 'seats', subBuilder: Seat.create)
    ..m<$core.String, $fixnum.Int64>(4, _omitFieldNames ? '' : 'payouts',
        entryClassName: 'TableSnapshot.PayoutsEntry',
        keyFieldType: $pb.PbFieldType.OS,
        valueFieldType: $pb.PbFieldType.O6,
        packageName: const $pb.PackageName('poker'))
    ..pPS(5, _omitFieldNames ? '' : 'winners')
    ..aInt64(6, _omitFieldNames ? '' : 'rake')
    ..aOS(7, _omitFieldNames ? '' : 'currentPlayerId')
    ..aOM<LegalActions>(8, _omitFieldNames ? '' : 'legalActions',
        subBuilder: LegalActions.create)
    ..aInt64(9, _omitFieldNames ? '' : 'actionDeadlineUnixMs')
    ..aInt64(10, _omitFieldNames ? '' : 'nextHandUnixMs')
    ..aOB(11, _omitFieldNames ? '' : 'wonWithoutShowdown')
    ..aOS(12, _omitFieldNames ? '' : 'shuffleCommitHash')
    ..aOS(13, _omitFieldNames ? '' : 'shuffleServerSeedHex')
    ..aOS(14, _omitFieldNames ? '' : 'smallBlindPlayerId')
    ..aOS(15, _omitFieldNames ? '' : 'bigBlindPlayerId')
    ..aOS(16, _omitFieldNames ? '' : 'dealerPlayerId')
    ..a<$fixnum.Int64>(
        17, _omitFieldNames ? '' : 'snapshotVersion', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..pPM<Pot>(18, _omitFieldNames ? '' : 'pots', subBuilder: Pot.create)
    ..aOS(19, _omitFieldNames ? '' : 'handId')
    ..pPM<PotResult>(20, _omitFieldNames ? '' : 'potResults',
        subBuilder: PotResult.create)
    ..aI(21, _omitFieldNames ? '' : 'protocolVersion',
        fieldType: $pb.PbFieldType.OU3)
    ..aInt64(22, _omitFieldNames ? '' : 'idleRemovalUnixMs')
    ..aInt64(23, _omitFieldNames ? '' : 'actionBaseDeadlineUnixMs')
    ..pPM<ChatMessage>(24, _omitFieldNames ? '' : 'chatMessages',
        subBuilder: ChatMessage.create)
    ..pPM<TableReaction>(25, _omitFieldNames ? '' : 'reactions',
        subBuilder: TableReaction.create)
    ..aOS(26, _omitFieldNames ? '' : 'actionPreselection')
    ..aInt64(27, _omitFieldNames ? '' : 'actionPreselectionAmount')
    ..aInt64(28, _omitFieldNames ? '' : 'prospectiveCallAmount')
    ..aOS(29, _omitFieldNames ? '' : 'rootCommitHash')
    ..m<$core.int, RevealedSalt>(30, _omitFieldNames ? '' : 'revealedCardSalts',
        entryClassName: 'TableSnapshot.RevealedCardSaltsEntry',
        keyFieldType: $pb.PbFieldType.O3,
        valueFieldType: $pb.PbFieldType.OM,
        valueCreator: RevealedSalt.create,
        valueDefaultOrMaker: RevealedSalt.getDefault,
        packageName: const $pb.PackageName('poker'))
    ..m<$core.int, $core.String>(
        31, _omitFieldNames ? '' : 'unrevealedCardHashes',
        entryClassName: 'TableSnapshot.UnrevealedCardHashesEntry',
        keyFieldType: $pb.PbFieldType.O3,
        valueFieldType: $pb.PbFieldType.OS,
        packageName: const $pb.PackageName('poker'))
    ..pPS(32, _omitFieldNames ? '' : 'runoutCards')
    ..pPS(33, _omitFieldNames ? '' : 'boardTwo')
    ..aI(34, _omitFieldNames ? '' : 'boardSplitAt')
    ..aOM<WinnerCardsRequest>(35, _omitFieldNames ? '' : 'pendingWinnerCards',
        subBuilder: WinnerCardsRequest.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TableSnapshot clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  TableSnapshot copyWith(void Function(TableSnapshot) updates) =>
      super.copyWith((message) => updates(message as TableSnapshot))
          as TableSnapshot;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static TableSnapshot create() => TableSnapshot._();
  @$core.override
  TableSnapshot createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static TableSnapshot getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<TableSnapshot>(create);
  static TableSnapshot? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get stage => $_getSZ(0);
  @$pb.TagNumber(1)
  set stage($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasStage() => $_has(0);
  @$pb.TagNumber(1)
  void clearStage() => $_clearField(1);

  @$pb.TagNumber(2)
  $pb.PbList<$core.String> get board => $_getList(1);

  @$pb.TagNumber(3)
  $pb.PbList<Seat> get seats => $_getList(2);

  @$pb.TagNumber(4)
  $pb.PbMap<$core.String, $fixnum.Int64> get payouts => $_getMap(3);

  @$pb.TagNumber(5)
  $pb.PbList<$core.String> get winners => $_getList(4);

  @$pb.TagNumber(6)
  $fixnum.Int64 get rake => $_getI64(5);
  @$pb.TagNumber(6)
  set rake($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasRake() => $_has(5);
  @$pb.TagNumber(6)
  void clearRake() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get currentPlayerId => $_getSZ(6);
  @$pb.TagNumber(7)
  set currentPlayerId($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasCurrentPlayerId() => $_has(6);
  @$pb.TagNumber(7)
  void clearCurrentPlayerId() => $_clearField(7);

  @$pb.TagNumber(8)
  LegalActions get legalActions => $_getN(7);
  @$pb.TagNumber(8)
  set legalActions(LegalActions value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasLegalActions() => $_has(7);
  @$pb.TagNumber(8)
  void clearLegalActions() => $_clearField(8);
  @$pb.TagNumber(8)
  LegalActions ensureLegalActions() => $_ensure(7);

  @$pb.TagNumber(9)
  $fixnum.Int64 get actionDeadlineUnixMs => $_getI64(8);
  @$pb.TagNumber(9)
  set actionDeadlineUnixMs($fixnum.Int64 value) => $_setInt64(8, value);
  @$pb.TagNumber(9)
  $core.bool hasActionDeadlineUnixMs() => $_has(8);
  @$pb.TagNumber(9)
  void clearActionDeadlineUnixMs() => $_clearField(9);

  @$pb.TagNumber(10)
  $fixnum.Int64 get nextHandUnixMs => $_getI64(9);
  @$pb.TagNumber(10)
  set nextHandUnixMs($fixnum.Int64 value) => $_setInt64(9, value);
  @$pb.TagNumber(10)
  $core.bool hasNextHandUnixMs() => $_has(9);
  @$pb.TagNumber(10)
  void clearNextHandUnixMs() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.bool get wonWithoutShowdown => $_getBF(10);
  @$pb.TagNumber(11)
  set wonWithoutShowdown($core.bool value) => $_setBool(10, value);
  @$pb.TagNumber(11)
  $core.bool hasWonWithoutShowdown() => $_has(10);
  @$pb.TagNumber(11)
  void clearWonWithoutShowdown() => $_clearField(11);

  @$pb.TagNumber(12)
  $core.String get shuffleCommitHash => $_getSZ(11);
  @$pb.TagNumber(12)
  set shuffleCommitHash($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasShuffleCommitHash() => $_has(11);
  @$pb.TagNumber(12)
  void clearShuffleCommitHash() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.String get shuffleServerSeedHex => $_getSZ(12);
  @$pb.TagNumber(13)
  set shuffleServerSeedHex($core.String value) => $_setString(12, value);
  @$pb.TagNumber(13)
  $core.bool hasShuffleServerSeedHex() => $_has(12);
  @$pb.TagNumber(13)
  void clearShuffleServerSeedHex() => $_clearField(13);

  @$pb.TagNumber(14)
  $core.String get smallBlindPlayerId => $_getSZ(13);
  @$pb.TagNumber(14)
  set smallBlindPlayerId($core.String value) => $_setString(13, value);
  @$pb.TagNumber(14)
  $core.bool hasSmallBlindPlayerId() => $_has(13);
  @$pb.TagNumber(14)
  void clearSmallBlindPlayerId() => $_clearField(14);

  @$pb.TagNumber(15)
  $core.String get bigBlindPlayerId => $_getSZ(14);
  @$pb.TagNumber(15)
  set bigBlindPlayerId($core.String value) => $_setString(14, value);
  @$pb.TagNumber(15)
  $core.bool hasBigBlindPlayerId() => $_has(14);
  @$pb.TagNumber(15)
  void clearBigBlindPlayerId() => $_clearField(15);

  @$pb.TagNumber(16)
  $core.String get dealerPlayerId => $_getSZ(15);
  @$pb.TagNumber(16)
  set dealerPlayerId($core.String value) => $_setString(15, value);
  @$pb.TagNumber(16)
  $core.bool hasDealerPlayerId() => $_has(15);
  @$pb.TagNumber(16)
  void clearDealerPlayerId() => $_clearField(16);

  @$pb.TagNumber(17)
  $fixnum.Int64 get snapshotVersion => $_getI64(16);
  @$pb.TagNumber(17)
  set snapshotVersion($fixnum.Int64 value) => $_setInt64(16, value);
  @$pb.TagNumber(17)
  $core.bool hasSnapshotVersion() => $_has(16);
  @$pb.TagNumber(17)
  void clearSnapshotVersion() => $_clearField(17);

  @$pb.TagNumber(18)
  $pb.PbList<Pot> get pots => $_getList(17);

  @$pb.TagNumber(19)
  $core.String get handId => $_getSZ(18);
  @$pb.TagNumber(19)
  set handId($core.String value) => $_setString(18, value);
  @$pb.TagNumber(19)
  $core.bool hasHandId() => $_has(18);
  @$pb.TagNumber(19)
  void clearHandId() => $_clearField(19);

  @$pb.TagNumber(20)
  $pb.PbList<PotResult> get potResults => $_getList(19);

  /// Allows clients to apply explicit compatibility fallbacks during rolling
  /// deployments instead of mistaking absent proto3 fields for false.
  @$pb.TagNumber(21)
  $core.int get protocolVersion => $_getIZ(20);
  @$pb.TagNumber(21)
  set protocolVersion($core.int value) => $_setUnsignedInt32(20, value);
  @$pb.TagNumber(21)
  $core.bool hasProtocolVersion() => $_has(20);
  @$pb.TagNumber(21)
  void clearProtocolVersion() => $_clearField(21);

  @$pb.TagNumber(22)
  $fixnum.Int64 get idleRemovalUnixMs => $_getI64(21);
  @$pb.TagNumber(22)
  set idleRemovalUnixMs($fixnum.Int64 value) => $_setInt64(21, value);
  @$pb.TagNumber(22)
  $core.bool hasIdleRemovalUnixMs() => $_has(21);
  @$pb.TagNumber(22)
  void clearIdleRemovalUnixMs() => $_clearField(22);

  /// End of the normal room clock. The interval between this and
  /// action_deadline_unix_ms belongs to the current player's time bank.
  @$pb.TagNumber(23)
  $fixnum.Int64 get actionBaseDeadlineUnixMs => $_getI64(22);
  @$pb.TagNumber(23)
  set actionBaseDeadlineUnixMs($fixnum.Int64 value) => $_setInt64(22, value);
  @$pb.TagNumber(23)
  $core.bool hasActionBaseDeadlineUnixMs() => $_has(22);
  @$pb.TagNumber(23)
  void clearActionBaseDeadlineUnixMs() => $_clearField(23);

  @$pb.TagNumber(24)
  $pb.PbList<ChatMessage> get chatMessages => $_getList(23);

  @$pb.TagNumber(25)
  $pb.PbList<TableReaction> get reactions => $_getList(24);

  /// Viewer-scoped one-shot value: check_fold | fold | call | call_any | all_in | empty.
  @$pb.TagNumber(26)
  $core.String get actionPreselection => $_getSZ(25);
  @$pb.TagNumber(26)
  set actionPreselection($core.String value) => $_setString(25, value);
  @$pb.TagNumber(26)
  $core.bool hasActionPreselection() => $_has(25);
  @$pb.TagNumber(26)
  void clearActionPreselection() => $_clearField(26);

  /// Frozen amount for fixed call/check_fold preselections; zero for all_in and unconditional modes.
  @$pb.TagNumber(27)
  $fixnum.Int64 get actionPreselectionAmount => $_getI64(26);
  @$pb.TagNumber(27)
  set actionPreselectionAmount($fixnum.Int64 value) => $_setInt64(26, value);
  @$pb.TagNumber(27)
  $core.bool hasActionPreselectionAmount() => $_has(26);
  @$pb.TagNumber(27)
  void clearActionPreselectionAmount() => $_clearField(27);

  /// What this viewer would owe if action reached them now, even before their
  /// turn. Viewer-scoped so the UI can offer an exact fixed-call preselection.
  @$pb.TagNumber(28)
  $fixnum.Int64 get prospectiveCallAmount => $_getI64(27);
  @$pb.TagNumber(28)
  set prospectiveCallAmount($fixnum.Int64 value) => $_setInt64(27, value);
  @$pb.TagNumber(28)
  $core.bool hasProspectiveCallAmount() => $_has(27);
  @$pb.TagNumber(28)
  void clearProspectiveCallAmount() => $_clearField(28);

  @$pb.TagNumber(29)
  $core.String get rootCommitHash => $_getSZ(28);
  @$pb.TagNumber(29)
  set rootCommitHash($core.String value) => $_setString(28, value);
  @$pb.TagNumber(29)
  $core.bool hasRootCommitHash() => $_has(28);
  @$pb.TagNumber(29)
  void clearRootCommitHash() => $_clearField(29);

  @$pb.TagNumber(30)
  $pb.PbMap<$core.int, RevealedSalt> get revealedCardSalts => $_getMap(29);

  @$pb.TagNumber(31)
  $pb.PbMap<$core.int, $core.String> get unrevealedCardHashes => $_getMap(30);

  @$pb.TagNumber(32)
  $pb.PbList<$core.String> get runoutCards => $_getList(31);

  @$pb.TagNumber(33)
  $pb.PbList<$core.String> get boardTwo => $_getList(32);

  @$pb.TagNumber(34)
  $core.int get boardSplitAt => $_getIZ(33);
  @$pb.TagNumber(34)
  set boardSplitAt($core.int value) => $_setSignedInt32(33, value);
  @$pb.TagNumber(34)
  $core.bool hasBoardSplitAt() => $_has(33);
  @$pb.TagNumber(34)
  void clearBoardSplitAt() => $_clearField(34);

  /// Viewer-scoped: present only for the winner being asked and the requester
  /// waiting on the answer. Nobody else learns a request exists.
  @$pb.TagNumber(35)
  WinnerCardsRequest get pendingWinnerCards => $_getN(34);
  @$pb.TagNumber(35)
  set pendingWinnerCards(WinnerCardsRequest value) => $_setField(35, value);
  @$pb.TagNumber(35)
  $core.bool hasPendingWinnerCards() => $_has(34);
  @$pb.TagNumber(35)
  void clearPendingWinnerCards() => $_clearField(35);
  @$pb.TagNumber(35)
  WinnerCardsRequest ensurePendingWinnerCards() => $_ensure(34);
}

/// WinnerCardsRequest is one outstanding paid request to reveal the sole
/// uncontested winner's mucked hole cards. The requester has already been
/// charged; the winner has until expires_at_unix_ms to accept or decline, and
/// a decline or timeout refunds in full.
class WinnerCardsRequest extends $pb.GeneratedMessage {
  factory WinnerCardsRequest({
    $core.String? requesterId,
    $core.String? requesterName,
    $core.String? winnerId,
    $fixnum.Int64? fee,
    $fixnum.Int64? expiresAtUnixMs,
  }) {
    final result = create();
    if (requesterId != null) result.requesterId = requesterId;
    if (requesterName != null) result.requesterName = requesterName;
    if (winnerId != null) result.winnerId = winnerId;
    if (fee != null) result.fee = fee;
    if (expiresAtUnixMs != null) result.expiresAtUnixMs = expiresAtUnixMs;
    return result;
  }

  WinnerCardsRequest._();

  factory WinnerCardsRequest.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory WinnerCardsRequest.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'WinnerCardsRequest',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'requesterId')
    ..aOS(2, _omitFieldNames ? '' : 'requesterName')
    ..aOS(3, _omitFieldNames ? '' : 'winnerId')
    ..aInt64(4, _omitFieldNames ? '' : 'fee')
    ..aInt64(5, _omitFieldNames ? '' : 'expiresAtUnixMs')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WinnerCardsRequest clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  WinnerCardsRequest copyWith(void Function(WinnerCardsRequest) updates) =>
      super.copyWith((message) => updates(message as WinnerCardsRequest))
          as WinnerCardsRequest;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static WinnerCardsRequest create() => WinnerCardsRequest._();
  @$core.override
  WinnerCardsRequest createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static WinnerCardsRequest getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<WinnerCardsRequest>(create);
  static WinnerCardsRequest? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get requesterId => $_getSZ(0);
  @$pb.TagNumber(1)
  set requesterId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasRequesterId() => $_has(0);
  @$pb.TagNumber(1)
  void clearRequesterId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get requesterName => $_getSZ(1);
  @$pb.TagNumber(2)
  set requesterName($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasRequesterName() => $_has(1);
  @$pb.TagNumber(2)
  void clearRequesterName() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get winnerId => $_getSZ(2);
  @$pb.TagNumber(3)
  set winnerId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasWinnerId() => $_has(2);
  @$pb.TagNumber(3)
  void clearWinnerId() => $_clearField(3);

  @$pb.TagNumber(4)
  $fixnum.Int64 get fee => $_getI64(3);
  @$pb.TagNumber(4)
  set fee($fixnum.Int64 value) => $_setInt64(3, value);
  @$pb.TagNumber(4)
  $core.bool hasFee() => $_has(3);
  @$pb.TagNumber(4)
  void clearFee() => $_clearField(4);

  @$pb.TagNumber(5)
  $fixnum.Int64 get expiresAtUnixMs => $_getI64(4);
  @$pb.TagNumber(5)
  set expiresAtUnixMs($fixnum.Int64 value) => $_setInt64(4, value);
  @$pb.TagNumber(5)
  $core.bool hasExpiresAtUnixMs() => $_has(4);
  @$pb.TagNumber(5)
  void clearExpiresAtUnixMs() => $_clearField(5);
}

class RevealedSalt extends $pb.GeneratedMessage {
  factory RevealedSalt({
    $core.String? card,
    $core.String? saltHex,
  }) {
    final result = create();
    if (card != null) result.card = card;
    if (saltHex != null) result.saltHex = saltHex;
    return result;
  }

  RevealedSalt._();

  factory RevealedSalt.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory RevealedSalt.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'RevealedSalt',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'card')
    ..aOS(2, _omitFieldNames ? '' : 'saltHex')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RevealedSalt clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  RevealedSalt copyWith(void Function(RevealedSalt) updates) =>
      super.copyWith((message) => updates(message as RevealedSalt))
          as RevealedSalt;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static RevealedSalt create() => RevealedSalt._();
  @$core.override
  RevealedSalt createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static RevealedSalt getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<RevealedSalt>(create);
  static RevealedSalt? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get card => $_getSZ(0);
  @$pb.TagNumber(1)
  set card($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasCard() => $_has(0);
  @$pb.TagNumber(1)
  void clearCard() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get saltHex => $_getSZ(1);
  @$pb.TagNumber(2)
  set saltHex($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasSaltHex() => $_has(1);
  @$pb.TagNumber(2)
  void clearSaltHex() => $_clearField(2);
}

/// ClientMessage is sent from the client to the server.
class ClientMessage extends $pb.GeneratedMessage {
  factory ClientMessage({
    $core.String? type,
    $core.String? token,
    $core.String? shareCode,
    $core.bool? ready,
    $core.String? action,
    $fixnum.Int64? amount,
    $core.String? actionId,
    $core.String? message,
    $fixnum.Int64? expectedSnapshotVersion,
    $core.String? expectedHandId,
    $core.int? cardIndex,
    $core.String? reactionId,
    $core.String? targetPlayerId,
    $core.String? turnstileToken,
    $core.bool? runItTwice,
    $core.String? expectedStage,
  }) {
    final result = create();
    if (type != null) result.type = type;
    if (token != null) result.token = token;
    if (shareCode != null) result.shareCode = shareCode;
    if (ready != null) result.ready = ready;
    if (action != null) result.action = action;
    if (amount != null) result.amount = amount;
    if (actionId != null) result.actionId = actionId;
    if (message != null) result.message = message;
    if (expectedSnapshotVersion != null)
      result.expectedSnapshotVersion = expectedSnapshotVersion;
    if (expectedHandId != null) result.expectedHandId = expectedHandId;
    if (cardIndex != null) result.cardIndex = cardIndex;
    if (reactionId != null) result.reactionId = reactionId;
    if (targetPlayerId != null) result.targetPlayerId = targetPlayerId;
    if (turnstileToken != null) result.turnstileToken = turnstileToken;
    if (runItTwice != null) result.runItTwice = runItTwice;
    if (expectedStage != null) result.expectedStage = expectedStage;
    return result;
  }

  ClientMessage._();

  factory ClientMessage.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ClientMessage.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ClientMessage',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'type')
    ..aOS(2, _omitFieldNames ? '' : 'token')
    ..aOS(3, _omitFieldNames ? '' : 'shareCode')
    ..aOB(4, _omitFieldNames ? '' : 'ready')
    ..aOS(5, _omitFieldNames ? '' : 'action')
    ..aInt64(6, _omitFieldNames ? '' : 'amount')
    ..aOS(7, _omitFieldNames ? '' : 'actionId')
    ..aOS(8, _omitFieldNames ? '' : 'message')
    ..a<$fixnum.Int64>(9, _omitFieldNames ? '' : 'expectedSnapshotVersion',
        $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aOS(10, _omitFieldNames ? '' : 'expectedHandId')
    ..aI(11, _omitFieldNames ? '' : 'cardIndex')
    ..aOS(12, _omitFieldNames ? '' : 'reactionId')
    ..aOS(13, _omitFieldNames ? '' : 'targetPlayerId')
    ..aOS(14, _omitFieldNames ? '' : 'turnstileToken')
    ..aOB(15, _omitFieldNames ? '' : 'runItTwice')
    ..aOS(16, _omitFieldNames ? '' : 'expectedStage')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMessage clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ClientMessage copyWith(void Function(ClientMessage) updates) =>
      super.copyWith((message) => updates(message as ClientMessage))
          as ClientMessage;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ClientMessage create() => ClientMessage._();
  @$core.override
  ClientMessage createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ClientMessage getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ClientMessage>(create);
  static ClientMessage? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get type => $_getSZ(0);
  @$pb.TagNumber(1)
  set type($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasType() => $_has(0);
  @$pb.TagNumber(1)
  void clearType() => $_clearField(1);

  /// payload fields
  @$pb.TagNumber(2)
  $core.String get token => $_getSZ(1);
  @$pb.TagNumber(2)
  set token($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasToken() => $_has(1);
  @$pb.TagNumber(2)
  void clearToken() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get shareCode => $_getSZ(2);
  @$pb.TagNumber(3)
  set shareCode($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasShareCode() => $_has(2);
  @$pb.TagNumber(3)
  void clearShareCode() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.bool get ready => $_getBF(3);
  @$pb.TagNumber(4)
  set ready($core.bool value) => $_setBool(3, value);
  @$pb.TagNumber(4)
  $core.bool hasReady() => $_has(3);
  @$pb.TagNumber(4)
  void clearReady() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get action => $_getSZ(4);
  @$pb.TagNumber(5)
  set action($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasAction() => $_has(4);
  @$pb.TagNumber(5)
  void clearAction() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get amount => $_getI64(5);
  @$pb.TagNumber(6)
  set amount($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasAmount() => $_has(5);
  @$pb.TagNumber(6)
  void clearAmount() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get actionId => $_getSZ(6);
  @$pb.TagNumber(7)
  set actionId($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasActionId() => $_has(6);
  @$pb.TagNumber(7)
  void clearActionId() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.String get message => $_getSZ(7);
  @$pb.TagNumber(8)
  set message($core.String value) => $_setString(7, value);
  @$pb.TagNumber(8)
  $core.bool hasMessage() => $_has(7);
  @$pb.TagNumber(8)
  void clearMessage() => $_clearField(8);

  @$pb.TagNumber(9)
  $fixnum.Int64 get expectedSnapshotVersion => $_getI64(8);
  @$pb.TagNumber(9)
  set expectedSnapshotVersion($fixnum.Int64 value) => $_setInt64(8, value);
  @$pb.TagNumber(9)
  $core.bool hasExpectedSnapshotVersion() => $_has(8);
  @$pb.TagNumber(9)
  void clearExpectedSnapshotVersion() => $_clearField(9);

  @$pb.TagNumber(10)
  $core.String get expectedHandId => $_getSZ(9);
  @$pb.TagNumber(10)
  set expectedHandId($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasExpectedHandId() => $_has(9);
  @$pb.TagNumber(10)
  void clearExpectedHandId() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.int get cardIndex => $_getIZ(10);
  @$pb.TagNumber(11)
  set cardIndex($core.int value) => $_setSignedInt32(10, value);
  @$pb.TagNumber(11)
  $core.bool hasCardIndex() => $_has(10);
  @$pb.TagNumber(11)
  void clearCardIndex() => $_clearField(11);

  @$pb.TagNumber(12)
  $core.String get reactionId => $_getSZ(11);
  @$pb.TagNumber(12)
  set reactionId($core.String value) => $_setString(11, value);
  @$pb.TagNumber(12)
  $core.bool hasReactionId() => $_has(11);
  @$pb.TagNumber(12)
  void clearReactionId() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.String get targetPlayerId => $_getSZ(12);
  @$pb.TagNumber(13)
  set targetPlayerId($core.String value) => $_setString(12, value);
  @$pb.TagNumber(13)
  $core.bool hasTargetPlayerId() => $_has(12);
  @$pb.TagNumber(13)
  void clearTargetPlayerId() => $_clearField(13);

  @$pb.TagNumber(14)
  $core.String get turnstileToken => $_getSZ(13);
  @$pb.TagNumber(14)
  set turnstileToken($core.String value) => $_setString(13, value);
  @$pb.TagNumber(14)
  $core.bool hasTurnstileToken() => $_has(13);
  @$pb.TagNumber(14)
  void clearTurnstileToken() => $_clearField(14);

  @$pb.TagNumber(15)
  $core.bool get runItTwice => $_getBF(14);
  @$pb.TagNumber(15)
  set runItTwice($core.bool value) => $_setBool(14, value);
  @$pb.TagNumber(15)
  $core.bool hasRunItTwice() => $_has(14);
  @$pb.TagNumber(15)
  void clearRunItTwice() => $_clearField(15);

  /// Preselections are scoped to one betting street. Unlike a real action they
  /// do not require an exact snapshot version: chat, reactions and unrelated
  /// player actions may advance it without invalidating the viewer's intent.
  @$pb.TagNumber(16)
  $core.String get expectedStage => $_getSZ(15);
  @$pb.TagNumber(16)
  set expectedStage($core.String value) => $_setString(15, value);
  @$pb.TagNumber(16)
  $core.bool hasExpectedStage() => $_has(15);
  @$pb.TagNumber(16)
  void clearExpectedStage() => $_clearField(16);
}

/// ServerMessage is sent from the server to the client.
class ServerMessage extends $pb.GeneratedMessage {
  factory ServerMessage({
    $core.String? type,
    $core.String? connId,
    TableSnapshot? snapshot,
    $core.String? playerId,
    $core.String? message,
    $core.String? code,
    $core.String? key,
    $core.int? stars,
    Room? room,
    $core.String? roomId,
    $core.int? seatsTaken,
    $fixnum.Int64? amount,
    $core.String? text,
    $core.String? actionId,
    $fixnum.Int64? snapshotVersion,
    $core.double? equity,
    $core.String? reactionId,
    $core.String? targetPlayerId,
    $core.String? purchaseId,
    SocialEvent? socialEvent,
    $core.int? unreadCount,
  }) {
    final result = create();
    if (type != null) result.type = type;
    if (connId != null) result.connId = connId;
    if (snapshot != null) result.snapshot = snapshot;
    if (playerId != null) result.playerId = playerId;
    if (message != null) result.message = message;
    if (code != null) result.code = code;
    if (key != null) result.key = key;
    if (stars != null) result.stars = stars;
    if (room != null) result.room = room;
    if (roomId != null) result.roomId = roomId;
    if (seatsTaken != null) result.seatsTaken = seatsTaken;
    if (amount != null) result.amount = amount;
    if (text != null) result.text = text;
    if (actionId != null) result.actionId = actionId;
    if (snapshotVersion != null) result.snapshotVersion = snapshotVersion;
    if (equity != null) result.equity = equity;
    if (reactionId != null) result.reactionId = reactionId;
    if (targetPlayerId != null) result.targetPlayerId = targetPlayerId;
    if (purchaseId != null) result.purchaseId = purchaseId;
    if (socialEvent != null) result.socialEvent = socialEvent;
    if (unreadCount != null) result.unreadCount = unreadCount;
    return result;
  }

  ServerMessage._();

  factory ServerMessage.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory ServerMessage.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'ServerMessage',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'type')
    ..aOS(2, _omitFieldNames ? '' : 'connId')
    ..aOM<TableSnapshot>(3, _omitFieldNames ? '' : 'snapshot',
        subBuilder: TableSnapshot.create)
    ..aOS(4, _omitFieldNames ? '' : 'playerId')
    ..aOS(5, _omitFieldNames ? '' : 'message')
    ..aOS(6, _omitFieldNames ? '' : 'code')
    ..aOS(7, _omitFieldNames ? '' : 'key')
    ..aI(8, _omitFieldNames ? '' : 'stars')
    ..aOM<Room>(9, _omitFieldNames ? '' : 'room', subBuilder: Room.create)
    ..aOS(10, _omitFieldNames ? '' : 'roomId')
    ..aI(11, _omitFieldNames ? '' : 'seatsTaken')
    ..aInt64(12, _omitFieldNames ? '' : 'amount')
    ..aOS(13, _omitFieldNames ? '' : 'text')
    ..aOS(14, _omitFieldNames ? '' : 'actionId')
    ..a<$fixnum.Int64>(
        15, _omitFieldNames ? '' : 'snapshotVersion', $pb.PbFieldType.OU6,
        defaultOrMaker: $fixnum.Int64.ZERO)
    ..aD(16, _omitFieldNames ? '' : 'equity')
    ..aOS(17, _omitFieldNames ? '' : 'reactionId')
    ..aOS(18, _omitFieldNames ? '' : 'targetPlayerId')
    ..aOS(19, _omitFieldNames ? '' : 'purchaseId')
    ..aOM<SocialEvent>(20, _omitFieldNames ? '' : 'socialEvent',
        subBuilder: SocialEvent.create)
    ..aI(21, _omitFieldNames ? '' : 'unreadCount')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerMessage clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  ServerMessage copyWith(void Function(ServerMessage) updates) =>
      super.copyWith((message) => updates(message as ServerMessage))
          as ServerMessage;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static ServerMessage create() => ServerMessage._();
  @$core.override
  ServerMessage createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static ServerMessage getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<ServerMessage>(create);
  static ServerMessage? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get type => $_getSZ(0);
  @$pb.TagNumber(1)
  set type($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasType() => $_has(0);
  @$pb.TagNumber(1)
  void clearType() => $_clearField(1);

  /// payload fields
  @$pb.TagNumber(2)
  $core.String get connId => $_getSZ(1);
  @$pb.TagNumber(2)
  set connId($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasConnId() => $_has(1);
  @$pb.TagNumber(2)
  void clearConnId() => $_clearField(2);

  @$pb.TagNumber(3)
  TableSnapshot get snapshot => $_getN(2);
  @$pb.TagNumber(3)
  set snapshot(TableSnapshot value) => $_setField(3, value);
  @$pb.TagNumber(3)
  $core.bool hasSnapshot() => $_has(2);
  @$pb.TagNumber(3)
  void clearSnapshot() => $_clearField(3);
  @$pb.TagNumber(3)
  TableSnapshot ensureSnapshot() => $_ensure(2);

  @$pb.TagNumber(4)
  $core.String get playerId => $_getSZ(3);
  @$pb.TagNumber(4)
  set playerId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasPlayerId() => $_has(3);
  @$pb.TagNumber(4)
  void clearPlayerId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get message => $_getSZ(4);
  @$pb.TagNumber(5)
  set message($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasMessage() => $_has(4);
  @$pb.TagNumber(5)
  void clearMessage() => $_clearField(5);

  @$pb.TagNumber(6)
  $core.String get code => $_getSZ(5);
  @$pb.TagNumber(6)
  set code($core.String value) => $_setString(5, value);
  @$pb.TagNumber(6)
  $core.bool hasCode() => $_has(5);
  @$pb.TagNumber(6)
  void clearCode() => $_clearField(6);

  @$pb.TagNumber(7)
  $core.String get key => $_getSZ(6);
  @$pb.TagNumber(7)
  set key($core.String value) => $_setString(6, value);
  @$pb.TagNumber(7)
  $core.bool hasKey() => $_has(6);
  @$pb.TagNumber(7)
  void clearKey() => $_clearField(7);

  @$pb.TagNumber(8)
  $core.int get stars => $_getIZ(7);
  @$pb.TagNumber(8)
  set stars($core.int value) => $_setSignedInt32(7, value);
  @$pb.TagNumber(8)
  $core.bool hasStars() => $_has(7);
  @$pb.TagNumber(8)
  void clearStars() => $_clearField(8);

  /// lobby payload fields
  @$pb.TagNumber(9)
  Room get room => $_getN(8);
  @$pb.TagNumber(9)
  set room(Room value) => $_setField(9, value);
  @$pb.TagNumber(9)
  $core.bool hasRoom() => $_has(8);
  @$pb.TagNumber(9)
  void clearRoom() => $_clearField(9);
  @$pb.TagNumber(9)
  Room ensureRoom() => $_ensure(8);

  @$pb.TagNumber(10)
  $core.String get roomId => $_getSZ(9);
  @$pb.TagNumber(10)
  set roomId($core.String value) => $_setString(9, value);
  @$pb.TagNumber(10)
  $core.bool hasRoomId() => $_has(9);
  @$pb.TagNumber(10)
  void clearRoomId() => $_clearField(10);

  @$pb.TagNumber(11)
  $core.int get seatsTaken => $_getIZ(10);
  @$pb.TagNumber(11)
  set seatsTaken($core.int value) => $_setSignedInt32(10, value);
  @$pb.TagNumber(11)
  $core.bool hasSeatsTaken() => $_has(10);
  @$pb.TagNumber(11)
  void clearSeatsTaken() => $_clearField(11);

  @$pb.TagNumber(12)
  $fixnum.Int64 get amount => $_getI64(11);
  @$pb.TagNumber(12)
  set amount($fixnum.Int64 value) => $_setInt64(11, value);
  @$pb.TagNumber(12)
  $core.bool hasAmount() => $_has(11);
  @$pb.TagNumber(12)
  void clearAmount() => $_clearField(12);

  @$pb.TagNumber(13)
  $core.String get text => $_getSZ(12);
  @$pb.TagNumber(13)
  set text($core.String value) => $_setString(12, value);
  @$pb.TagNumber(13)
  $core.bool hasText() => $_has(12);
  @$pb.TagNumber(13)
  void clearText() => $_clearField(13);

  @$pb.TagNumber(14)
  $core.String get actionId => $_getSZ(13);
  @$pb.TagNumber(14)
  set actionId($core.String value) => $_setString(13, value);
  @$pb.TagNumber(14)
  $core.bool hasActionId() => $_has(13);
  @$pb.TagNumber(14)
  void clearActionId() => $_clearField(14);

  @$pb.TagNumber(15)
  $fixnum.Int64 get snapshotVersion => $_getI64(14);
  @$pb.TagNumber(15)
  set snapshotVersion($fixnum.Int64 value) => $_setInt64(14, value);
  @$pb.TagNumber(15)
  $core.bool hasSnapshotVersion() => $_has(14);
  @$pb.TagNumber(15)
  void clearSnapshotVersion() => $_clearField(15);

  @$pb.TagNumber(16)
  $core.double get equity => $_getN(15);
  @$pb.TagNumber(16)
  set equity($core.double value) => $_setDouble(15, value);
  @$pb.TagNumber(16)
  $core.bool hasEquity() => $_has(15);
  @$pb.TagNumber(16)
  void clearEquity() => $_clearField(16);

  @$pb.TagNumber(17)
  $core.String get reactionId => $_getSZ(16);
  @$pb.TagNumber(17)
  set reactionId($core.String value) => $_setString(16, value);
  @$pb.TagNumber(17)
  $core.bool hasReactionId() => $_has(16);
  @$pb.TagNumber(17)
  void clearReactionId() => $_clearField(17);

  @$pb.TagNumber(18)
  $core.String get targetPlayerId => $_getSZ(17);
  @$pb.TagNumber(18)
  set targetPlayerId($core.String value) => $_setString(17, value);
  @$pb.TagNumber(18)
  $core.bool hasTargetPlayerId() => $_has(17);
  @$pb.TagNumber(18)
  void clearTargetPlayerId() => $_clearField(18);

  @$pb.TagNumber(19)
  $core.String get purchaseId => $_getSZ(18);
  @$pb.TagNumber(19)
  set purchaseId($core.String value) => $_setString(18, value);
  @$pb.TagNumber(19)
  $core.bool hasPurchaseId() => $_has(18);
  @$pb.TagNumber(19)
  void clearPurchaseId() => $_clearField(19);

  @$pb.TagNumber(20)
  SocialEvent get socialEvent => $_getN(19);
  @$pb.TagNumber(20)
  set socialEvent(SocialEvent value) => $_setField(20, value);
  @$pb.TagNumber(20)
  $core.bool hasSocialEvent() => $_has(19);
  @$pb.TagNumber(20)
  void clearSocialEvent() => $_clearField(20);
  @$pb.TagNumber(20)
  SocialEvent ensureSocialEvent() => $_ensure(19);

  @$pb.TagNumber(21)
  $core.int get unreadCount => $_getIZ(20);
  @$pb.TagNumber(21)
  set unreadCount($core.int value) => $_setSignedInt32(20, value);
  @$pb.TagNumber(21)
  $core.bool hasUnreadCount() => $_has(20);
  @$pb.TagNumber(21)
  void clearUnreadCount() => $_clearField(21);
}

/// Friend-visible presence. status is one of offline | online | in_table.
/// room_id is set only when the recipient's opt-in and the room's own
/// visibility both allow it (see internal/presence and internal/api/v1/social.go
/// joinableRoomIDs) — never published unconditionally.
class PlayerPresence extends $pb.GeneratedMessage {
  factory PlayerPresence({
    $core.String? playerId,
    $core.String? status,
    $core.String? roomId,
  }) {
    final result = create();
    if (playerId != null) result.playerId = playerId;
    if (status != null) result.status = status;
    if (roomId != null) result.roomId = roomId;
    return result;
  }

  PlayerPresence._();

  factory PlayerPresence.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory PlayerPresence.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'PlayerPresence',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'playerId')
    ..aOS(2, _omitFieldNames ? '' : 'status')
    ..aOS(3, _omitFieldNames ? '' : 'roomId')
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PlayerPresence clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  PlayerPresence copyWith(void Function(PlayerPresence) updates) =>
      super.copyWith((message) => updates(message as PlayerPresence))
          as PlayerPresence;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static PlayerPresence create() => PlayerPresence._();
  @$core.override
  PlayerPresence createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static PlayerPresence getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<PlayerPresence>(create);
  static PlayerPresence? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get playerId => $_getSZ(0);
  @$pb.TagNumber(1)
  set playerId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasPlayerId() => $_has(0);
  @$pb.TagNumber(1)
  void clearPlayerId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get status => $_getSZ(1);
  @$pb.TagNumber(2)
  set status($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasStatus() => $_has(1);
  @$pb.TagNumber(2)
  void clearStatus() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get roomId => $_getSZ(2);
  @$pb.TagNumber(3)
  set roomId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasRoomId() => $_has(2);
  @$pb.TagNumber(3)
  void clearRoomId() => $_clearField(3);
}

/// Push notification/invalidation payload for the in-app social inbox. HTTP
/// remains authoritative for mutations and full list pagination.
class SocialEvent extends $pb.GeneratedMessage {
  factory SocialEvent({
    $core.String? eventId,
    $core.String? type,
    $core.String? actorId,
    $core.String? roomId,
    $core.String? status,
    $fixnum.Int64? createdAt,
    $fixnum.Int64? expiresAt,
    PlayerPresence? presence,
  }) {
    final result = create();
    if (eventId != null) result.eventId = eventId;
    if (type != null) result.type = type;
    if (actorId != null) result.actorId = actorId;
    if (roomId != null) result.roomId = roomId;
    if (status != null) result.status = status;
    if (createdAt != null) result.createdAt = createdAt;
    if (expiresAt != null) result.expiresAt = expiresAt;
    if (presence != null) result.presence = presence;
    return result;
  }

  SocialEvent._();

  factory SocialEvent.fromBuffer($core.List<$core.int> data,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromBuffer(data, registry);
  factory SocialEvent.fromJson($core.String json,
          [$pb.ExtensionRegistry registry = $pb.ExtensionRegistry.EMPTY]) =>
      create()..mergeFromJson(json, registry);

  static final $pb.BuilderInfo _i = $pb.BuilderInfo(
      _omitMessageNames ? '' : 'SocialEvent',
      package: const $pb.PackageName(_omitMessageNames ? '' : 'poker'),
      createEmptyInstance: create)
    ..aOS(1, _omitFieldNames ? '' : 'eventId')
    ..aOS(2, _omitFieldNames ? '' : 'type')
    ..aOS(3, _omitFieldNames ? '' : 'actorId')
    ..aOS(4, _omitFieldNames ? '' : 'roomId')
    ..aOS(5, _omitFieldNames ? '' : 'status')
    ..aInt64(6, _omitFieldNames ? '' : 'createdAt')
    ..aInt64(7, _omitFieldNames ? '' : 'expiresAt')
    ..aOM<PlayerPresence>(8, _omitFieldNames ? '' : 'presence',
        subBuilder: PlayerPresence.create)
    ..hasRequiredFields = false;

  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SocialEvent clone() => deepCopy();
  @$core.Deprecated('See https://github.com/google/protobuf.dart/issues/998.')
  SocialEvent copyWith(void Function(SocialEvent) updates) =>
      super.copyWith((message) => updates(message as SocialEvent))
          as SocialEvent;

  @$core.override
  $pb.BuilderInfo get info_ => _i;

  @$core.pragma('dart2js:noInline')
  static SocialEvent create() => SocialEvent._();
  @$core.override
  SocialEvent createEmptyInstance() => create();
  @$core.pragma('dart2js:noInline')
  static SocialEvent getDefault() => _defaultInstance ??=
      $pb.GeneratedMessage.$_defaultFor<SocialEvent>(create);
  static SocialEvent? _defaultInstance;

  @$pb.TagNumber(1)
  $core.String get eventId => $_getSZ(0);
  @$pb.TagNumber(1)
  set eventId($core.String value) => $_setString(0, value);
  @$pb.TagNumber(1)
  $core.bool hasEventId() => $_has(0);
  @$pb.TagNumber(1)
  void clearEventId() => $_clearField(1);

  @$pb.TagNumber(2)
  $core.String get type => $_getSZ(1);
  @$pb.TagNumber(2)
  set type($core.String value) => $_setString(1, value);
  @$pb.TagNumber(2)
  $core.bool hasType() => $_has(1);
  @$pb.TagNumber(2)
  void clearType() => $_clearField(2);

  @$pb.TagNumber(3)
  $core.String get actorId => $_getSZ(2);
  @$pb.TagNumber(3)
  set actorId($core.String value) => $_setString(2, value);
  @$pb.TagNumber(3)
  $core.bool hasActorId() => $_has(2);
  @$pb.TagNumber(3)
  void clearActorId() => $_clearField(3);

  @$pb.TagNumber(4)
  $core.String get roomId => $_getSZ(3);
  @$pb.TagNumber(4)
  set roomId($core.String value) => $_setString(3, value);
  @$pb.TagNumber(4)
  $core.bool hasRoomId() => $_has(3);
  @$pb.TagNumber(4)
  void clearRoomId() => $_clearField(4);

  @$pb.TagNumber(5)
  $core.String get status => $_getSZ(4);
  @$pb.TagNumber(5)
  set status($core.String value) => $_setString(4, value);
  @$pb.TagNumber(5)
  $core.bool hasStatus() => $_has(4);
  @$pb.TagNumber(5)
  void clearStatus() => $_clearField(5);

  @$pb.TagNumber(6)
  $fixnum.Int64 get createdAt => $_getI64(5);
  @$pb.TagNumber(6)
  set createdAt($fixnum.Int64 value) => $_setInt64(5, value);
  @$pb.TagNumber(6)
  $core.bool hasCreatedAt() => $_has(5);
  @$pb.TagNumber(6)
  void clearCreatedAt() => $_clearField(6);

  @$pb.TagNumber(7)
  $fixnum.Int64 get expiresAt => $_getI64(6);
  @$pb.TagNumber(7)
  set expiresAt($fixnum.Int64 value) => $_setInt64(6, value);
  @$pb.TagNumber(7)
  $core.bool hasExpiresAt() => $_has(6);
  @$pb.TagNumber(7)
  void clearExpiresAt() => $_clearField(7);

  @$pb.TagNumber(8)
  PlayerPresence get presence => $_getN(7);
  @$pb.TagNumber(8)
  set presence(PlayerPresence value) => $_setField(8, value);
  @$pb.TagNumber(8)
  $core.bool hasPresence() => $_has(7);
  @$pb.TagNumber(8)
  void clearPresence() => $_clearField(8);
  @$pb.TagNumber(8)
  PlayerPresence ensurePresence() => $_ensure(7);
}

const $core.bool _omitFieldNames =
    $core.bool.fromEnvironment('protobuf.omit_field_names');
const $core.bool _omitMessageNames =
    $core.bool.fromEnvironment('protobuf.omit_message_names');
