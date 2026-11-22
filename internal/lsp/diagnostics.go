package lsp

import (
	"encoding/json"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// diagnosticSource は LSP の Diagnostic.source。
const diagnosticSource = "hozoc"

// LSP-1 は hozo.toml が見つからないときの warning（docs/lsp.md「プロジェクト」）。各 .hozo の 1 行 1 列に出す。
const (
	codeNoConfig    = "LSP-1"
	messageNoConfig = "hozo.toml が見つからない。設定の検査は行っていない"
)

// LSP-2 は、プラグインの check が、最後まで走らなかったときの warning（docs/lsp.md「診断」）。hozo.toml の 1 行 1 列に出す。
const (
	codePluginCheck    = "LSP-2"
	messagePluginCheck = "プラグインの check が、最後まで走らなかった（プラグインの診断は、出ていない）: "
)

// lspDiagnostics は、出す診断（diagnosticProject: 今の中身の、プラグインの check の答えがあれば、それ。なければ、直近の解析）を、
// URI ごとの LSP の診断にする（docs/lsp.md「診断」の表）。
// プロジェクトのすべてのファイル（語彙を含む）と hozo.toml は、診断がなくても空の列で入る（送るかどうかは publishAll が決める）。
func (s *server) lspDiagnostics() map[string][]protocol.Diagnostic {
	p := s.diagnosticProject()
	out := map[string][]protocol.Diagnostic{}
	for i := 0; i < p.Files.Len(); i++ {
		out[pathToURI(p.Root, p.Files.Path(source.FileID(i)))] = []protocol.Diagnostic{}
	}
	out[configURIOf(p)] = []protocol.Diagnostic{}
	if p.RunErr != nil {
		out[configURIOf(p)] = append(out[configURIOf(p)], protocol.Diagnostic{
			Severity: protocol.SeverityWarning,
			Code:     codePluginCheck,
			Source:   diagnosticSource,
			Message:  messagePluginCheck + p.RunErr.Error(),
		})
	}
	if p.NoConfig {
		for _, src := range p.Sources {
			uri := pathToURI(p.Root, src)
			out[uri] = append(out[uri], protocol.Diagnostic{
				Severity: protocol.SeverityWarning,
				Code:     codeNoConfig,
				Source:   diagnosticSource,
				Message:  messageNoConfig,
			})
		}
	}
	for i := range p.Diags {
		uri, d := s.toLSP(p, &p.Diags[i])
		out[uri] = append(out[uri], d)
	}
	return out
}

// toLSP は一件の診断を写す（docs/lsp.md「診断」）。Severity は Error → 1、Warning → 2。Span は range にそのまま
// （End は含まない）。位置のない診断はそのファイル（なければ hozo.toml）の 1 行 1 列、つまり range の零値。
func (s *server) toLSP(p *project.Project, d *diag.Diagnostic) (uri string, out protocol.Diagnostic) {
	out.Code = d.Code
	out.Message = d.Message
	out.Source = diagnosticSource
	switch d.Severity {
	case diag.Error:
		out.Severity = protocol.SeverityError
	case diag.Warning:
		out.Severity = protocol.SeverityWarning
	}
	switch {
	case d.Span != nil:
		uri = pathToURI(p.Root, p.Files.Path(d.Span.File))
		out.Range = s.enc.toRange(p.Files, *d.Span)
	case d.File != "":
		uri = pathToURI(p.Root, d.File)
	default:
		uri = configURIOf(p)
	}
	for _, r := range d.Related {
		out.RelatedInformation = append(out.RelatedInformation, protocol.DiagnosticRelatedInformation{
			Location: s.locationIn(p, r.Span),
			Message:  r.Message,
		})
	}
	if len(d.Data) > 0 {
		if b, err := json.Marshal(d.Data); err == nil {
			out.Data = b
		}
	}
	return uri, out
}
