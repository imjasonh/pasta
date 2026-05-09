function f() {
    var x = 5;                  // want "use `let` instead of `var`"
    var y, z = 1;               // want "use `let` instead of `var`"
    let already = 7;
    const constant = 9;
    return x + y + z + already + constant;
}
