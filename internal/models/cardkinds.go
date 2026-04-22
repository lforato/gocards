package models

// CardKind bundles the metadata every screen needs about a card type. To add
// a new card type, add a CardType constant, register a CardKind here, and
// extend the per-screen dispatch maps (screens.cardUI for color, study
// behavior table, edit field lists).
type CardKind struct {
	Type           CardType
	Name           string
	Description    string
	IsAIGraded     bool
	UsesCodeEditor bool
	UsesChoices    bool
	UsesBlanks     bool
}

var cardKinds = []CardKind{
	{Type: CardCode, Name: "code", Description: "write code to solve a problem",
		IsAIGraded: true, UsesCodeEditor: true},
	{Type: CardMCQ, Name: "mcq", Description: "multiple choice question",
		UsesChoices: true},
	{Type: CardFill, Name: "fill", Description: "fill in the blanks",
		UsesBlanks: true},
	{Type: CardExp, Name: "exp", Description: "annotate / explain a code snippet",
		IsAIGraded: true, UsesCodeEditor: true},
}

var cardKindByType = func() map[CardType]CardKind {
	m := make(map[CardType]CardKind, len(cardKinds))
	for _, k := range cardKinds {
		m[k.Type] = k
	}
	return m
}()

// Kind returns the registered metadata for t. Unknown types return the zero
// CardKind so callers can still switch on flags without panicking.
func Kind(t CardType) CardKind { return cardKindByType[t] }

// AllKinds returns the registered card types in their canonical order
// (matches menu ordering in the create screen).
func AllKinds() []CardKind { return cardKinds }

func IsKnownCardType(t CardType) bool {
	_, ok := cardKindByType[t]
	return ok
}
