import {render, screen} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {Dialog, DialogContent, DialogDescription, DialogTitle} from './dialog';

describe('DialogContent', () => {
  test('keeps forgotten dialogs reachable inside the dynamic viewport', () => {
    render(<Dialog open><DialogContent>
      <DialogTitle>Preferências</DialogTitle>
      <DialogDescription>Configurações da mesa.</DialogDescription>
    </DialogContent></Dialog>);

    expect(screen.getByRole('dialog')).toHaveClass('max-h-[calc(100dvh-2rem)]', 'overflow-y-auto', 'overscroll-contain');
    expect(screen.getByRole('button', {name: 'Fechar'})).toBeInTheDocument();
  });
});
