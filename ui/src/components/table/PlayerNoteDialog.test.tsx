import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, test, vi} from 'vitest';

import {PlayerNoteDialog} from './PlayerNoteDialog';

const {savePlayerNote, pushNotification} = vi.hoisted(() => ({
  savePlayerNote: vi.fn(),
  pushNotification: vi.fn(),
}));

vi.mock('@/lib/api/playerNotes', async importOriginal => ({
  ...await importOriginal<typeof import('@/lib/api/playerNotes')>(),
  savePlayerNote,
}));
vi.mock('@/lib/notify', () => ({pushNotification}));

describe('PlayerNoteDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  test('loads an existing private note and saves edited content and tag', async () => {
    const saved = {
      opponent_id: 'opponent-1',
      tag: 'blue',
      note: 'Aposta forte no river',
      updated_at: '2026-07-28T12:00:00Z',
    };
    savePlayerNote.mockResolvedValue(saved);
    const onSaved = vi.fn();
    const onOpenChange = vi.fn();

    render(<PlayerNoteDialog
      opponent={{player_id: 'opponent-1', name: 'Ana'}}
      existing={{...saved, tag: 'red', note: 'Nota antiga'}}
      open
      onOpenChange={onOpenChange}
      onSaved={onSaved}
    />);

    expect(screen.getByRole('heading', {name: 'Nota sobre Ana'})).toBeInTheDocument();
    expect(screen.getByText('Só você pode ver esta anotação.')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Vermelho'})).toHaveAttribute('aria-pressed', 'true');

    await userEvent.click(screen.getByRole('button', {name: 'Azul'}));
    fireEvent.change(screen.getByLabelText('Anotação'), {target: {value: 'Aposta forte no river'}});
    expect(screen.getByText('21/500')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Salvar'}));

    await waitFor(() => expect(savePlayerNote).toHaveBeenCalledWith('opponent-1', {
      tag: 'blue',
      note: 'Aposta forte no river',
    }));
    expect(onSaved).toHaveBeenCalledWith(saved);
    expect(pushNotification).toHaveBeenCalledWith('Anotação privada atualizada.', 'info');
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  test('maps an empty response to note deletion and prevents duplicate saves while pending', async () => {
    let finish!: (value: {deleted: true}) => void;
    savePlayerNote.mockReturnValue(new Promise(resolve => {
      finish = resolve;
    }));
    const onSaved = vi.fn();

    render(<PlayerNoteDialog opponent={{player_id: 'opponent-2'}} open
                             onOpenChange={vi.fn()} onSaved={onSaved}/>);
    await userEvent.click(screen.getByRole('button', {name: 'Verde'}));
    await userEvent.click(screen.getByRole('button', {name: 'Sem tag'}));
    const save = screen.getByRole('button', {name: 'Salvar'});
    await userEvent.click(save);

    expect(save).toBeDisabled();
    fireEvent.click(save);
    expect(savePlayerNote).toHaveBeenCalledTimes(1);
    expect(savePlayerNote).toHaveBeenCalledWith('opponent-2', {tag: undefined, note: ''});

    finish({deleted: true});
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(null));
    expect(save).not.toBeDisabled();
  });

  test('does not save without an opponent and supports cancelling', async () => {
    const onOpenChange = vi.fn();
    render(<PlayerNoteDialog opponent={null} open onOpenChange={onOpenChange} onSaved={vi.fn()}/>);

    expect(screen.getByRole('heading', {name: 'Nota sobre jogador'})).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', {name: 'Salvar'}));
    expect(savePlayerNote).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', {name: 'Cancelar'}));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
