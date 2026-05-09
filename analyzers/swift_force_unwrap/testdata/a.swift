func test1(x: Int?) -> Int {
    return x!                           // want "force-unwrap"
}

func test2(x: Int?) -> Int {
    return x ?? 0                       // OK: nil-coalescing
}

func test3(x: Int?) -> Int? {
    if let unwrapped = x {              // OK: optional binding
        return unwrapped
    }
    return nil
}

func test4(x: Int?) -> String? {
    return x?.description               // OK: optional chaining
}
