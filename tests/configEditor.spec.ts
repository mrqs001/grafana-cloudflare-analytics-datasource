import { test, expect } from '@grafana/plugin-e2e';

test('configuration uses secure token input and explains required access', async ({
  createDataSourceConfigPage,
  readProvisionedDataSource,
  page,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await createDataSourceConfigPage({ type: ds.type });
  await expect(page.getByRole('combobox', { name: 'Default zones' })).toBeVisible();
  await expect(page.locator('#cf-api-token')).toHaveAttribute('type', 'password');
  await expect(page.getByText('Use a read-only token', { exact: false })).toBeVisible();
});

test('missing credential fails Save & test with an actionable message', async ({
  createDataSourceConfigPage,
  readProvisionedDataSource,
}) => {
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  const config = await createDataSourceConfigPage({ type: ds.type });
  await expect(config.saveAndTest()).not.toBeOK();
  await expect(config).toHaveAlert('error', { hasText: 'API token is missing' });
});

test('live provisioned credentials remain configured and health check works', async ({
  gotoDataSourceConfigPage,
  readProvisionedDataSource,
  page,
}) => {
  test.skip(process.env.RUN_LIVE_TESTS !== '1', 'Requires a read-only Cloudflare token');
  const ds = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  const config = await gotoDataSourceConfigPage(ds.uid);
  await expect(page.locator('#cf-api-token')).toHaveValue('configured');
  await expect(config.saveAndTest()).toBeOK();
  await page.screenshot({ path: '.local/configuration.png', fullPage: true });
});
