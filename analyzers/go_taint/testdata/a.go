package a

import (
	"os"
	"os/exec"
)

func direct() {
	cmd := os.Getenv("CMD")
	// want:+1 `tainted data from environment flows into exec.Command`
	exec.Command(cmd)
}

func two_step() {
	a_two := os.Getenv("CMD")
	b_two := a_two
	// want:+1 `tainted data from environment flows into exec.Command`
	exec.Command(b_two)
}

func four_step() {
	a_four := os.Getenv("CMD")
	b_four := a_four
	c_four := b_four
	d_four := c_four
	// want:+1 `tainted data from environment flows into exec.Command`
	exec.Command(d_four)
}

func safe() {
	a_safe := "ls"
	b_safe := a_safe
	exec.Command(b_safe) // OK: literal
}

func partial() {
	_ = os.Getenv("UNUSED")
	cmd_part := "ls"
	exec.Command(cmd_part) // OK: cmd_part is from a literal
}
