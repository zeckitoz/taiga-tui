// Package domain holds the glossary types shared by all other packages. It has
// no dependencies and knows nothing about Taiga's wire format.
package domain

// TeamMember is a person with their own Taiga account.
type TeamMember struct {
	ID       int64
	Username string
	// FullName is the name Taiga displays, falling back to the username.
	FullName string
}
