package db

import (
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm/clause"
)

func GetFileMeta(path string, storage uint) (*model.FileMeta, error) {
	meta := model.FileMeta{Storage: storage, Path: path}
	if err := db.Where(meta).First(&meta).Error; err != nil {
		return nil, errors.Wrapf(err, "failed select file meta")
	}
	return &meta, nil
}

func CreateOrUpdateFileMeta(meta *model.FileMeta) error {
	return errors.WithStack(db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "storage"},
			{Name: "path"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"mod_time"}),
	}).Create(meta).Error)
}

func UpdateFileMetaPath(oldPath, newPath string, storage uint) error {
	return errors.WithStack(db.Model(&model.FileMeta{}).
		Where("storage = ? AND path = ?", storage, oldPath).
		Update("path", newPath).Error)
}

func DeleteFileMeta(path string, storage uint) error {
	return errors.WithStack(db.Where("path = ? AND storage = ?", path, storage).Delete(&model.FileMeta{}).Error)
}

func GetFileMetas(paths []string, storage uint) (map[string]int64, error) {
	var metas []model.FileMeta
	if err := db.Where("storage = ? AND path IN ?", storage, paths).Find(&metas).Error; err != nil {
		return nil, errors.Wrapf(err, "failed select file metas")
	}
	result := make(map[string]int64)
	for i := range metas {
		result[metas[i].Path] = metas[i].ModTime
	}
	return result, nil
}

func CopyFileMeta(srcPath, dstPath string, storage uint) error {
	return errors.WithStack(db.Exec(`
		INSERT INTO file_metas (storage, path, mod_time)
		SELECT storage, ?, mod_time
		FROM file_metas 
		WHERE storage = ? AND path = ?`,
		dstPath, storage, srcPath).Error)
}
