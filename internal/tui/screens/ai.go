package screens

import (
	"errors"

	"github.com/lforato/gocards/internal/ai"
	"github.com/lforato/gocards/internal/store"
)

// errNoAPIKey is returned by resolveAIClient when no Anthropic key is set
// via environment or DB. Surfaced to the user as a toast so they know to
// open Settings.
var errNoAPIKey = errors.New("no API key configured — add one in Settings (s)")

// resolveAIClient builds an AI client using ANTHROPIC_API_KEY if set, else
// the DB-stored apiKey setting. Returns errNoAPIKey when neither is set.
func resolveAIClient(s *store.Store) (*ai.Client, error) {
	key := ai.ResolveAPIKey(func() (string, bool, error) {
		return s.GetSetting(settingKeyAPIKey)
	})
	if key == "" {
		return nil, errNoAPIKey
	}
	return ai.New(key), nil
}
