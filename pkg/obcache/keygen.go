package obcache

import (
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

const (
	noArgsKey = "no-args"

	// maxKeyLen is the longest key kept readable; anything longer is hashed.
	// Readable keys start with t or n, so they cannot equal a hex digest.
	maxKeyLen = 64

	// keySpill is how much encoded key is buffered before it goes to the hash, so
	// a large argument is not held twice.
	keySpill = 4096

	maxKeyDepth = 64
)

// DefaultKeyFunc encodes all of args. Distinct arguments never share a key.
// It panics on args with no deterministic encoding.
func DefaultKeyFunc(args []any) string {
	key, err := encodeArgs(args)
	if err != nil {
		panic("obcache.DefaultKeyFunc: " + err.Error())
	}
	return key
}

func encodeArgs(args []any) (string, error) {
	if len(args) == 0 {
		return noArgsKey, nil
	}
	var e keyEncoder
	for _, arg := range args {
		if err := e.typed(reflect.ValueOf(arg), 0); err != nil {
			return "", err
		}
	}
	return e.sum(), nil
}

// keyEncoder writes a self-delimiting encoding: every value carries a tag, and
// anything variable-length carries its length, so no argument can imitate a
// boundary between arguments.
type keyEncoder struct {
	buf  []byte
	hash hash.Hash
}

func (e *keyEncoder) spill() {
	if e.hash == nil {
		e.hash = sha256.New()
	}
	e.hash.Write(e.buf)
	e.buf = e.buf[:0]
}

func (e *keyEncoder) sum() string {
	if e.hash == nil && len(e.buf) <= maxKeyLen {
		return string(e.buf)
	}
	e.spill()
	return hex.EncodeToString(e.hash.Sum(nil))
}

func (e *keyEncoder) tag(c byte) {
	e.buf = append(e.buf, c)
	if len(e.buf) >= keySpill {
		e.spill()
	}
}

func (e *keyEncoder) length(n int) {
	e.buf = append(strconv.AppendInt(e.buf, int64(n), 10), ':')
}

func (e *keyEncoder) text(s string) {
	e.length(len(s))
	if len(s) < keySpill {
		e.buf = append(e.buf, s...)
		return
	}
	e.spill()
	_, _ = io.WriteString(e.hash, s)
}

func (e *keyEncoder) bytes(b []byte) {
	e.length(len(b))
	if len(b) < keySpill {
		e.buf = append(e.buf, b...)
		return
	}
	e.spill()
	e.hash.Write(b)
}

// typed tags a value with its dynamic type, which a parameter of interface type
// does not fix. Type.String alone is not unique across packages.
func (e *keyEncoder) typed(v reflect.Value, depth int) error {
	if !v.IsValid() {
		e.tag('n')
		return nil
	}
	name := v.Type().String()
	if pkg := v.Type().PkgPath(); pkg != "" {
		name = pkg + "." + name
	}
	e.tag('t')
	e.text(name)
	return e.value(v, depth)
}

func (e *keyEncoder) value(v reflect.Value, depth int) error {
	if depth > maxKeyDepth {
		return errors.New("argument is cyclic or nested too deeply")
	}
	t := v.Type()

	// An interface decides by its dynamic value. A struct that only inherits
	// MarshalText from an embedded field decides by its fields: the marshaler
	// would otherwise hide every other field from the key.
	if t.Kind() != reflect.Interface && !promotes(t, textMarshaler) {
		var marshaler encoding.TextMarshaler
		if t.Implements(textMarshaler) {
			if t.Kind() == reflect.Pointer && v.IsNil() {
				e.tag('n')
				return nil
			}
			marshaler, _ = v.Interface().(encoding.TextMarshaler)
		} else if reflect.PointerTo(t).Implements(textMarshaler) {
			p := reflect.New(t)
			p.Elem().Set(v)
			marshaler, _ = p.Interface().(encoding.TextMarshaler)
		}
		if marshaler != nil {
			text, err := marshaler.MarshalText()
			if err != nil {
				return err
			}
			e.tag('m')
			e.bytes(text)
			return nil
		}
	}

	switch t.Kind() {
	case reflect.String:
		e.tag('s')
		e.text(v.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.tag('i')
		e.buf = append(strconv.AppendInt(e.buf, v.Int(), 10), ';')
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		e.tag('u')
		e.buf = append(strconv.AppendUint(e.buf, v.Uint(), 10), ';')
	case reflect.Float32, reflect.Float64:
		e.tag('f')
		e.buf = append(strconv.AppendFloat(e.buf, v.Float(), 'g', -1, 64), ';')
	case reflect.Complex64, reflect.Complex128:
		e.tag('c')
		e.text(fmt.Sprint(v.Complex()))
	case reflect.Bool:
		e.tag('b')
		e.buf = strconv.AppendBool(e.buf, v.Bool())
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			e.tag('n')
			return nil
		}
		if t.Kind() == reflect.Interface {
			return e.typed(v.Elem(), depth+1)
		}
		e.tag('p')
		return e.value(v.Elem(), depth+1)
	case reflect.Slice, reflect.Array:
		return e.sequence(v, depth)
	case reflect.Map:
		return e.mapping(v, depth)
	case reflect.Struct:
		e.tag('(')
		for i := range v.NumField() {
			if !t.Field(i).IsExported() {
				return fmt.Errorf("%s has unexported field %s", t, t.Field(i).Name)
			}
			if err := e.value(v.Field(i), depth+1); err != nil {
				return err
			}
		}
		e.tag(')')
	default:
		return fmt.Errorf("cannot key a value of type %s", t)
	}
	return nil
}

func (e *keyEncoder) sequence(v reflect.Value, depth int) error {
	t := v.Type()
	if t.Kind() == reflect.Slice {
		if v.IsNil() {
			e.tag('n')
			return nil
		}
		if t.Elem().Kind() == reflect.Uint8 && !implementsEither(t.Elem(), textMarshaler) {
			e.tag('B')
			e.bytes(v.Bytes())
			return nil
		}
	}
	e.tag('[')
	e.length(v.Len())
	for i := range v.Len() {
		if err := e.value(v.Index(i), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (e *keyEncoder) mapping(v reflect.Value, depth int) error {
	if v.IsNil() {
		e.tag('n')
		return nil
	}
	pairs := make([]string, 0, v.Len())
	for iter := v.MapRange(); iter.Next(); {
		var pair keyEncoder
		if err := pair.value(iter.Key(), depth+1); err != nil {
			return err
		}
		if err := pair.value(iter.Value(), depth+1); err != nil {
			return err
		}
		pairs = append(pairs, pair.sum())
	}
	slices.Sort(pairs)
	e.tag('{')
	e.length(len(pairs))
	for _, pair := range pairs {
		e.text(pair)
	}
	return nil
}

// SimpleKeyFunc joins string representations of args. It is not injective:
// ("a:b", "c") and ("a", "b:c") share a key.
//
// Deprecated: use DefaultKeyFunc. Never pass caller-controlled strings to this.
func SimpleKeyFunc(args []any) string {
	if len(args) == 0 {
		return noArgsKey
	}

	var parts []string
	for _, arg := range args {
		parts = append(parts, fmt.Sprintf("%v", arg))
	}

	return strings.Join(parts, ":")
}

var textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()

func implementsEither(t, iface reflect.Type) bool {
	return t.Implements(iface) || reflect.PointerTo(t).Implements(iface)
}

// promotes reports whether a struct, or a pointer to one, gets iface from an
// embedded field.
func promotes(t, iface reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := range t.NumField() {
		if f := t.Field(i); f.Anonymous && implementsEither(f.Type, iface) {
			return true
		}
	}
	return false
}

// walkType refuses types with no faithful encoding. leaf stops the walk at a type
// that encodes itself.
func walkType(t reflect.Type, leaf func(reflect.Type) bool, seen map[reflect.Type]bool) error {
	if seen[t] {
		return nil
	}
	seen[t] = true
	if leaf(t) {
		return nil
	}
	walk := func(e reflect.Type) error { return walkType(e, leaf, seen) }
	switch t.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return fmt.Errorf("%s has no stable encoding", t)
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return walk(t.Elem())
	case reflect.Map:
		if err := walk(t.Key()); err != nil {
			return err
		}
		return walk(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				return fmt.Errorf("%s has unexported field %s", t, f.Name)
			}
			if err := walk(f.Type); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkParams refuses parameter types the default key would collapse: walking
// unexported fields of something like *sql.DB reads live state, and skipping them
// is how distinct arguments came to share a key.
func checkParams(fnType reflect.Type) error {
	leaf := func(t reflect.Type) bool {
		return implementsEither(t, textMarshaler) && !promotes(t, textMarshaler)
	}
	for i := range fnType.NumIn() {
		in := fnType.In(i)
		if i == 0 && in == contextType {
			continue
		}
		if err := walkType(in, leaf, map[reflect.Type]bool{}); err != nil {
			return fmt.Errorf("parameter %d: %w; use WithKeyFunc", i, err)
		}
	}
	return nil
}
