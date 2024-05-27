package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/traefik-contrib/yaegi-debug-adapter/internal/jsonx"
)

func fatalf(frmt string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, frmt, args...)
	os.Exit(1)
}

func noerr(fn func() error, msg string, args ...interface{}) {
	err := fn()
	if err != nil {
		fatalf(msg, append(args, err)...)
	}
}

var (
	schemaURL   = flag.String("url", "", "URL of the schema")
	packageName = flag.String("name", "", "Package name")
	filePath    = flag.String("path", "", "File to write to")
	jsonPatch   = flag.String("patch", "", "JSON file to apply as a patch")
	chooseTypes = flag.String("choose", "", "Comma-separated list of types to extract, instead of all types")
	verbose     = flag.Bool("verbose", false, "Print more messages")
)

var dapMode = flag.Bool("dap-mode", false, "Used for generating Debug Adapter Protocol types")

func main() {
	flag.Parse()

	if *schemaURL == "" {
		fatalf("--url is required")
	}
	if *packageName == "" {
		fatalf("--name is required")
	}
	if *filePath == "" {
		fatalf("--path is required")
	}

	resp, err := http.Get(*schemaURL)
	if err != nil {
		fatalf("http: %v\n", err)
	}

	if resp.StatusCode != http.StatusOK {
		noerr(resp.Body.Close, "failed to close response body: %v")
		fatalf("http: expected status 200, got %d\n", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	noerr(resp.Body.Close, "failed to close response body: %v")
	if err != nil {
		fatalf("read (schema): %v\n", err)
	}

	if *jsonPatch != "" {
		b, err = jsonx.ApplyPatch(b, *jsonPatch)
		if err != nil {
			fatalf("%v\n", err)
		}
	}

	var schema jsonx.Schema
	err = json.Unmarshal(b, &schema)
	if err != nil {
		fatalf("json (schema): %v\n", err)
	}

	buf := new(bytes.Buffer)
	w := &writer{
		Writer:     buf,
		Schema:     &schema,
		Name:       "Schema",
		Embed:      *dapMode,
		OmitEmpty:  *dapMode,
		NoOptional: !*dapMode,
	}
	w.init()

	fmt.Fprintf(w, "package %s\n", *packageName)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "// Code generated by 'go run ../internal/cmd/genschema'. DO NOT EDIT.\n")
	fmt.Fprintf(w, "\n")

	var m map[string]bool
	if *chooseTypes != "" {
		m = map[string]bool{}
		for _, typ := range strings.Split(*chooseTypes, ",") {
			m[typ] = true
		}
	}

	forEachOrdered(w.Schema.Definitions, func(name string, s *jsonx.Schema) {
		name = camelCase(name)
		if m == nil || m[name] {
			w.writeSchema(name, s)
		} else if *verbose {
			fmt.Fprintf(os.Stderr, "Omitting %s\n", name)
		}
	})

	if w.Schema.Properties != nil {
		w.writeSchema(w.Name, w.Schema)
	}

	f, err := os.Create(*filePath)
	if err != nil {
		fatalf("json: %v\n", err)
	}
	defer noerr(f.Close, "failed to close %s: %v", *filePath)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, *filePath, buf, parser.ParseComments)
	if err != nil {
		fatalf("json: %v\n", err)
	}

	err = format.Node(f, fset, file)
	if err != nil {
		fatalf("json: %v\n", err)
	}
}

func isPlain(s *jsonx.Schema) bool {
	return s.Default == nil && isPlainExceptDefault(s)
}

func isPlainExceptDefault(s *jsonx.Schema) bool {
	return true &&
		s.Items == nil &&
		s.AdditionalProperties == nil &&
		s.Definitions == nil &&
		s.Properties == nil &&
		s.PatternProperties == nil &&
		s.Enum == nil &&
		s.AllOf == nil &&
		s.AnyOf == nil &&
		s.OneOf == nil &&
		s.Not == nil
}

func howMany(v ...interface{}) int {
	c := 0
	for _, v := range v {
		if v != nil && !reflect.ValueOf(v).IsNil() {
			c++
		}
	}
	return c
}

func resolveRef(s *jsonx.Schema, ref string) (string, *jsonx.Schema) {
	if ref == "#" {
		return "", s
	}

	const schemaDefs = "#/definitions/"
	if strings.HasPrefix(ref, schemaDefs) {
		name := ref[len(schemaDefs):]
		def, ok := s.Definitions[name]
		if !ok {
			fatalf("missing definition for %q\n", name)
		}
		return name, def
	}

	fatalf("unsupported ref %q\n", ref)
	panic("not reachable")
}

func forEachOrdered(m map[string]*jsonx.Schema, fn func(string, *jsonx.Schema)) {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}

	sort.StringSlice(names).Sort()

	for _, name := range names {
		fn(name, m[name])
	}
}

func unsupported(name string, s *jsonx.Schema) {
	fmt.Fprintf(os.Stderr, "type %q: unsupported schema\n", name)
	enc := json.NewEncoder(os.Stderr)
	enc.SetIndent("", "    ")
	_ = enc.Encode(s)
	os.Exit(1)
}

type mergeOpts struct {
	Base        *jsonx.Schema
	ResolveRefs bool
	Recurse     bool
}

func schemaMerge(opts mergeOpts, name string, s, r *jsonx.Schema) {
	if opts.ResolveRefs && r.Ref != "" {
		if !isPlain(r) {
			fatalf("type %q: non-plain ref types are not supported\n", name)
		}
		_, r = resolveRef(opts.Base, r.Ref)
	}

	if opts.Recurse && r.AllOf != nil {
		sch := new(jsonx.Schema)
		for i, r := range r.AllOf {
			schemaMerge(opts, fmt.Sprintf("%s[%d]", name, i), sch, r)
		}
		r = sch
	}

	if s.Description == "" {
		s.Description = r.Description
	} else if r.Description != "" {
		s.Description += "\n" + r.Description
	}

	schemaReplaceField(opts, name, s, r, "Default")
	schemaReplaceField(opts, name, s, r, "AdditionalItems")
	schemaReplaceField(opts, name, s, r, "Items")
	schemaReplaceField(opts, name, s, r, "Required")
	schemaReplaceField(opts, name, s, r, "AdditionalProperties")
	schemaReplaceField(opts, name, s, r, "Definitions")
	schemaReplaceField(opts, name, s, r, "Properties")
	schemaReplaceField(opts, name, s, r, "PatternProperties")
	schemaReplaceField(opts, name, s, r, "Dependencies")
	schemaReplaceField(opts, name, s, r, "Enum")
	schemaReplaceField(opts, name, s, r, "Type")
	schemaReplaceField(opts, name, s, r, "AllOf")
	schemaReplaceField(opts, name, s, r, "AnyOf")
	schemaReplaceField(opts, name, s, r, "OneOf")
	schemaReplaceField(opts, name, s, r, "Not")
}

func schemaReplaceField(opts mergeOpts, name string, s, r *jsonx.Schema, field string) {
	rf := reflect.ValueOf(r).Elem().FieldByName(field)
	rv := rf.Interface()
	if rf.IsNil() {
		return
	}

	sf := reflect.ValueOf(s).Elem().FieldByName(field)
	sv := sf.Interface()
	if sf.IsNil() {
		if _, ok := sv.(map[string]*jsonx.Schema); ok {
			sv = map[string]*jsonx.Schema{}
			sf.Set(reflect.ValueOf(sv))
		} else {
			sf.Set(rf)
			return
		}
	}

	switch sv := sv.(type) {
	case jsonx.Schema_Type:
		m := map[jsonx.SimpleTypes]bool{}
		for _, v := range rv.(jsonx.Schema_Type) {
			m[v] = true
		}
		nv := jsonx.Schema_Type{}
		for _, v := range sv {
			if m[v] {
				nv = append(nv, v)
			}
		}
		sf.Set(reflect.ValueOf(nv))

	case map[string]*jsonx.Schema:
		for k, v := range rv.(map[string]*jsonx.Schema) {
			if sv[k] == nil {
				sv[k] = v
			} else {
				schemaMerge(opts, name+"["+k+"]", sv[k], v)
			}
		}

	default:
		if !reflect.DeepEqual(sv, rv) && field != "Enum" {
			fatalf("type %q: unsupported operation: attempting to overwrite field %s\n", name, field)
		}
	}
}

func camelCase(s string) string {
	sb := new(strings.Builder)
	sb.Grow(len(s))

	upcase := true
	for _, r := range s {
		switch {
		case r == '$':
			// ignore
		case r == '_':
			upcase = true
		case upcase:
			sb.WriteRune(unicode.ToTitle(r))
			upcase = false
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
