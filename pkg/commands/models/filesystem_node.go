package models

// FileSystemNode is a single entry (file or directory) in the working-directory
// browser. Unlike File, it is not derived from git status; it is read straight
// from the filesystem, so it can represent files git knows nothing about.
type FileSystemNode struct {
	Name  string
	Path  string
	IsDir bool
}

func (n *FileSystemNode) ID() string {
	return n.Path
}

func (n *FileSystemNode) URN() string {
	return n.Path
}
