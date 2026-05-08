#[deprecated]
fn old_fn() -> i32 { 42 }

#[deprecated(note = "use new_fn instead")]
fn old_with_note() -> i32 { 1 }

fn new_fn() -> i32 { 100 }

fn caller() -> i32 {
    let a = old_fn();          // want `old_fn is deprecated`
    let b = new_fn();           // OK
    let c = old_with_note();    // want `old_with_note is deprecated`
    a + b + c
}
