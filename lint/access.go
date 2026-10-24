package lint

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	descruntime "ocm.software/open-component-model/bindings/go/descriptor/runtime"
	"ocm.software/open-component-model/bindings/go/runtime"
)

// Access holds the access-spec fields rules look at, whatever the concrete type.
type Access struct {
	Type           string          `json:"type"` // without "/version"
	ImageReference string          `json:"imageReference"`
	Reference      string          `json:"reference"`
	MediaType      string          `json:"mediaType"`
	ReferenceName  string          `json:"referenceName"`
	GlobalAccess   json.RawMessage `json:"globalAccess"`
}

func AccessOf(a runtime.Typed) (Access, error) {
	// A nil *runtime.Raw inside the interface is not == nil, and marshalling it panics.
	if r, ok := a.(*runtime.Raw); a == nil || (ok && r == nil) {
		return Access{}, errors.New("resource has no access")
	}
	b, err := json.Marshal(a)
	if err != nil {
		return Access{}, fmt.Errorf("encode access: %w", err)
	}
	return decodeAccess(b)
}

func decodeAccess(b []byte) (Access, error) {
	var acc Access
	if err := json.Unmarshal(b, &acc); err != nil {
		return Access{}, fmt.Errorf("decode access: %w", err)
	}
	acc.Type, _, _ = strings.Cut(acc.Type, "/")
	return acc, nil
}

func (a Access) IsOCI() bool {
	switch a.Type {
	case "OCIImage", "ociArtifact", "ociRegistry", "ociImage":
		return true
	}
	return false
}

func (a Access) IsLocalBlob() bool { return a.Type == "LocalBlob" || a.Type == "localBlob" }

func (a Access) IsRelative() bool { return a.Type == "relativeOciReference" }

// HoldsOCIContent reports a local blob that carries an OCI manifest or index.
func (a Access) HoldsOCIContent() bool {
	return a.IsLocalBlob() &&
		(strings.HasPrefix(a.MediaType, "application/vnd.oci.image.manifest.v1") ||
			strings.HasPrefix(a.MediaType, "application/vnd.oci.image.index.v1"))
}

// Global returns a local blob's globalAccess, if it has a decodable one.
func (a Access) Global() (Access, bool) {
	if len(a.GlobalAccess) == 0 || string(a.GlobalAccess) == "null" {
		return Access{}, false
	}
	g, err := decodeAccess(a.GlobalAccess)
	return g, err == nil
}

// LabelString returns the named label's value when it is a JSON string.
func LabelString(labels []descruntime.Label, name string) (string, bool) {
	for _, l := range labels {
		if l.Name != name {
			continue
		}
		var s string
		if json.Unmarshal(l.Value, &s) == nil {
			return s, true
		}
		return "", false
	}
	return "", false
}
