package worker

import (
	"testing"
	"time"
)

func TestSendRate(t *testing.T) {
	cases := []struct {
		n    int
		d    time.Duration
		want string
	}{
		{5, 400 * time.Millisecond, "12.5/s"},
		{780000, 13 * time.Minute, "1000.0/s"},
		{3, 0, "3 in <1ms"},
	}
	for _, c := range cases {
		if got := sendRate(c.n, c.d); got != c.want {
			t.Errorf("sendRate(%d, %v) = %q, want %q", c.n, c.d, got, c.want)
		}
	}
}
