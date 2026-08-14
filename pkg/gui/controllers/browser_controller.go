package controllers

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/gui/context"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

// browserPreviewMaxBytes caps how much of a file we read for the main-panel
// preview, so selecting a huge file doesn't read it all into memory.
const browserPreviewMaxBytes = 1024 * 1024

type BrowserController struct {
	baseController
	*ListControllerTrait[*models.FileSystemNode]
	c *ControllerCommon
}

var _ types.IController = &BrowserController{}

func NewBrowserController(
	c *ControllerCommon,
) *BrowserController {
	return &BrowserController{
		baseController: baseController{},
		ListControllerTrait: NewListControllerTrait(
			c,
			c.Contexts().Browser,
			c.Contexts().Browser.GetSelected,
			c.Contexts().Browser.GetSelectedItems,
		),
		c: c,
	}
}

func (self *BrowserController) GetKeybindings(opts types.KeybindingsOpts) []*types.Binding {
	return []*types.Binding{
		{
			Keys:              opts.GetKeys(opts.Config.Universal.GoInto),
			Handler:           self.withItem(self.enter),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.Enter,
			DisplayOnScreen:   true,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Browser.GoInto),
			Handler:           self.withItem(self.enter),
			GetDisabledReason: self.require(self.singleItemSelected()),
		},
		{
			Keys:        opts.GetKeys(opts.Config.Browser.GoUp),
			Handler:     self.goUp,
			Description: self.c.Tr.BrowserGoUp,
		},
		{
			Keys:              opts.GetKeys(opts.Config.Universal.OpenFile),
			Handler:           self.withItem(self.open),
			GetDisabledReason: self.require(self.singleItemSelected()),
			Description:       self.c.Tr.OpenFile,
			DisplayOnScreen:   true,
		},
		{
			Keys:            opts.GetKeys(opts.Config.Browser.ToggleHidden),
			Handler:         self.toggleHidden,
			Description:     self.c.Tr.BrowserToggleHidden,
			DisplayOnScreen: true,
		},
	}
}

// toggleHidden shows or hides dotfiles in the listing.
func (self *BrowserController) toggleHidden() error {
	self.context().ToggleShowHidden()
	self.c.PostRefreshUpdate(self.context())

	return nil
}

// enter descends into a directory, or opens a file in the configured editor.
func (self *BrowserController) enter(node *models.FileSystemNode) error {
	if node.IsDir {
		self.setCwd(node.Path)
		return nil
	}

	return self.c.Helpers().Files.EditFiles([]string{node.Path})
}

// open opens the selected node with the OS default application.
func (self *BrowserController) open(node *models.FileSystemNode) error {
	return self.c.Helpers().Files.OpenFile(node.Path)
}

// goUp moves to the parent of the current directory. At the filesystem root
// filepath.Dir is a no-op, so this stops there rather than looping.
func (self *BrowserController) goUp() error {
	self.setCwd(filepath.Dir(self.context().GetCwd()))

	return nil
}

func (self *BrowserController) setCwd(dir string) {
	self.context().SetCwd(dir)
	self.context().SetSelection(0)
	self.c.PostRefreshUpdate(self.context())
}

func (self *BrowserController) GetOnRenderToMain() func() {
	return func() {
		node := self.context().GetSelected()

		var content string
		if node == nil {
			content = ""
		} else if node.IsDir {
			content = self.directoryPreview(node.Path)
		} else {
			content = self.filePreview(node.Path)
		}

		self.c.RenderToMainViews(types.RefreshMainOpts{
			Pair: self.c.MainViewPairs().Normal,
			Main: &types.ViewUpdateOpts{
				Task: types.NewRenderStringWithoutScrollTask(content),
			},
		})
	}
}

// filePreview returns the file's contents up to browserPreviewMaxBytes, or a
// placeholder if it looks binary or can't be read.
func (self *BrowserController) filePreview(path string) string {
	file, err := os.Open(path)
	if err != nil {
		self.c.Log.Error(err)
		return ""
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, browserPreviewMaxBytes))
	if err != nil {
		self.c.Log.Error(err)
		return ""
	}

	if bytes.IndexByte(data, 0) != -1 {
		return self.c.Tr.BrowserBinaryFile
	}

	return string(data)
}

// directoryPreview lists the entries of a directory the way the browser lists
// the current one: directories first, each with a trailing slash.
func (self *BrowserController) directoryPreview(path string) string {
	entries, err := os.ReadDir(path)
	if err != nil {
		self.c.Log.Error(err)
		return ""
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !self.context().IsShowingHidden() && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}

	return strings.Join(names, "\n")
}

func (self *BrowserController) GetOnFocus() func(types.OnFocusOpts) {
	return func(types.OnFocusOpts) {
		// Seed the browser at the working directory the first time it's focused,
		// and re-read the current directory on every subsequent focus so changes
		// made outside lazygit show up when you tab back in.
		if self.context().GetCwd() == "" {
			dir, err := os.Getwd()
			if err != nil {
				self.c.Log.Error(err)
				return
			}
			self.context().SetCwd(dir)
		} else {
			self.context().Reload()
		}
		// Render directly rather than via PostRefreshUpdate: for the focused
		// view the latter re-runs HandleFocus, which would call back into here.
		self.context().HandleRender()
	}
}

func (self *BrowserController) context() *context.BrowserContext {
	return self.c.Contexts().Browser
}
