# By-name fact lookup is scope-blind, so we use distinct variable
# names across functions to avoid bleed. Real-world taint analysis
# would need scope-aware fact keys; that's a known limitation.


def chain():
    # Direct: source -> sink.
    x_chain = input()
    exec(x_chain)  # want `tainted data flows into exec`


def two_step():
    # source -> propagate -> sink.
    x_two = input()
    y_two = x_two
    eval(y_two)  # want `tainted data flows into eval`


def four_step():
    # Demonstrates fixpoint convergence — three propagation hops
    # before the sink.
    a_four = input()
    b_four = a_four
    c_four = b_four
    d_four = c_four
    system(d_four)  # want `tainted data flows into system`


def safe():
    # No source; nothing should fire.
    x_safe = "hello"
    y_safe = x_safe
    eval(y_safe)


def partial():
    # source taints x_part but y_part is from a literal.
    x_part = input()
    y_part = "literal"
    eval(y_part)  # OK: y_part is not tainted
