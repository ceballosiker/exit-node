package gcp

import "testing"

func TestStateString(t *testing.T) {
	cases := []struct {
		in   State
		want string
	}{
		{StatePending, "pending"},
		{StateRunning, "running"},
		{StateStopped, "stopped"},
		{StateTerminated, "terminated"},
		{StateUnknown, "unknown"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("State(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseState(t *testing.T) {
	cases := []struct {
		in      string
		want    State
		wantErr bool
	}{
		{"pending", StatePending, false},
		{"PENDING", StatePending, false},
		{"running", StateRunning, false},
		{"stopped", StateStopped, false},
		{"terminated", StateTerminated, false},
		{"unknown", StateUnknown, false},
		{"banana", StateUnknown, true},
		{"", StateUnknown, true},
	}
	for _, c := range cases {
		got, err := ParseState(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseState(%q) err = %v, wantErr = %v", c.in, err, c.wantErr)
		}
		if got != c.want {
			t.Errorf("ParseState(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
