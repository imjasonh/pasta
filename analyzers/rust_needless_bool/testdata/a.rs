fn identity(x: bool) -> bool {
    if x { true } else { false } // want `this if-expression can be replaced by its condition`
}

fn negate(x: bool) -> bool {
    if x { false } else { true } // want "this if-expression can be replaced by `!(condition)`"
}

fn compound(x: bool, y: bool) -> bool {
    if x && y { true } else { false } // want `this if-expression can be replaced by its condition`
}

fn already_simple(x: bool) -> bool {
    x
}

fn returns_value(x: bool) -> bool {
    if x { false } else { false }
}

fn no_else(x: bool) -> bool {
    if x { return true; }
    false
}

fn nontrivial_branches(x: bool, y: bool) -> bool {
    if x { y } else { false }
}
