use std::env;
use std::process::Command;

fn direct() {
    let cmd_d = env::var("CMD").unwrap();
    // want:+1 `tainted data from environment flows into Command::new`
    let _ = Command::new(cmd_d).spawn();
}

fn two_step() {
    let a_two = env::var("CMD").unwrap();
    let b_two = a_two;
    // want:+1 `tainted data from environment flows into Command::new`
    let _ = Command::new(b_two).spawn();
}

fn four_step() {
    let a_four = env::var("CMD").unwrap();
    let b_four = a_four;
    let c_four = b_four;
    let d_four = c_four;
    // want:+1 `tainted data from environment flows into Command::new`
    let _ = Command::new(d_four).spawn();
}

fn safe() {
    let safe_x = "ls".to_string();
    let safe_y = safe_x;
    let _ = Command::new(safe_y).spawn(); // OK: literal
}
