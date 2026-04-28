package model

type FileMeta struct {
	ID      uint   `json:"id" gorm:"primaryKey"`
	Storage uint   `json:"storage" gorm:"uniqueIndex:idx_storage_path"`
	Path    string `json:"path" gorm:"uniqueIndex:idx_storage_path"`
	ModTime int64  `json:"mod_time"`
}
