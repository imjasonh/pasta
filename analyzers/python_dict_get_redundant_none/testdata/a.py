def f(d, k):
    a = d.get(k, None)        # want `dict.get\(k, None\)`
    b = d.get(k)              # OK
    c = d.get(k, "fallback")  # OK: real default
    e = d.get(k, 0)           # OK: real default
    return a, b, c, e


# Surgical delete preserves anything outside the `, None` span — both
# the trailing-line comment and a comment around `d.get(` survive.
def keeps_comments(d, k):
    return d.get(k, None)  # trailing # want `dict.get\(k, None\)`
