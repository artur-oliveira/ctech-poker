export const OG_PREVIEWS = [
  {slug: 'home', route: '/', title: 'Página inicial'},
  {slug: 'guide', route: '/guide', title: 'Como jogar'},
  {slug: 'poker-rules', route: '/poker-rules', title: 'Regras do Texas Hold’em'},
  {slug: 'profile', route: '/profile?id=mock_player_ana', title: 'Vitrine do jogador'},
  {slug: 'lobby', route: '/lobby', title: 'Lobby'},
  {slug: 'table', route: '/table?id=01ARZ3NDEKTSV4RRFFQ69G5FAV&scenario=flop', title: 'Mesa de poker'},
  {slug: 'hands', route: '/hands', title: 'Mãos jogadas'},
  {
    slug: 'hand-history',
    route: '/hands/history?table_id=01ARZ3NDEKTSV4RRFFQ69G5FAV&hand_id=hand_0003',
    title: 'Detalhes da mão'
  },
  {
    slug: 'hand-replay',
    route: '/hands/replay?table_id=01ARZ3NDEKTSV4RRFFQ69G5FAV&hand_id=hand_0003',
    title: 'Replay da mão'
  },
  {slug: 'shared-hand', route: '/share?id=mock-share-demo', title: 'Mão compartilhada'},
  {slug: 'leaderboard', route: '/leaderboard', title: 'Ranking da comunidade'},
  {slug: 'achievements', route: '/achievements', title: 'Conquistas'}
] as const;
