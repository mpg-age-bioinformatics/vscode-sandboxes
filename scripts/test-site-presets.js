#!/usr/bin/env node
const assert = require('node:assert/strict');
const SiteListState = require('../internal/startui/site-presets.js');
const presets = {
  'python-sandbox':['registry.example.org','pypi.org'],
  'r-sandbox':['registry.example.org','cran.r-project.org'],
  'bioinformatics-sandbox':['registry.example.org','pypi.org','cran.r-project.org']
};
const state = new SiteListState(presets);
assert.equal(state.select('python-sandbox',''),presets['python-sandbox'].join('\n'));
// Explicitly remove a default and add a private mirror.
const custom = 'pypi.org\nmy-python-mirror.example.org';
assert.equal(state.select('r-sandbox',custom),presets['r-sandbox'].join('\n'));
assert.equal(state.select('bioinformatics-sandbox','my-r-mirror.example.org'),presets['bioinformatics-sandbox'].join('\n'));
assert.equal(state.select('python-sandbox',presets['bioinformatics-sandbox'].join('\n')),custom);
const saved = {sandbox:'python-sandbox',sites:custom,siteLists:state.snapshot(custom)};
const reopened = new SiteListState(presets,JSON.parse(JSON.stringify(saved)));
assert.equal(reopened.select('python-sandbox',''),custom);
assert.equal(reopened.select('r-sandbox',custom),'my-r-mirror.example.org');
assert.equal(reopened.reset(),presets['r-sandbox'].join('\n'));
assert.equal(reopened.select('python-sandbox',''),custom);
assert.equal(reopened.select('r-sandbox',''), ''); // Intentional empty edits are preserved.
const legacy = new SiteListState(presets,{sandbox:'python-sandbox',sites:'MY-MIRROR.EXAMPLE.ORG, pypi.org'},'python-sandbox');
assert.equal(legacy.select('python-sandbox',''),'registry.example.org\npypi.org\nmy-mirror.example.org');
assert.equal(legacy.select('r-sandbox',legacy.defaults()),presets['r-sandbox'].join('\n'));
console.log('Language presets, custom edits, saved lists, legacy settings, and reset checks passed.');

const managed = new SiteListState(presets);
let list=managed.select('python-sandbox','');
list=managed.endpoint('models.example',list);
const restored=new SiteListState(presets,{siteLists:managed.snapshot(list),endpointSites:managed.managedSnapshot()});
list=restored.select('python-sandbox','');
assert(!restored.endpoint('replacement.example',list).includes('models.example'));
list=restored.endpoint('manual.example','manual.example');
assert.equal(restored.endpoint('',list),'manual.example','Manually supplied hosts must survive clearing the endpoint');
