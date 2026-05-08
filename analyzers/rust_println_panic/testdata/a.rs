fn bad(x: i32) {
    println!("about to die: {}", x); // want `println! before panic!`
    panic!("explanation");
}

fn ok(x: i32) {
    panic!("just panic: {}", x);
}

fn debug_only(x: i32) {
    println!("normal debug");
    println!("more debug");
}
