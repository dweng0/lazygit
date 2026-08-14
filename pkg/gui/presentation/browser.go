package presentation

import (
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/gui/style"
	"github.com/jesseduffield/lazygit/pkg/theme"
	"github.com/samber/lo"
)

func GetFileSystemNodeListDisplayStrings(nodes []*models.FileSystemNode) [][]string {
	return lo.Map(nodes, func(node *models.FileSystemNode, _ int) []string {
		return []string{getFileSystemNodeDisplayString(node)}
	})
}

func getFileSystemNodeDisplayString(node *models.FileSystemNode) string {
	if node.IsDir {
		return style.FgBlue.Sprint(node.Name + "/")
	}
	return theme.DefaultTextColor.Sprint(node.Name)
}
