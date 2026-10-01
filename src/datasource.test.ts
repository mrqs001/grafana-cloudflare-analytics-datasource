import { getTemplateSrv } from '@grafana/runtime';
import { expandValues } from './datasource';

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
