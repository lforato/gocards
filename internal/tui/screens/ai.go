package screens

import (
	"errors"

	"github.com/lforato/gocards/internal/ai"
	"github.com/lforato/gocards/internal/store"
)

var errNoAPIKey = errors.New("no API key configured — add one in Settings (s)")

func resolveAIClient(s *store.Store) (*ai.Client, error) {
	key := ai.ResolveAPIKey(func() (string, bool, error) {
		return s.GetSetting("apiKey")
	})
	if key == "" {
		return nil, errNoAPIKey
	}
	return ai.New(key), nil
}
