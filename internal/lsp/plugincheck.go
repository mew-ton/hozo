package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/lsp/protocol"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// docs/lsp.md「プラグインの診断」: プラグインの check（docs/cli.md「検査」の 4）は、解析とは分けて、裏で走らせる。
// 状態（pluginChecks）に触るのは、run の goroutine だけ。裏の goroutine は、project.LoadWith を一度呼び、答えを results に
// 置いて終わる。

// pluginChecks は、裏で走らせる、プラグインの check の状態。
type pluginChecks struct {
	// allowed は、クライアントが、プラグインを起動してよい、と言っているか（docs/lsp.md「プラグインの診断」の「許し」）。
	// 既定は、偽: どのきっかけでも、起動しない。
	allowed bool
	// content は、直近の解析の中身（fingerprint）。
	content string
	// answer は、最後に終わった check の答え（プラグインの check まで済んだ解析）。answerOf は、その中身。出すのは、
	// answerOf が content と同じ間だけ。まだなければ nil。
	answer   *project.Project
	answerOf string

	// running は、裏の check が走っているか。runningOf は、それを求めた中身。cancel は、それを、やめさせる。
	running   bool
	runningOf string
	cancel    context.CancelFunc
	// queued は、走っている check が終わったら、始めるもの（最後に求めた一つ）。なければ nil。
	queued *checkRequest

	// results は、裏の goroutine が、答えを置く所。quit は、サーバーが終わったことを、裏の goroutine に伝える。
	results chan checkResult
	quit    chan struct{}
}

// checkRequest は、裏の check 一つの入力: それを求めた中身と、そのときの、開いている文書の内容。
type checkRequest struct {
	content string
	overlay map[string][]byte
}

// checkResult は、裏の check 一つの答え。content は、proj が、実際に読んだ中身。canceled は、やめさせたもの（答えは、使わない）。
type checkResult struct {
	proj     *project.Project
	content  string
	canceled bool
}

// fingerprint は、解析 p が読んだ中身（docs/lsp.md「プラグインの診断」の「中身」）: すべてのファイル（ユーザーのファイル、
// 語彙のファイル）と、hozo.toml の内容。二つの解析の中身が同じなら、診断の位置も、同じ。
func fingerprint(p *project.Project) string {
	h := sha256.New()
	put := func(name string, content []byte) {
		h.Write([]byte(strconv.Itoa(len(name)) + ":" + name + ":" + strconv.Itoa(len(content)) + ":"))
		h.Write(content)
	}
	for i := 0; i < p.Files.Len(); i++ {
		id := source.FileID(i)
		put(p.Files.Path(id), p.Files.Content(id))
	}
	if p.Config != nil && p.Config.Path != "" {
		if b, err := os.ReadFile(p.Config.Path); err == nil {
			put("hozo.toml", b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// diagnosticProject は、診断を出す解析: 今の中身の、プラグインの check の答えがあれば、それ（hozoc 自身の診断に、プラグインの
// 診断を足したもの）。なければ、直近の解析（hozoc 自身の診断だけ）。
func (s *server) diagnosticProject() *project.Project {
	if c := &s.checks; c.answer != nil && c.answerOf == c.content {
		return c.answer
	}
	return s.proj
}

// contentChanged は、解析し直した後に、その中身を覚える。走っている check が、別の中身のものなら、やめさせる
// （その答えは、今の中身のものでない）。待たせている check も、別の中身のものなら、捨てる。
func (s *server) contentChanged(content string) {
	c := &s.checks
	c.content = content
	if c.running && c.runningOf != content {
		c.cancel()
	}
	if c.queued != nil && c.queued.content != content {
		c.queued = nil
	}
}

// needsPluginChecks は、直近の解析で、プラグインの check を走らせる意味があるか（docs/lsp.md「プラグインの診断」の
// 「走らせないとき」）: hozo.toml があり、hozoc 自身の診断に error がなく、起動する相手（受け持つ出力のある、使うプラグイン）がある。
func (s *server) needsPluginChecks() bool {
	p := s.proj
	if !s.checks.allowed || p == nil || p.NoConfig {
		return false
	}
	if errs, _ := diag.Count(p.Diags); errs > 0 {
		return false
	}
	return p.NeedsPlugins()
}

// requestPluginChecks は、今の中身について、プラグインの check を求める（docs/lsp.md「プラグインの診断」の「きっかけ」）。
// force（didSave）なら、今の中身の答えを、もう持っていても、走らせ直す。同じ中身の check が、走っているか、待っていれば、
// それを待つ。別の中身の check が走っていれば、それが終わってから、始める。
func (s *server) requestPluginChecks(force bool) {
	c := &s.checks
	if s.shutdown || !s.needsPluginChecks() {
		return
	}
	if c.running && c.runningOf == c.content {
		return
	}
	if c.queued != nil && c.queued.content == c.content {
		return
	}
	if !force && c.answer != nil && c.answerOf == c.content {
		return
	}
	overlay := make(map[string][]byte, len(s.overlay))
	for k, v := range s.overlay {
		overlay[k] = v
	}
	req := &checkRequest{content: c.content, overlay: overlay}
	if c.running {
		c.cancel() // contentChanged が、もう、やめさせている。ここは、念のため
		c.queued = req
		return
	}
	s.startPluginChecks(req)
}

// startPluginChecks は、裏の check を、一つ始める。
func (s *server) startPluginChecks(req *checkRequest) {
	c := &s.checks
	ctx, cancel := context.WithCancel(context.Background())
	c.running, c.runningOf, c.cancel = true, req.content, cancel
	s.log.Printf("plugin check: start")
	opts := s.loadOptions(req.overlay, ctx)
	results, quit := c.results, c.quit
	go func() {
		p := project.LoadWith(opts)
		r := checkResult{proj: p, content: fingerprint(p), canceled: ctx.Err() != nil}
		select {
		case results <- r:
		case <-quit:
		}
	}()
}

// pluginCheckDone は、裏の check の答えを受け取る。やめさせたものでなければ、答えを覚え、今の中身のものなら、診断を出し直す
// （古い答えは、覚えるだけで、出さない）。待たせている check があれば、始める。
func (s *server) pluginCheckDone(r checkResult) {
	c := &s.checks
	c.running, c.runningOf = false, ""
	c.cancel()
	c.cancel = nil
	if r.canceled {
		s.log.Printf("plugin check: canceled")
	} else if !c.allowed {
		// 走り終えた後に、許しがなくなった: 答えは、使わない
		s.log.Printf("plugin check: 許されていないので、答えを捨てる")
	} else {
		if r.proj.RunErr != nil {
			s.log.Printf("plugin check: %v", r.proj.RunErr)
		}
		s.log.Printf("plugin check: done, %d diagnostics", len(r.proj.Diags))
		c.answer, c.answerOf = r.proj, r.content
		if r.content == c.content && s.initialized && !s.shutdown {
			s.publishAll()
		}
	}
	if req := c.queued; req != nil {
		c.queued = nil
		if req.content == c.content && !s.shutdown {
			s.startPluginChecks(req)
		}
	}
}

// stopPluginChecks は、終わるときに、走っている check を、やめさせて、それが止まるのを待つ（exitWait まで）。待つのは、
// プラグインが、プロジェクトのディレクトリで、動いているため。
func (s *server) stopPluginChecks() {
	c := &s.checks
	c.queued = nil
	if c.running {
		c.cancel()
		select {
		case <-c.results:
		case <-time.After(exitWait):
			s.log.Printf("plugin check: 止まるのを、待ちきれなかった")
		}
		c.running = false
	}
	close(c.quit)
}

// pluginChecksOption は、JSON のオブジェクト raw の、項目 pluginChecks（真偽値）を読む（docs/lsp.md「プラグインの診断」の
// 「許し」）。項目がない、真偽値でない、raw がオブジェクトでない、は ok が偽。
func pluginChecksOption(raw json.RawMessage) (allowed, ok bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return false, false
	}
	v, has := obj["pluginChecks"]
	if !has {
		return false, false
	}
	if err := json.Unmarshal(v, &allowed); err != nil || string(v) == "null" {
		return false, false
	}
	return allowed, true
}

// setPluginChecksAllowed は、許しを変える。与えられたら、今の中身について、プラグインの check を求める。なくなったら、
// 走っている check を、やめさせ、待たせている check と、覚えている答えを、捨てて、診断を、hozoc 自身のものだけで、出し直す。
func (s *server) setPluginChecksAllowed(allowed bool) {
	c := &s.checks
	if c.allowed == allowed {
		return
	}
	c.allowed = allowed
	s.log.Printf("plugin check: allowed=%v", allowed)
	if allowed {
		if s.proj != nil {
			s.requestPluginChecks(false)
		}
		return
	}
	if c.running {
		c.cancel()
	}
	c.queued = nil
	c.answer, c.answerOf = nil, ""
	if s.proj != nil && !s.shutdown {
		s.publishAll()
	}
}

// didChangeConfiguration は、settings.hozo.pluginChecks が、真偽値なら、許しを、その値に変える。なければ、変えない。
func (s *server) didChangeConfiguration(p protocol.DidChangeConfigurationParams) {
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(p.Settings, &settings); err != nil || settings == nil {
		return
	}
	if allowed, ok := pluginChecksOption(settings["hozo"]); ok {
		s.setPluginChecksAllowed(allowed)
	}
}
