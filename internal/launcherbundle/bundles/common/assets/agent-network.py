#!/usr/bin/env python3
"""Configure sandbox-local agent web tools, separately from transport policy."""
import copy
import datetime
import ipaddress
import json
import math
import os
from pathlib import Path
import re
import shlex
import sys
import tempfile
import tomllib

HOOK = r'''import json,sys
from pathlib import Path
from urllib.parse import urlsplit
import fnmatch
try:
 p=json.loads((Path.home()/'.claude/.sandbox-network.json').read_text())
 event=json.load(sys.stdin);name=event.get('tool_name');args=event.get('tool_input',{})
 if p['network']=='open':sys.exit(0)
 if name=='WebSearch' and p['network']=='restricted' and p['domains']:
  requested=args.get('allowed_domains')
  domains=p['domains']
  if requested is not None:
   # Narrow to the policy domains, never replace them with model-supplied ones.
   domains=list(dict.fromkeys(x for d in domains for r in requested for x in ([r] if r==d or r.endswith('.'+d) else [d] if d.endswith('.'+r) else [])))
  if not domains:raise ValueError('No allowed search domains')
  updated=dict(args,allowed_domains=domains)
  print(json.dumps({'hookSpecificOutput':{'hookEventName':'PreToolUse','updatedInput':updated}}))
 elif name=='WebFetch' and p['network']=='restricted':
  u=urlsplit(args.get('url',''));port=u.port or (443 if u.scheme=='https' else 80)
  if u.scheme not in ('http','https') or not u.hostname or u.username or u.password:raise ValueError('Invalid URL')
  host=u.hostname.lower().rstrip('.')
  allowed=False
  for rule in p['sites']:
   parts=rule.rsplit(':',1);domain=parts[0] if len(parts)==2 else rule
   ports=[int(parts[1])] if len(parts)==2 else [80,443]
   match=(host==domain or (domain.startswith('**.') and (host==domain[3:] or host.endswith('.'+domain[3:]))) or (domain.startswith('*.') and host.endswith(domain[1:]) and host!=domain[2:]))
   allowed=allowed or (match and port in ports)
  if not allowed:raise ValueError('URL outside Selected sites')
 else:raise ValueError('Hosted web tool disabled by sandbox policy')
except Exception:
 print('Sandbox web policy blocked this request. Use an allowed domain or a sandbox terminal request.',file=sys.stderr)
 sys.exit(2)
'''


def atomic(path, text):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    if path.is_symlink() or any(p.is_symlink() for p in path.parents):
        raise ValueError('Symlinked agent configuration')
    with tempfile.NamedTemporaryFile(mode='w', dir=path.parent, delete=False) as out:
        out.write(text)
        temp = out.name
    os.chmod(temp, 0o600)
    os.replace(temp, path)


def value(v):
    # Inline TOML tables preserve arbitrary nested configuration without a new
    # runtime dependency. Parsed settings, including credentials, stay in home.
    if isinstance(v, bool): return 'true' if v else 'false'
    if isinstance(v, str): return json.dumps(v, ensure_ascii=True)
    if isinstance(v, dict): return '{' + ', '.join(json.dumps(k)+ ' = '+value(x) for k,x in v.items()) + '}'
    if isinstance(v, list): return '[' + ', '.join(value(x) for x in v) + ']'
    if isinstance(v, (datetime.datetime, datetime.date, datetime.time)): return v.isoformat()
    if isinstance(v, (int, float)): return str(v).lower()
    raise ValueError('Unsupported TOML value')


def policy(profile):
    mode = profile.get('network') or 'open'
    if mode not in ('open','restricted','none'): raise ValueError('Invalid network mode')
    raw_sites=profile.get('sites','')
    if isinstance(raw_sites,list):raw_sites=','.join(raw_sites)
    sites = list(dict.fromkeys(x.lower().strip().rstrip('.') for x in re.split(r'[\s,]+',raw_sites) if x.strip()))
    domains = []
    for site in sites:
        # Hosted search cannot faithfully represent a port or IP rule. Omit it
        # rather than dropping its port and silently granting broader access.
        if ':' in site: continue
        domain = re.sub(r'^\*\*?\.', '', site)
        try: ipaddress.ip_address(domain); continue
        except ValueError: pass
        if domain=='localhost' or not re.fullmatch(r'[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?',domain) or '.' not in domain: continue
        if domain not in domains: domains.append(domain)
    if mode != 'restricted' or len(domains)>100: domains=[]
    return {'network':mode,'sites':sites,'domains':domains}


def patch(path, edits, toml=False):
    if path.is_symlink(): raise ValueError('Symlinked agent configuration')
    marker=path.with_name('.'+path.name+'.sandbox-web-original.json')
    if all(v is RESTORE for v in edits.values()) and not marker.exists():return
    text=path.read_text() if path.exists() else ''
    original=tomllib.loads(text) if toml else (json.loads(text) if text.strip() else {})
    data=copy.deepcopy(original)
    marker=path.with_name('.'+path.name+'.sandbox-web-original.json')
    saved=json.loads(marker.read_text()) if marker.exists() else {}
    for key,new in edits.items():
        parts=key.split('/')
        parent=data
        for part in parts[:-1]:
            if not isinstance(parent.get(part),dict): parent[part]={}
            parent=parent[part]
        leaf=parts[-1]
        if new is RESTORE:
            if key not in saved:continue
            old=saved.pop(key)
            if old['present']:parent[leaf]=old['value']
            else:parent.pop(leaf,None)
        else:
            if key not in saved:saved[key]={'present':leaf in parent,'value':parent.get(leaf)}
            parent[leaf]=new
    restoring=all(v is RESTORE for v in edits.values())
    if not restoring:atomic(marker,json.dumps(saved,indent=2)+'\n')
    if data!=original:
        # Preserve a one-time full backup as well, including original comments.
        backup=path.with_name('.'+path.name+'.before-sandbox-web')
        if text and not backup.exists():atomic(backup,text)
        output='\n'.join(json.dumps(k)+' = '+value(v) for k,v in data.items())+'\n' if toml else json.dumps(data,indent=2)+'\n'
        if toml:assert tomllib.loads(output)==data
        atomic(path,output)
    if restoring:
        if saved:atomic(marker,json.dumps(saved,indent=2)+'\n')
        else:marker.unlink(missing_ok=True)


RESTORE=object()


def apply(profile):
    p=policy(profile);home=Path.home().resolve();mode=p['network'];restricted=mode!='open'
    # Hosted search-domain filters did not constrain Code Mode open-page calls
    # in the installed extension. Disable the entire hosted surface in restricted
    # and offline modes; direct sandbox fetches remain transport-enforced.
    codex={'web_search':'disabled','tools/web_search':False,
           'features/apps':False,'apps/_default/enabled':False,
           'apps/_default/default_tools_enabled':False,'apps/_default/open_world_enabled':False}
    patch(home/'.codex/config.toml',codex if restricted else {k:RESTORE for k in codex},toml=True)
    claude=home/'.claude';claude.mkdir(mode=0o700,exist_ok=True)
    atomic(claude/'.sandbox-network.json',json.dumps(p)+'\n')
    atomic(claude/'vscode-sandboxes-web.py',HOOK)
    target=claude/'settings.json'
    settings=json.loads(target.read_text()) if target.exists() else {}
    if restricted and settings.get('disableAllHooks'):
        raise ValueError('Claude hooks are disabled; cannot enforce hosted web policy')
    hooks=settings.setdefault('hooks',{})
    entries=hooks.get('PreToolUse',[])
    entries=[e for e in entries if not any('vscode-sandboxes-web.py' in h.get('command','') for h in e.get('hooks',[]))]
    if restricted:
        entries.append({'matcher':'WebSearch|WebFetch','hooks':[{'type':'command','command':'python3 -I '+shlex.quote(str(claude/'vscode-sandboxes-web.py'))}]})
    if entries:hooks['PreToolUse']=entries
    else:hooks.pop('PreToolUse',None)
    atomic(target,json.dumps(settings,indent=2)+'\n')
    atomic(home/'.continue/.sandbox-network.json',json.dumps(p)+'\n')
    # The core fetch/browser filter is APPLICATION-scoped and cannot be set per
    # remote sandbox. Copilot's Anthropic BYOK search properties are WINDOW scope.
    editor={'github.copilot.chat.anthropic.tools.websearch.allowedDomains':p['domains'],
            'github.copilot.chat.anthropic.tools.websearch.enabled':RESTORE if p['domains'] else False}
    # Restore any experimental keys written by earlier in-session builds, only
    # if our own snapshot proves ownership. Never remove an unowned user key.
    legacy=['chat.agent.networkFilter','chat.agent.allowedNetworkDomains','chat.agent.sandbox.network.allowedDomains',
            'chat.anthropic.tools.websearch.allowedDomains','chat.anthropic.tools.websearch.enabled']
    for server in ('.vscode-server','.vscode-server-insiders'):
        edits=editor.copy() if restricted else {k:RESTORE for k in editor}
        edits.update({k:RESTORE for k in legacy})
        patch(home/server/'data/Machine/settings.json',edits)
    print('AGENT_WEB_POLICY_CONFIGURED',mode,'search_domains='+str(len(p['domains'])))


if __name__=='__main__':
    try:apply(json.load(sys.stdin) if len(sys.argv)==1 else json.loads(Path(sys.argv[1]).read_text()))
    except Exception:
        print('Could not configure sandbox agent web policy; startup remains blocked. Check sandbox configuration syntax and permissions.',file=sys.stderr)
        sys.exit(1)
