// Package gogenconf provides a versioned declarative configuration format and
// schema system with generated strongly typed Go bindings.
//
// The line-oriented document grammar is:
//
//	document = { blank | comment | version | entry | section }
//	version  = "config_version" integer newline
//	section  = "section" name newline { blank | comment | entry } "end" newline
//	entry    = identifier [ expression ] newline
//	comment  = "#" user-text | "##" schema-documentation
//
// Expressions use bare or Go-quoted literals, or identifier "(" arguments ")".
// Arguments are recursively parsed expressions separated by commas. Commas in
// a top-level bare value remain literal for compatibility; quote punctuation
// in arguments. Quoted strings can encode arbitrary bytes, including NUL.
// Comments occupy their own lines. Indentation is insignificant. Section names
// may have a space-separated instance suffix; schemas decide their meaning.
//
// Parse accepts unversioned documents as legacy v0. Schema version checks and
// comment ownership are version-aware: all v0 comments are user-owned, while
// versioned v1+ documents distinguish # user comments from ## managed docs.
// Legacy headings are escaped on upgrade so ownership survives a reload.
// Schema version checks and
// registered migrations are separate from syntax. Unknown fields/sections are
// ordinary nodes and survive formatting/editing. Duplicate fields/sections,
// nested sections and malformed delimiters fail rather than being discarded.
//
// Registry uses Go result types only for service dispatch, never reflection to
// populate application structures. codegen emits those structures, explicit
// typed resolution calls, and application-owned dependency-free native runtimes.
package gogenconf
