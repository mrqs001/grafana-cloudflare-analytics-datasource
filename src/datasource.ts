import { DataSourceInstanceSettings, CoreApp, ScopedVars, MetricFindValue } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { CloudflareQuery, CloudflareOptions, DEFAULT_QUERY, Zone, DatasetSettings, Account, DIMENSIONS } from './types';

export function expandValues(values: string[], scopedVars: ScopedVars): string[] {
  return values.flatMap((text) => {
    // A whole variable may expand into many exact values, preserving commas and quotes.
    if (/^(\$[a-zA-Z_][\w]*|\$\{[a-zA-Z_][\w]*\})$/.test(text)) {
      const result: string[] = [];
      getTemplateSrv().replace(text, scopedVars, (value: string | string[]) => {
        result.push(...(Array.isArray(value) ? value : [value]));
        return '';
      });
      return result.length ? result : [text];
    }
    return [getTemplateSrv().replace(text, scopedVars, 'raw')];
  });
}
export class DataSource extends DataSourceWithBackend<CloudflareQuery, CloudflareOptions> {
  readonly defaultZoneId: string;
  constructor(settings: DataSourceInstanceSettings<CloudflareOptions>) {
    super(settings);
    this.defaultZoneId = settings.jsonData.defaultZoneId ?? '';
  }
  getDefaultQuery(_: CoreApp): Partial<CloudflareQuery> {
    return {
      ...DEFAULT_QUERY,
      zoneId: this.defaultZoneId,
      filters: DEFAULT_QUERY.filters.map((f) => ({ ...f, values: [...f.values] })),
    };
  }
  applyTemplateVariables(query: CloudflareQuery, scopedVars: ScopedVars): CloudflareQuery {
    return {
      ...query,
      zoneId: getTemplateSrv().replace(query.zoneId || this.defaultZoneId, scopedVars, 'raw'),
      filters: (query.filters ?? DEFAULT_QUERY.filters).map((f) => ({
        ...f,
        values: expandValues(f.values, scopedVars),
      })),
    };
  }
  filterQuery(query: CloudflareQuery): boolean {
    return !query.hide;
  }
  zones(): Promise<Zone[]> {
    return this.getResource('zones');
  }
  accounts(): Promise<Account[]> {
    return this.getResource('accounts');
  }
  settings(zoneId: string): Promise<DatasetSettings> {
    return this.getResource('settings', { zoneId: getTemplateSrv().replace(zoneId || this.defaultZoneId, {}, 'raw') });
  }
  async metricFindQuery(query: string): Promise<MetricFindValue[]> {
    const resolved = getTemplateSrv().replace(query, {}, 'raw').trim();
    if (resolved === 'zones()') {
      return (await this.zones()).map((z) => ({ text: z.name, value: z.id }));
    }
    if (resolved === 'accounts()') {
      return (await this.accounts()).map((a) => ({ text: a.name, value: a.id }));
    }
    const match = /^values\(([a-zA-Z]+),\s*([a-fA-F0-9]{32})\)$/.exec(resolved);
    if (match && DIMENSIONS.some((d) => d.value === match[1])) {
      const values: string[] = await this.getResource('values', { field: match[1], zoneId: match[2] });
      return values.map((text) => ({ text, value: text }));
    }
    throw new Error('Use zones(), accounts(), or values(hostname, $zone). Values use the last complete hour.');
  }
}
