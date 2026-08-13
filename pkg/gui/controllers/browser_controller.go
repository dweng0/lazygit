package controllers

import (
	"os"
	"path/filepath"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/gui/context"
	"github.com/jesseduffield/lazygit/pkg/gui/types"
)

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
	}
}

// enter descends into a directory. Files are handled in a later change.
func (self *BrowserController) enter(node *models.FileSystemNode) error {
	if node.IsDir {
		self.setCwd(node.Path)
	}

	return nil
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
