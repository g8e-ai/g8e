import urllib.request, urllib.error, json, ssl
import os

token = open('/g8es/internal_auth_token').read().strip()

ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

headers = {
    'X-Internal-Auth': token,
    'Content-Type': 'application/json'
}

req = urllib.request.Request('https://g8es:9000/db/operators/_query', data=json.dumps({"filter":{"is_g8ep": True}}).encode(), headers=headers, method='POST')
with urllib.request.urlopen(req, context=ctx) as resp:
    docs = json.loads(resp.read().decode())
    
if docs:
    doc = docs[0]
    api_key = doc['api_key']
    print(f"Found g8ep API key: {api_key}")
    
    # Manually persist to platform_settings
    update_req = urllib.request.Request(
        'https://g8es:9000/db/settings/platform_settings', 
        data=json.dumps({
            "settings": {
                "g8ep_operator_api_key": api_key
            }
        }).encode(),
        headers=headers,
        method='PATCH'
    )
    with urllib.request.urlopen(update_req, context=ctx) as up_resp:
        print("Updated platform_settings")
else:
    print("No g8ep operator found")
