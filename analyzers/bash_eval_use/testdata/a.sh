#!/bin/bash

dynamic_cmd="echo hello"
eval "$dynamic_cmd"      # want `eval re-parses its arguments`

x=42
echo "$x"

eval echo "$dynamic_cmd" # want `eval re-parses its arguments`

# OK: not a call to eval
my_eval_var=foo
echo "$my_eval_var"
