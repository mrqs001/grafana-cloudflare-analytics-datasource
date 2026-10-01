import { test, expect } from '@grafana/plugin-e2e';

const zones = [
  { id: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', name: 'alpha.example', account: { name: 'private@example.com' } },
  { id: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', name: 'beta.example', account: { name: 'private@example.com' } },
];

test('Explore selects one, multiple, all and default zones in a compact editor', async ({
  explorePage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await page.route('**/resources/zones', (route) => route.fulfill({ json: zones }));
  const queries: Array<{ zoneMode: string; zoneIds: string[] }> = [];
  await page.route('**/api/ds/query*', (route) => {
    queries.push(...route.request().postDataJSON().queries);
    return route.fulfill({ json: { results: { A: { status: 200, frames: [] } } } });
  });
  await explorePage.goto();
  await explorePage.datasource.set(ds.name);
  const editor = page.getByTestId('cloudflare-query-editor');
  const picker = editor.getByRole('combobox', { name: 'Zones', exact: true });
  await picker.click();
  await expect(page.getByRole('option', { name: 'alpha.example', exact: true })).toBeVisible();
  await expect(page.getByText('private@example.com')).toHaveCount(0);
  await page.getByRole('option', { name: 'alpha.example', exact: true }).click();
  await page.getByTestId('cloudflare-query-editor').click({ position: { x: 2, y: 2 } });
  await explorePage.runQuery();
  expect(queries.at(-1)).toMatchObject({ zoneMode: 'selected', zoneIds: [zones[0].id] });
  await picker.click();
  await page.getByRole('option', { name: 'beta.example', exact: true }).click();
  await page.getByTestId('cloudflare-query-editor').click({ position: { x: 2, y: 2 } });
  await explorePage.runQuery();
  expect(queries.at(-1)).toMatchObject({ zoneMode: 'selected', zoneIds: zones.map((z) => z.id) });
  await picker.click();
  await page.getByRole('option', { name: 'All zones', exact: true }).click();
  await page.getByTestId('cloudflare-query-editor').click({ position: { x: 2, y: 2 } });
  await explorePage.runQuery();
  expect(queries.at(-1)).toMatchObject({ zoneMode: 'all', zoneIds: [] });
  const bounds = await editor.boundingBox();
  expect(bounds?.height).toBeLessThan(320);
  await page.screenshot({ path: '.local/explore-zones.png', fullPage: true });
  await editor.getByRole('button', { name: 'Options', exact: true }).click();
  await expect(editor.getByRole('spinbutton', { name: 'Top series per zone' })).toBeVisible();
  await picker.click();
  await page.getByRole('option', { name: 'Datasource default', exact: true }).click();
  await page.getByTestId('cloudflare-query-editor').click({ position: { x: 2, y: 2 } });
  await explorePage.runQuery();
  expect(queries.at(-1)).toMatchObject({ zoneMode: 'default', zoneIds: [] });
  await page.setViewportSize({ width: 900, height: 900 });
  await expect(picker).toBeVisible();
  expect(await editor.evaluate((e) => e.scrollWidth <= e.clientWidth)).toBeTruthy();
});

test('datasource settings can save one, multiple and all default zones', async ({
  gotoDataSourceConfigPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  let saved = {};
  await page.route(
    (url) => url.pathname === `/api/datasources/uid/${ds.uid}`,
    async (route) => {
      if (route.request().method() === 'PUT') {
        saved = route.request().postDataJSON().jsonData;
        return route.fulfill({ json: { message: 'Datasource updated', datasource: route.request().postDataJSON() } });
      }
      const response = await route.fetch();
      const body = await response.json();
      return route.fulfill({
        json: { ...body, readOnly: false, jsonData: saved, secureJsonFields: { apiToken: true } },
      });
    }
  );
  await page.route('**/resources/zones', (route) => route.fulfill({ json: zones }));
  await page.route('**/health', (route) => route.fulfill({ json: { status: 'OK', message: 'Test health' } }));
  const config = await gotoDataSourceConfigPage(ds.uid);
  const picker = page.getByRole('combobox', { name: 'Default zones' });
  for (const [label, mode, ids] of [
    ['alpha.example', 'selected', [zones[0].id]],
    ['beta.example', 'selected', zones.map((z) => z.id)],
    ['All zones', 'all', []],
  ] as const) {
    await picker.click();
    await page.getByRole('option', { name: label, exact: true }).click();
    await picker.press('Escape');
    const save = page.waitForRequest((r) => r.method() === 'PUT' && r.url().includes(`/api/datasources/uid/${ds.uid}`));
    await expect(config.saveAndTest()).toBeOK();
    expect((await save).postDataJSON().jsonData).toMatchObject({ defaultZoneMode: mode, defaultZoneIds: ids });
  }
});

test('alert rule editor exposes the same compact zone picker', async ({
  alertRuleEditPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await page.route('**/resources/zones', (route) => route.fulfill({ json: zones }));
  const row = await alertRuleEditPage.getQueryRow();
  await row.datasource.set(ds.name);
  const editor = page.getByTestId('cloudflare-query-editor');
  await expect(editor.getByRole('combobox', { name: 'Zones', exact: true })).toBeVisible();
  await editor.getByRole('combobox', { name: 'Zones', exact: true }).click();
  await page.getByRole('option', { name: 'All zones', exact: true }).click();
  await page.getByTestId('cloudflare-query-editor').click({ position: { x: 2, y: 2 } });
  await expect(editor.getByRole('button', { name: 'Options', exact: true })).toBeVisible();
  await page.screenshot({ path: '.local/alert-editor.png', fullPage: true });
});
