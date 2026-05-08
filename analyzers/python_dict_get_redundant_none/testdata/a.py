def f(d, k):
    a = d.get(k, None)        # want `dict.get\(k, None\)`
    b = d.get(k)              # OK
    c = d.get(k, "fallback")  # OK: real default
    e = d.get(k, 0)           # OK: real default
    return a, b, c, e
