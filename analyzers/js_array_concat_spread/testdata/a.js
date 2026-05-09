const a = [].concat(items); // want `[].concat(x) can be written as [...x]`

const b = [1, 2].concat(more); // OK: not empty literal

const c = [].concat(items, more); // OK: 2 args, not handled

const d = [...items]; // OK
