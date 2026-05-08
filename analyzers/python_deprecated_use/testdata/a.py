from typing_extensions import deprecated


@deprecated("use new_fn instead")
def old_fn():
    return 1


@deprecated
def really_old():
    return 2


def new_fn():
    return 3


def caller():
    a = old_fn()       # want `old_fn is deprecated`
    b = new_fn()        # OK
    c = really_old()    # want `really_old is deprecated`
    return a + b + c
