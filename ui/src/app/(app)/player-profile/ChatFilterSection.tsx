'use client';
import {useState} from 'react';
import {X} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Skeleton} from '@/components/ui/skeleton';
import {CHAT_PREFS_MAX_WORD_LENGTH, CHAT_PREFS_MAX_WORDS} from '@/lib/api/chatPrefs';
import {useChatPrefs} from '@/lib/hooks/useChatPrefs';

/**
 * #327: a personal addition on top of the table-wide chat filter, never a
 * replacement for it. There is no control here that could remove or disable
 * the floor — the floor lives entirely server-side and is applied before this
 * screen's words even get a chance to (`chatfilter.Effective` only unions),
 * so the copy below says exactly that instead of implying a toggle exists.
 */
export function ChatFilterSection() {
  const {extraWords, isLoading, isError, refetch, save, isSaving} = useChatPrefs();
  const [draft, setDraft] = useState('');
  const [error, setError] = useState('');

  function addWord(event: React.FormEvent) {
    event.preventDefault();
    const word = draft.trim().toLowerCase();
    if (!word) return;
    if (word.length > CHAT_PREFS_MAX_WORD_LENGTH) {
      setError(`Cada palavra pode ter no máximo ${CHAT_PREFS_MAX_WORD_LENGTH} caracteres.`);
      return;
    }
    if (extraWords.includes(word)) {
      setDraft('');
      return;
    }
    if (extraWords.length >= CHAT_PREFS_MAX_WORDS) {
      setError(`Você pode adicionar até ${CHAT_PREFS_MAX_WORDS} palavras.`);
      return;
    }
    setError('');
    setDraft('');
    save([...extraWords, word]);
  }

  function removeWord(word: string) {
    save(extraWords.filter(w => w !== word));
  }

  return <section className="player-profile-section" aria-labelledby="player-profile-chat-filter-title">
    <header className="player-profile-section-head">
      <h2 id="player-profile-chat-filter-title">Filtro de chat</h2>
      <p>
        Palavras que você não quer ver no chat da mesa, além do filtro padrão que todas as mesas já
        aplicam. Isso nunca remove ou enfraquece esse filtro padrão — só adiciona à sua própria visão do chat.
      </p>
    </header>

    {isLoading
      ? <div className="chatprefs-skeleton">
        <Skeleton style={{height: '44px'}}/>
        <Skeleton style={{height: '32px', width: '60%'}}/>
      </div>
      : isError
        ? <div className="lobby-empty hands-state" role="alert">
          <div>
            <strong>Seu filtro de chat não abriu desta vez</strong>
            <p>O filtro padrão da mesa continua valendo. Tente carregar sua lista pessoal de novo.</p>
          </div>
          <Button variant="outline" size="sm" onClick={() => void refetch()}>Tentar novamente</Button>
        </div>
        : <div className="chatprefs-editor">
          <form className="chatprefs-add" onSubmit={addWord}>
            <Label htmlFor="chatprefs-word-input" className="sr-only">Nova palavra</Label>
            <Input
              id="chatprefs-word-input"
              value={draft}
              onChange={e => {
                setDraft(e.target.value);
                if (error) setError('');
              }}
              placeholder="Adicionar palavra…"
              maxLength={CHAT_PREFS_MAX_WORD_LENGTH}
              disabled={isSaving}
              aria-describedby={error ? 'chatprefs-error' : undefined}
              aria-invalid={error ? true : undefined}
            />
            <Button type="submit" variant="outline" disabled={isSaving || !draft.trim()} loading={isSaving}>
              Adicionar
            </Button>
          </form>
          {error && <p id="chatprefs-error" className="form-error" role="alert">{error}</p>}

          {extraWords.length === 0
            ? <p className="player-profile-empty">
              Você ainda não adicionou nenhuma palavra. O filtro padrão da mesa continua valendo do mesmo jeito.
            </p>
            : <ul className="chatprefs-word-list" aria-label="Suas palavras adicionadas">
              {extraWords.map(word => <li key={word} className="chatprefs-word-pill">
                <span>{word}</span>
                <button
                  type="button"
                  onClick={() => removeWord(word)}
                  disabled={isSaving}
                  aria-label={`Remover "${word}" do seu filtro`}
                >
                  <X aria-hidden="true"/>
                </button>
              </li>)}
            </ul>}
        </div>}
  </section>;
}
