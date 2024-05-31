package dbg

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"sync"

	"github.com/traefik-contrib/yaegi-debug-adapter/pkg/dap"
	"github.com/traefik/yaegi/interp"
)

//nolint:deadcode,varcheck // references
const (
	rBool          = reflect.Bool
	rInt           = reflect.Int
	rInt8          = reflect.Int8
	rInt16         = reflect.Int16
	rInt32         = reflect.Int32
	rInt64         = reflect.Int64
	rUint          = reflect.Uint
	rUint8         = reflect.Uint8
	rUint16        = reflect.Uint16
	rUint32        = reflect.Uint32
	rUint64        = reflect.Uint64
	rUintptr       = reflect.Uintptr
	rFloat32       = reflect.Float32
	rFloat64       = reflect.Float64
	rComplex64     = reflect.Complex64
	rComplex128    = reflect.Complex128
	rArray         = reflect.Array
	rChan          = reflect.Chan
	rFunc          = reflect.Func
	rInterface     = reflect.Interface
	rMap           = reflect.Map
	rPtr           = reflect.Ptr
	rSlice         = reflect.Slice
	rString        = reflect.String
	rStruct        = reflect.Struct
	rUnsafePointer = reflect.UnsafePointer
)

type variables struct {
	mu     *sync.Mutex
	values []variableScope
	id     int
}

func newVariables() *variables {
	v := new(variables)
	v.mu = new(sync.Mutex)
	return v
}

func (r *variables) Purge() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.id = 0
	if r.values != nil {
		r.values = r.values[:0]
	}
}

func (r *variables) Add(v variableScope) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.id++
	r.values = append(r.values, v)
	return r.id
}

func (r *variables) Get(i int) (variableScope, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if i < 1 || i > len(r.values) {
		return nil, false
	}
	return r.values[i-1], true
}

func (a *Adapter) newVar(name string, rv reflect.Value) *dap.Variable {
	v := new(dap.Variable)
	v.Name = name
	v.Type = dap.Str(rv.Type().String())

	k := rv.Kind()
	if canBeNil(k) && rv.IsNil() {
		v.Value = "nil"
		return v
	}

	vp := newValuePrinter(128)
	vp.print(rv)
	v.Value = vp.String()

	switch rv.Kind() {
	case rInterface, rPtr:
		v.VariablesReference = a.vars.Add(&elemVars{rv})
	case rArray, rSlice:
		v.VariablesReference = a.vars.Add(&arrayVars{rv})
	case rStruct:
		v.VariablesReference = a.vars.Add(&structVars{rv})
	case rMap:
		v.VariablesReference = a.vars.Add(&mapVars{rv})
	}

	return v
}

func canBeNil(k reflect.Kind) bool {
	return k == rChan || k == rFunc || k == rInterface || k == rMap || k == rPtr || k == rSlice
}

type variableScope interface {
	Variables(a *Adapter) []*dap.Variable
}

type frameVars struct {
	*interp.DebugFrameScope
}

func (f *frameVars) Variables(a *Adapter) []*dap.Variable {
	fv := f.DebugFrameScope.Variables()
	vars := make([]*dap.Variable, 0, len(fv))

	for _, v := range fv {
		vars = append(vars, a.newVar(v.Name, v.Value))
	}
	return vars
}

type elemVars struct {
	reflect.Value
}

func (v *elemVars) Variables(a *Adapter) []*dap.Variable {
	return []*dap.Variable{a.newVar("", v.Elem())}
}

type arrayVars struct {
	reflect.Value
}

func (v *arrayVars) Variables(a *Adapter) []*dap.Variable {
	vars := make([]*dap.Variable, v.Len())
	for i := range vars {
		vars[i] = a.newVar(strconv.Itoa(i), v.Index(i))
	}
	return vars
}

type structVars struct {
	reflect.Value
}

func (v *structVars) Variables(a *Adapter) []*dap.Variable {
	vars := make([]*dap.Variable, v.NumField())
	typ := v.Type()
	for i := range vars {
		f := typ.Field(i)
		name := f.Name
		if name == "" {
			name = f.Type.Name()
		}
		vars[i] = a.newVar(name, v.Field(i))
	}
	return vars
}

type mapVars struct {
	reflect.Value
}

func (v *mapVars) Variables(a *Adapter) []*dap.Variable {
	keys := v.MapKeys()
	vars := make([]*dap.Variable, len(keys))
	for i, k := range keys {
		vars[i] = a.newVar(k.String(), v.MapIndex(k))
	}
	return vars
}

// valuePrinter is for printing reflect.Value instances on a bounded buffer.
type valuePrinter struct {
	maxLength int
	size      int
	buffer    *bytes.Buffer
}

func newValuePrinter(max int) *valuePrinter {
	return &valuePrinter{maxLength: max, buffer: new(bytes.Buffer)}
}

func (p *valuePrinter) print(rv reflect.Value) {
	switch rv.Kind() {
	case rChan, rFunc, rInterface, rPtr, rStruct:
		fmt.Fprint(p, rv.Type().String())
	case rInt, rInt8, rInt16, rInt32, rInt64:
		fmt.Fprint(p, strconv.FormatInt(rv.Int(), 10))
	case rUint8, rUint16, rUint, rUint32, rUint64, rUintptr:
		fmt.Fprintf(p, "%v", rv)
	case rBool:
		fmt.Fprint(p, strconv.FormatBool(rv.Bool()))
	case rFloat32, rFloat64, rComplex128, rComplex64:
		fmt.Fprintf(p, "%v", rv)
	case rString:
		fmt.Fprintf(p, "%q", rv.String())
	case rMap:
		fmt.Fprint(p, rv.Type().String())
		fmt.Fprint(p, "{")
		for i, k := range rv.MapKeys() {
			if i > 0 {
				fmt.Fprint(p, ",")
			}
			p.print(k)
			fmt.Fprint(p, ":")
			p.print(rv.MapIndex(k))
		}
		fmt.Fprint(p, "}")

	case rSlice, rArray:
		fmt.Fprint(p, rv.Type().String())
		fmt.Fprint(p, "{")
		for i := 0; i < rv.Len(); i++ {
			if i > 0 {
				fmt.Fprint(p, ",")
			}
			p.print(rv.Index(i))
		}
		fmt.Fprint(p, "}")
	default:
		fmt.Fprintf(p, "%[1]T %[1]v", rv)
	}
}

func (p *valuePrinter) full() bool {
	return p.maxLength-p.buffer.Len() == 0
}

// String implements Stringer
func (p *valuePrinter) String() string {
	s := p.buffer.String()
	if p.full() {
		s += "..."
	}
	return s
}

// Write implements io.Writer
func (p *valuePrinter) Write(b []byte) (n int, err error) {
	rem := p.maxLength - p.size
	if rem <= 0 {
		return 0, nil
	}
	if len(b) > rem {
		p.size += rem
		return p.buffer.Write(b[:rem])
	}
	p.size += len(b)
	return p.buffer.Write(b)
}
