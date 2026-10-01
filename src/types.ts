import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export const DIMENSIONS = [
  { value: 'status', label: 'Edge status', field: 'edgeResponseStatus' },
  { value: 'originStatus', label: 'Origin status', field: 'originResponseStatus' },
  { value: 'hostname', label: 'Hostname', field: 'clientRequestHTTPHost' },
  { value: 'cacheStatus', label: 'Cache status', field: 'cacheStatus' },
  { value: 'country', label: 'Country', field: 'clientCountryName' },
  { value: 'colo', label: 'Cloudflare colo', field: 'coloCode' },
  { value: 'method', label: 'HTTP method', field: 'clientRequestHTTPMethodName' },
  { value: 'requestSource', label: 'Request source', field: 'requestSource' },
  { value: 'path', label: 'URI path', field: 'clientRequestPath' },
  { value: 'securityAction', label: 'Security action', field: 'securityAction' },
  { value: 'securitySource', label: 'Security source', field: 'securitySource' },
];
export const METRICS = [
  { value: 'requests', label: 'Requests', description: 'Estimated requests per bucket or over the selected range' },
  { value: 'requestRate', label: 'Request rate', description: 'Estimated requests per second' },
  { value: 'bytes', label: 'Response bytes', description: 'Total edge response bytes' },
  { value: 'bandwidth', label: 'Bandwidth', description: 'Edge response bytes per second' },
];
export interface QueryFilter {
  field: string;
  operator: 'eq' | 'neq' | 'in' | 'notIn';
  values: string[];
}
export type ZoneMode = 'default' | 'selected' | 'all';
export interface CloudflareQuery extends DataQuery {
  zoneMode?: ZoneMode;
  zoneIds?: string[];
  zoneId: string;
  metric: string;
  groupBy: string[];
  filters: QueryFilter[];
  interval: string;
  format: 'timeSeries' | 'total';
  maxSeries: number;
  fill: 'null' | 'zero';
}
export interface CloudflareOptions extends DataSourceJsonData {
  defaultZoneId?: string;
  defaultZoneIds?: string[];
  defaultZoneMode?: Exclude<ZoneMode, 'default'>;
}
export interface CloudflareSecrets {
  apiToken?: string;
}
export interface Account {
  id: string;
  name: string;
}
export interface Zone {
  id: string;
  name: string;
  account?: Account;
}
export interface DatasetSettings {
  enabled: boolean;
  availableFields: string[];
  maxDuration: number;
  notOlderThan: number;
  maxPageSize: number;
}
export const DEFAULT_QUERY: Omit<CloudflareQuery, 'refId'> = {
  zoneId: '',
  metric: 'requests',
  groupBy: [],
  filters: [{ field: 'requestSource', operator: 'eq', values: ['eyeball'] }],
  interval: 'auto',
  format: 'timeSeries',
  maxSeries: 20,
  fill: 'null',
};
