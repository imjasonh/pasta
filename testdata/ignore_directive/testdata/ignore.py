print("plain")  # want "print A" # want "print B"
print("ignore all")  # pasta:ignore
print("ignore one")  # pasta:ignore print_call # want "print B"
print("ignore both")  # pasta:ignore print_call, other_print
print("still fires")  # pasta:ignore unrelated_rule # want "print A" # want "print B"
