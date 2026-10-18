package domain

// CollectionDelete names the groups to dissolve and the smart folders to delete together; videos are never deleted.
type CollectionDelete struct {
	GroupIDs       []int64
	SmartFolderIDs []int64
}

// CollectionsDeleted counts distinct requested collections that existed when the deletion started.
type CollectionsDeleted struct {
	DissolvedGroups     int
	DeletedSmartFolders int
}
