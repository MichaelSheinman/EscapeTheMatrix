package content

import (
	"bytes"
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// Decode reads one YAML file strictly: an unknown field is a failed build
// rather than an authored line nobody notices has no effect.
func Decode(files fs.FS, name string, into any) error {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := DecodeBytes(raw, into); err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return nil
}

// DecodeBytes decodes raw YAML strictly, for a loader that words its own
// read errors.
func DecodeBytes(raw []byte, into any) error {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	return dec.Decode(into)
}
