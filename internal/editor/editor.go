package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Theme is embedded below and passed to vim as an -S script.
const themeVimrc = `" gocards vim theme — loaded via -S
set number
set relativenumber
set expandtab
set shiftwidth=2
set softtabstop=2
set tabstop=2
set cursorline
set laststatus=2
syntax on
set background=dark
if has('termguicolors') | set termguicolors | endif
try | colorscheme habamax | catch | try | colorscheme desert | catch | endtry | endtry
hi Normal       ctermbg=NONE guibg=#0b0f14
hi CursorLine               guibg=#15202b
hi LineNr       ctermfg=241  guifg=#4b5563
hi CursorLineNr ctermfg=223  guifg=#f59e0b  cterm=bold gui=bold
hi Comment      ctermfg=244  guifg=#94a3b8  cterm=italic gui=italic
hi StatusLine   ctermfg=15   ctermbg=238    guifg=#e5e7eb guibg=#1f2937
hi StatusLineNC ctermfg=244  ctermbg=236    guifg=#9ca3af guibg=#111827
hi Visual                    guibg=#1e293b
hi Search       ctermfg=0    ctermbg=222    guifg=#0b0f14 guibg=#f59e0b
hi MatchParen   ctermfg=223  guifg=#fbbf24  cterm=bold gui=bold
hi Pmenu        ctermbg=236                 guibg=#111827
hi PmenuSel     ctermbg=238                 guibg=#1f2937
hi Statement    ctermfg=204  guifg=#fb7185
hi Identifier   ctermfg=75   guifg=#60a5fa
hi Function     ctermfg=222  guifg=#f59e0b
hi Type         ctermfg=114  guifg=#34d399
hi String       ctermfg=108  guifg=#a7f3d0
hi Constant     ctermfg=180  guifg=#fbbf24
hi Number       ctermfg=180  guifg=#fbbf24
hi PreProc      ctermfg=141  guifg=#c4b5fd
hi Special      ctermfg=203  guifg=#f87171
set showmatch
set incsearch
set ignorecase
set smartcase
" Save & quit with <leader>w / ZZ as usual; :wq also fine.
`

// writeTheme stores the theme vimrc in the gocards data dir and returns its path.
func writeTheme() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".gocards")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "theme.vimrc")
	if err := os.WriteFile(p, []byte(themeVimrc), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// langExt maps a card.Language to a filename extension for syntax highlighting.
func langExt(lang string) string {
	switch strings.ToLower(lang) {
	case "javascript", "js":
		return ".js"
	case "typescript", "ts":
		return ".ts"
	case "tsx":
		return ".tsx"
	case "jsx":
		return ".jsx"
	case "python", "py":
		return ".py"
	case "go", "golang":
		return ".go"
	case "rust", "rs":
		return ".rs"
	case "c":
		return ".c"
	case "cpp", "c++":
		return ".cpp"
	case "csharp", "cs", "c#":
		return ".cs"
	case "java":
		return ".java"
	case "ruby", "rb":
		return ".rb"
	case "sql":
		return ".sql"
	case "sh", "bash", "shell":
		return ".sh"
	case "html":
		return ".html"
	case "css":
		return ".css"
	case "json":
		return ".json"
	case "md", "markdown":
		return ".md"
	default:
		return ".txt"
	}
}

// OpenResult carries the edited content and any tempfile path back to the caller.
type OpenResult struct {
	Content string
	Err     error
}

// Open launches vim on a tempfile pre-populated with initial content.
// Returns a tea.Cmd — Bubble Tea will suspend the TUI while vim is running.
func Open(initial, language string, onDone func(OpenResult) tea.Msg) tea.Cmd {
	themePath, themeErr := writeTheme()

	tmp, err := os.CreateTemp("", "gocards-*"+langExt(language))
	if err != nil {
		return func() tea.Msg { return onDone(OpenResult{Err: err}) }
	}
	if _, err := tmp.WriteString(initial); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return func() tea.Msg { return onDone(OpenResult{Err: err}) }
	}
	tmp.Close()

	editorCmd := os.Getenv("EDITOR")
	if editorCmd == "" {
		editorCmd = "vim"
	}

	args := []string{}
	// Only pass -S theme if we're launching vim/nvim.
	base := filepath.Base(editorCmd)
	if themeErr == nil && (base == "vim" || base == "nvim" || base == "vi") {
		args = append(args, "-S", themePath)
	}
	args = append(args, tmp.Name())

	cmd := exec.Command(editorCmd, args...)

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer os.Remove(tmp.Name())
		if err != nil {
			return onDone(OpenResult{Err: fmt.Errorf("editor exited: %w", err)})
		}
		b, rerr := os.ReadFile(tmp.Name())
		if rerr != nil {
			return onDone(OpenResult{Err: rerr})
		}
		return onDone(OpenResult{Content: string(b)})
	})
}
