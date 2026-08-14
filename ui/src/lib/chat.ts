// Mirrors the server-side cap enforced in api/internal/api/v1/tablews.go's "chat" case.
export const CHAT_MESSAGE_MAX_LENGTH = 50;
// Client-side history cap: keep chat rendering/scrolling cheap on long sessions.
export const CHAT_HISTORY_LIMIT = 40;
