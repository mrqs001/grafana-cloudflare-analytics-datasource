import React, { useEffect, useState } from 'react';
import { css } from '@emotion/css';
import { QueryEditorProps } from '@grafana/data';
import { getTemplateSrv } from '@grafana/runtime';
import { Button, Combobox, Field, Input, TextArea, MultiCombobox, Tooltip, Icon } from '@grafana/ui';
import { DataSource, zoneSelection } from '../datasource';
import { CloudflareOptions, CloudflareQuery, DEFAULT_QUERY, DIMENSIONS, METRICS, Zone, QueryFilter } from '../types';
import { ZonePicker } from './ZonePicker';

type Props = QueryEditorProps<DataSource, CloudflareQuery, CloudflareOptions>;
const operators = [
  { value: 'eq', label: 'equals' },
  { value: 'neq', label: 'does not equal' },
  { value: 'in', label: 'is one of' },
  { value: 'notIn', label: 'is not one of' },
];
const layout = css`
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 8px 0;
  .cf-row {
    display: flex;
    align-items: end;
    flex-wrap: wrap;
    gap: 12px;
  }
  .cf-wide {
    flex: 2 1 280px;
    min-width: 0;
  }
  .cf-medium {
    flex: 1 1 190px;
    min-width: 0;
  }
  .cf-small {
    flex: 0 1 150px;
    min-width: 120px;
  }
  .cf-filter {
    display: flex;
    align-items: end;
    flex-wrap: wrap;
    gap: 8px;
  }
  .cf-actions {
    display: flex;
    gap: 12px;
    align-items: center;
  }
`;
export function QueryEditor({ query, datasource, onChange, onRunQuery }: Props) {
  const q = { ...DEFAULT_QUERY, ...query };
  const selection = zoneSelection(query);
  const [zones, setZones] = useState<Zone[]>([]);
  const [discoveryError, setDiscoveryError] = useState('');
  const [optionsOpen, setOptionsOpen] = useState(false);
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
          setDiscoveryError('Zone discovery needs Zone Read. Enter zone IDs or use a dashboard variable.');
        }
      });
    return () => {
      active = false;
    };
  }, [datasource]);
  const update = (patch: Partial<CloudflareQuery>, run = true) => {
    onChange({ ...q, ...patch });
    if (run) {
      onRunQuery();
    }
  };
  const updateFilter = (index: number, patch: Partial<QueryFilter>, run = true) =>
    update({ filters: q.filters.map((f, i) => (i === index ? { ...f, ...patch } : f)) }, run);
  return (
    <div className={layout} data-testid="cloudflare-query-editor">
      <div className="cf-row">
        <Field noMargin className="cf-wide" label="Zones">
          <ZonePicker
            label="Zones"
            mode={selection.mode}
            ids={selection.ids}
            zones={zones}
            allowDefault
            variables={getTemplateSrv()
              .getVariables()
              .map((v) => `$${v.name}`)}
            onChange={(zoneMode, zoneIds) => update({ zoneMode, zoneIds, zoneId: '' })}
          />
        </Field>
        <Field noMargin className="cf-medium" label="Metric">
          <Combobox options={METRICS} value={q.metric} onChange={(v) => update({ metric: v.value })} />
        </Field>
        <Field noMargin className="cf-medium" label="Result">
          <Combobox
            options={[
              { value: 'timeSeries', label: 'Time series' },
              { value: 'total', label: 'Range totals / top values' },
            ]}
            value={q.format}
            onChange={(v) => update({ format: v.value as CloudflareQuery['format'] })}
          />
        </Field>
      </div>
      {discoveryError && <div role="status">{discoveryError}</div>}
      <div className="cf-row">
        <Field noMargin className="cf-wide" label="Group by">
          <MultiCombobox
            options={DIMENSIONS}
            value={q.groupBy}
            placeholder="None — one series per zone"
            onChange={(v) => update({ groupBy: v.map((x) => x.value) })}
          />
        </Field>
        <Field noMargin className="cf-small" label="Interval">
          <Combobox
            value={q.interval}
            options={['auto', '1m', '5m', '15m', '1h', '6h', '24h'].map((value) => ({
              value,
              label: value === 'auto' ? 'Auto' : value,
            }))}
            onChange={(v) => update({ interval: v.value })}
          />
        </Field>
      </div>
      {q.filters.map((f, i) => (
        <div className="cf-filter" key={i}>
          <Field noMargin className="cf-medium" label={`Filter ${i + 1}`}>
            <Combobox options={DIMENSIONS} value={f.field} onChange={(v) => updateFilter(i, { field: v.value })} />
          </Field>
          <Field noMargin className="cf-medium" label={`Operator ${i + 1}`}>
            <Combobox
              options={operators}
              value={f.operator}
              onChange={(v) => updateFilter(i, { operator: v.value as QueryFilter['operator'] })}
            />
          </Field>
          <Field noMargin className="cf-wide" label={`Values ${i + 1}`}>
            {f.operator === 'in' || f.operator === 'notIn' ? (
              <TextArea
                id={`cf-filter-${q.refId}-${i}`}
                rows={Math.min(4, Math.max(1, f.values.length))}
                value={f.values.join('\n')}
                placeholder="One value per line, or $variable"
                onChange={(e) => updateFilter(i, { values: e.currentTarget.value.split('\n') }, false)}
                onBlur={onRunQuery}
              />
            ) : (
              <Input
                id={`cf-filter-${q.refId}-${i}`}
                value={f.values.join('\n')}
                placeholder="Value or $variable"
                onChange={(e) => updateFilter(i, { values: [e.currentTarget.value] }, false)}
                onBlur={onRunQuery}
              />
            )}
          </Field>
          <Button
            variant="secondary"
            icon="trash-alt"
            aria-label={`Remove filter ${i + 1}`}
            onClick={() => update({ filters: q.filters.filter((_, index) => index !== i) })}
          />
        </div>
      ))}
      <div className="cf-actions">
        <Button
          size="sm"
          variant="secondary"
          icon="plus"
          onClick={() =>
            update({ filters: [...q.filters, { field: 'hostname', operator: 'eq', values: [''] }] }, false)
          }
        >
          Add filter
        </Button>
        <Button
          size="sm"
          variant="secondary"
          icon={optionsOpen ? 'angle-up' : 'angle-down'}
          aria-expanded={optionsOpen}
          onClick={() => setOptionsOpen(!optionsOpen)}
        >
          Options
        </Button>
        <Tooltip content="Select up to 20 zones and three grouping dimensions. Each zone produces separate series. Multi-value filters accept one value per line or a whole dashboard variable. UTC buckets cover the exact selected range; Cloudflare adaptive estimates are already scaled.">
          <Icon name="info-circle" tabIndex={0} aria-label="Query help" />
        </Tooltip>
      </div>
      {optionsOpen && (
        <div className="cf-row">
          <Field noMargin className="cf-small" label="Top series per zone">
            <Input
              id={`cf-series-${q.refId}`}
              type="number"
              min={1}
              max={200}
              value={q.maxSeries}
              onChange={(e) => update({ maxSeries: Number(e.currentTarget.value) }, false)}
              onBlur={onRunQuery}
            />
          </Field>
          <Field noMargin className="cf-medium" label="Missing buckets">
            <Combobox
              value={q.fill}
              options={[
                { value: 'null', label: 'Null (unknown)' },
                { value: 'zero', label: 'Zero (assume no traffic)' },
              ]}
              onChange={(v) => update({ fill: v.value as CloudflareQuery['fill'] })}
            />
          </Field>
        </div>
      )}
    </div>
  );
}
