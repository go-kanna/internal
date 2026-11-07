// Package hidden checks fields whose Validate is promoted through what may be
// nil, which it cannot test.
package hidden

import "github.com/go-kanna/internal/validation/testdata/hidden/user"

//kanna:validate
type Request struct {
	Pointer   user.ByPointer
	Interface user.ByInterface
	Below     user.Below
	Elements  []user.ByPointer
}
