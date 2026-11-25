#!/usr/bin/env python3
"""Exercise real agent configuration and Claude hooks without model requests."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import tomllib
import unittest
from unittest.mock import patch

sys.dont_write_bytecode=True
ASSET=Path(__file__).resolve().parents[1]/'internal/launcherbundle/bundles/common/assets/agent-network.py'
spec=importlib.util.spec_from_file_location('agent_network',ASSET)
n=importlib.util.module_from_spec(spec);spec.loader.exec_module(n)


class AgentNetworkTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
  self.home=Path(self.tmp.name).resolve()
  self.home_patch=patch.object(n.Path,'home',return_value=self.home);self.home_patch.start();self.addCleanup(self.home_patch.stop)
 def read(self,p):return json.loads((self.home/p).read_text())
 def hook(self,name,args):
  return subprocess.run([sys.executable,'-I',str(self.home/'.claude/vscode-sandboxes-web.py')],input=json.dumps({'tool_name':name,'tool_input':args}),text=True,capture_output=True,env={**os.environ,'HOME':str(self.home)})
 def test_transitions_preserve_credentials_custom_settings_and_approvals(self):
  c=self.home/'.codex/config.toml';c.parent.mkdir()
  c.write_text('model="my-model"\nweb_search="cached"\n[model_providers.private]\nbase_url="https://models.example/v1"\nexperimental_bearer_token="fixture-secret"\n')
  original=tomllib.loads(c.read_text())
  claude=self.home/'.claude/settings.json';claude.parent.mkdir()
  custom={'hooks':{'PreToolUse':[{'matcher':'Bash','hooks':[{'type':'command','command':'echo original'}]}]},'permissions':{'deny':['Read(secret)']}}
  claude.write_text(json.dumps(custom))
  profile={'network':'restricted','sites':'example.org, *.python.org, models.example:8443, 127.0.0.1, localhost, example.org'}
  n.apply(profile);n.apply(n.policy(profile))
  data=tomllib.loads(c.read_text());self.assertEqual(data['model_providers'],original['model_providers'])
  self.assertIs(data['tools']['web_search'],False)
  self.assertEqual(data['web_search'],'disabled')
  self.assertIs(data['features']['apps'],False)
  self.assertIs(data['apps']['_default']['enabled'],False)
  self.assertIs(data['apps']['_default']['default_tools_enabled'],False)
  self.assertEqual(len(self.read('.claude/settings.json')['hooks']['PreToolUse']),2)
  self.assertEqual(self.read('.claude/settings.json')['permissions'],custom['permissions'])
  self.assertEqual(self.read('.continue/.sandbox-network.json')['network'],'restricted')
  machine=self.read('.vscode-server/data/Machine/settings.json');self.assertNotIn('chat.agent.networkFilter',machine,'application-scoped filters cannot be enforced through remote Machine settings')
  self.assertEqual(machine['github.copilot.chat.anthropic.tools.websearch.allowedDomains'],['example.org','python.org'])
  for tool,args in [('WebSearch',{'query':'test','allowed_domains':['evil.org']}),('WebFetch',{'url':'https://evil.org'}),('WebFetch',{'url':'https://models.example:443'}),('WebFetch',{'url':'https://example.org.evil.org'})]:
   result=self.hook(tool,args);self.assertEqual(result.returncode,2,result.stdout);self.assertNotIn('fixture-secret',result.stderr)
  for url in ['https://example.org','https://docs.python.org','https://models.example:8443']:
   self.assertEqual(self.hook('WebFetch',{'url':url}).returncode,0,url)
  result=self.hook('WebSearch',{'query':'test','allowed_domains':['docs.python.org']});self.assertEqual(result.returncode,0,result.stderr)
  decision=json.loads(result.stdout)['hookSpecificOutput'];self.assertNotIn('permissionDecision',decision)
  self.assertEqual(decision['updatedInput']['allowed_domains'],['docs.python.org'])
  n.apply({'network':'none'})
  self.assertEqual(tomllib.loads(c.read_text())['web_search'],'disabled')
  self.assertEqual(self.hook('WebSearch',{'query':'test'}).returncode,2)
  n.apply(profile);self.assertEqual(tomllib.loads(c.read_text())['web_search'],'disabled')
  n.apply({'network':'open'})
  self.assertEqual(tomllib.loads(c.read_text())['model_providers'],original['model_providers'])
  self.assertEqual(tomllib.loads(c.read_text())['web_search'],'cached')
  self.assertNotIn('apps',tomllib.loads(c.read_text()).get('features',{}),'Open restores the original connector feature')
  self.assertEqual(self.read('.claude/settings.json'),custom)
  self.assertNotIn('chat.agent.networkFilter',self.read('.vscode-server/data/Machine/settings.json'))
 def test_no_domains_and_limit_fail_closed(self):
  for sites in ['', '127.0.0.1, models.example:8443', ','.join('host'+str(i)+'.example' for i in range(101))]:
   n.apply({'network':'restricted','sites':sites})
   self.assertEqual(tomllib.loads((self.home/'.codex/config.toml').read_text())['web_search'],'disabled')
   self.assertEqual(self.hook('WebSearch',{'query':'test'}).returncode,2)
 def test_disabled_search_and_malformed_config(self):
  c=self.home/'.codex/config.toml';c.parent.mkdir();c.write_text('web_search="disabled"\n[features]\napps=true\n[apps._default]\nenabled=true\ndefault_tools_enabled=true\nopen_world_enabled=true\n')
  n.apply({'network':'restricted','sites':'example.org'})
  self.assertEqual(tomllib.loads(c.read_text())['web_search'],'disabled')
  self.assertFalse(tomllib.loads(c.read_text())['features']['apps'])
  n.apply({'network':'open'})
  restored=tomllib.loads(c.read_text());self.assertTrue(restored['features']['apps']);self.assertTrue(restored['apps']['_default']['enabled'])
  c.write_text('broken = [')
  with self.assertRaises(tomllib.TOMLDecodeError):n.apply({'network':'restricted','sites':'example.org'})
  self.assertEqual(c.read_text(),'broken = [')


if __name__=='__main__':unittest.main()
