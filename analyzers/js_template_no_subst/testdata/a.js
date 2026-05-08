const a = `hello world`; // want `template literal without interpolation`

const b = `hello ${name}`;  // OK: has interpolation

const c = "already a regular string";

const d = `simple`; // want `template literal without interpolation`

const multi = `line1
line2`; // OK: multi-line, skip

const withSingleQuote = `it's`; // OK: contains ', skip
