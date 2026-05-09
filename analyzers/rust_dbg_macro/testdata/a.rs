fn compute(x: i32) -> i32 {
    let y = dbg!(x + 1); // want `dbg!() macro invocation`
    y * 2
}

fn ok(x: i32) -> i32 {
    let y = x + 1;
    y * 2
}

fn nested(xs: Vec<i32>) -> i32 {
    dbg!(xs.iter().sum::<i32>()) // want `dbg!() macro invocation`
}
