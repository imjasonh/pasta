const hang = new Promise(() => {}); // want `Promise with empty executor`

const ok = new Promise((res) => res(1));

const lazy = new Promise((res, rej) => {
    setTimeout(() => res(42), 100);
});

const empty2 = new Promise(() => { // want `Promise with empty executor`
    // body is comments-only; pasta's predicates skip comments, so
    // this is treated as empty just like `() => {}`.
});
