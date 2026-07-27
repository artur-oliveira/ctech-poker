// Unlike TermsGate, this page never forces a login redirect. It has a real
// public variant. It only checks whether a session already exists (silent
// refresh, same call TermsGate makes) so a returning player sees their own
// progress without a hard gate blocking a first-time or logged-out visitor.

import {useEffect, useState} from "react";
import {useQuery} from "@tanstack/react-query";
import {getAccessToken, setAccessToken, setPlayerId, setUsername, subscribeAccessToken} from "@/lib/api/client";
import {USE_MOCK} from "@/lib/mock";
import {doRefresh} from "@/lib/auth/oauth";
import {getMe} from "@/lib/api/player";

export function useOptionalSession() {
  const [token, setToken] = useState<string | null>(() => getAccessToken());
  const [checking, setChecking] = useState(() => !USE_MOCK && !getAccessToken());

  useEffect(() => {
    const unsubscribe = subscribeAccessToken(setToken);
    if (!USE_MOCK && !getAccessToken()) {
      void doRefresh().then(result => {
        if (result) {
          setAccessToken(result.accessToken);
          setUsername(result.username);
        }
      }).finally(() => setChecking(false));
    }
    return unsubscribe;
  }, []);

  // Pages using this hook (leaderboard, guide, etc.) never mount TermsGate,
  // so nothing else here ever calls setPlayerId. Without this, getViewerId()
  // silently returns undefined for a real session and viewer highlighting
  // breaks, unless the player happened to already visit a TermsGate page first.
  const me = useQuery({queryKey: ['player', 'me'], queryFn: getMe, enabled: Boolean(token)});
  useEffect(() => {
    setPlayerId(me.data?.user_id ?? null);
  }, [me.data?.user_id]);

  return {authed: Boolean(token), checking};
}