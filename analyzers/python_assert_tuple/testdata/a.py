def f(x):
    assert (x > 0, "x must be positive")  # want `assert with a tuple argument`
    assert x > 0, "x must be positive"     # OK: comma-separated args
    assert x > 0                            # OK: no message


def g(x):
    assert (x > 0)  # OK: parenthesized expression, not a tuple
