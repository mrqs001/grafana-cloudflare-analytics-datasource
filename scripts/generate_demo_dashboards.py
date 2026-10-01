#!/usr/bin/env python3
"""Build deterministic screenshot fixtures from the example dashboards.

No credentials, APIs, environment secrets or live analytics are read. Output is
isolated under .local/demo and uses Grafana's built-in TestData CSV scenario.
"""
import csv
import datetime as dt
import io
import json
import math
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / '.local/demo/provisioning'
DS = {'type': 'grafana-testdata-datasource', 'uid': 'synthetic-demo'}
START = dt.datetime(2026, 9, 24, 9, tzinfo=dt.timezone.utc)
N = 180
TIMES = [(START + dt.timedelta(minutes=2*i)).isoformat() for i in range(N)]
# Smooth diurnal demand, a lunchtime campaign and small deterministic variation.
EDGE = [round(430 + 75*math.sin(i/26) + 28*math.sin(i/4.7) + 240*math.exp(-((i-99)/16)**2), 2) for i in range(N)]
API = [round(135 + 30*math.sin(i/21+.6) + 9*math.cos(i/3.8) + 55*math.exp(-((i-101)/19)**2), 2) for i in range(N)]
NGINX = [round(v*(.22+.025*math.sin(i/19)) + 6*math.exp(-((i-103)/14)**2), 2) for i,v in enumerate(EDGE)]


def csv_target(headers, rows, ref='A'):
    buf = io.StringIO()
    writer = csv.writer(buf)
    writer.writerow(headers)
    writer.writerows(rows)
    return {'refId': ref, 'datasource': DS, 'scenarioId': 'csv_content', 'csvContent': buf.getvalue()}


def series(names, columns):
    return csv_target(['time', *names], zip(TIMES, *columns))


def scale(values, factor):
    return [round(v*factor, 2) for v in values]


def setup(name, title):
    d = json.loads((ROOT / f'src/dashboards/{name}.json').read_text())
    d.update(uid=f'demo-{name}', title=title, tags=['demo', 'synthetic-data'], refresh='', editable=False,
             description='Illustrative synthetic data only. All domains are fictional. Nginx values are simulated.',
             time={'from': TIMES[0], 'to': (START + dt.timedelta(hours=6)).isoformat()})
    d['templating']['list'] = [{'name': 'zone', 'label': 'Demo zones', 'type': 'custom',
        'query': 'shop.example.com,api.example.com', 'multi': True, 'includeAll': True,
        'current': {'text': 'All', 'value': '$__all'}, 'options': []}]
    for p in d['panels']:
        p['datasource'] = DS
        p['description'] = 'Synthetic demonstration data. No live account or traffic information.'
        p['targets'] = []
        if p['type'] == 'timeseries':
            p['fieldConfig']['defaults']['custom'].update(lineInterpolation='smooth', fillOpacity=9, lineWidth=2)
    return d


def save(d, filename):
    (OUT / 'dashboards' / filename).write_text(json.dumps(d, indent=2)+'\n')


def main():
    (OUT / 'datasources').mkdir(parents=True, exist_ok=True)
    (OUT / 'dashboards').mkdir(parents=True, exist_ok=True)
    (OUT / 'datasources/demo.yml').write_text('''apiVersion: 1
datasources:
  - name: Synthetic demo data
    uid: synthetic-demo
    type: grafana-testdata-datasource
    access: proxy
    isDefault: true
''')
    (OUT / 'dashboards/provider.yml').write_text('''apiVersion: 1
providers:
  - name: Synthetic screenshots
    type: file
    editable: false
    options:
      path: /etc/grafana/provisioning/dashboards
''')
    d = setup('overview', 'Cloudflare HTTP overview · DEMO DATA')
    for p in d['panels']:
        i = p['id']
        if i == 1:
            p['targets'] = [csv_target(['Requests'], [[round(sum(EDGE+API)*120)]])]
        elif i == 2:
            p['targets'] = [csv_target(['Requests / second'], [[round(sum(EDGE+API)/N)]])]
        elif i == 3:
            p['targets'] = [csv_target(['Response bytes'], [[round(sum(EDGE)*120*22500+sum(API)*120*5800)]])]
        elif i == 4:
            p['targets'] = [series(['shop.example.com', 'api.example.com'], [EDGE, API])]
        elif i == 5:
            p['targets'] = [series(['shop.example.com', 'api.example.com'], [scale(EDGE,22500),scale(API,5800)])]
        elif i == 6:
            p['targets'] = [series(['200 · OK','304 · Not modified','404 · Not found','429 · Rate limited'], [scale(EDGE,120*.83),scale(EDGE,120*.12),scale(EDGE,120*.035),scale(EDGE,120*.015)])]
        elif i == 7:
            p['targets'] = [series(['HIT','MISS','DYNAMIC'],[scale(EDGE,120*.72),scale(EDGE,120*.19),scale(EDGE,120*.09)])]
        elif i == 8:
            p['targets'] = [series(['shop.example.com', 'api.example.com'], [EDGE, API])]
        elif i == 9:
            p['targets'] = [series(['LIS', 'MAD', 'CDG', 'LHR', 'FRA'], [scale([a+b for a,b in zip(EDGE,API)],120*f) for f in [.31,.26,.19,.15,.09]])]
        elif i == 10:
            p['targets'] = [csv_target(['URI path','Requests'], [[k,v] for k,v in [('/',2164000),('/products',1648000),('/api/catalog',952000),('/checkout',586000),('/assets/app.js',432000)]])]
        else:
            p['options']['content'] = '**DEMO DATA — fictional zones and synthetic traffic.** These images illustrate the bundled dashboard. Real Cloudflare analytics uses adaptive estimates; missing buckets and origin comparisons require care.'
    save(d,'overview.json')
    d = setup('origin-comparison','Cloudflare edge vs Nginx · SIMULATED DEMO')
    for p in d['panels']:
        i=p['id']
        if i==1:
            p['title']='Cloudflare edge vs Nginx origin · simulated requests / second'
            p['targets']=[series(['Cloudflare edge · shop.example.com', 'Nginx origin · shop.example.com'], [EDGE,NGINX])]
            p['fieldConfig']['overrides']=[{'matcher':{'id':'byName','options':name}, 'properties':[{'id':'color','value':{'mode':'fixed','fixedColor':color}}]} for name,color in [('Cloudflare edge · shop.example.com','#FF9830'),('Nginx origin · shop.example.com','#73BF69')]]
        elif i==2:
            p['targets']=[series(['HIT','MISS','DYNAMIC'],[scale(EDGE,.72),scale(EDGE,.19),scale(EDGE,.09)])]
        elif i==3:
            p['targets']=[series(['Edge 200','Origin 200','Edge 404','Origin 502'],[scale(EDGE,.94),scale(NGINX,.985),scale(EDGE,.035),scale(NGINX,.015)])]
        elif i==4:
            p['targets']=[series(['allow','managed_challenge','block'],[scale(EDGE,.97),scale(EDGE,.021),scale(EDGE,.009)])]
        elif i==5:
            p['targets']=[series(['eyeball','edgeWorkerFetch','healthcheck'],[EDGE,scale(EDGE,.048),[1]*N])]
        else:
            p['title']='About this illustration'
            p['gridPos']['h']=4
            p['options']['content']='**SIMULATED DEMO — not production measurements.** Fictional `shop.example.com` and `api.example.com` zones. Nginx traffic is generated to illustrate an origin comparison; it is not supplied by the Cloudflare plugin. In the real example dashboard, add your existing Prometheus or Loki query to the mixed-source comparison panel.'
    save(d,'origin-comparison.json')
    print('Generated two synthetic Grafana dashboards. No credentials or live data used.')


if __name__ == '__main__':
    main()
