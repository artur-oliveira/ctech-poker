import {render, screen} from '@testing-library/react';
import {describe, expect, test} from 'vitest';
import {Field} from './field';
import {Input} from './input';

// Issue #107: `Input` styled `aria-invalid` but every form wired the label,
// hint and error association by hand — or not at all.
describe('Field', () => {
  test('associates the label with the control by id', () => {
    render(<Field label="Código de um amigo">{control => <Input {...control}/>}</Field>);
    const input = screen.getByLabelText('Código de um amigo');
    expect(input).toHaveAttribute('id');
    expect(input).not.toHaveAttribute('aria-invalid');
    expect(input).not.toHaveAttribute('aria-describedby');
  });

  test('describes the control with the persistent hint', () => {
    render(<Field label="Código de um amigo" description="Só o código encontra alguém.">
      {control => <Input {...control}/>}
    </Field>);
    expect(screen.getByLabelText('Código de um amigo'))
      .toHaveAccessibleDescription('Só o código encontra alguém.');
  });

  test('marks the control invalid and keeps the hint alongside the error', () => {
    render(<Field label="Código de um amigo" description="Só o código encontra alguém."
                  error="Código não encontrado.">
      {control => <Input {...control}/>}
    </Field>);
    const input = screen.getByLabelText('Código de um amigo');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAccessibleDescription('Só o código encontra alguém. Código não encontrado.');
    expect(input.getAttribute('aria-errormessage'))
      .toBe(screen.getByRole('alert').getAttribute('id'));
    expect(screen.getByRole('alert')).toHaveTextContent('Código não encontrado.');
  });

  test('takes an extra class without losing the field layout', () => {
    render(<Field className="friend-code-search" label="Código">{control => <Input {...control}/>}</Field>);
    const field = document.querySelector('.field')!;
    expect(field).toHaveClass('friend-code-search');
  });
});
