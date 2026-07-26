import {useQuery} from '@tanstack/react-query';
import {getAccessToken} from '@/lib/api/client';
import {getMe} from '@/lib/api/player';
import {DEFAULT_DECK_VARIANT, type DeckVariantId} from '@/lib/cardVariants';

// Shares the ['player', 'me'] query TermsGate/useOptionalSession already
// populate, so this never triggers an extra fetch on those pages.
export function useDeckVariant(): DeckVariantId {
  const {data} = useQuery({queryKey: ['player', 'me'], queryFn: getMe, enabled: Boolean(getAccessToken())});
  return data?.deck_variant ?? DEFAULT_DECK_VARIANT;
}
