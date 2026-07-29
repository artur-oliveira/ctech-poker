'use client';

export type FilterOption<T extends string> = {
  value: T;
  label: string;
};

export function FilterGroup<T extends string>({label, value, options, onChange}: {
  label: string;
  value: T;
  options: readonly FilterOption<T>[];
  onChange: (value: T) => void;
}) {
  return <div className="filter-tabs" role="group" aria-label={label}>
    {options.map(option => <button
      key={option.value}
      type="button"
      aria-pressed={value === option.value}
      className={`filter-tab${value === option.value ? ' active' : ''}`}
      onClick={() => onChange(option.value)}
    >
      {option.label}
    </button>)}
  </div>;
}
