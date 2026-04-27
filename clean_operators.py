import urllib.request, urllib.error, json, ssl
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

req = urllib.request.Request('https://g8es:9000/db/operators/_query', data=json.dumps({}).encode(), method='POST')
with urllib.request.urlopen(req, context=ctx) as resp:
    res = json.loads(resp.read().decode())
    
for doc in res:
    print("Deleting", doc['id'])
    req = urllib.request.Request(f'https://g8es:9000/db/operators/{doc["id"]}', method='DELETE')
    with urllib.request.urlopen(req, context=ctx) as r:
        pass
