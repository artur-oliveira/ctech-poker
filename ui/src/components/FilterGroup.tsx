'use client';

import {useId} from 'react';

export type FilterOption<T extends string> = {
  value: T;
  label: string;
  disabled?: boolean;
  title?: string;
};

export function FilterGroup<T extends string>({label, value, options, onChangeAction, className, showLabel}: {
  label: string;
  value: T;
  options: readonly FilterOption<T>[];
  onChangeAction: (value: T) => void;
  className?: string;
  /** Renders `label` inline ahead of the chips instead of only naming the
   * group for assistive tech. Use it where several groups sit side by side and
   * the chips alone no longer say which axis they belong to; a lone group on a
   * page reads fine without it. The group is then named by that same visible
   * text, so nobody hears the label twice. */
  showLabel?: boolean;
}) {
  const labelId = useId();
  return <div
    className={`filter-tabs${className ? ` ${className}` : ''}`}
    role="group"
    {...(showLabel ? {'aria-labelledby': labelId} : {'aria-label': label})}
  >
    {showLabel && <span className="filter-tabs-label" id={labelId}>{label}</span>}
    {options.map(option => <button
      key={option.value}
      type="button"
      aria-pressed={value === option.value}
      disabled={option.disabled}
      title={option.title}
      className={`filter-tab${value === option.value ? ' active' : ''}`}
      onClick={() => onChangeAction(option.value)}
    >
      {option.label}
    </button>)}
  </div>;
}
