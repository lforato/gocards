package i18n

// Key is the typed identifier for every user-facing string. Keys are
// grouped by screen/feature; the value is a stable short slug used as a
// last-resort fallback in T() when neither the current nor default map
// holds an entry (makes missing translations visible instead of blank).
type Key string

// App chrome — internal/tui/app.go
const (
	KeyAppSubtitle      Key = "app.subtitle"
	KeyTerminalTooSmall Key = "app.terminal_too_small"
	KeyTerminalSizeHint Key = "app.terminal_size_hint"
)

// Dashboard — internal/tui/screens/dashboard.go
const (
	KeyDashboardLoading       Key = "dashboard.loading"
	KeyErrorPrefix            Key = "common.error_prefix"
	KeyDashboardDeleteConfirm Key = "dashboard.delete_confirm"
	KeyStatStreak             Key = "dashboard.stat.streak"
	KeyStatReviewed           Key = "dashboard.stat.reviewed"
	KeyStatRetention          Key = "dashboard.stat.retention"
	KeyStatDueToday           Key = "dashboard.stat.due_today"
	KeyActionNewCards         Key = "dashboard.action.new_cards"
	KeyActionGenerateAI       Key = "dashboard.action.generate_ai"
	KeyActionSettings         Key = "dashboard.action.settings"
	KeyDashboardDeckCount     Key = "dashboard.deck_count"
	KeyDeckRowDue             Key = "dashboard.deck_row_due"
)

// Common confirm / lifecycle
const (
	KeyHelpYDelete    Key = "common.help.y_delete"
	KeyHelpNCancel    Key = "common.help.n_cancel"
	KeyHelpSelect     Key = "common.help.select"
	KeyHelpMove       Key = "common.help.move"
	KeyHelpOpen       Key = "common.help.open"
	KeyHelpPick       Key = "common.help.pick"
	KeyHelpCancel     Key = "common.help.cancel"
	KeyHelpStudy      Key = "common.help.study"
	KeyHelpDelete     Key = "common.help.delete"
	KeyHelpNew        Key = "common.help.new"
	KeyHelpAI         Key = "common.help.ai"
	KeyHelpSettings   Key = "common.help.settings"
	KeyHelpReload     Key = "common.help.reload"
	KeyHelpQuit       Key = "common.help.quit"
	KeyHelpBack       Key = "common.help.back"
	KeyHelpEnd        Key = "common.help.end"
	KeyHelpEdit       Key = "common.help.edit"
	KeyHelpNext       Key = "common.help.next"
	KeyHelpPrev       Key = "common.help.prev"
	KeyHelpSave       Key = "common.help.save"
	KeyHelpCycle      Key = "common.help.cycle"
	KeyHelpSubmit     Key = "common.help.submit"
	KeyHelpInsert     Key = "common.help.insert"
	KeyHelpNormal     Key = "common.help.normal"
	KeyHelpEscEnd     Key = "common.help.esc_end_from_normal"
	KeyHelpSend       Key = "common.help.send"
	KeyHelpScroll     Key = "common.help.scroll"
	KeyHelpAccept     Key = "common.help.accept"
	KeyHelpReject     Key = "common.help.reject"
	KeyHelpDiscard    Key = "common.help.discard"
	KeyHelpDeck       Key = "common.help.deck"
	KeyHelpBackNormal Key = "common.help.back_normal"
	KeyHelpTopBottom  Key = "common.help.top_bottom"
	KeyHelpRegen      Key = "common.help.regen"
)

// Create screen — internal/tui/screens/create.go
const (
	KeyCreateDeckNamePlaceholder Key = "create.deck_name_placeholder"
	KeyCreateDeckDescPlaceholder Key = "create.deck_desc_placeholder"
	KeyCreateDeckColorDefault    Key = "create.deck_color_default"
	KeyCreateValidationName      Key = "create.validation_name"
	KeyCreateFailedPrefix        Key = "create.failed_prefix"
	KeyCreatePickDeckTitle       Key = "create.pick_deck_title"
	KeyCreateNoDecks             Key = "create.no_decks"
	KeyCreateNewDeckAction       Key = "create.new_deck_action"
	KeyCreateDeckTitle           Key = "create.deck_title"
	KeyCreateFormName            Key = "create.form.name"
	KeyCreateFormDesc            Key = "create.form.description"
	KeyCreateFormColor           Key = "create.form.color"
	KeyCreatePickTypeTitle       Key = "create.pick_type_title"
)

// Edit — internal/tui/screens/edit.go
const (
	KeyEditCardTitle       Key = "edit.card_title"
	KeyEditNewCard         Key = "edit.new_card"
	KeyEditTypeLegend      Key = "edit.type_legend"
	KeyEditFieldType       Key = "edit.field.type"
	KeyEditFieldLanguage   Key = "edit.field.language"
	KeyEditFieldQuestion   Key = "edit.field.question"
	KeyEditFieldInitCode   Key = "edit.field.initial_code"
	KeyEditFieldExpected   Key = "edit.field.expected_answer"
	KeyEditFieldChoices    Key = "edit.field.choices"
	KeyEditFieldTemplate   Key = "edit.field.template"
	KeyEditTemplateHint    Key = "edit.template_hint"
	KeyEditMaxChoices      Key = "edit.max_choices"
	KeyEditNoChoicesHint   Key = "edit.no_choices_hint"
	KeyEditEmptyChoice     Key = "edit.empty_choice"
	KeyEditCardCreated     Key = "edit.card_created"
	KeyEditCardSaved       Key = "edit.card_saved"
	KeyEditSaveFailedPfx   Key = "edit.save_failed_prefix"
)

// Deck screen — internal/tui/screens/deck.go
const (
	KeyDeckNothingDue     Key = "deck.nothing_due"
	KeyDeckLoading        Key = "deck.loading"
	KeyDeckDeleteMany     Key = "deck.delete_many"
	KeyDeckDeleteOne      Key = "deck.delete_one"
	KeyDeckNoCardsYet     Key = "deck.no_cards_yet"
	KeyDeckSummary        Key = "deck.summary"
)

// Study — internal/tui/screens/study.go + study_*
const (
	KeyStudyLoadFailPfx    Key = "study.load_fail_prefix"
	KeyStudyCardDeleted    Key = "study.card_deleted"
	KeyStudyReviewSaveFail Key = "study.review_save_fail_prefix"
	KeyStudyLoading        Key = "study.loading"
	KeyStudyDeleteConfirm  Key = "study.delete_confirm"
	KeyStudyCardCounter    Key = "study.card_counter"
	KeyStudyNothingDue     Key = "study.nothing_due"
	KeyStudySessionDone    Key = "study.session_done"
	KeyStudyMCQNoChoices   Key = "study.mcq.no_choices"
	KeyStudyFillMalformed  Key = "study.fill.malformed"
	KeyStudyFillBlanks     Key = "study.fill.blanks"
	KeyStudyFillAnswers    Key = "study.fill.answers"
	KeyStudyGrading        Key = "study.grading"
	KeyStudyRethinking     Key = "study.rethinking"
	KeyStudyAnswerLabel    Key = "study.answer_label"
	KeyStudyExpLabel       Key = "study.explanation_label"
	KeyStudyMCQResult      Key = "study.mcq_result"
	KeyStudyMCQCorrect     Key = "study.mcq_correct"
	KeyStudyMCQIncorrect   Key = "study.mcq_incorrect"
	KeyStudyGradeLabel     Key = "study.grade_label"
	KeyStudyGraderNoGrade  Key = "study.grader_no_grade"
	KeyStudyGraderLabel    Key = "study.grader_label"
	KeyStudyUnknownType    Key = "study.unknown_type"
)

// Generate — internal/tui/screens/generate.go + generate_preview.go
const (
	KeyGenerateSavedPfx    Key = "generate.saved_prefix"
	KeyGenerateSavedFmt    Key = "generate.saved_format"
	KeyGenerateYouTag      Key = "generate.you_tag"
	KeyGenerateClaudeTag   Key = "generate.claude_tag"
	KeyGenerateAddingTo    Key = "generate.adding_to"
	KeyGenerateDeckSwitch  Key = "generate.deck_switch_hint"
	KeyGenerateWaiting     Key = "generate.waiting"
	KeyGenerateThinking    Key = "generate.thinking"
	KeyGenerateNoMoreCards Key = "generate.no_more_cards"
	KeyGenerateChangeDeck  Key = "generate.change_deck"
	KeyGenerateReviewTitle Key = "generate.review_title"
	KeyGenerateReviewInfo  Key = "generate.review_info"
	KeyGeneratePrevType    Key = "generate.preview.type"
	KeyGeneratePrevLang    Key = "generate.preview.lang"
	KeyGeneratePrevPrompt  Key = "generate.preview.prompt"
	KeyGeneratePrevChoices Key = "generate.preview.choices"
	KeyGeneratePrevTmpl    Key = "generate.preview.template"
	KeyGeneratePrevBlanks  Key = "generate.preview.blanks"
	KeyGeneratePrevExpect  Key = "generate.preview.expected_answer"
)

// Cheatsheet — internal/tui/screens/cheatsheet.go + ai/struggle tier labels
const (
	KeyCheatsheetSaveFailPfx Key = "cheatsheet.save_fail_prefix"
	KeyCheatsheetSaved       Key = "cheatsheet.saved"
	KeyCheatsheetNoCards     Key = "cheatsheet.no_cards"
	KeyCheatsheetAsking      Key = "cheatsheet.asking"
	KeyCheatsheetEmptyDeck   Key = "cheatsheet.empty_deck"
	KeyCheatsheetNoSheet     Key = "cheatsheet.no_sheet"
	KeyCheatsheetLoading     Key = "cheatsheet.loading"
	KeyCheatsheetTitleSuffix Key = "cheatsheet.title_suffix"
	KeyCheatsheetGenerating  Key = "cheatsheet.generating"
	KeyTierStruggling        Key = "tier.struggling"
	KeyTierShaky             Key = "tier.shaky"
	KeyTierSolid             Key = "tier.solid"
	KeyTierNew               Key = "tier.new"
)

// Settings — internal/tui/screens/settings.go
const (
	KeySettingsTitle      Key = "settings.title"
	KeySettingsDailyLimit Key = "settings.daily_limit"
	KeySettingsPrefLangs  Key = "settings.preferred_langs"
	KeySettingsAPIKey     Key = "settings.api_key"
	KeySettingsLanguage   Key = "settings.language"
	KeySettingsSaveFail   Key = "settings.save_fail_prefix"
	KeySettingsSaved      Key = "settings.saved"
)
