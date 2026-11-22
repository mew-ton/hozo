package gobase

import (
	"fmt"
	"path"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// Coverage は direction "coverage" の中身（docs/plugin-go.md「カバレッジを読む」）。カバレッジが go test -coverprofile の形で
// なければ Recognized が false。形の違う行があれば error。応答の Files は、要求の Files のうち、カバレッジに行があるもの
// （要求の順）。モジュールの名前は、要求の Outputs から取る（ファイルは、読まない）: ファイルのディレクトリか、その上にある dir の
// うち、いちばん深い出力の、オプション module。そういう出力がない、module がないファイルは、測られていないものとして、出さない。
func Coverage(req *plugin.CoverageRequest) (*plugin.CoverageResponse, error) {
	profile, recognized, err := golang.ReadCoverProfile(req.Coverage.Content)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", req.Coverage.Path, err)
	}
	if !recognized {
		return &plugin.CoverageResponse{Recognized: false}, nil
	}
	resp := &plugin.CoverageResponse{Recognized: true}
	for _, f := range req.Files {
		dir := path.Dir(f)
		module, found := moduleFor(req.Outputs, dir)
		if !found {
			continue
		}
		lines, measured := profile.Lines(golang.ImportPath(module, dir) + "/" + path.Base(f))
		if !measured {
			continue
		}
		resp.Files = append(resp.Files, plugin.CoveredFile{Path: f, Lines: lines})
	}
	return resp, nil
}

// moduleFor は、ディレクトリ dir（プロジェクトのルートからの相対パス）のファイルを出した出力の、モジュールの名前: outputs のうち、
// dir そのものか、その上にある Dir の、いちばん深いものの、オプション module。そういう出力がない、module がないなら false。
func moduleFor(outputs []ir.NamespaceConfig, dir string) (string, bool) {
	best := -1
	for i := range outputs {
		d := path.Clean(outputs[i].Dir)
		if d != "." && d != dir && !strings.HasPrefix(dir, d+"/") {
			continue
		}
		if best < 0 || len(d) > len(path.Clean(outputs[best].Dir)) {
			best = i
		}
	}
	if best < 0 {
		return "", false
	}
	module, err := golang.ModuleOf(outputs[best])
	return module, err == nil
}
