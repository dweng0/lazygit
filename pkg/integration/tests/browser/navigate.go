package browser

import (
	"github.com/jesseduffield/lazygit/pkg/config"
	. "github.com/jesseduffield/lazygit/pkg/integration/components"
)

var Navigate = NewIntegrationTest(NewIntegrationTestArgs{
	Description:  "Browse the working directory tree, descending into a directory and back out",
	ExtraCmdArgs: []string{},
	Skip:         false,
	SetupConfig:  func(config *config.AppConfig) {},
	SetupRepo: func(shell *Shell) {
		shell.CreateFile("subdir/nested.txt", "nested content")
		shell.CreateFile("toplevel.txt", "top-level content")
		shell.GitAddAll()
		shell.Commit("initial commit")
	},
	Run: func(t *TestDriver, keys config.KeybindingConfig) {
		t.Views().Browser().
			Focus().
			ContainsLines(
				// directories are listed first, then files, each sorted by name
				Contains("subdir/"),
				Contains("toplevel.txt"),
			).
			NavigateToLine(Contains("subdir/")).
			Press(keys.Browser.GoInto).
			ContainsLines(
				Contains("nested.txt"),
			).
			Press(keys.Browser.GoUp).
			ContainsLines(
				Contains("subdir/"),
				Contains("toplevel.txt"),
			)
	},
})
