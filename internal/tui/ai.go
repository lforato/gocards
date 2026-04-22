package tui

import (
	"errors"

	"github.com/lforato/gocards/internal/ai"
	"github.com/lforato/gocards/internal/store"
)

// errNoAPIKey is returned by resolveAIClient when neither the env var nor the
// stored setting has an Anthropic API key. Callers show a hint to the user and
// fall back to manual grading where applicable.
var errNoAPIKey = errors.New("no API key configured — add one in Settings (s)")

// resolveAIClient centralizes the client-bootstrap dance shared by every screen
// that streams from Claude: read env → read setting → bail if missing.
func resolveAIClient(s *store.Store) (*ai.Client, error) {
	key := ai.ResolveAPIKey(func() (string, bool, error) {
		return s.GetSetting("apiKey")
	})
	if key == "" {
		return nil, errNoAPIKey
	}
	return ai.New(key), nil
}
