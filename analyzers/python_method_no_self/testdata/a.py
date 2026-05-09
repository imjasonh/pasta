class Good:
    def ok(self):
        return 1

    def with_args(self, x, y):
        return x + y

    def cls_method(cls, x):  # also acceptable
        return cls

    def annotated(self: "Good") -> int:
        return 0


class Bad:
    def no_self():  # want "method `no_self` is missing `self`"
        return 1

    def wrong_name(other):  # want "method `wrong_name` is missing `self`"
        return other


# Top-level functions are fine — not in a class.
def freestanding():
    return 1


def takes_other(other):
    return other
