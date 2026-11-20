# Credentials arrive on stdin through encrypted SSH, never process arguments.
import json,sys,urllib.request,urllib.error,urllib.parse
try:
    config=json.load(sys.stdin)
    origin=urllib.parse.urlsplit(config['modelsURL'])
    class Redirect(urllib.request.HTTPRedirectHandler):
        count=0
        def redirect_request(self,request,fp,code,message,headers,newurl):
            self.count+=1
            target=urllib.parse.urlsplit(newurl)
            if self.count>3 or (target.scheme,target.netloc.lower())!=(origin.scheme,origin.netloc.lower()):
                raise ValueError('Redirect refused')
            return super().redirect_request(request,fp,code,message,headers,newurl)
    opener=urllib.request.build_opener(Redirect())
    request=urllib.request.Request(config['modelsURL'],headers={'Authorization':'Bearer '+config['apiKey'],'Accept':'application/json'})
    with opener.open(request,timeout=20) as response:
        raw=response.read((2<<20)+1)
    if len(raw)>2<<20: raise ValueError('Response too large')
    records=json.loads(raw)['data']
    models=sorted(set(item['id'].strip() for item in records if isinstance(item.get('id'),str) and item['id'].strip() and len(item['id'].strip())<=512 and not any(ord(c)<32 or ord(c)==127 for c in item['id']) and item['id'].strip()!=config['apiKey']))
    if not models or len(models)>2000: raise ValueError('Invalid model count')
    print(json.dumps({'models':models}))
except Exception:
    print('Private model discovery failed from the remote container. Check endpoint reachability, certificate trust, API key and the /models response.',file=sys.stderr)
    sys.exit(1)
