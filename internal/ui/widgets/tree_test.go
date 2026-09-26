package widgets

import "testing"

func TestConnector(t *testing.T) {
	cases := []struct {
		anc  []bool
		next bool
		want string
	}{
		{nil, true, "├─"},
		{nil, false, "└─"},
		{[]bool{true}, true, "│ ├─"},
		{[]bool{false}, false, "  └─"},
		{[]bool{true, false}, true, "│   ├─"},
	}
	for _, c := range cases {
		if got := Connector(c.anc, c.next); got != c.want {
			t.Errorf("Connector(%v,%v) = %q want %q", c.anc, c.next, got, c.want)
		}
	}
}
