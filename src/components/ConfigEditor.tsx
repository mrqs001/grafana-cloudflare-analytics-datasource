import React from 'react';
import { Alert, Field, Input, SecretInput, Stack } from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { CloudflareOptions, CloudflareSecrets } from '../types';

type Props = DataSourcePluginOptionsEditorProps<CloudflareOptions, CloudflareSecrets>;
export function ConfigEditor({ options, onOptionsChange }: Props) {
  return (
    <Stack direction="column" gap={2}>
      <Alert title="Read-only Cloudflare Analytics" severity="info">
        Use a scoped API token with Analytics Read and Zone Read for discovery. Credentials are stored in Grafana secure
        settings and used only by the Go backend.
      </Alert>
      <Field label="API token" description="Cloudflare API token. No write permissions are required.">
        <SecretInput
          id="cf-api-token"
          width={60}
          isConfigured={options.secureJsonFields.apiToken}
          value={options.secureJsonData?.apiToken ?? ''}
          onChange={(e) =>
            onOptionsChange({
              ...options,
              secureJsonData: { ...options.secureJsonData, apiToken: e.currentTarget.value },
            })
          }
          onReset={() =>
            onOptionsChange({
              ...options,
              secureJsonFields: { ...options.secureJsonFields, apiToken: false },
              secureJsonData: { ...options.secureJsonData, apiToken: '' },
            })
          }
        />
      </Field>
      <Field
        label="Default zone ID"
        description="Optional 32-character zone ID. Allows querying without Zone Read discovery permission."
      >
        <Input
          id="cf-default-zone"
          width={60}
          value={options.jsonData.defaultZoneId ?? ''}
          placeholder="Zone ID from Cloudflare Overview"
          onChange={(e) =>
            onOptionsChange({
              ...options,
              jsonData: { ...options.jsonData, defaultZoneId: e.currentTarget.value.trim() },
            })
          }
        />
      </Field>
    </Stack>
  );
}
