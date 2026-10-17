// Package apierr gives the errors the web interface shows a stable code,
// which it translates into Portuguese. The message stays in English: agents,
// the CLI and the logs read it.
package apierr

import "errors"

// Error is an error with a code ("role.name_taken") and the values its
// translation uses ({"name": "Revisor"}). errors.Is and errors.As see
// through it to Err.
type Error struct {
	Code   string
	Params map[string]string
	Err    error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// With gives err a code, with params as key, value pairs. A nil err stays nil.
func With(err error, code string, params ...string) error {
	if err == nil {
		return nil
	}
	e := &Error{Code: code, Err: err}
	if len(params) > 1 {
		e.Params = make(map[string]string, len(params)/2)
		for i := 0; i+1 < len(params); i += 2 {
			e.Params[params[i]] = params[i+1]
		}
	}
	return e
}

// Body is the JSON of an error response: {"error": message}, plus "code"
// and "params" when err has a code.
func Body(err error) map[string]any {
	body := map[string]any{"error": err.Error()}
	var e *Error
	if errors.As(err, &e) {
		body["code"] = e.Code
		if len(e.Params) > 0 {
			body["params"] = e.Params
		}
	}
	return body
}
