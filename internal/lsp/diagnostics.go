package lsp

import (
	"encoding/json"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/source"
)

// diagnosticSource は LSP の Diagnostic.source。
const diagnosticSource = "hozoc"

// LSP-1 は hozo.toml が見つからないときの warning（docs/lsp.md「プロジェクト」）。各 .hozo の 1 行 1 列に出す。
const (
	codeNoConfig    = "LSP-1"
	messageNoConfig = "hozo.toml が見つからない。設定の検査は行っていない"
)

// lspDiagnostics は直近の解析の診断を URI ごとの LSP の診断にする（docs/lsp.md「診断」の表）。
// プロジェクトのすべてのファイル（語彙を含む）と hozo.toml は、診断がなくても空の列で入る（送るかどうかは publishAll が決める）。
func (s *server) lspDiagnostics() map[string][]protocol.Diagnostic {
	p := s.proj
	out := map[string][]protocol.Diagnostic{}
	for i := 0; i < p.Files.Len(); i++ {
		out[pathToURI(p.Root, p.Files.Path(source.FileID(i)))] = []protocol.Diagnostic{}
	}
	out[s.configURI()] = []protocol.Diagnostic{}
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
		uri, d := s.toLSP(&p.Diags[i])
		out[uri] = append(out[uri], d)
	}
	return out
}

// toLSP は一件の診断を写す（docs/lsp.md「診断」）。Severity は Error → 1、Warning → 2。Span は range にそのまま
// （End は含まない）。位置のない診断はそのファイル（なければ hozo.toml）の 1 行 1 列、つまり range の零値。
func (s *server) toLSP(d *diag.Diagnostic) (uri string, out protocol.Diagnostic) {
	p := s.proj
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
		uri = s.configURI()
	}
	for _, r := range d.Related {
		out.RelatedInformation = append(out.RelatedInformation, protocol.DiagnosticRelatedInformation{
			Location: s.location(r.Span),
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
