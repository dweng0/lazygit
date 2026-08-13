package context

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/gui/presentation"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

type BrowserContext struct {
	*FilteredListViewModel[*models.FileSystemNode]
	*ListContextTrait

	// cwd is the directory currently being browsed. entries is the cached
	// listing of cwd; it is only re-read when cwd changes or Reload is called,
	// so that the several getModel() calls per keystroke don't each hit the
	// filesystem.
	cwd     string
	entries []*models.FileSystemNode
}

var _ types.IListContext = (*BrowserContext)(nil)

func NewBrowserContext(c *ContextCommon) *BrowserContext {
	ctx := &BrowserContext{}

	viewModel := NewFilteredListViewModel(
		func() []*models.FileSystemNode { return ctx.entries },
		func(node *models.FileSystemNode) []string {
			return []string{node.Name}
		},
	)

	getDisplayStrings := func(_ int, _ int) [][]string {
		return presentation.GetFileSystemNodeListDisplayStrings(viewModel.GetItems())
	}

	ctx.FilteredListViewModel = viewModel
	ctx.ListContextTrait = &ListContextTrait{
		Context: NewSimpleContext(NewBaseContext(NewBaseContextOpts{
			View:       c.Views().Browser,
			WindowName: "files",
			Key:        BROWSER_CONTEXT_KEY,
			Kind:       types.SIDE_CONTEXT,
			Focusable:  true,
		})),
		ListRenderer: ListRenderer{
			list:              viewModel,
			getDisplayStrings: getDisplayStrings,
		},
		c: c,
	}

	return ctx
}

func (self *BrowserContext) GetCwd() string {
	return self.cwd
}

// SetCwd points the browser at dir and reloads its listing. Passing the same
// dir still reloads, so it doubles as a refresh.
func (self *BrowserContext) SetCwd(dir string) {
	self.cwd = dir
	self.Reload()
}

// Reload re-reads the current directory from disk into the cached listing,
// keeping any active filter applied to the fresh entries.
func (self *BrowserContext) Reload() {
	self.entries = readDir(self.cwd)
	self.ReApplyFilter(self.c.UserConfig().Gui.UseFuzzySearch())
}

// readDir lists dir with directories first, each group sorted by name. A
// directory that can't be read yields an empty listing rather than an error, so
// browsing into an unreadable directory just shows nothing.
func readDir(dir string) []*models.FileSystemNode {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	nodes := make([]*models.FileSystemNode, 0, len(dirEntries))
	for _, entry := range dirEntries {
		nodes = append(nodes, &models.FileSystemNode{
			Name:  entry.Name(),
			Path:  filepath.Join(dir, entry.Name()),
			IsDir: entry.IsDir(),
		})
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return nodes[i].Name < nodes[j].Name
	})

	return nodes
}
