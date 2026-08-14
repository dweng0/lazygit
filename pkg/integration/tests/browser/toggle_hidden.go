package browser

import (
	"github.com/jesseduffield/lazygit/pkg/config"
	. "github.com/jesseduffield/lazygit/pkg/integration/components"
)

var ToggleHidden = NewIntegrationTest(NewIntegrationTestArgs{
	Description:  "Toggle whether dotfiles are shown in the browser tab",
	ExtraCmdArgs: []string{},
	Skip:         false,
	SetupConfig:  func(config *config.AppConfig) {},
	SetupRepo: func(shell *Shell) {
		shell.CreateFile(".hidden.txt", "secret")
		shell.CreateFile("visible.txt", "shown")
		shell.GitAddAll()
		shell.Commit("initial commit")
	},
	Run: func(t *TestDriver, keys config.KeybindingConfig) {
		t.Views().Browser().
			Focus().
			// dotfiles (including .git) are hidden by default
			Content(DoesNotContain(".hidden.txt")).
			Content(DoesNotContain(".git")).
			Press(keys.Browser.ToggleHidden).
			Content(Contains(".hidden.txt")).
			Content(Contains(".git")).
			Press(keys.Browser.ToggleHidden).
			Content(DoesNotContain(".hidden.txt"))
	},
})
