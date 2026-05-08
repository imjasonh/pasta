package a

// TODO: fix this later                          // want `TODO/FIXME without owner`
// TODO(jason): proper owner is fine
// TODO(#123): issue ref is fine
// FIXME: nobody assigned                        // want `TODO/FIXME without owner`
// FIXME(team-platform): assigned, fine
// XXX: scary code here                          // want `TODO/FIXME without owner`
// HACK: works for now                           // want `TODO/FIXME without owner`
// HACK(jason): explained
// regular comment, not flagged
func f() {}
