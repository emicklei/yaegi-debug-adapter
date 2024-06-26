package dbg

import (
	"fmt"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

func Test_valuePrinter(t *testing.T) {
	a := &Adapter{vars: newVariables()}
	var b chan bool
	u := unsafe.Pointer(a) //nolint:gosec // This is a valid used of unsafe.
	now := time.Date(2024, 6, 6, 1, 2, 3, 0, time.UTC)
	var then *time.Time
	i := 42

	cases := []struct {
		name  string
		value reflect.Value
		out   string
	}{
		{"nil chan", reflect.ValueOf(b), "nil"},
		{"int", reflect.ValueOf(int(-42)), "-42"},
		{"int8", reflect.ValueOf(int8(-42)), "-42"},
		{"*int", reflect.ValueOf(&i), "*42"},
		{"int16", reflect.ValueOf(int16(-42)), "-42"},
		{"int32", reflect.ValueOf(int32(-42)), "-42"},
		{"int64", reflect.ValueOf(int64(12345)), "12345"},
		{"uintptr", reflect.ValueOf(uintptr(2)), "2"},
		{"bool", reflect.ValueOf(true), "true"},
		{"uint", reflect.ValueOf(1), "1"},
		{"uint8", reflect.ValueOf(uint8(1)), "1"},
		{"uint16", reflect.ValueOf(uint16(1)), "1"},
		{"uint32", reflect.ValueOf(uint32(1)), "1"},
		{"uint64", reflect.ValueOf(uint64(1)), "1"},
		{"string", reflect.ValueOf("shoe"), `"shoe"`},
		{"float32", reflect.ValueOf(float32(3.14159)), "3.14159"},
		{"float64", reflect.ValueOf(float64(3.14159)), "3.14159"},
		{"[]int", reflect.ValueOf([]int{}), "[]int{}"},
		{"[]int{21,42}", reflect.ValueOf([]int{21, 42}), "[]int{21,42}"},
		{"[2]bool{false, true}", reflect.ValueOf([2]bool{false, true}), "[2]bool{false,true}"},
		{"func", reflect.ValueOf(a.newVar), "func(string, reflect.Value) *dap.Variable"},
		{"struct", reflect.ValueOf(a), "*dbg.Adapter"},
		{"map", reflect.ValueOf(map[bool]bool{}), "map[bool]bool{}"},
		{"map[int]int{-1:2}", reflect.ValueOf(map[int]int{-1: 2}), "map[int]int{-1:2}"},
		{"unsafe", reflect.ValueOf(u), fmt.Sprintf("reflect.Value %v", u)},
		{"time", reflect.ValueOf(now), "time.Time 2024-06-06 01:02:03 +0000 UTC"},
		{"*time", reflect.ValueOf(&now), "*time.Time 2024-06-06 01:02:03 +0000 UTC"},
		{"nil *time", reflect.ValueOf(then), "nil"},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			dapVar := a.newVar(each.name, each.value)
			if got, want := dapVar.Value, each.out; got != want {
				t.Errorf("got [%[1]v:%[1]T] want [%[2]v:%[2]T]", got, want)
			}
		})
	}
}

func Test_valuePrinter_print_smallMaxLength(t *testing.T) {
	cases := []struct {
		name  string
		value reflect.Value
		out   string
	}{
		{"string", reflect.ValueOf("some long text that is cut"), `"some lo...`},
		{"[]int{12345678}", reflect.ValueOf([]int{12345678, 23456789}), "[]int{12..."},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			vp := newValuePrinter(8)
			vp.print(each.value)
			if got, want := vp.String(), each.out; got != want {
				t.Errorf("got [%[1]v:%[1]T] want [%[2]v:%[2]T]", got, want)
			}
		})
	}
}

func Test_valuePrinter_printString(t *testing.T) {
	vp := newValuePrinter(7)
	rv := reflect.ValueOf("yaegi")
	if got, want := vp.printString(rv), `"yaegi"`; got != want {
		t.Errorf("got [%[1]v:%[1]T] want [%[2]v:%[2]T]", got, want)
	}
	// buffer is reset
	if got, want := vp.printString(rv), `"yaegi"`; got != want {
		t.Errorf("got [%[1]v:%[1]T] want [%[2]v:%[2]T]", got, want)
	}
}
