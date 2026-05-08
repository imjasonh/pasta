def f():
    try:
        return risky()
    except:  # want `bare except: catches everything`
        return None


def g():
    try:
        return risky()
    except Exception:
        return None


def h():
    try:
        return risky()
    except (ValueError, KeyError):
        return None
