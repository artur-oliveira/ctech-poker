// Unlike TermsGate, this page never forces a login redirect — it has a real
// public variant. It only checks whether a session already exists (silent
// refresh, same call TermsGate makes) so a returning player sees their own
// progress without a hard gate blocking a first-time or logged-out visitor.

import {useEffect, useState} from "react";
import {getAccessToken, setAccessToken, setUsername, subscribeAccessToken} from "@/lib/api/client";
import {USE_MOCK} from "@/lib/mock";
import {doRefresh} from "@/lib/auth/oauth";

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

  return {authed: Boolean(token), checking};
}