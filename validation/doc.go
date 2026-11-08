// Package validation knows what a validate tag means: how kanna-validator reads
// one into its rules, which rules fit which types, and the checks generated
// code makes of a value. kanna-validator writes the checks into a Validate
// method; a generator that reads a struct from elsewhere writes the same checks
// where it builds the struct, so that a tag means one thing in both.
package validation
