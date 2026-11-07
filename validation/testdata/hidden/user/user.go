// Package user declares types with Validate through what they embed and do not
// export, for package hidden to check.
package user

type status string

func (status) Validate() error { return nil }

type checker interface{ Validate() error }

type Status string

func (Status) Validate() error { return nil }

type hidden struct{ *Status }

// ByPointer has Validate through a pointer it does not export.
type ByPointer struct{ *status }

// ByInterface has Validate through an interface it does not export.
type ByInterface struct{ checker }

// Below has Validate through a pointer below a field it does not export.
type Below struct{ hidden }
