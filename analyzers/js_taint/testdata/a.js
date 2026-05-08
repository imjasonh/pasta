// js_taint testdata. Variable names are unique per scope to avoid
// scope-blind by-name taint bleed.

function direct(req) {
    const cmd_d = req.query.cmd;
    // want:+1 `tainted user input flows into eval`
    eval(cmd_d);
}

function twoStep(req) {
    const a_two = req.query.cmd;
    const b_two = a_two;
    // want:+1 `tainted user input flows into eval`
    eval(b_two);
}

function fourStep(req) {
    const a_four = req.body;
    const b_four = a_four;
    const c_four = b_four;
    const d_four = c_four;
    // want:+1 `tainted user input flows into Function`
    Function(d_four);
}

function safe(req) {
    const safe_x = "static code";
    const safe_y = safe_x;
    eval(safe_y); // OK: from a literal
}

function partial(req) {
    const _ignored = req.params.id;
    const partial_y = "literal";
    eval(partial_y); // OK: partial_y not tainted
}
