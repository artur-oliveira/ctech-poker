class PokerConfig {
  static const api = String.fromEnvironment(
    'POKER_API_URL',
    defaultValue: 'https://poker-api.aoctech.app',
  );
  static const accounts = String.fromEnvironment(
    'ACCOUNTS_API_URL',
    defaultValue: 'https://accounts-api.aoctech.app',
  );
  static const clientId = 'poker-mobile';
  static const callback = 'app.aoctech.poker:/oauth/callback';
  static const scopes = [
    'openid',
    'profile',
    'poker:rooms:read',
    'poker:players:read',
    'poker:sessions:read',
    'poker:hands:read',
    'poker:achievements:read',
    'poker:stats:read',
    'poker:leaderboard:read',
    'poker:daily-reward:read',
    'poker:player-notes:read',
    'poker:sandbox-purchases:read',
    'poker:reaction-purchases:read',
    'poker:cosmetic-purchases:read',
  ];
}
