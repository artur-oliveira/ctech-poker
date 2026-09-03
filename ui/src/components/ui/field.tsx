'use client';
import {useId} from 'react';
import {Label} from '@/components/ui/label';
import {cn} from '@/lib/utils';

/**
 * The props a `Field` hands to the control it wraps. Spread them onto the
 * `Input` (or any focusable control) so the label, the description and the
 * error are associated by id instead of by proximity.
 */
export interface FieldControlProps {
  id: string;
  'aria-describedby'?: string;
  'aria-invalid'?: true;
  'aria-errormessage'?: string;
}

export interface FieldProps {
  label: string;
  /** Persistent hint. Announced with the control, never instead of the label. */
  description?: string;
  /** When set, the control is marked invalid and the message is announced. */
  error?: string;
  className?: string;
  children: (control: FieldControlProps) => React.ReactNode;
}

/**
 * Label + description + error region around one control, with the ids wired
 * up. A render prop rather than a wrapper element because the control varies
 * (`Input`, a search row with a trailing button, a range) while the
 * association rules do not.
 */
export function Field({label, description, error, className, children}: FieldProps) {
  const id = useId();
  const descriptionId = description ? `${id}-description` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  // aria-describedby carries both so the hint is not lost the moment the field
  // goes invalid; aria-errormessage is what names the failure specifically.
  const describedBy = [descriptionId, errorId].filter(Boolean).join(' ') || undefined;

  return <div className={cn('field', className)}>
    <Label htmlFor={id}>{label}</Label>
    {children({
      id,
      'aria-describedby': describedBy,
      'aria-invalid': error ? true : undefined,
      'aria-errormessage': errorId,
    })}
    {description && <small id={descriptionId} className="field-description">{description}</small>}
    {error && <p id={errorId} className="form-error" role="alert">{error}</p>}
  </div>;
}
