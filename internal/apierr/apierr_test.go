package apierr

import (
	"errors"
	"fmt"
	"testing"
)

var errInvalid = errors.New("invalid")

func TestBody(t *testing.T) {
	err := With(fmt.Errorf("%w: there is already a role named %q", errInvalid, "Revisor"), "role.name_taken", "name", "Revisor")
	if !errors.Is(err, errInvalid) {
		t.Fatal("With hides the wrapped error from errors.Is")
	}
	b := Body(fmt.Errorf("saving: %w", err))
	if b["code"] != "role.name_taken" || b["params"].(map[string]string)["name"] != "Revisor" {
		t.Fatalf("body: %v", b)
	}
	if b["error"] != `saving: invalid: there is already a role named "Revisor"` {
		t.Fatalf("message: %v", b["error"])
	}
	if b := Body(errInvalid); b["code"] != nil || b["error"] != "invalid" {
		t.Fatalf("plain error: %v", b)
	}
	if With(nil, "x") != nil {
		t.Fatal("With(nil) is not nil")
	}
}
