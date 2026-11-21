package gobase

import (
	"errors"
	"fmt"
	"path"

	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// Coverage は direction "coverage" の中身（docs/plugin-go.md「カバレッジを読む」）。root はプロジェクトのルート
// （go.mod を探す起点）。カバレッジが go test -coverprofile の形でなければ Recognized が false。形の違う行があれば error。
// 応答の Files は、要求の Files のうち、カバレッジに行があるもの（要求の順）。go.mod が見つからないファイルは、測られて
// いないものとして、出さない。
func Coverage(req *plugin.CoverageRequest, root string) (*plugin.CoverageResponse, error) {
	profile, recognized, err := golang.ReadCoverProfile(req.Coverage.Content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", req.Coverage.Path, err)
	}
	if !recognized {
		return &plugin.CoverageResponse{Recognized: false}, nil
	}
	resp := &plugin.CoverageResponse{Recognized: true}
	for _, f := range req.Files {
		pkg, err := golang.ImportPath(root, path.Dir(f))
		if errors.Is(err, golang.ErrNoGoMod) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s の import パスを求められない: %w", f, err)
		}
		lines, measured := profile.Lines(pkg + "/" + path.Base(f))
		if !measured {
			continue
		}
		resp.Files = append(resp.Files, plugin.CoveredFile{Path: f, Lines: lines})
	}
	return resp, nil
}
