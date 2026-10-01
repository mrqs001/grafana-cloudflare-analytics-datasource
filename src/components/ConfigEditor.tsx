import React, { useEffect, useState } from 'react';
import { Button, Field, SecretInput, Stack } from '@grafana/ui';
import { getBackendSrv } from '@grafana/runtime';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { CloudflareOptions, CloudflareSecrets, Zone } from '../types';
import { ZonePicker } from './ZonePicker';

type Props = DataSourcePluginOptionsEditorProps<CloudflareOptions, CloudflareSecrets>;
export function ConfigEditor({ options, onOptionsChange }: Props) {
  const [zones, setZones] = useState<Zone[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const canDiscover = Boolean(options.uid && options.secureJsonFields.apiToken && !options.secureJsonData?.apiToken);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    let active = true;
    if (canDiscover) {
      getBackendSrv()
        .get<Zone[]>(`/api/datasources/uid/${encodeURIComponent(options.uid)}/resources/zones`)
        .then((value) => {
          if (active) {
            setZones(value);
            setError('');
          }
        })
        .catch(() => {
          if (active) {
            setError('Zone discovery requires Zone Read. You can enter zone IDs instead.');
          }
        })
        .finally(() => {
          if (active) {
            setLoading(false);
          }
        });
    }
    return () => {
      active = false;
    };
  }, [canDiscover, options.uid, refresh]);
  const ids = options.jsonData.defaultZoneIds?.length
    ? options.jsonData.defaultZoneIds
    : options.jsonData.defaultZoneId
      ? [options.jsonData.defaultZoneId]
      : [];
  const mode = options.jsonData.defaultZoneMode ?? (ids.length ? 'selected' : 'all');
  return (
    <Stack direction="column" gap={2}>
      <Field
        label="API token"
        description="Use a read-only token with Analytics Read and Zone Read. Grafana stores it securely."
      >
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
      <div style={{ width: '100%', maxWidth: 600 }}>
        <Field
          label="Default zones"
          description="Choose one, several, or all zones. New queries inherit this selection."
        >
          <ZonePicker
            label="Default zones"
            mode={mode}
            ids={ids}
            zones={zones}
            loading={loading}
            onChange={(nextMode, nextIds) =>
              onOptionsChange({
                ...options,
                jsonData: {
                  ...options.jsonData,
                  defaultZoneId: '',
                  defaultZoneMode: nextMode === 'all' ? 'all' : 'selected',
                  defaultZoneIds: nextIds,
                },
              })
            }
          />
        </Field>
        {canDiscover ? (
          <Button
            size="sm"
            variant="secondary"
            icon="sync"
            disabled={loading}
            onClick={() => {
              setLoading(true);
              setRefresh((n) => n + 1);
            }}
          >
            Refresh zones
          </Button>
        ) : (
          <div>Save &amp; test your token to discover zones, or enter zone IDs.</div>
        )}
        {error && <div role="status">{error}</div>}
      </div>
    </Stack>
  );
}
