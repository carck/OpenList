package meta

import (
	"context"
	"net/http"
	stdpath "path"
	"strconv"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
	"github.com/OpenListTeam/OpenList/v4/internal/db"
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/fs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
)

type FileMeta struct {
	model.Storage
	Addition
}

func (d *FileMeta) Config() driver.Config {
	return config
}

func (d *FileMeta) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *FileMeta) Init(ctx context.Context) error {
	d.RemotePath = utils.FixAndCleanPath(d.RemotePath)
	return nil
}

func (d *FileMeta) Drop(ctx context.Context) error {
	return nil
}

func (Addition) GetRootPath() string {
	return "/"
}

func normalizePath(p string) string {
	return stdpath.Clean("/" + p)
}

func extractMtime(ctx context.Context, fallback time.Time) time.Time {
	if header, ok := ctx.Value(conf.RequestHeaderKey).(http.Header); ok {
		if v := header.Get("X-OC-Mtime"); v != "" {
			if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
				return time.Unix(ts, 0)
			}
		}
		if v := header.Get("X-Rclone-Mtime"); v != "" {
			if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
				return time.Unix(ts, 0)
			}
		}
	}
	return fallback
}

func (d *FileMeta) Get(ctx context.Context, path string) (model.Obj, error) {
	remoteStorage, remoteActualPath, err := op.GetStorageAndActualPath(d.RemotePath)
	if err != nil {
		return nil, err
	}
	remoteActualPath = stdpath.Join(remoteActualPath, path)

	// Try to get from remote storage first
	remoteObj, err := op.Get(ctx, remoteStorage, remoteActualPath)
	if err != nil {
		return nil, err
	}

	obj := &model.Object{
		Path:     path,
		Name:     remoteObj.GetName(),
		Size:     remoteObj.GetSize(),
		Modified: remoteObj.ModTime(),
		Ctime:    remoteObj.CreateTime(),
		IsFolder: remoteObj.IsDir(),
		HashInfo: remoteObj.GetHash(),
		Mask:     model.GetObjMask(remoteObj),
	}

	// Check if we have stored metadata
	if !remoteObj.IsDir() {
		if storedMeta, err := db.GetFileMeta(normalizePath(path), d.Storage.ID); err == nil {
			obj.Modified = time.Unix(storedMeta.ModTime, 0)
		}
	}

	return obj, nil
}

func (d *FileMeta) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	remoteStorage, remoteActualPath, err := op.GetStorageAndActualPath(d.RemotePath)
	if err != nil {
		return nil, err
	}
	remoteActualDir := stdpath.Join(remoteActualPath, dir.GetPath())
	remoteObjs, err := op.List(ctx, remoteStorage, remoteActualDir, args)
	if err != nil {
		return nil, err
	}

	// Collect file paths for bulk metadata query
	var filePaths []string
	for _, obj := range remoteObjs {
		if !obj.IsDir() {
			filePaths = append(filePaths, normalizePath(stdpath.Join(dir.GetPath(), obj.GetName())))
		}
	}

	// Bulk query metadata
	metaMap := make(map[string]int64)
	if len(filePaths) > 0 {
		if metas, err := db.GetFileMetas(filePaths, d.Storage.ID); err == nil {
			metaMap = metas
		}
	}

	result := make([]model.Obj, 0, len(remoteObjs))
	for _, obj := range remoteObjs {
		rawName := obj.GetName()
		if obj.IsDir() {
			result = append(result, &model.Object{
				Name:     rawName,
				Size:     obj.GetSize(),
				Modified: obj.ModTime(),
				Ctime:    obj.CreateTime(),
				IsFolder: true,
				Mask:     model.GetObjMask(obj),
			})
			continue
		}

		// Check for stored metadata
		objPath := normalizePath(stdpath.Join(dir.GetPath(), rawName))
		storedObj := &model.Object{
			Path:     objPath,
			Name:     rawName,
			Size:     obj.GetSize(),
			Modified: obj.ModTime(),
			Ctime:    obj.CreateTime(),
			IsFolder: false,
			HashInfo: obj.GetHash(),
			Mask:     model.GetObjMask(obj),
		}

		if storedModTime, exists := metaMap[objPath]; exists {
			storedObj.Modified = time.Unix(storedModTime, 0)
		}

		result = append(result, storedObj)
	}
	return result, nil
}

func (d *FileMeta) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	remoteStorage, remoteActualPath, err := op.GetStorageAndActualPath(d.RemotePath)
	if err != nil {
		return nil, err
	}
	remoteActualPath = stdpath.Join(remoteActualPath, file.GetPath())
	link, _, err := op.Link(ctx, remoteStorage, remoteActualPath, args)
	return link, err
}

func (d *FileMeta) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	path := stdpath.Join(d.RemotePath, parentDir.GetPath(), dirName)
	return fs.MakeDir(ctx, path)
}

func (d *FileMeta) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	src := stdpath.Join(d.RemotePath, srcObj.GetPath())
	dst := stdpath.Join(d.RemotePath, dstDir.GetPath())
	_, err := fs.Move(ctx, src, dst)
	if err != nil {
		return err
	}
	// Update metadata path
	oldPath := normalizePath(srcObj.GetPath())
	newPath := normalizePath(stdpath.Join(dstDir.GetPath(), srcObj.GetName()))
	return db.UpdateFileMetaPath(oldPath, newPath, d.Storage.ID)
}

func (d *FileMeta) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	oldPath := srcObj.GetPath()
	normalizedOldPath := normalizePath(oldPath)
	parentDir := stdpath.Dir(normalizedOldPath)
	newPath := stdpath.Join(parentDir, newName)

	err := fs.Rename(ctx, stdpath.Join(d.RemotePath, oldPath), newName)
	if err != nil {
		return err
	}
	// Update metadata path
	return db.UpdateFileMetaPath(normalizedOldPath, normalizePath(newPath), d.GetStorage().ID)
}

func (d *FileMeta) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	dst := stdpath.Join(d.RemotePath, dstDir.GetPath())
	src := stdpath.Join(d.RemotePath, srcObj.GetPath())
	_, err := fs.Copy(ctx, src, dst)
	if err != nil {
		return err
	}

	// Copy metadata to new location
	srcPath := normalizePath(srcObj.GetPath())
	dstPath := normalizePath(stdpath.Join(dstDir.GetPath(), srcObj.GetName()))
	return db.CopyFileMeta(srcPath, dstPath, d.Storage.ID)
}

func (d *FileMeta) Remove(ctx context.Context, obj model.Obj) error {
	err := fs.Remove(ctx, stdpath.Join(d.RemotePath, obj.GetPath()))
	if err != nil {
		return err
	}
	db.DeleteFileMeta(normalizePath(obj.GetPath()), d.Storage.ID)
	return nil
}

func (d *FileMeta) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	remoteStorage, remoteActualPath, err := op.GetStorageAndActualPath(d.RemotePath)
	if err != nil {
		return err
	}

	dstPath := stdpath.Join(remoteActualPath, dstDir.GetPath())

	// Put the file
	err = op.Put(ctx, remoteStorage, dstPath, file, up)
	if err != nil {
		return err
	}

	// Store metadata
	filePath := normalizePath(stdpath.Join(dstDir.GetPath(), file.GetName()))
	modTime := extractMtime(ctx, file.ModTime())
	meta := &model.FileMeta{
		Path:    filePath,
		Storage: d.Storage.ID,
		ModTime: modTime.Unix(),
	}
	db.CreateOrUpdateFileMeta(meta)

	return nil
}

func (d *FileMeta) GetDetails(ctx context.Context) (*model.StorageDetails, error) {
	remoteStorage, err := fs.GetStorage(d.RemotePath, &fs.GetStoragesArgs{})
	if err != nil {
		return nil, errs.NotImplement
	}
	remoteDetails, err := op.GetStorageDetails(ctx, remoteStorage)
	if err != nil {
		return nil, err
	}
	return &model.StorageDetails{
		DiskUsage: remoteDetails.DiskUsage,
	}, nil
}

var _ driver.Driver = (*FileMeta)(nil)
