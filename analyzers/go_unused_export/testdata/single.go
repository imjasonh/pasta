// Single-file scenario: top-level testdata files are run as their
// own one-file group, so only same-file calls satisfy the `called`
// fact lookup.

package single

func UsedSingle() {} // OK

func UnusedSingle() {} // want `exported func UnusedSingle is never called`

func driver() {
	UsedSingle()
}
