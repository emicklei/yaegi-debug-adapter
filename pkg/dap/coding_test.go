package dap

import "testing"

func TestEvent_UnmarshalJSON(t *testing.T) {
	ev := new(Event)

	err := ev.UnmarshalJSON([]byte(`{"seq":2,"type":"event","event":"initialized"}`))
	if err != nil {
		t.Error(err)
	}

	if got, want := ev.Event, "initialized"; got != want {
		t.Errorf("got [%[1]v:%[1]T] want [%[2]v:%[2]T]", got, want)
	}

	if got, want := ev.Seq, 2; got != want {
		t.Errorf("got [%[1]v:%[1]T] want [%[2]v:%[2]T]", got, want)
	}
}
