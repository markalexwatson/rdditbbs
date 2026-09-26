package widgets

import "strings"

// Connector returns the tree prefix for a node whose ancestors above the
// top level are described by ancestorsHaveNext (true when that ancestor has
// later siblings, so its rail continues) and whose own hasNext says whether a
// sibling follows. Two cells per level.
func Connector(ancestorsHaveNext []bool, hasNext bool) string {
	var b strings.Builder
	for _, a := range ancestorsHaveNext {
		if a {
			b.WriteString("│ ")
		} else {
			b.WriteString("  ")
		}
	}
	if hasNext {
		b.WriteString("├─")
	} else {
		b.WriteString("└─")
	}
	return b.String()
}
