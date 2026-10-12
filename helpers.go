package remotecontrol

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/xconnio/deskconn/common"
)

const extSVG = ".svg"

func fileURIToPath(raw string) string {
	if strings.HasPrefix(raw, "file://") {
		u, err := url.Parse(raw)
		if err == nil && u.Path != "" {
			return u.Path
		}
	}
	return raw
}

func imageMimeTypeByExt(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case common.ExtPng:
		return "image/png"
	case common.ExtWebp:
		return "image/webp"
	case common.ExtGif:
		return "image/gif"
	case common.ExtBmp:
		return "image/bmp"
	case extSVG:
		return "image/svg+xml"
	default:
		return "image/jpeg"
	}
}
