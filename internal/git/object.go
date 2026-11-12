package git

// ObjectType is the type of a Git objet.
type ObjectType string

// A list of object types.
const (
	ObjectCommit ObjectType = "commit"
	ObjectTree   ObjectType = "tree"
	ObjectBlob   ObjectType = "blob"
	ObjectTag    ObjectType = "tag"
)
