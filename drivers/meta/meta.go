package meta

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

type Addition struct {
	RemotePath string `json:"remote_path" required:"true"`
	// Custom header to extract mod time from (for reference, handled by WebDAV server)
	ModTimeHeader string `json:"mod_time_header" default:"X-OC-Mtime" help:"Header used by WebDAV server to extract mod time (X-OC-Mtime for NextCloud, etc.)"`
	// Rclone mod time header (for reference)
	RcloneModTimeHeader string `json:"rclone_mod_time_header" default:"X-Rclone-Mtime" help:"Rclone mod time header"`
}

var config = driver.Config{
	Name:        "File Meta",
	LocalSort:   true,
	OnlyProxy:   true,
	NoCache:     true,
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &FileMeta{}
	})
}