const hang = new Promise(() => {}); // want `Promise with empty executor`

const ok = new Promise((res) => res(1));

const lazy = new Promise((res, rej) => {
    setTimeout(() => res(42), 100);
});

const empty2 = new Promise(() => {
    // even with a comment, no resolve/reject — but this body is non-empty
    // by tree-sitter's reckoning (the comment is a child).
});
