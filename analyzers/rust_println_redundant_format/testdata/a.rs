fn demo(x: &str) {
    println!("{}", "hello");      // want "redundant format placeholder"
    println!("{}", "world");      // want "redundant format placeholder"
    println!("{}", x);            // OK: argument is not a literal
    println!("hi");               // OK: no format args
    println!("{} {}", "a", "b");  // OK: more than one arg
}
