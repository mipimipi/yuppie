package main

import (
	"html/template"
)

var typesTmpl = template.Must(template.New("types").Parse(`
// ***********************************************************
// GENERATED FILE - DO NOT EDIT BY HAND. 
// ***********************************************************

package yuppie

import (
	"fmt"
	"net/url"
	"reflect"
	"sync"
	"time"

	r "gitlab.com/go-utilities/reflect"
)

// constructors maps UPnP types to constructor functions
var constructors = map[string]func(string) (StateVar, error){ {{range .TypeMapping}}
	"{{.UPnPType}}": func(v string) (StateVar, error) { return new{{.TypeNameTitle}}(v) },{{end}}
}

{{range .TypeMapping}}
// {{.TypeName}} is the representation of the UPnP type {{.UPnPType}} as Golang type
type {{.TypeName}} struct {
	val {{.GoType}}
	*sync.Mutex
}

// new{{.TypeNameTitle}} creates a new state variable that represents the UPnP type {{.UPnPType}}
func new{{.TypeNameTitle}}(s string) (t *{{.TypeName}}, err error) {
	v, err := unmarshal{{.TypeNameTitle}}(s)
	if err != nil {
		return nil, err
	}

	t = &{{.TypeName}}{ v, &sync.Mutex{} }
	return
}

// Get returns the value of t
func (t {{.TypeName}}) Get() interface{} {
	return t.val
}

// Set sets t to the new value v
func (t *{{.TypeName}}) Set(v interface{}) (err error) {
	if reflect.TypeOf(v).String() != "{{.GoType}}" {
		err = fmt.Errorf("expected type {{.GoType}}, received: %s", reflect.TypeOf(v))
		return
	}
	t.val = v.({{.GoType}})
	return
}

// Init initializes t with a new value v
func (t *{{.TypeName}}) Init(v interface{}) (err error) {
	return t.Set(v)
}

// SetFromString sets the value of t from s
func (t *{{.TypeName}}) SetFromString(s string) (err error) {
	t.val, err = unmarshal{{.TypeNameTitle}}(s)
	return
}

// String returns the string representation of the value of t
func (t {{.TypeName}}) String() string {
	s, _ := marshal{{.TypeNameTitle}}(t.val)

	return s
}

// Type returns the UPnP type of t
func (t {{.TypeName}}) Type() string { return "{{.UPnPType}}" }

// IsNumeric returns true is the value of t is numeric, otherwise false is
// returned
func (t *{{.TypeName}}) IsNumeric() bool { return isNumeric(t.val) }

// IsString returns true is the value of t is a string, otherwise false is
// returned
func (t *{{.TypeName}}) IsString() bool { return isString(t.val) }

// IsZero returns true is the value of t is the zero value of that type,
// otherwise false is returned
func (t {{.TypeName}}) IsZero() bool { return reflect.ValueOf(t.val).IsZero() }
{{end}}

// isNumeric returns true is val is a numeric value, otherwise false is returned
func isNumeric(val interface{}) bool {
	return r.Contains(
		[]reflect.Kind{reflect.Float32, reflect.Float64,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		},
		reflect.ValueOf(val).Kind())
}

// isString returns true is val is a string, otherwise false is returned
func isString(val interface{}) bool { return reflect.ValueOf(val).Kind() == reflect.String }
`,
))
