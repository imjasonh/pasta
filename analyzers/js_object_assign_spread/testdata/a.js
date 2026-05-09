const a = Object.assign({}, src); // want `Object.assign({}, x) can be written as {...x}`

const b = Object.assign(target, src); // OK: target is not empty {}

const c = Object.assign({}, src1, src2); // OK: 3 args, not handled

const d = {...src}; // OK: already spread
