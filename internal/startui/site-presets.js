// Retain explicit edits (including removals) independently for every type.
class SiteListState {
  constructor(presets, saved = {}, initialType) {
    this.presets = presets;
    this.lists = {...(saved.siteLists || {})};
    this.managed = {...(saved.endpointSites || {})};
    this.current = null;
    // Upgrade older saved settings once, retaining their custom destinations.
    if (!saved.siteLists && saved.sites?.trim()) {
      const type = saved.sandbox || initialType;
      this.lists[type] = [...new Set([...(presets[type] || []), ...saved.sites.toLowerCase().split(/[\s,]+/).filter(Boolean)])].join('\n');
    }
  }
  select(type, previousValue) {
    if (this.current !== null) this.lists[this.current] = previousValue;
    this.current = type;
    return Object.prototype.hasOwnProperty.call(this.lists, type) ? this.lists[type] : this.defaults();
  }
  defaults() { return (this.presets[this.current] || []).join('\n'); }
  reset() { delete this.managed[this.current];return this.lists[this.current] = this.defaults(); }
  endpoint(host, value) {
    if(this.current===null)return value;
    const previous=this.managed[this.current];
    let entries=value.split(/[\s,]+/).filter(Boolean),changed=false;
    if(previous&&previous!==host){const kept=entries.filter(entry=>entry.toLowerCase()!==previous);changed=kept.length!==entries.length;entries=kept;delete this.managed[this.current];}
    if(host&&!entries.some(entry=>entry.toLowerCase()===host)){entries.push(host);changed=true;this.managed[this.current]=host;}
    else if(host&&previous!==host)delete this.managed[this.current];
    const result=changed?entries.join('\n'):value;
    this.lists[this.current]=result;return result;
  }
  managedSnapshot() { return {...this.managed}; }
  snapshot(value) {
    if (this.current !== null) this.lists[this.current] = value;
    return {...this.lists};
  }
}
if (typeof module !== 'undefined') module.exports = SiteListState;
