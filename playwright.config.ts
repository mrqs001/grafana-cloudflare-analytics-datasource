import type { PluginOptions } from '@grafana/plugin-e2e';
import { defineConfig } from '@playwright/test';
import baseConfig from './.config/playwright.config';

export default defineConfig<PluginOptions>(baseConfig, {
  workers: 1,
  use: { baseURL: process.env.GRAFANA_URL || 'http://localhost:3300', trace: 'off' },
});
