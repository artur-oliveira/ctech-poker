// This is a generated file - do not edit.
//
// Generated from poker.proto.

// @dart = 3.3

// ignore_for_file: annotate_overrides, camel_case_types, comment_references
// ignore_for_file: constant_identifier_names
// ignore_for_file: curly_braces_in_flow_control_structures
// ignore_for_file: deprecated_member_use_from_same_package, library_prefixes
// ignore_for_file: non_constant_identifier_names, prefer_relative_imports
// ignore_for_file: unused_import

import 'dart:convert' as $convert;
import 'dart:core' as $core;
import 'dart:typed_data' as $typed_data;

@$core.Deprecated('Use cardDescriptor instead')
const Card$json = {
  '1': 'Card',
  '2': [
    {'1': 'rank', '3': 1, '4': 1, '5': 9, '10': 'rank'},
    {'1': 'suit', '3': 2, '4': 1, '5': 9, '10': 'suit'},
  ],
};

/// Descriptor for `Card`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List cardDescriptor = $convert.base64Decode(
    'CgRDYXJkEhIKBHJhbmsYASABKAlSBHJhbmsSEgoEc3VpdBgCIAEoCVIEc3VpdA==');

@$core.Deprecated('Use seatDescriptor instead')
const Seat$json = {
  '1': 'Seat',
  '2': [
    {'1': 'player_id', '3': 1, '4': 1, '5': 9, '10': 'playerId'},
    {'1': 'name', '3': 2, '4': 1, '5': 9, '10': 'name'},
    {'1': 'stack', '3': 3, '4': 1, '5': 3, '10': 'stack'},
    {'1': 'state', '3': 4, '4': 1, '5': 9, '10': 'state'},
    {'1': 'contributed', '3': 5, '4': 1, '5': 3, '10': 'contributed'},
    {'1': 'hole_cards', '3': 6, '4': 3, '5': 9, '10': 'holeCards'},
    {'1': 'equity', '3': 7, '4': 1, '5': 1, '9': 0, '10': 'equity', '17': true},
    {'1': 'hand_category', '3': 8, '4': 1, '5': 9, '10': 'handCategory'},
    {'1': 'connection_state', '3': 9, '4': 1, '5': 9, '10': 'connectionState'},
    {
      '1': 'dealt_in',
      '3': 10,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'dealtIn',
      '17': true
    },
    {'1': 'ready', '3': 11, '4': 1, '5': 8, '9': 2, '10': 'ready', '17': true},
    {
      '1': 'hole_cards_revealed',
      '3': 12,
      '4': 3,
      '5': 8,
      '10': 'holeCardsRevealed'
    },
    {
      '1': 'stack_at_hand_start',
      '3': 13,
      '4': 1,
      '5': 3,
      '9': 3,
      '10': 'stackAtHandStart',
      '17': true
    },
    {'1': 'time_bank_ms', '3': 14, '4': 1, '5': 3, '10': 'timeBankMs'},
    {'1': 'hand_score', '3': 15, '4': 1, '5': 13, '10': 'handScore'},
    {
      '1': 'avatar_url',
      '3': 16,
      '4': 1,
      '5': 9,
      '9': 4,
      '10': 'avatarUrl',
      '17': true
    },
    {
      '1': 'playstyle_badge',
      '3': 17,
      '4': 1,
      '5': 9,
      '9': 5,
      '10': 'playstyleBadge',
      '17': true
    },
    {
      '1': 'run_it_twice',
      '3': 18,
      '4': 1,
      '5': 8,
      '9': 6,
      '10': 'runItTwice',
      '17': true
    },
    {
      '1': 'auto_rebuy',
      '3': 19,
      '4': 1,
      '5': 8,
      '9': 7,
      '10': 'autoRebuy',
      '17': true
    },
    {'1': 'current_streak', '3': 20, '4': 1, '5': 5, '10': 'currentStreak'},
    {
      '1': 'pending_exit',
      '3': 21,
      '4': 1,
      '5': 8,
      '9': 8,
      '10': 'pendingExit',
      '17': true
    },
  ],
  '8': [
    {'1': '_equity'},
    {'1': '_dealt_in'},
    {'1': '_ready'},
    {'1': '_stack_at_hand_start'},
    {'1': '_avatar_url'},
    {'1': '_playstyle_badge'},
    {'1': '_run_it_twice'},
    {'1': '_auto_rebuy'},
    {'1': '_pending_exit'},
  ],
};

/// Descriptor for `Seat`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List seatDescriptor = $convert.base64Decode(
    'CgRTZWF0EhsKCXBsYXllcl9pZBgBIAEoCVIIcGxheWVySWQSEgoEbmFtZRgCIAEoCVIEbmFtZR'
    'IUCgVzdGFjaxgDIAEoA1IFc3RhY2sSFAoFc3RhdGUYBCABKAlSBXN0YXRlEiAKC2NvbnRyaWJ1'
    'dGVkGAUgASgDUgtjb250cmlidXRlZBIdCgpob2xlX2NhcmRzGAYgAygJUglob2xlQ2FyZHMSGw'
    'oGZXF1aXR5GAcgASgBSABSBmVxdWl0eYgBARIjCg1oYW5kX2NhdGVnb3J5GAggASgJUgxoYW5k'
    'Q2F0ZWdvcnkSKQoQY29ubmVjdGlvbl9zdGF0ZRgJIAEoCVIPY29ubmVjdGlvblN0YXRlEh4KCG'
    'RlYWx0X2luGAogASgISAFSB2RlYWx0SW6IAQESGQoFcmVhZHkYCyABKAhIAlIFcmVhZHmIAQES'
    'LgoTaG9sZV9jYXJkc19yZXZlYWxlZBgMIAMoCFIRaG9sZUNhcmRzUmV2ZWFsZWQSMgoTc3RhY2'
    'tfYXRfaGFuZF9zdGFydBgNIAEoA0gDUhBzdGFja0F0SGFuZFN0YXJ0iAEBEiAKDHRpbWVfYmFu'
    'a19tcxgOIAEoA1IKdGltZUJhbmtNcxIdCgpoYW5kX3Njb3JlGA8gASgNUgloYW5kU2NvcmUSIg'
    'oKYXZhdGFyX3VybBgQIAEoCUgEUglhdmF0YXJVcmyIAQESLAoPcGxheXN0eWxlX2JhZGdlGBEg'
    'ASgJSAVSDnBsYXlzdHlsZUJhZGdliAEBEiUKDHJ1bl9pdF90d2ljZRgSIAEoCEgGUgpydW5JdF'
    'R3aWNliAEBEiIKCmF1dG9fcmVidXkYEyABKAhIB1IJYXV0b1JlYnV5iAEBEiUKDmN1cnJlbnRf'
    'c3RyZWFrGBQgASgFUg1jdXJyZW50U3RyZWFrEiYKDHBlbmRpbmdfZXhpdBgVIAEoCEgIUgtwZW'
    '5kaW5nRXhpdIgBAUIJCgdfZXF1aXR5QgsKCV9kZWFsdF9pbkIICgZfcmVhZHlCFgoUX3N0YWNr'
    'X2F0X2hhbmRfc3RhcnRCDQoLX2F2YXRhcl91cmxCEgoQX3BsYXlzdHlsZV9iYWRnZUIPCg1fcn'
    'VuX2l0X3R3aWNlQg0KC19hdXRvX3JlYnV5Qg8KDV9wZW5kaW5nX2V4aXQ=');

@$core.Deprecated('Use blindEscalationDescriptor instead')
const BlindEscalation$json = {
  '1': 'BlindEscalation',
  '2': [
    {'1': 'interval_minutes', '3': 1, '4': 1, '5': 5, '10': 'intervalMinutes'},
    {'1': 'multiplier', '3': 2, '4': 1, '5': 5, '10': 'multiplier'},
    {'1': 'max', '3': 3, '4': 1, '5': 3, '10': 'max'},
  ],
};

/// Descriptor for `BlindEscalation`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List blindEscalationDescriptor = $convert.base64Decode(
    'Cg9CbGluZEVzY2FsYXRpb24SKQoQaW50ZXJ2YWxfbWludXRlcxgBIAEoBVIPaW50ZXJ2YWxNaW'
    '51dGVzEh4KCm11bHRpcGxpZXIYAiABKAVSCm11bHRpcGxpZXISEAoDbWF4GAMgASgDUgNtYXg=');

@$core.Deprecated('Use roomDescriptor instead')
const Room$json = {
  '1': 'Room',
  '2': [
    {'1': 'room_id', '3': 1, '4': 1, '5': 9, '10': 'roomId'},
    {'1': 'visibility', '3': 2, '4': 1, '5': 9, '10': 'visibility'},
    {'1': 'currency_mode', '3': 3, '4': 1, '5': 9, '10': 'currencyMode'},
    {'1': 'small_blind', '3': 4, '4': 1, '5': 3, '10': 'smallBlind'},
    {'1': 'big_blind', '3': 5, '4': 1, '5': 3, '10': 'bigBlind'},
    {'1': 'max_seats', '3': 6, '4': 1, '5': 5, '10': 'maxSeats'},
    {'1': 'buy_in_min', '3': 7, '4': 1, '5': 3, '10': 'buyInMin'},
    {'1': 'buy_in_max', '3': 8, '4': 1, '5': 3, '10': 'buyInMax'},
    {'1': 'entry_fee_cents', '3': 9, '4': 1, '5': 3, '10': 'entryFeeCents'},
    {'1': 'share_code', '3': 10, '4': 1, '5': 9, '10': 'shareCode'},
    {
      '1': 'blind_escalation',
      '3': 11,
      '4': 1,
      '5': 11,
      '6': '.poker.BlindEscalation',
      '10': 'blindEscalation'
    },
    {
      '1': 'turn_timeout_seconds',
      '3': 12,
      '4': 1,
      '5': 5,
      '10': 'turnTimeoutSeconds'
    },
    {
      '1': 'equity_display_enabled',
      '3': 13,
      '4': 1,
      '5': 8,
      '10': 'equityDisplayEnabled'
    },
    {'1': 'status', '3': 14, '4': 1, '5': 9, '10': 'status'},
    {'1': 'seats_taken', '3': 15, '4': 1, '5': 5, '10': 'seatsTaken'},
    {'1': 'created_by', '3': 16, '4': 1, '5': 9, '10': 'createdBy'},
    {'1': 'created_at', '3': 17, '4': 1, '5': 9, '10': 'createdAt'},
    {
      '1': 'run_it_twice_enabled',
      '3': 18,
      '4': 1,
      '5': 8,
      '10': 'runItTwiceEnabled'
    },
  ],
};

/// Descriptor for `Room`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List roomDescriptor = $convert.base64Decode(
    'CgRSb29tEhcKB3Jvb21faWQYASABKAlSBnJvb21JZBIeCgp2aXNpYmlsaXR5GAIgASgJUgp2aX'
    'NpYmlsaXR5EiMKDWN1cnJlbmN5X21vZGUYAyABKAlSDGN1cnJlbmN5TW9kZRIfCgtzbWFsbF9i'
    'bGluZBgEIAEoA1IKc21hbGxCbGluZBIbCgliaWdfYmxpbmQYBSABKANSCGJpZ0JsaW5kEhsKCW'
    '1heF9zZWF0cxgGIAEoBVIIbWF4U2VhdHMSHAoKYnV5X2luX21pbhgHIAEoA1IIYnV5SW5NaW4S'
    'HAoKYnV5X2luX21heBgIIAEoA1IIYnV5SW5NYXgSJgoPZW50cnlfZmVlX2NlbnRzGAkgASgDUg'
    '1lbnRyeUZlZUNlbnRzEh0KCnNoYXJlX2NvZGUYCiABKAlSCXNoYXJlQ29kZRJBChBibGluZF9l'
    'c2NhbGF0aW9uGAsgASgLMhYucG9rZXIuQmxpbmRFc2NhbGF0aW9uUg9ibGluZEVzY2FsYXRpb2'
    '4SMAoUdHVybl90aW1lb3V0X3NlY29uZHMYDCABKAVSEnR1cm5UaW1lb3V0U2Vjb25kcxI0ChZl'
    'cXVpdHlfZGlzcGxheV9lbmFibGVkGA0gASgIUhRlcXVpdHlEaXNwbGF5RW5hYmxlZBIWCgZzdG'
    'F0dXMYDiABKAlSBnN0YXR1cxIfCgtzZWF0c190YWtlbhgPIAEoBVIKc2VhdHNUYWtlbhIdCgpj'
    'cmVhdGVkX2J5GBAgASgJUgljcmVhdGVkQnkSHQoKY3JlYXRlZF9hdBgRIAEoCVIJY3JlYXRlZE'
    'F0Ei8KFHJ1bl9pdF90d2ljZV9lbmFibGVkGBIgASgIUhFydW5JdFR3aWNlRW5hYmxlZA==');

@$core.Deprecated('Use legalActionsDescriptor instead')
const LegalActions$json = {
  '1': 'LegalActions',
  '2': [
    {'1': 'actions', '3': 1, '4': 3, '5': 9, '10': 'actions'},
    {'1': 'call_amount', '3': 2, '4': 1, '5': 3, '10': 'callAmount'},
    {'1': 'min_raise_to', '3': 3, '4': 1, '5': 3, '10': 'minRaiseTo'},
    {'1': 'max_raise_to', '3': 4, '4': 1, '5': 3, '10': 'maxRaiseTo'},
    {'1': 'step', '3': 5, '4': 1, '5': 3, '10': 'step'},
    {
      '1': 'current_contribution',
      '3': 6,
      '4': 1,
      '5': 3,
      '10': 'currentContribution'
    },
    {'1': 'current_bet', '3': 7, '4': 1, '5': 3, '10': 'currentBet'},
    {
      '1': 'one_third_pot_raise_to',
      '3': 8,
      '4': 1,
      '5': 3,
      '10': 'oneThirdPotRaiseTo'
    },
    {'1': 'half_pot_raise_to', '3': 9, '4': 1, '5': 3, '10': 'halfPotRaiseTo'},
    {
      '1': 'two_thirds_pot_raise_to',
      '3': 10,
      '4': 1,
      '5': 3,
      '10': 'twoThirdsPotRaiseTo'
    },
    {'1': 'pot_raise_to', '3': 11, '4': 1, '5': 3, '10': 'potRaiseTo'},
  ],
};

/// Descriptor for `LegalActions`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List legalActionsDescriptor = $convert.base64Decode(
    'CgxMZWdhbEFjdGlvbnMSGAoHYWN0aW9ucxgBIAMoCVIHYWN0aW9ucxIfCgtjYWxsX2Ftb3VudB'
    'gCIAEoA1IKY2FsbEFtb3VudBIgCgxtaW5fcmFpc2VfdG8YAyABKANSCm1pblJhaXNlVG8SIAoM'
    'bWF4X3JhaXNlX3RvGAQgASgDUgptYXhSYWlzZVRvEhIKBHN0ZXAYBSABKANSBHN0ZXASMQoUY3'
    'VycmVudF9jb250cmlidXRpb24YBiABKANSE2N1cnJlbnRDb250cmlidXRpb24SHwoLY3VycmVu'
    'dF9iZXQYByABKANSCmN1cnJlbnRCZXQSMgoWb25lX3RoaXJkX3BvdF9yYWlzZV90bxgIIAEoA1'
    'ISb25lVGhpcmRQb3RSYWlzZVRvEikKEWhhbGZfcG90X3JhaXNlX3RvGAkgASgDUg5oYWxmUG90'
    'UmFpc2VUbxI0Chd0d29fdGhpcmRzX3BvdF9yYWlzZV90bxgKIAEoA1ITdHdvVGhpcmRzUG90Um'
    'Fpc2VUbxIgCgxwb3RfcmFpc2VfdG8YCyABKANSCnBvdFJhaXNlVG8=');

@$core.Deprecated('Use potDescriptor instead')
const Pot$json = {
  '1': 'Pot',
  '2': [
    {'1': 'amount', '3': 1, '4': 1, '5': 3, '10': 'amount'},
    {
      '1': 'eligible_player_ids',
      '3': 2,
      '4': 3,
      '5': 9,
      '10': 'eligiblePlayerIds'
    },
  ],
};

/// Descriptor for `Pot`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List potDescriptor = $convert.base64Decode(
    'CgNQb3QSFgoGYW1vdW50GAEgASgDUgZhbW91bnQSLgoTZWxpZ2libGVfcGxheWVyX2lkcxgCIA'
    'MoCVIRZWxpZ2libGVQbGF5ZXJJZHM=');

@$core.Deprecated('Use potResultDescriptor instead')
const PotResult$json = {
  '1': 'PotResult',
  '2': [
    {'1': 'amount', '3': 1, '4': 1, '5': 3, '10': 'amount'},
    {'1': 'payout_amount', '3': 2, '4': 1, '5': 3, '10': 'payoutAmount'},
    {
      '1': 'eligible_player_ids',
      '3': 3,
      '4': 3,
      '5': 9,
      '10': 'eligiblePlayerIds'
    },
    {'1': 'winner_player_ids', '3': 4, '4': 3, '5': 9, '10': 'winnerPlayerIds'},
    {
      '1': 'payouts',
      '3': 5,
      '4': 3,
      '5': 11,
      '6': '.poker.PotResult.PayoutsEntry',
      '10': 'payouts'
    },
    {'1': 'refund', '3': 6, '4': 1, '5': 8, '10': 'refund'},
    {'1': 'runout', '3': 7, '4': 1, '5': 5, '10': 'runout'},
  ],
  '3': [PotResult_PayoutsEntry$json],
};

@$core.Deprecated('Use potResultDescriptor instead')
const PotResult_PayoutsEntry$json = {
  '1': 'PayoutsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 3, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `PotResult`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List potResultDescriptor = $convert.base64Decode(
    'CglQb3RSZXN1bHQSFgoGYW1vdW50GAEgASgDUgZhbW91bnQSIwoNcGF5b3V0X2Ftb3VudBgCIA'
    'EoA1IMcGF5b3V0QW1vdW50Ei4KE2VsaWdpYmxlX3BsYXllcl9pZHMYAyADKAlSEWVsaWdpYmxl'
    'UGxheWVySWRzEioKEXdpbm5lcl9wbGF5ZXJfaWRzGAQgAygJUg93aW5uZXJQbGF5ZXJJZHMSNw'
    'oHcGF5b3V0cxgFIAMoCzIdLnBva2VyLlBvdFJlc3VsdC5QYXlvdXRzRW50cnlSB3BheW91dHMS'
    'FgoGcmVmdW5kGAYgASgIUgZyZWZ1bmQSFgoGcnVub3V0GAcgASgFUgZydW5vdXQaOgoMUGF5b3'
    'V0c0VudHJ5EhAKA2tleRgBIAEoCVIDa2V5EhQKBXZhbHVlGAIgASgDUgV2YWx1ZToCOAE=');

@$core.Deprecated('Use chatMessageDescriptor instead')
const ChatMessage$json = {
  '1': 'ChatMessage',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'player_id', '3': 2, '4': 1, '5': 9, '10': 'playerId'},
    {'1': 'message', '3': 3, '4': 1, '5': 9, '10': 'message'},
    {'1': 'timestamp', '3': 4, '4': 1, '5': 3, '10': 'timestamp'},
  ],
};

/// Descriptor for `ChatMessage`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List chatMessageDescriptor = $convert.base64Decode(
    'CgtDaGF0TWVzc2FnZRIOCgJpZBgBIAEoCVICaWQSGwoJcGxheWVyX2lkGAIgASgJUghwbGF5ZX'
    'JJZBIYCgdtZXNzYWdlGAMgASgJUgdtZXNzYWdlEhwKCXRpbWVzdGFtcBgEIAEoA1IJdGltZXN0'
    'YW1w');

@$core.Deprecated('Use tableReactionDescriptor instead')
const TableReaction$json = {
  '1': 'TableReaction',
  '2': [
    {'1': 'id', '3': 1, '4': 1, '5': 9, '10': 'id'},
    {'1': 'player_id', '3': 2, '4': 1, '5': 9, '10': 'playerId'},
    {'1': 'reaction_id', '3': 3, '4': 1, '5': 9, '10': 'reactionId'},
    {'1': 'target_player_id', '3': 4, '4': 1, '5': 9, '10': 'targetPlayerId'},
    {'1': 'timestamp', '3': 5, '4': 1, '5': 3, '10': 'timestamp'},
    {'1': 'expires_at', '3': 6, '4': 1, '5': 3, '10': 'expiresAt'},
  ],
};

/// Descriptor for `TableReaction`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List tableReactionDescriptor = $convert.base64Decode(
    'Cg1UYWJsZVJlYWN0aW9uEg4KAmlkGAEgASgJUgJpZBIbCglwbGF5ZXJfaWQYAiABKAlSCHBsYX'
    'llcklkEh8KC3JlYWN0aW9uX2lkGAMgASgJUgpyZWFjdGlvbklkEigKEHRhcmdldF9wbGF5ZXJf'
    'aWQYBCABKAlSDnRhcmdldFBsYXllcklkEhwKCXRpbWVzdGFtcBgFIAEoA1IJdGltZXN0YW1wEh'
    '0KCmV4cGlyZXNfYXQYBiABKANSCWV4cGlyZXNBdA==');

@$core.Deprecated('Use tableSnapshotDescriptor instead')
const TableSnapshot$json = {
  '1': 'TableSnapshot',
  '2': [
    {'1': 'stage', '3': 1, '4': 1, '5': 9, '10': 'stage'},
    {'1': 'board', '3': 2, '4': 3, '5': 9, '10': 'board'},
    {'1': 'seats', '3': 3, '4': 3, '5': 11, '6': '.poker.Seat', '10': 'seats'},
    {
      '1': 'payouts',
      '3': 4,
      '4': 3,
      '5': 11,
      '6': '.poker.TableSnapshot.PayoutsEntry',
      '10': 'payouts'
    },
    {'1': 'winners', '3': 5, '4': 3, '5': 9, '10': 'winners'},
    {'1': 'rake', '3': 6, '4': 1, '5': 3, '10': 'rake'},
    {'1': 'current_player_id', '3': 7, '4': 1, '5': 9, '10': 'currentPlayerId'},
    {
      '1': 'legal_actions',
      '3': 8,
      '4': 1,
      '5': 11,
      '6': '.poker.LegalActions',
      '10': 'legalActions'
    },
    {
      '1': 'action_deadline_unix_ms',
      '3': 9,
      '4': 1,
      '5': 3,
      '10': 'actionDeadlineUnixMs'
    },
    {'1': 'next_hand_unix_ms', '3': 10, '4': 1, '5': 3, '10': 'nextHandUnixMs'},
    {
      '1': 'won_without_showdown',
      '3': 11,
      '4': 1,
      '5': 8,
      '10': 'wonWithoutShowdown'
    },
    {
      '1': 'shuffle_commit_hash',
      '3': 12,
      '4': 1,
      '5': 9,
      '10': 'shuffleCommitHash'
    },
    {
      '1': 'shuffle_server_seed_hex',
      '3': 13,
      '4': 1,
      '5': 9,
      '10': 'shuffleServerSeedHex'
    },
    {
      '1': 'small_blind_player_id',
      '3': 14,
      '4': 1,
      '5': 9,
      '10': 'smallBlindPlayerId'
    },
    {
      '1': 'big_blind_player_id',
      '3': 15,
      '4': 1,
      '5': 9,
      '10': 'bigBlindPlayerId'
    },
    {'1': 'dealer_player_id', '3': 16, '4': 1, '5': 9, '10': 'dealerPlayerId'},
    {'1': 'snapshot_version', '3': 17, '4': 1, '5': 4, '10': 'snapshotVersion'},
    {'1': 'pots', '3': 18, '4': 3, '5': 11, '6': '.poker.Pot', '10': 'pots'},
    {'1': 'hand_id', '3': 19, '4': 1, '5': 9, '10': 'handId'},
    {
      '1': 'pot_results',
      '3': 20,
      '4': 3,
      '5': 11,
      '6': '.poker.PotResult',
      '10': 'potResults'
    },
    {
      '1': 'protocol_version',
      '3': 21,
      '4': 1,
      '5': 13,
      '10': 'protocolVersion'
    },
    {
      '1': 'idle_removal_unix_ms',
      '3': 22,
      '4': 1,
      '5': 3,
      '10': 'idleRemovalUnixMs'
    },
    {
      '1': 'action_base_deadline_unix_ms',
      '3': 23,
      '4': 1,
      '5': 3,
      '10': 'actionBaseDeadlineUnixMs'
    },
    {
      '1': 'chat_messages',
      '3': 24,
      '4': 3,
      '5': 11,
      '6': '.poker.ChatMessage',
      '10': 'chatMessages'
    },
    {
      '1': 'reactions',
      '3': 25,
      '4': 3,
      '5': 11,
      '6': '.poker.TableReaction',
      '10': 'reactions'
    },
    {
      '1': 'action_preselection',
      '3': 26,
      '4': 1,
      '5': 9,
      '10': 'actionPreselection'
    },
    {
      '1': 'action_preselection_amount',
      '3': 27,
      '4': 1,
      '5': 3,
      '10': 'actionPreselectionAmount'
    },
    {
      '1': 'prospective_call_amount',
      '3': 28,
      '4': 1,
      '5': 3,
      '10': 'prospectiveCallAmount'
    },
    {'1': 'root_commit_hash', '3': 29, '4': 1, '5': 9, '10': 'rootCommitHash'},
    {
      '1': 'revealed_card_salts',
      '3': 30,
      '4': 3,
      '5': 11,
      '6': '.poker.TableSnapshot.RevealedCardSaltsEntry',
      '10': 'revealedCardSalts'
    },
    {
      '1': 'unrevealed_card_hashes',
      '3': 31,
      '4': 3,
      '5': 11,
      '6': '.poker.TableSnapshot.UnrevealedCardHashesEntry',
      '10': 'unrevealedCardHashes'
    },
    {'1': 'runout_cards', '3': 32, '4': 3, '5': 9, '10': 'runoutCards'},
    {'1': 'board_two', '3': 33, '4': 3, '5': 9, '10': 'boardTwo'},
    {'1': 'board_split_at', '3': 34, '4': 1, '5': 5, '10': 'boardSplitAt'},
    {
      '1': 'pending_winner_cards',
      '3': 35,
      '4': 1,
      '5': 11,
      '6': '.poker.WinnerCardsRequest',
      '10': 'pendingWinnerCards'
    },
  ],
  '3': [
    TableSnapshot_PayoutsEntry$json,
    TableSnapshot_RevealedCardSaltsEntry$json,
    TableSnapshot_UnrevealedCardHashesEntry$json
  ],
};

@$core.Deprecated('Use tableSnapshotDescriptor instead')
const TableSnapshot_PayoutsEntry$json = {
  '1': 'PayoutsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 9, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 3, '10': 'value'},
  ],
  '7': {'7': true},
};

@$core.Deprecated('Use tableSnapshotDescriptor instead')
const TableSnapshot_RevealedCardSaltsEntry$json = {
  '1': 'RevealedCardSaltsEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 5, '10': 'key'},
    {
      '1': 'value',
      '3': 2,
      '4': 1,
      '5': 11,
      '6': '.poker.RevealedSalt',
      '10': 'value'
    },
  ],
  '7': {'7': true},
};

@$core.Deprecated('Use tableSnapshotDescriptor instead')
const TableSnapshot_UnrevealedCardHashesEntry$json = {
  '1': 'UnrevealedCardHashesEntry',
  '2': [
    {'1': 'key', '3': 1, '4': 1, '5': 5, '10': 'key'},
    {'1': 'value', '3': 2, '4': 1, '5': 9, '10': 'value'},
  ],
  '7': {'7': true},
};

/// Descriptor for `TableSnapshot`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List tableSnapshotDescriptor = $convert.base64Decode(
    'Cg1UYWJsZVNuYXBzaG90EhQKBXN0YWdlGAEgASgJUgVzdGFnZRIUCgVib2FyZBgCIAMoCVIFYm'
    '9hcmQSIQoFc2VhdHMYAyADKAsyCy5wb2tlci5TZWF0UgVzZWF0cxI7CgdwYXlvdXRzGAQgAygL'
    'MiEucG9rZXIuVGFibGVTbmFwc2hvdC5QYXlvdXRzRW50cnlSB3BheW91dHMSGAoHd2lubmVycx'
    'gFIAMoCVIHd2lubmVycxISCgRyYWtlGAYgASgDUgRyYWtlEioKEWN1cnJlbnRfcGxheWVyX2lk'
    'GAcgASgJUg9jdXJyZW50UGxheWVySWQSOAoNbGVnYWxfYWN0aW9ucxgIIAEoCzITLnBva2VyLk'
    'xlZ2FsQWN0aW9uc1IMbGVnYWxBY3Rpb25zEjUKF2FjdGlvbl9kZWFkbGluZV91bml4X21zGAkg'
    'ASgDUhRhY3Rpb25EZWFkbGluZVVuaXhNcxIpChFuZXh0X2hhbmRfdW5peF9tcxgKIAEoA1IObm'
    'V4dEhhbmRVbml4TXMSMAoUd29uX3dpdGhvdXRfc2hvd2Rvd24YCyABKAhSEndvbldpdGhvdXRT'
    'aG93ZG93bhIuChNzaHVmZmxlX2NvbW1pdF9oYXNoGAwgASgJUhFzaHVmZmxlQ29tbWl0SGFzaB'
    'I1ChdzaHVmZmxlX3NlcnZlcl9zZWVkX2hleBgNIAEoCVIUc2h1ZmZsZVNlcnZlclNlZWRIZXgS'
    'MQoVc21hbGxfYmxpbmRfcGxheWVyX2lkGA4gASgJUhJzbWFsbEJsaW5kUGxheWVySWQSLQoTYm'
    'lnX2JsaW5kX3BsYXllcl9pZBgPIAEoCVIQYmlnQmxpbmRQbGF5ZXJJZBIoChBkZWFsZXJfcGxh'
    'eWVyX2lkGBAgASgJUg5kZWFsZXJQbGF5ZXJJZBIpChBzbmFwc2hvdF92ZXJzaW9uGBEgASgEUg'
    '9zbmFwc2hvdFZlcnNpb24SHgoEcG90cxgSIAMoCzIKLnBva2VyLlBvdFIEcG90cxIXCgdoYW5k'
    'X2lkGBMgASgJUgZoYW5kSWQSMQoLcG90X3Jlc3VsdHMYFCADKAsyEC5wb2tlci5Qb3RSZXN1bH'
    'RSCnBvdFJlc3VsdHMSKQoQcHJvdG9jb2xfdmVyc2lvbhgVIAEoDVIPcHJvdG9jb2xWZXJzaW9u'
    'Ei8KFGlkbGVfcmVtb3ZhbF91bml4X21zGBYgASgDUhFpZGxlUmVtb3ZhbFVuaXhNcxI+ChxhY3'
    'Rpb25fYmFzZV9kZWFkbGluZV91bml4X21zGBcgASgDUhhhY3Rpb25CYXNlRGVhZGxpbmVVbml4'
    'TXMSNwoNY2hhdF9tZXNzYWdlcxgYIAMoCzISLnBva2VyLkNoYXRNZXNzYWdlUgxjaGF0TWVzc2'
    'FnZXMSMgoJcmVhY3Rpb25zGBkgAygLMhQucG9rZXIuVGFibGVSZWFjdGlvblIJcmVhY3Rpb25z'
    'Ei8KE2FjdGlvbl9wcmVzZWxlY3Rpb24YGiABKAlSEmFjdGlvblByZXNlbGVjdGlvbhI8ChphY3'
    'Rpb25fcHJlc2VsZWN0aW9uX2Ftb3VudBgbIAEoA1IYYWN0aW9uUHJlc2VsZWN0aW9uQW1vdW50'
    'EjYKF3Byb3NwZWN0aXZlX2NhbGxfYW1vdW50GBwgASgDUhVwcm9zcGVjdGl2ZUNhbGxBbW91bn'
    'QSKAoQcm9vdF9jb21taXRfaGFzaBgdIAEoCVIOcm9vdENvbW1pdEhhc2gSWwoTcmV2ZWFsZWRf'
    'Y2FyZF9zYWx0cxgeIAMoCzIrLnBva2VyLlRhYmxlU25hcHNob3QuUmV2ZWFsZWRDYXJkU2FsdH'
    'NFbnRyeVIRcmV2ZWFsZWRDYXJkU2FsdHMSZAoWdW5yZXZlYWxlZF9jYXJkX2hhc2hlcxgfIAMo'
    'CzIuLnBva2VyLlRhYmxlU25hcHNob3QuVW5yZXZlYWxlZENhcmRIYXNoZXNFbnRyeVIUdW5yZX'
    'ZlYWxlZENhcmRIYXNoZXMSIQoMcnVub3V0X2NhcmRzGCAgAygJUgtydW5vdXRDYXJkcxIbCgli'
    'b2FyZF90d28YISADKAlSCGJvYXJkVHdvEiQKDmJvYXJkX3NwbGl0X2F0GCIgASgFUgxib2FyZF'
    'NwbGl0QXQSSwoUcGVuZGluZ193aW5uZXJfY2FyZHMYIyABKAsyGS5wb2tlci5XaW5uZXJDYXJk'
    'c1JlcXVlc3RSEnBlbmRpbmdXaW5uZXJDYXJkcxo6CgxQYXlvdXRzRW50cnkSEAoDa2V5GAEgAS'
    'gJUgNrZXkSFAoFdmFsdWUYAiABKANSBXZhbHVlOgI4ARpZChZSZXZlYWxlZENhcmRTYWx0c0Vu'
    'dHJ5EhAKA2tleRgBIAEoBVIDa2V5EikKBXZhbHVlGAIgASgLMhMucG9rZXIuUmV2ZWFsZWRTYW'
    'x0UgV2YWx1ZToCOAEaRwoZVW5yZXZlYWxlZENhcmRIYXNoZXNFbnRyeRIQCgNrZXkYASABKAVS'
    'A2tleRIUCgV2YWx1ZRgCIAEoCVIFdmFsdWU6AjgB');

@$core.Deprecated('Use winnerCardsRequestDescriptor instead')
const WinnerCardsRequest$json = {
  '1': 'WinnerCardsRequest',
  '2': [
    {'1': 'requester_id', '3': 1, '4': 1, '5': 9, '10': 'requesterId'},
    {'1': 'requester_name', '3': 2, '4': 1, '5': 9, '10': 'requesterName'},
    {'1': 'winner_id', '3': 3, '4': 1, '5': 9, '10': 'winnerId'},
    {'1': 'fee', '3': 4, '4': 1, '5': 3, '10': 'fee'},
    {
      '1': 'expires_at_unix_ms',
      '3': 5,
      '4': 1,
      '5': 3,
      '10': 'expiresAtUnixMs'
    },
  ],
};

/// Descriptor for `WinnerCardsRequest`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List winnerCardsRequestDescriptor = $convert.base64Decode(
    'ChJXaW5uZXJDYXJkc1JlcXVlc3QSIQoMcmVxdWVzdGVyX2lkGAEgASgJUgtyZXF1ZXN0ZXJJZB'
    'IlCg5yZXF1ZXN0ZXJfbmFtZRgCIAEoCVINcmVxdWVzdGVyTmFtZRIbCgl3aW5uZXJfaWQYAyAB'
    'KAlSCHdpbm5lcklkEhAKA2ZlZRgEIAEoA1IDZmVlEisKEmV4cGlyZXNfYXRfdW5peF9tcxgFIA'
    'EoA1IPZXhwaXJlc0F0VW5peE1z');

@$core.Deprecated('Use revealedSaltDescriptor instead')
const RevealedSalt$json = {
  '1': 'RevealedSalt',
  '2': [
    {'1': 'card', '3': 1, '4': 1, '5': 9, '10': 'card'},
    {'1': 'salt_hex', '3': 2, '4': 1, '5': 9, '10': 'saltHex'},
  ],
};

/// Descriptor for `RevealedSalt`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List revealedSaltDescriptor = $convert.base64Decode(
    'CgxSZXZlYWxlZFNhbHQSEgoEY2FyZBgBIAEoCVIEY2FyZBIZCghzYWx0X2hleBgCIAEoCVIHc2'
    'FsdEhleA==');

@$core.Deprecated('Use clientMessageDescriptor instead')
const ClientMessage$json = {
  '1': 'ClientMessage',
  '2': [
    {'1': 'type', '3': 1, '4': 1, '5': 9, '10': 'type'},
    {'1': 'token', '3': 2, '4': 1, '5': 9, '10': 'token'},
    {'1': 'share_code', '3': 3, '4': 1, '5': 9, '10': 'shareCode'},
    {'1': 'ready', '3': 4, '4': 1, '5': 8, '10': 'ready'},
    {'1': 'action', '3': 5, '4': 1, '5': 9, '10': 'action'},
    {'1': 'amount', '3': 6, '4': 1, '5': 3, '10': 'amount'},
    {'1': 'action_id', '3': 7, '4': 1, '5': 9, '10': 'actionId'},
    {'1': 'message', '3': 8, '4': 1, '5': 9, '10': 'message'},
    {
      '1': 'expected_snapshot_version',
      '3': 9,
      '4': 1,
      '5': 4,
      '10': 'expectedSnapshotVersion'
    },
    {'1': 'expected_hand_id', '3': 10, '4': 1, '5': 9, '10': 'expectedHandId'},
    {
      '1': 'card_index',
      '3': 11,
      '4': 1,
      '5': 5,
      '9': 0,
      '10': 'cardIndex',
      '17': true
    },
    {'1': 'reaction_id', '3': 12, '4': 1, '5': 9, '10': 'reactionId'},
    {'1': 'target_player_id', '3': 13, '4': 1, '5': 9, '10': 'targetPlayerId'},
    {'1': 'turnstile_token', '3': 14, '4': 1, '5': 9, '10': 'turnstileToken'},
    {
      '1': 'run_it_twice',
      '3': 15,
      '4': 1,
      '5': 8,
      '9': 1,
      '10': 'runItTwice',
      '17': true
    },
    {'1': 'expected_stage', '3': 16, '4': 1, '5': 9, '10': 'expectedStage'},
  ],
  '8': [
    {'1': '_card_index'},
    {'1': '_run_it_twice'},
  ],
};

/// Descriptor for `ClientMessage`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List clientMessageDescriptor = $convert.base64Decode(
    'Cg1DbGllbnRNZXNzYWdlEhIKBHR5cGUYASABKAlSBHR5cGUSFAoFdG9rZW4YAiABKAlSBXRva2'
    'VuEh0KCnNoYXJlX2NvZGUYAyABKAlSCXNoYXJlQ29kZRIUCgVyZWFkeRgEIAEoCFIFcmVhZHkS'
    'FgoGYWN0aW9uGAUgASgJUgZhY3Rpb24SFgoGYW1vdW50GAYgASgDUgZhbW91bnQSGwoJYWN0aW'
    '9uX2lkGAcgASgJUghhY3Rpb25JZBIYCgdtZXNzYWdlGAggASgJUgdtZXNzYWdlEjoKGWV4cGVj'
    'dGVkX3NuYXBzaG90X3ZlcnNpb24YCSABKARSF2V4cGVjdGVkU25hcHNob3RWZXJzaW9uEigKEG'
    'V4cGVjdGVkX2hhbmRfaWQYCiABKAlSDmV4cGVjdGVkSGFuZElkEiIKCmNhcmRfaW5kZXgYCyAB'
    'KAVIAFIJY2FyZEluZGV4iAEBEh8KC3JlYWN0aW9uX2lkGAwgASgJUgpyZWFjdGlvbklkEigKEH'
    'RhcmdldF9wbGF5ZXJfaWQYDSABKAlSDnRhcmdldFBsYXllcklkEicKD3R1cm5zdGlsZV90b2tl'
    'bhgOIAEoCVIOdHVybnN0aWxlVG9rZW4SJQoMcnVuX2l0X3R3aWNlGA8gASgISAFSCnJ1bkl0VH'
    'dpY2WIAQESJQoOZXhwZWN0ZWRfc3RhZ2UYECABKAlSDWV4cGVjdGVkU3RhZ2VCDQoLX2NhcmRf'
    'aW5kZXhCDwoNX3J1bl9pdF90d2ljZQ==');

@$core.Deprecated('Use serverMessageDescriptor instead')
const ServerMessage$json = {
  '1': 'ServerMessage',
  '2': [
    {'1': 'type', '3': 1, '4': 1, '5': 9, '10': 'type'},
    {'1': 'conn_id', '3': 2, '4': 1, '5': 9, '10': 'connId'},
    {
      '1': 'snapshot',
      '3': 3,
      '4': 1,
      '5': 11,
      '6': '.poker.TableSnapshot',
      '10': 'snapshot'
    },
    {'1': 'player_id', '3': 4, '4': 1, '5': 9, '10': 'playerId'},
    {'1': 'message', '3': 5, '4': 1, '5': 9, '10': 'message'},
    {'1': 'code', '3': 6, '4': 1, '5': 9, '10': 'code'},
    {'1': 'key', '3': 7, '4': 1, '5': 9, '10': 'key'},
    {'1': 'stars', '3': 8, '4': 1, '5': 5, '10': 'stars'},
    {'1': 'room', '3': 9, '4': 1, '5': 11, '6': '.poker.Room', '10': 'room'},
    {'1': 'room_id', '3': 10, '4': 1, '5': 9, '10': 'roomId'},
    {'1': 'seats_taken', '3': 11, '4': 1, '5': 5, '10': 'seatsTaken'},
    {'1': 'amount', '3': 12, '4': 1, '5': 3, '10': 'amount'},
    {'1': 'text', '3': 13, '4': 1, '5': 9, '10': 'text'},
    {'1': 'action_id', '3': 14, '4': 1, '5': 9, '10': 'actionId'},
    {'1': 'snapshot_version', '3': 15, '4': 1, '5': 4, '10': 'snapshotVersion'},
    {
      '1': 'equity',
      '3': 16,
      '4': 1,
      '5': 1,
      '9': 0,
      '10': 'equity',
      '17': true
    },
    {'1': 'reaction_id', '3': 17, '4': 1, '5': 9, '10': 'reactionId'},
    {'1': 'target_player_id', '3': 18, '4': 1, '5': 9, '10': 'targetPlayerId'},
    {'1': 'purchase_id', '3': 19, '4': 1, '5': 9, '10': 'purchaseId'},
    {
      '1': 'social_event',
      '3': 20,
      '4': 1,
      '5': 11,
      '6': '.poker.SocialEvent',
      '10': 'socialEvent'
    },
    {'1': 'unread_count', '3': 21, '4': 1, '5': 5, '10': 'unreadCount'},
  ],
  '8': [
    {'1': '_equity'},
  ],
};

/// Descriptor for `ServerMessage`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List serverMessageDescriptor = $convert.base64Decode(
    'Cg1TZXJ2ZXJNZXNzYWdlEhIKBHR5cGUYASABKAlSBHR5cGUSFwoHY29ubl9pZBgCIAEoCVIGY2'
    '9ubklkEjAKCHNuYXBzaG90GAMgASgLMhQucG9rZXIuVGFibGVTbmFwc2hvdFIIc25hcHNob3QS'
    'GwoJcGxheWVyX2lkGAQgASgJUghwbGF5ZXJJZBIYCgdtZXNzYWdlGAUgASgJUgdtZXNzYWdlEh'
    'IKBGNvZGUYBiABKAlSBGNvZGUSEAoDa2V5GAcgASgJUgNrZXkSFAoFc3RhcnMYCCABKAVSBXN0'
    'YXJzEh8KBHJvb20YCSABKAsyCy5wb2tlci5Sb29tUgRyb29tEhcKB3Jvb21faWQYCiABKAlSBn'
    'Jvb21JZBIfCgtzZWF0c190YWtlbhgLIAEoBVIKc2VhdHNUYWtlbhIWCgZhbW91bnQYDCABKANS'
    'BmFtb3VudBISCgR0ZXh0GA0gASgJUgR0ZXh0EhsKCWFjdGlvbl9pZBgOIAEoCVIIYWN0aW9uSW'
    'QSKQoQc25hcHNob3RfdmVyc2lvbhgPIAEoBFIPc25hcHNob3RWZXJzaW9uEhsKBmVxdWl0eRgQ'
    'IAEoAUgAUgZlcXVpdHmIAQESHwoLcmVhY3Rpb25faWQYESABKAlSCnJlYWN0aW9uSWQSKAoQdG'
    'FyZ2V0X3BsYXllcl9pZBgSIAEoCVIOdGFyZ2V0UGxheWVySWQSHwoLcHVyY2hhc2VfaWQYEyAB'
    'KAlSCnB1cmNoYXNlSWQSNQoMc29jaWFsX2V2ZW50GBQgASgLMhIucG9rZXIuU29jaWFsRXZlbn'
    'RSC3NvY2lhbEV2ZW50EiEKDHVucmVhZF9jb3VudBgVIAEoBVILdW5yZWFkQ291bnRCCQoHX2Vx'
    'dWl0eQ==');

@$core.Deprecated('Use playerPresenceDescriptor instead')
const PlayerPresence$json = {
  '1': 'PlayerPresence',
  '2': [
    {'1': 'player_id', '3': 1, '4': 1, '5': 9, '10': 'playerId'},
    {'1': 'status', '3': 2, '4': 1, '5': 9, '10': 'status'},
    {'1': 'room_id', '3': 3, '4': 1, '5': 9, '10': 'roomId'},
  ],
};

/// Descriptor for `PlayerPresence`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List playerPresenceDescriptor = $convert.base64Decode(
    'Cg5QbGF5ZXJQcmVzZW5jZRIbCglwbGF5ZXJfaWQYASABKAlSCHBsYXllcklkEhYKBnN0YXR1cx'
    'gCIAEoCVIGc3RhdHVzEhcKB3Jvb21faWQYAyABKAlSBnJvb21JZA==');

@$core.Deprecated('Use socialEventDescriptor instead')
const SocialEvent$json = {
  '1': 'SocialEvent',
  '2': [
    {'1': 'event_id', '3': 1, '4': 1, '5': 9, '10': 'eventId'},
    {'1': 'type', '3': 2, '4': 1, '5': 9, '10': 'type'},
    {'1': 'actor_id', '3': 3, '4': 1, '5': 9, '10': 'actorId'},
    {'1': 'room_id', '3': 4, '4': 1, '5': 9, '10': 'roomId'},
    {'1': 'status', '3': 5, '4': 1, '5': 9, '10': 'status'},
    {'1': 'created_at', '3': 6, '4': 1, '5': 3, '10': 'createdAt'},
    {'1': 'expires_at', '3': 7, '4': 1, '5': 3, '10': 'expiresAt'},
    {
      '1': 'presence',
      '3': 8,
      '4': 1,
      '5': 11,
      '6': '.poker.PlayerPresence',
      '10': 'presence'
    },
  ],
};

/// Descriptor for `SocialEvent`. Decode as a `google.protobuf.DescriptorProto`.
final $typed_data.Uint8List socialEventDescriptor = $convert.base64Decode(
    'CgtTb2NpYWxFdmVudBIZCghldmVudF9pZBgBIAEoCVIHZXZlbnRJZBISCgR0eXBlGAIgASgJUg'
    'R0eXBlEhkKCGFjdG9yX2lkGAMgASgJUgdhY3RvcklkEhcKB3Jvb21faWQYBCABKAlSBnJvb21J'
    'ZBIWCgZzdGF0dXMYBSABKAlSBnN0YXR1cxIdCgpjcmVhdGVkX2F0GAYgASgDUgljcmVhdGVkQX'
    'QSHQoKZXhwaXJlc19hdBgHIAEoA1IJZXhwaXJlc0F0EjEKCHByZXNlbmNlGAggASgLMhUucG9r'
    'ZXIuUGxheWVyUHJlc2VuY2VSCHByZXNlbmNl');
