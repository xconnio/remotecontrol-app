package remotecontrol

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

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

// setConfigSection sets the top-level key of cfgDirectory's config.yml to value. Every
// other section stays as it is, including ones this app doesn't know about, like the
// profiles deskconn keeps there.
func setConfigSection(cfgDirectory, key string, value any) error {
	cfgPath := filepath.Join(cfgDirectory, "config.yml")

	var doc yaml.Node
	data, err := os.ReadFile(cfgPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.Kind == 0 { // no config.yml yet, or an empty one
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s isn't a mapping", cfgPath)
	}

	var valueNode yaml.Node
	if err := valueNode.Encode(value); err != nil {
		return err
	}
	replaced := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			root.Content[i+1] = &valueNode
			replaced = true
			break
		}
	}
	if !replaced {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &valueNode)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, out, 0600)
}
