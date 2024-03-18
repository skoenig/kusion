package storages

import (
	"errors"
)

const (
	metadataFile     = ".metadata.yml"
	workspaceTable   = "workspace"
	yamlSuffix       = ".yaml"
	defaultWorkspace = "default"
)

var (
	ErrWorkspaceNotExist     = errors.New("workspace does not exist")
	ErrWorkspaceAlreadyExist = errors.New("workspace has already existed")
)

// workspacesMetaData contains the name of current workspace and all workspaces, whose serialization
// result contains in the metadataFile for LocalStorage, OssStorage and S3Storage.
type workspacesMetaData struct {
	// The name of Current workspace.
	Current string `yaml:"current,omitempty" json:"current,omitempty"`

	// AvailableWorkspaces is the name list of all the existing workspaces.
	AvailableWorkspaces []string `yaml:"availableWorkspaces,omitempty" json:"availableWorkspaces,omitempty"`
}

// checkWorkspaceExistence returns the workspace exists or not.
func checkWorkspaceExistence(meta *workspacesMetaData, name string) bool {
	for _, ws := range meta.AvailableWorkspaces {
		if name == ws {
			return true
		}
	}
	return false
}

// addAvailableWorkspaces adds the workspace name to the available list, should be called if checkWorkspaceExistence
// returns false.
func addAvailableWorkspaces(meta *workspacesMetaData, name string) {
	meta.AvailableWorkspaces = append(meta.AvailableWorkspaces, name)
}

// removeAvailableWorkspaces deletes the workspace name from the available list.
func removeAvailableWorkspaces(meta *workspacesMetaData, name string) {
	for i, ws := range meta.AvailableWorkspaces {
		if name == ws {
			meta.AvailableWorkspaces = append(meta.AvailableWorkspaces[:i], meta.AvailableWorkspaces[i+1:]...)
		}
	}

	// if the current workspace is the removing one, set current to default.
	if meta.Current == name {
		meta.Current = defaultWorkspace
	}
}