#!/usr/bin/env python3
"""Opt-in multi-zone integration checks; requires two zones and a running Grafana.

Uses CF_API_TOKEN through live_test. Only pass/fail summaries are printed.
Run with the datasource default set to All zones.
"""
from live_test import gf, UID, dt, ok, run_queries, query
status,zones=gf(f'/api/datasources/uid/{UID}/resources/zones')
assert status==200 and len(zones)>=2
assert all(set(z)=={'id','name'} for z in zones), 'Zone resource exposed extra account data'
end=dt.datetime.now(dt.timezone.utc).replace(second=0,microsecond=0)-dt.timedelta(minutes=5)
start=end-dt.timedelta(hours=6)
ids=[z['id'] for z in zones]
for fmt in ['timeSeries','total']:
 singles={z['id']:ok(run_queries(start,end,[query(z['id'],fmt=fmt)]))['results']['A']['frames'] for z in zones}
 for mode in ['selected','all','default']:
  result=ok(run_queries(start,end,[query('',fmt=fmt,zoneMode=mode,zoneIds=ids if mode=='selected' else [])]))
  frames=result['results']['A']['frames']
  if fmt=='timeSeries':
   byzone={f['schema']['fields'][-1]['labels']['zoneId']:f for f in frames}
   assert set(byzone)==set(ids)
   for zid,frame in byzone.items():
    assert frame['data']==singles[zid][0]['data'], 'Time series data differs'
    assert frame['schema']['fields'][-1]['config']['displayNameFromDS'] in [z['name'] for z in zones]
  else:
   assert len(frames)==1
   vals=frames[0]['data']['values']
   byzone=dict(zip(vals[1],vals[-1]))
   for zid,fs in singles.items():
    assert byzone.get(zid)==(fs[0]['data']['values'][-1][0] if fs[0]['data']['values'][-1] else None)
   assert len(frames[0]['schema']['meta']['custom']['zones'])==len(zones)
  print('PASS',fmt,mode,': matches each single-zone query; names and IDs preserved',flush=True)
print('PASS zone resource excludes account names/emails',flush=True)
