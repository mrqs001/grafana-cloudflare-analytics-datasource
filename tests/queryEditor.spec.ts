import { test, expect } from '@grafana/plugin-e2e';

test('query editor offers typed metrics, dimensions and filters', async ({
  panelEditPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(ds.name);
  const editor = panelEditPage.getQueryEditorRow('A');
  await expect(editor.getByRole('combobox', { name: 'Metric', exact: true })).toBeVisible();
  await expect(editor.getByRole('combobox', { name: /^Group by/ })).toBeVisible();
  await expect(editor.getByRole('textbox', { name: /^Values 1/ })).toHaveValue('eyeball');
  await editor.getByRole('button', { name: 'Add filter' }).click();
  await expect(editor.getByRole('textbox', { name: /^Values 2/ })).toBeVisible();
  await editor.getByRole('button', { name: 'Remove filter 2' }).click();
  await expect(editor.getByRole('textbox', { name: /^Values 2/ })).toHaveCount(0);
  await page.screenshot({ path: '.local/query-editor.png', fullPage: true });
});

test('live dashboard resolves zone variable and renders actual analytics', async ({
  readProvisionedDashboard,
  gotoDashboardPage,
  page,
}) => {
  test.skip(process.env.RUN_LIVE_TESTS !== '1', 'Requires a read-only Cloudflare token');
  test.setTimeout(120000);
  const dashboard = await readProvisionedDashboard({ fileName: 'overview.json' });
  const requests: Array<{ zoneMode: string; zoneIds: string[]; metric: string }> = [];
  const responses: Array<
    Promise<{
      status: number;
      body: { results: Record<string, { error?: string; frames?: Array<{ data: { values: unknown[][] } }> }> };
    }>
  > = [];
  page.on('response', (r) => {
    if (r.url().includes('/api/ds/query') && r.request().method() === 'POST') {
      responses.push(r.json().then((body) => ({ status: r.status(), body })));
    }
  });
  page.on('request', (r) => {
    if (r.url().includes('/api/ds/query') && r.method() === 'POST') {
      requests.push(...(r.postDataJSON().queries ?? []));
    }
  });
  const view = await gotoDashboardPage({ uid: dashboard.uid, timeRange: { from: 'now-6h', to: 'now-2m' } });
  await view.waitForPanelsQueriesToComplete({ timeout: 90000, scrollAll: true });
  expect(
    requests.some(
      (q) => (q.zoneMode === 'all' || q.zoneIds?.some((id) => /^[a-f0-9]{32}$/.test(id))) && q.metric === 'requests'
    )
  ).toBeTruthy();
  const completed = await Promise.all(responses);
  expect(completed.length).toBeGreaterThan(0);
  for (const response of completed) {
    expect(response.status).toBe(200);
    for (const result of Object.values(response.body.results)) {
      expect(result.error).toBeUndefined();
    }
  }
  expect(
    completed.some((r) =>
      Object.values(r.body.results).some((v) =>
        v.frames?.some((f) => f.data.values.some((values) => values.length > 0))
      )
    )
  ).toBeTruthy();
  await expect(page.getByText('Reading this dashboard', { exact: true })).toBeVisible();
  await page.screenshot({ path: '.local/dashboard.png', fullPage: true });
});

test('live query editor changes the metric and grouping through Grafana', async ({
  readProvisionedDashboard,
  gotoPanelEditPage,
  page,
}) => {
  test.skip(process.env.RUN_LIVE_TESTS !== '1', 'Requires a read-only Cloudflare token');
  const dashboard = await readProvisionedDashboard({ fileName: 'overview.json' });
  const panel = await gotoPanelEditPage({ dashboard: { uid: dashboard.uid }, id: '4' });
  const editor = panel.getQueryEditorRow('A');
  await editor.getByRole('combobox', { name: 'Metric', exact: true }).click();
  await page.getByRole('option', { name: /Response bytes/ }).click();
  await editor.getByRole('combobox', { name: /^Group by/ }).click();
  const response = panel.waitForQueryDataResponse((r) =>
    r
      .request()
      .postDataJSON()
      .queries.some(
        (q: { metric: string; groupBy?: string[] }) => q.metric === 'bytes' && q.groupBy?.includes('status')
      )
  );
  await page.getByRole('option', { name: 'Edge status', exact: true }).click();
  await expect(response).toBeOK();
  await page.getByTestId('cloudflare-query-editor').click({ position: { x: 2, y: 2 } });
  await page.setViewportSize({ width: 1600, height: 1100 });
  await editor.getByRole('combobox', { name: 'Metric', exact: true }).scrollIntoViewIfNeeded();
  await page.screenshot({ path: '.local/live-query-editor.png', fullPage: true });
});
