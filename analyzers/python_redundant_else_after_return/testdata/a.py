def f(x):
    if x > 0:                       # want "redundant `else`"
        return 1
    else:
        return 2


def g(x):
    if x > 0:                       # OK: already inverted
        return 1
    return 2


def h(x):
    if x > 0:
        do_thing()                  # OK: multi-statement if-body (rule limitation)
        return 1
    else:
        return 2


def i(x):
    if x > 0:                       # OK: no return
        return 1
    elif x < 0:
        return -1
    else:
        return 0
