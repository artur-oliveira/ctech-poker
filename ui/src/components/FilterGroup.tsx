'use client';

export type FilterOption<T extends string> = {
  value: T;
  label: string;
  disabled?: boolean;
  title?: string;
};

export function FilterGroup<T extends string>({label, value, options, onChangeAction}: {
  label: string;
  value: T;
  options: readonly FilterOption<T>[];
  onChangeAction: (value: T) => void;
}) {
  return <div className="filter-tabs" role="group" aria-label={label}>
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
