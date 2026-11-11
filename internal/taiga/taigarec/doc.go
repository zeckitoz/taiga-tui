// Package taigarec records responses of a live Taiga instance as test
// fixtures. Responses stay in memory until Save has anonymised them, so
// personal data never reaches the disk. Only tests import it.
package taigarec
