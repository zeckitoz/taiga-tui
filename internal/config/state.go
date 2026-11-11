package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zeckitoz/taiga-tui/internal/atomicfile"
	"github.com/zeckitoz/taiga-tui/internal/domain"
)

// StateFileName is the name of the app-owned state file inside the config directory.
const StateFileName = "state.json"

// State is what the app remembers between runs. Only the app writes it.
type State struct {
	// UserID is the Taiga ID of the Team Member the state belongs to.
	UserID int64 `json:"user_id,omitempty"`
	// Username is the Taiga username, which names the stored tokens.
	Username string `json:"username,omitempty"`
	// Login is what the Team Member typed as username or email, used to
	// prefill the login dialog.
	Login string `json:"login,omitempty"`
	// FullName is the display name of the Team Member.
	FullName string `json:"full_name,omitempty"`
}

// LoggedIn returns the state after a successful login of m with the given
// login input. The state of a different Team Member is discarded.
func (s State) LoggedIn(m domain.TeamMember, login string) State {
	if s.UserID != m.ID {
		s = State{}
	}
	s.UserID = m.ID
	s.Username = m.Username
	s.Login = login
	s.FullName = m.FullName
	return s
}

// StateFile reads and writes state.json.
type StateFile struct{ path string }

// NewStateFile returns the state file in the config directory dir.
func NewStateFile(dir string) *StateFile {
	return &StateFile{path: filepath.Join(dir, StateFileName)}
}

// Load reads the state. A missing file yields the defaults; an unreadable or
// corrupt file yields the defaults and a warning.
func (f *StateFile) Load() (State, []Warning) {
	data, err := os.ReadFile(f.path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, []Warning{Warning(fmt.Sprintf("Couldn't read %s, using defaults: %v", StateFileName, err))}
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, []Warning{Warning(StateFileName + " is corrupt, using defaults")}
	}
	return st, nil
}

// Save writes the state atomically.
func (f *StateFile) Save(st State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(f.path, append(data, '\n'))
}
