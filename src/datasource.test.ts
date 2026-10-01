import { getTemplateSrv } from '@grafana/runtime';
import { expandValues, resolveZoneVariables } from './datasource';

jest.mock('@grafana/runtime', () => ({
  DataSourceWithBackend: class {},
  getTemplateSrv: jest.fn(),
}));

test('multi-value variables preserve literal commas and quotes without query interpolation', () => {
  const replace = jest.fn((text, _vars, format) =>
    typeof format === 'function' ? format(['a,b.example', 'quoted"host']) : text
  );
  jest.mocked(getTemplateSrv).mockReturnValue({ replace } as unknown as ReturnType<typeof getTemplateSrv>);
  expect(expandValues(['$host'], {})).toEqual(['a,b.example', 'quoted"host']);
  expect(expandValues(['literal,value'], {})).toEqual(['literal,value']);
});

test('unresolved variables remain visible for backend validation', () => {
  jest
    .mocked(getTemplateSrv)
    .mockReturnValue({ replace: jest.fn((text) => text) } as unknown as ReturnType<typeof getTemplateSrv>);
  expect(expandValues(['$missing'], {})).toEqual(['$missing']);
});

const base = {
  refId: 'A',
  zoneId: '$zone',
  metric: 'requests',
  groupBy: [],
  filters: [],
  interval: 'auto',
  format: 'timeSeries' as const,
  maxSeries: 20,
  fill: 'null' as const,
};
test('legacy zone variables expand to several IDs and deduplicate', () => {
  jest.mocked(getTemplateSrv).mockReturnValue({
    replace: jest.fn((_text, _vars, format) => (typeof format === 'function' ? format(['a', 'b', 'a']) : '')),
  } as unknown as ReturnType<typeof getTemplateSrv>);
  expect(resolveZoneVariables(base, {})).toEqual({ zoneId: '', zoneMode: 'selected', zoneIds: ['a', 'b'] });
});
test('custom All variable resolves to dynamic all-zone mode', () => {
  jest.mocked(getTemplateSrv).mockReturnValue({
    replace: jest.fn((_text, _vars, format) => (typeof format === 'function' ? format('*') : '')),
  } as unknown as ReturnType<typeof getTemplateSrv>);
  expect(resolveZoneVariables(base, {})).toEqual({ zoneId: '', zoneMode: 'all', zoneIds: [] });
  expect(resolveZoneVariables({ ...base, zoneMode: 'default' }, {})).toEqual({
    zoneId: '',
    zoneMode: 'default',
    zoneIds: [],
  });
});

test('Grafana custom All replacement can bypass the formatter callback', () => {
  jest
    .mocked(getTemplateSrv)
    .mockReturnValue({ replace: jest.fn(() => '*') } as unknown as ReturnType<typeof getTemplateSrv>);
  expect(resolveZoneVariables(base, {})).toEqual({ zoneId: '', zoneMode: 'all', zoneIds: [] });
});
