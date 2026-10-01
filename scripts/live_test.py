#!/usr/bin/env python3
"""Opt-in read-only integration checks. Credentials stay in memory; output is redacted.

Run after starting the local Grafana: python3 scripts/live_test.py
Uses CF_API_TOKEN from the environment, with the ignored .env as a local fallback.
"""
import base64
import datetime as dt
import json
import math
import os
from pathlib import Path
import sys
import time
import urllib.error
import urllib.request


def load_env():
    if Path('.env').exists():
        for line in Path('.env').read_text().splitlines():
            if '=' in line and not line.lstrip().startswith('#'):
                key, value = line.split('=', 1)
                os.environ.setdefault(key.strip(), value.strip().strip('"').strip("'"))

load_env()
TOKEN = os.environ.get('CF_API_TOKEN', '')
BASE = os.environ.get('GRAFANA_URL', 'http://localhost:3300')
UID = 'cloudflare-analytics'
PASSWORD = os.environ.get('GRAFANA_ADMIN_PASSWORD', 'admin')
AUTH = base64.b64encode(('admin:' + PASSWORD).encode()).decode()

def request(url, body=None, cloudflare=False, method=None):
    headers = {'Authorization': 'Bearer ' + TOKEN if cloudflare else 'Basic ' + AUTH,
               'Content-Type': 'application/json'}
    req = urllib.request.Request(url, data=json.dumps(body).encode() if body is not None else None, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=120) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        return error.code, json.loads(error.read())

def gf(path, body=None, method=None):
    return request(BASE + path, body, method=method)

def gql(query, variables):
    status, result = request('https://api.cloudflare.com/client/v4/graphql', {'query': query, 'variables': variables}, True)
    assert status == 200 and not result.get('errors'), 'Direct GraphQL comparison failed'
    return result['data']['viewer']['zones'][0]['rows']

def query(zone, metric='requests', group=None, filters=None, fmt='timeSeries', ref='A', **kwargs):
    return dict(refId=ref, datasource={'type':'mrqs001-cloudflareanalytics-datasource', 'uid':UID},
                zoneId=zone, metric=metric, groupBy=group or [],
                filters=filters if filters is not None else [{'field':'requestSource','operator':'eq','values':['eyeball']}],
                format=fmt, interval='auto', maxSeries=200, fill='null', maxDataPoints=600, intervalMs=60000, **kwargs)

def run_queries(start, end, queries):
    return gf('/api/ds/query', {'from':str(int(start.timestamp()*1000)), 'to':str(int(end.timestamp()*1000)), 'queries':queries})

def values(result, ref='A'):
    return [v for f in result['results'][ref]['frames'] for v in f['data']['values'][-1] if v is not None]

def ok(result):
    assert result[0] == 200, 'Grafana query HTTP failure: ' + str(result[0]) + ' ' + '; '.join(v.get('error','') for v in result[1].get('results',{}).values())
    assert all(not r.get('error') for r in result[1]['results'].values()), 'Grafana returned a query error'
    return result[1]

def main():
    assert TOKEN, 'Set CF_API_TOKEN (never pass it as a command argument)'
    status, health = gf(f'/api/datasources/uid/{UID}/health')
    assert status == 200 and health['status'] == 'OK', 'Health check failed'
    status, zones = gf(f'/api/datasources/uid/{UID}/resources/zones')
    assert status == 200 and zones, 'Zone discovery failed'
    status, accounts = gf(f'/api/datasources/uid/{UID}/resources/accounts')
    assert status == 200 and accounts, 'Account discovery failed'
    end = dt.datetime.now(dt.timezone.utc).replace(second=0, microsecond=0) - dt.timedelta(minutes=5)
    # Select the authorized zone with the most traffic without printing names or IDs.
    scored = []
    for z in zones:
        r = ok(run_queries(end-dt.timedelta(days=1), end, [query(z['id'], fmt='total')]))
        scored.append((sum(values(r)), z['id']))
    zone = max(scored)[1]
    print('PASS health, zone discovery and secure backend queries', flush=True)
    s, settings = gf(f'/api/datasources/uid/{UID}/resources/settings?zoneId={zone}')
    assert s == 200 and settings['enabled']
    windows = [('15m',900),('1h',3600),('6h',21600),('24h',86400),('3d',259200),('7d',604800),('30d',2592000),('30.5d',2635200)]
    sampled = False
    for label,seconds in windows:
        if seconds + 600 >= settings['notOlderThan']:
            print('SKIP', label, 'outside plan retention', flush=True)
            continue
        start = end - dt.timedelta(seconds=seconds)
        before = time.monotonic()
        result = ok(run_queries(start,end,[query(zone), query(zone,'requestRate',ref='B'), query(zone,fmt='total',ref='C')]))
        frames = result['results']['A']['frames']
        sampled |= any(f['schema'].get('meta',{}).get('custom',{}).get('maxSampleInterval',0)>1 for f in frames)
        # Compare identical bucket selection directly to Cloudflare, avoiding changed
        # grouping/sampling levels. Totals from a different query may differ statistically.
        interval = frames[0]['schema']['meta']['custom']['intervalSeconds']
        native = 'date' if interval>=86400 else 'datetimeHour' if interval>=3600 else 'datetimeFifteenMinutes' if interval>=900 else 'datetimeFiveMinutes' if interval>=300 else 'datetimeMinute'
        direct = []
        cursor = start
        while cursor < end:
            chunk_end = min(end, cursor + dt.timedelta(seconds=settings['maxDuration']))
            limit = min(10000, settings['maxPageSize'])
            chunk = gql('query($z:string!,$f:ZoneHttpRequestsAdaptiveGroupsFilter_InputObject!,$limit:uint64!){viewer{zones(filter:{zoneTag:$z}){rows:httpRequestsAdaptiveGroups(limit:$limit,filter:$f){count avg{sampleInterval} dimensions{'+native+'}}}}}', {'z':zone,'limit':limit,'f':{'datetime_geq':cursor.isoformat(),'datetime_lt':chunk_end.isoformat(),'requestSource':'eyeball'}})
            assert len(chunk) < limit, 'Direct comparison hit its row limit'
            direct.extend(chunk)
            cursor = chunk_end
        if seconds > settings['maxDuration']:
            assert frames[0]['schema']['meta']['custom']['apiRequests'] > 1, 'Expected plan-aware range splitting'
        a,b = sum(values(result)),sum(x['count'] for x in direct)
        # Stable historical queries should agree closely; sampling can change between requests.
        relative = abs(a-b)/max(1,b)
        assert relative <= .05, f'{label}: direct API sum differs by {relative:.2%}'
        av,bv = values(result),values(result,'B')
        assert all(math.isfinite(v) and v >= 0 for v in av+bv)
        # Integrate rates using the actual covered duration of each UTC bucket.
        integrated = 0
        for f in result['results']['B']['frames']:
            times, rates = f['data']['values']
            for timestamp,rate in zip(times,rates):
                if rate is not None:
                    begin = dt.datetime.fromtimestamp(timestamp/1000,dt.timezone.utc)
                    bucket_end = dt.datetime.fromtimestamp((math.floor(begin.timestamp()/interval)+1)*interval,dt.timezone.utc)
                    integrated += rate*(min(end,bucket_end)-begin).total_seconds()
        assert abs(integrated-a)/max(1,a) <= .05, 'Rate/count mismatch'
        print(f'PASS {label}: count/rate/total, direct API delta {relative:.2%}, {time.monotonic()-before:.2f}s', flush=True)
    start = end-dt.timedelta(hours=6)
    for group in ['status','hostname','cacheStatus','country','colo','method','path','requestSource','originStatus','securityAction','securitySource']:
        r=ok(run_queries(start,end,[query(zone,group=[group],fmt='total' if group=='path' else 'timeSeries')]))
        print('PASS grouping:',group,flush=True)
    r=ok(run_queries(start,end,[query(zone,'bytes'),query(zone,'bandwidth',ref='B')]))
    assert all(v>=0 for v in values(r)+values(r,'B'))
    filters=[{'field':'status','operator':'in','values':['200','301','404']},{'field':'method','operator':'neq','values':['POST']}]
    ok(run_queries(start,end,[query(zone,filters=filters)]))
    ok(run_queries(start,end,[query(zone,filters=[{'field':'status','operator':'notIn','values':['500','502']}])]))
    empty=ok(run_queries(start,end,[query(zone,filters=[{'field':'hostname','operator':'eq','values':['does-not-exist.invalid']}])]))
    assert not values(empty), 'Expected empty result'
    expression = {'refId':'B','datasource':{'type':'__expr__','uid':'__expr__'},'type':'reduce','expression':'A','reducer':'sum','settings':{'mode':'dropNN'}}
    computed = ok(run_queries(start,end,[query(zone),expression]))
    assert 'B' in computed['results'], 'Server expression result missing'
    print('PASS bytes/bandwidth, in/neq/notIn filters, empty results and server-side expressions',flush=True)
    for name,qs in [('invalid zone',[query('invalid')]),('unknown zone',[query('0'*32)]),('invalid filter',[query(zone,filters=[{'field':'status','operator':'eq','values':['bad']}])])]:
        _,result=run_queries(start,end,qs)
        assert result['results']['A'].get('error'),name+' was accepted'
        print('PASS rejection:',name,flush=True)
    _,outside=run_queries(end-dt.timedelta(seconds=settings['notOlderThan']+600),end,[query(zone)])
    assert outside['results']['A'].get('error'), 'Out-of-retention query was accepted'
    # A real invalid-token datasource in disposable Grafana. No Cloudflare mutations.
    bad_uid='cloudflare-invalid-integration'
    gf('/api/datasources/uid/'+bad_uid,method='DELETE')
    status,_=gf('/api/datasources',{'uid':bad_uid,'name':'Invalid token integration test','type':'mrqs001-cloudflareanalytics-datasource','access':'proxy','jsonData':{'defaultZoneId':zone},'secureJsonData':{'apiToken':'invalid-test-token'}})
    assert status == 200
    try:
        status,result=gf(f'/api/datasources/uid/{bad_uid}/health')
        assert status!=200 or result.get('status')!='OK', 'Invalid token accepted'
    finally:
        gf('/api/datasources/uid/'+bad_uid,method='DELETE')
    status,public=gf('/api/datasources/uid/'+UID)
    assert status==200 and TOKEN not in json.dumps(public) and not public.get('secureJsonData'), 'Secret exposed to browser'
    status,found=gf(f'/api/datasources/uid/{UID}/resources/values?field=hostname&zoneId={zone}')
    assert status==200 and isinstance(found,list)
    print('PASS retention, invalid-token health check, variable values and secret isolation',flush=True)
    print('LIVE SAMPLING OBSERVED:',sampled,flush=True)
    print('All live checks passed. No Cloudflare configuration was modified.',flush=True)

if __name__=='__main__':
    try: main()
    except Exception as error:
        message=str(error)
        for secret in (TOKEN,PASSWORD):
            if secret: message=message.replace(secret,'[REDACTED]')
        print('FAIL:',message,file=sys.stderr)
        sys.exit(1)
