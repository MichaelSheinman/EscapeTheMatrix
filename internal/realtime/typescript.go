package realtime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// The client's message types are generated from the Go ones (technical.md §8,
// protocol rule 1). Two hand-maintained lists drift — a field renamed on one
// side, a type widened on the other — and the drift is found in production, by
// a player, as a screen that did not update.
//
// Generating in this direction rather than the other is the same decision the
// command layer already made: the server owns the shape, because the server is
// what has to still be right when a client from last week is still open.
//
// The output is a file in the repository rather than a build step, so that a
// change to a payload shows up as a diff in the same commit as the change that
// caused it, and a stale file fails a test rather than a build nobody ran.

// header is emitted verbatim at the top of the generated file.
const header = `// Code generated from the Go message definitions in internal/realtime.
// DO NOT EDIT — change the Go struct and run ` + "`make generate`" + `.
//
// technical.md §8: every server→client message is {type, seq, payload}, and
// these types are generated rather than hand-maintained because two lists of
// the same protocol drift and you find out in production.
//
// A list or record here is never null. internal/wire encodes everything the
// server sends and replaces a nil collection with an empty one, so ` + "`?? []`" + `
// on a field of these types is guarding against something that cannot arrive.
`

// TypeScript renders every registered message type as a TypeScript module.
//
// It is deterministic: the registry is sorted, and a struct's fields are
// emitted in declaration order, so regenerating without changing a Go type
// produces a byte-identical file.
func TypeScript() []byte {
	var b strings.Builder
	b.WriteString(header)

	names := definedNames()

	for _, name := range names {
		b.WriteString("\n/** Payload of the `" + name + "` message. */\n")
		b.WriteString("export type " + tsName(name) + " = " + tsType(definitions[name], nil, "") + "\n")
	}

	// The union is what makes a switch over `message.type` exhaustive on the
	// client: adding a message here turns an unhandled case into a type error
	// there rather than into a message silently ignored at runtime.
	b.WriteString("\n/** Every message the server can send, discriminated by `type`. */\nexport type ServerMessage =")
	for _, name := range names {
		b.WriteString("\n  | { type: '" + name + "'; seq: number; payload: " + tsName(name) + " }")
	}
	b.WriteString("\n\n/** The name of any server message. */\nexport type ServerMessageType = ServerMessage['type']\n")

	return []byte(b.String())
}

// tsName turns a wire name into a TypeScript type name: `probe.recorded`
// becomes `ProbeRecorded`. Derived from the message name rather than from the
// Go struct's name, so a payload type can be called whatever suits the package
// that owns it.
func tsName(name string) string {
	var b strings.Builder
	for _, part := range strings.Split(name, ".") {
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

// timeType and rawType are the two Go types whose JSON form is not their
// structure.
var (
	timeType = reflect.TypeFor[time.Time]()
	rawType  = reflect.TypeFor[json.RawMessage]()
)

// tsType renders one Go type as the TypeScript for its JSON form.
//
// path is the chain of structs currently being rendered, which is what turns a
// self-referential payload into a panic at generation time rather than a hang.
func tsType(t reflect.Type, path []reflect.Type, indent string) string {
	switch t {
	case timeType:
		// RFC 3339, and left as a string: parsing it into a Date belongs to the
		// component that formats it, not to the transport.
		return "string"
	case rawType:
		return "unknown"
	}

	switch t.Kind() {
	case reflect.Pointer:
		// An absent optional field is null on the wire, not missing, unless the
		// struct also says omitempty — which the field renderer handles.
		return tsType(t.Elem(), path, indent) + " | null"

	case reflect.String:
		return "string"

	case reflect.Bool:
		return "boolean"

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"

	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// []byte is base64 in JSON.
			return "string"
		}
		elem := tsType(t.Elem(), path, indent)
		if strings.ContainsAny(elem, " |") {
			elem = "(" + elem + ")"
		}
		// Not `| null`, though a nil slice does marshal to null: nothing
		// reaches the client except through internal/wire, which turns one
		// into `[]` first. Saying `| null` here was honest about encoding/json
		// and cost sixty-two hand-written `?? []` on the client, one of which
		// was always going to be missing.
		return elem + "[]"

	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("realtime: a map keyed by %s has no JSON object form", t.Key()))
		}
		// Non-null for the reason the slice case gives.
		return "Record<string, " + tsType(t.Elem(), path, indent) + ">"

	case reflect.Struct:
		return tsStruct(t, path, indent)

	default:
		panic(fmt.Sprintf("realtime: %s has no TypeScript form", t))
	}
}

// tsStruct renders a struct as an object type, inline.
//
// Inline rather than named because the payload's *shape* is the contract: two
// messages that happen to share a Go struct are still two independent messages,
// and naming the shared type would make a change to one of them silently a
// change to both.
func tsStruct(t reflect.Type, path []reflect.Type, indent string) string {
	for _, seen := range path {
		if seen == t {
			panic(fmt.Sprintf("realtime: %s refers to itself, which has no finite TypeScript form", t))
		}
	}
	path = append(path, t)

	inner := indent + "  "
	var b strings.Builder
	b.WriteString("{\n")
	for _, f := range fields(t) {
		b.WriteString(inner + f.name)
		if f.optional {
			b.WriteString("?")
		}
		b.WriteString(": " + tsType(f.typ, path, inner) + "\n")
	}
	b.WriteString(indent + "}")
	return b.String()
}

type field struct {
	name     string
	typ      reflect.Type
	optional bool
}

// fields flattens a struct the way encoding/json sees it: json tags decide the
// names, `-` and unexported fields are absent, and an embedded struct's fields
// are the outer struct's.
func fields(t reflect.Type) []field {
	var out []field
	for i := range t.NumField() {
		f := t.Field(i)
		tag, tagged := f.Tag.Lookup("json")
		name, opts, _ := strings.Cut(tag, ",")

		if f.Anonymous && !tagged && f.Type.Kind() == reflect.Struct {
			out = append(out, fields(f.Type)...)
			continue
		}
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, field{
			name:     name,
			typ:      f.Type,
			optional: strings.Contains(","+opts+",", ",omitempty,"),
		})
	}
	return out
}
