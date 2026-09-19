'use client';
import {useMutation, useQuery, useQueryClient} from '@tanstack/react-query';
import {CHAT_PREFS_KEY, getChatPrefs, saveChatPrefs} from '@/lib/api/chatPrefs';
import {pushNotification} from '@/lib/notify';

/** One save per resource (#327): the whole extra-word list writes in a
 * single request, same convention `useTablePreferenceSave` and the showcase
 * mutation already follow. */
export function useChatPrefs() {
  const queryClient = useQueryClient();
  const query = useQuery({queryKey: CHAT_PREFS_KEY, queryFn: getChatPrefs});
  const save = useMutation({
    mutationFn: saveChatPrefs,
    onSuccess: prefs => {
      queryClient.setQueryData(CHAT_PREFS_KEY, prefs);
      pushNotification('Filtro de chat pessoal atualizado.', 'info');
    },
    onError: () => pushNotification('Não foi possível salvar seu filtro de chat. Tente novamente.'),
  });
  return {
    extraWords: query.data?.extra_words ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    refetch: query.refetch,
    save: save.mutate,
    isSaving: save.isPending,
  };
}
