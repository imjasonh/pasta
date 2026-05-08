def f(x=[]):  # want `mutable default argument`
    x.append(1)
    return x


def g(x={}):  # want `mutable default argument`
    return x


def h(x=set()):  # OK: function call, not a literal
    return x


def i(x=None):  # OK: idiomatic
    if x is None:
        x = []
    return x


def j(x: list = [1, 2]):  # want `mutable default argument`
    return x
