// Package wire encodes what the server sends the client, and it exists to make
// one bug impossible rather than to be caught once per field.
//
// Go marshals a nil slice as `null` and an empty one as `[]`. Nothing in a
// handler distinguishes the two: `out.Board = append(out.Board, …)` produces a
// nil slice when the loop does not run, `out.Wants = wants(asked)` produces one
// whenever its callee returns nothing, and both read as "no rows" everywhere in
// Go, because len(nil) is 0 and ranging over nil is fine. The difference only
// exists on the wire, where the client does `.length` on it and takes the
// React tree down.
//
// It was being defended against in two languages at once: an `X: []thing{}`
// initialiser in whichever DTO constructor someone remembered, and a `?? []` on
// whichever client field someone remembered. Sixty-two of the latter, against
// eighty nullable collections on the wire — which is the arithmetic of a rule
// that is written down rather than enforced. On 2026-08-15 the miss was
// `openingView.Experience`, twenty lines under a comment on its neighbour
// explaining this exact failure.
//
// So the rule moved into the encoder. Every collection leaving the server is
// `[]` or `{}`, never `null`, and internal/realtime's TypeScript generator
// states that in the client's types. A handler cannot forget, because there is
// nothing left for it to remember.
package wire

import (
	"encoding"
	"encoding/json"
	"io"
	"reflect"
)

// Marshal encodes v as the client's JSON: like json.Marshal, but no slice or
// map in the result is null.
func Marshal(v any) ([]byte, error) { return json.Marshal(Normalized(v)) }

// Encode writes Marshal's output to w, followed by a newline, matching
// json.Encoder so a caller can swap one for the other.
func Encode(w io.Writer, v any) error {
	body, err := Marshal(v)
	if err != nil {
		return err
	}
	if _, err = w.Write(append(body, '\n')); err != nil {
		return err
	}
	return nil
}

// Normalized returns a copy of v with every nil slice and nil map replaced by
// an empty one, for a caller that needs the value rather than its bytes — the
// realtime outbox, which stores the payload before anything sends it.
//
// It copies rather than filling in place. Replacing a nil slice with an empty
// one is invisible to every reader in Go, so mutating the caller's value would
// be harmless on its own; it stops being harmless the moment a payload shares
// memory with something another goroutine is reading, and that is a data race
// nobody would find. A payload is small and built to be sent, so the copy is
// not worth the argument.
func Normalized(v any) any {
	if v == nil {
		return nil
	}
	return normalize(reflect.ValueOf(v), 0).Interface()
}

// depth caps the recursion. Nothing legitimate nests this far, so reaching it
// means a cycle — which json.Marshal will refuse in a moment with a better
// error than a stack overflow.
const depth = 64

var (
	jsonMarshaler = reflect.TypeFor[json.Marshaler]()
	textMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
)

// normalize returns v with its collections made non-nil, all the way down.
func normalize(v reflect.Value, at int) reflect.Value {
	t := v.Type()

	// A type that writes its own JSON is opaque: its fields are not what goes
	// on the wire, and its unexported state would not survive the copy anyway.
	// time.Time is the one that matters here.
	if at > depth || t.Implements(jsonMarshaler) || t.Implements(textMarshaler) {
		return v
	}

	switch t.Kind() {
	case reflect.Pointer:
		// A nil pointer stays null. That is an absent *thing*, which the
		// generated types spell `T | null` and the client already guards with
		// `&&` — unlike a collection, it has no empty form to fall back to.
		if v.IsNil() {
			return v
		}
		out := reflect.New(t.Elem())
		out.Elem().Set(normalize(v.Elem(), at+1))
		return out

	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		out := reflect.New(t).Elem()
		out.Set(normalize(v.Elem(), at+1))
		return out

	case reflect.Slice:
		// []byte is a base64 string, not a collection, and json.RawMessage is
		// an already-encoded payload. Neither has an empty form to impose.
		if t.Elem().Kind() == reflect.Uint8 {
			return v
		}
		// MakeSlice is why the nil case needs no branch: length zero gives an
		// empty slice, which is exactly the repair.
		out := reflect.MakeSlice(t, v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(normalize(v.Index(i), at+1))
		}
		return out

	case reflect.Map:
		out := reflect.MakeMapWithSize(t, v.Len())
		for iter := v.MapRange(); iter.Next(); {
			out.SetMapIndex(iter.Key(), normalize(iter.Value(), at+1))
		}
		return out

	case reflect.Array:
		out := reflect.New(t).Elem()
		for i := range v.Len() {
			out.Index(i).Set(normalize(v.Index(i), at+1))
		}
		return out

	case reflect.Struct:
		out := reflect.New(t).Elem()
		for i := range t.NumField() {
			f := t.Field(i)
			// An unexported field is not on the wire and cannot be set. Losing
			// it is only safe because a type whose JSON depends on unexported
			// state has to implement json.Marshaler, which returned above.
			if !f.IsExported() || f.Tag.Get("json") == "-" {
				continue
			}
			out.Field(i).Set(normalize(v.Field(i), at+1))
		}
		return out

	default:
		return v
	}
}
