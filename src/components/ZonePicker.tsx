import React from 'react';
import { MultiCombobox } from '@grafana/ui';
import { Zone, ZoneMode } from '../types';

const ALL = '__all_zones__';
const DEFAULT = '__default_zones__';
interface Props {
  mode: ZoneMode;
  ids: string[];
  zones: Zone[];
  variables?: string[];
  allowDefault?: boolean;
  loading?: boolean;
  label: string;
  onChange: (mode: ZoneMode, ids: string[]) => void;
}
export function ZonePicker({ mode, ids, zones, variables = [], allowDefault, loading, label, onChange }: Props) {
  const value = mode === 'all' ? [ALL] : mode === 'default' ? [DEFAULT] : ids;
  return (
    <MultiCombobox
      aria-label={label}
      value={value}
      loading={loading}
      isClearable
      createCustomValue
      customValueDescription="Use zone ID or variable"
      placeholder="Select zones or enter zone IDs"
      options={[
        ...(allowDefault ? [{ value: DEFAULT, label: 'Datasource default' }] : []),
        { value: ALL, label: 'All zones' },
        ...zones.map((z) => ({ value: z.id, label: z.name })),
        ...variables.map((v) => ({ value: v, label: v })),
      ]}
      onChange={(options) => {
        const next = options.map((o) => o.value);
        const added = next.find((id) => !value.includes(id));
        if (added === ALL || added === DEFAULT) {
          onChange(added === ALL ? 'all' : 'default', []);
        } else {
          onChange(
            'selected',
            next.filter((id) => id !== ALL && id !== DEFAULT)
          );
        }
      }}
    />
  );
}
