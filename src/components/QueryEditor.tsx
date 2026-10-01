import React, { useEffect, useState } from 'react';
import { QueryEditorProps } from '@grafana/data';
import { getTemplateSrv } from '@grafana/runtime';
import { Alert, Button, Combobox, Field, Input, TextArea, MultiCombobox, Stack } from '@grafana/ui';
import { DataSource } from '../datasource';
import {
  CloudflareOptions,
  CloudflareQuery,
  DEFAULT_QUERY,
  DIMENSIONS,
  METRICS,
  Zone,
  DatasetSettings,
  QueryFilter,
} from '../types';

type Props = QueryEditorProps<DataSource, CloudflareQuery, CloudflareOptions>;
const operators = [
  { value: 'eq', label: 'equals' },
  { value: 'neq', label: 'does not equal' },
  { value: 'in', label: 'is one of' },
  { value: 'notIn', label: 'is not one of' },
];
export function QueryEditor({ query, datasource, onChange, onRunQuery }: Props) {
  const q = { ...DEFAULT_QUERY, ...query };
  const [zones, setZones] = useState<Zone[]>([]);
  const [settingsResult, setSettings] = useState<{ zone: string; value: DatasetSettings }>();
  const [discoveryError, setDiscoveryError] = useState('');
  const [settingsError, setSettingsError] = useState('');
  useEffect(() => {
    let active = true;
    datasource
      .zones()
      .then((value) => {
        if (active) {
          setZones(value);
          setDiscoveryError('');
        }
      })
      .catch(() => {
        if (active) {
          setDiscoveryError(
            'Zone discovery is unavailable. Enter a zone ID or $zone variable. Discovery requires Zone Read.'
          );
        }
      });
    return () => {
      active = false;
    };
  }, [datasource]);
  const settings = settingsResult?.zone === q.zoneId ? settingsResult.value : undefined;
  useEffect(() => {
    let active = true;
    if (q.zoneId || datasource.defaultZoneId) {
      datasource
        .settings(q.zoneId)
        .then((value) => {
          if (active) {
            setSettings({ zone: q.zoneId, value });
            setSettingsError('');
          }
        })
        .catch(() => {
          if (active) {
            setSettingsError(
              'Could not discover plan limits for this zone. Run the query for the detailed backend error.'
            );
          }
        });
    }
    return () => {
      active = false;
    };
  }, [datasource, q.zoneId]);
  const update = (patch: Partial<CloudflareQuery>, run = true) => {
    onChange({ ...q, ...patch });
    if (run) {
      onRunQuery();
    }
  };
  const updateFilter = (index: number, patch: Partial<QueryFilter>, run = true) =>
    update({ filters: q.filters.map((f, i) => (i === index ? { ...f, ...patch } : f)) }, run);
  const available = DIMENSIONS.filter((d) => !settings || settings.availableFields.includes(`dimensions_${d.field}`));
  const zoneOptions = [
    ...zones.map((z) => ({ value: z.id, label: z.name, description: z.account.name })),
    ...getTemplateSrv()
      .getVariables()
      .map((v) => ({ value: `$${v.name}`, label: `$${v.name}` })),
  ];
  return (
    <Stack direction="column" gap={2}>
      {discoveryError && <Alert title={discoveryError} severity="warning" />}
      {settingsError && <Alert title={settingsError} severity="warning" />}
      <Stack direction="row" gap={2} wrap="wrap">
        <Field label="Zone" description="Select a zone, enter its ID, or use a single-value variable.">
          <Combobox
            width={40}
            options={zoneOptions}
            value={q.zoneId || datasource.defaultZoneId}
            createCustomValue
            onChange={(v) => update({ zoneId: v.value })}
          />
        </Field>
        <Field label="Metric">
          <Combobox width={28} options={METRICS} value={q.metric} onChange={(v) => update({ metric: v.value })} />
        </Field>
        <Field label="Result">
          <Combobox
            width={24}
            options={[
              { value: 'timeSeries', label: 'Time series' },
              { value: 'total', label: 'Range totals / top values' },
            ]}
            value={q.format}
            onChange={(v) => update({ format: v.value as CloudflareQuery['format'] })}
          />
        </Field>
      </Stack>
      <Stack direction="row" gap={2} wrap="wrap">
        <Field label="Group by" description="Up to three dimensions. Top series are ranked over the full time range.">
          <MultiCombobox
            width={50}
            options={available}
            value={q.groupBy}
            onChange={(v) => update({ groupBy: v.map((x) => x.value) })}
          />
        </Field>
        <Field label="Interval">
          <Combobox
            width={20}
            value={q.interval}
            options={['auto', '1m', '5m', '15m', '1h', '6h', '24h'].map((value) => ({
              value,
              label: value === 'auto' ? 'Auto' : value,
            }))}
            onChange={(v) => update({ interval: v.value })}
          />
        </Field>
        <Field label="Top series">
          <Input
            id={`cf-series-${q.refId}`}
            width={12}
            type="number"
            min={1}
            max={200}
            value={q.maxSeries}
            onChange={(e) => update({ maxSeries: Number(e.currentTarget.value) }, false)}
            onBlur={onRunQuery}
          />
        </Field>
        <Field label="Missing buckets">
          <Combobox
            width={22}
            value={q.fill}
            options={[
              { value: 'null', label: 'Null (unknown)' },
              { value: 'zero', label: 'Zero (assume no traffic)' },
            ]}
            onChange={(v) => update({ fill: v.value as CloudflareQuery['fill'] })}
          />
        </Field>
      </Stack>
      {q.filters.map((f, i) => (
        <Stack key={i} direction="row" gap={1} alignItems="end" wrap="wrap">
          <Field label={`Filter ${i + 1}`}>
            <Combobox
              width={26}
              options={available}
              value={f.field}
              onChange={(v) => updateFilter(i, { field: v.value })}
            />
          </Field>
          <Field label={`Operator ${i + 1}`}>
            <Combobox
              width={24}
              options={operators}
              value={f.operator}
              onChange={(v) => updateFilter(i, { operator: v.value as QueryFilter['operator'] })}
            />
          </Field>
          <Field
            label={`Values ${i + 1}`}
            description="One value per line; a whole $variable expands into exact values. Use ‘is one of’ for multi-value variables."
          >
            <TextArea
              id={`cf-filter-${q.refId}-${i}`}
              width={50}
              value={f.values.join('\n')}
              placeholder="eyeball, example.com, or $hostname"
              onChange={(e) => updateFilter(i, { values: e.currentTarget.value.split('\n') }, false)}
              onBlur={onRunQuery}
            />
          </Field>
          <Button
            variant="secondary"
            icon="trash-alt"
            aria-label={`Remove filter ${i + 1}`}
            onClick={() => update({ filters: q.filters.filter((_, index) => index !== i) })}
          />
        </Stack>
      ))}
      <div>
        <Button
          variant="secondary"
          icon="plus"
          onClick={() =>
            update({ filters: [...q.filters, { field: 'hostname', operator: 'eq', values: [''] }] }, false)
          }
        >
          Add filter
        </Button>
      </div>
      <div>
        UTC bucket starts · exact [from, to) range · rates use covered seconds · adaptive estimates, already scaled by
        Cloudflare.
      </div>
      {settings && (
        <div>
          Zone limits: {Math.round(settings.notOlderThan / 86400)} days retention ·{' '}
          {Math.round(settings.maxDuration / 86400)} days per API request · {settings.maxPageSize.toLocaleString()} rows
          per response. Larger requests are split automatically.
        </div>
      )}
    </Stack>
  );
}
