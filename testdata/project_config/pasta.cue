// Project config + manifest in one file. Empty `imports:` exercises
// the LoadManifest path (so this fixture proves both readers
// cooperate on the same bytes) without triggering any network fetch;
// the disabled_rules entry exercises the LoadConfig + applyConfig
// path. Only the `kept` rule should fire.
imports: {}
disabled_rules: ["dropped"]
