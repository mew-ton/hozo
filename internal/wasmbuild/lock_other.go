//go:build !unix

package wasmbuild

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// lock は、out のビルドの鍵を取る（OS の一時ディレクトリの、鍵のファイルを、作れた者が持つ。十分より古い鍵は、捨てる）。
func lock(out string) (unlock func(), err error) {
	sum := sha256.Sum256([]byte(out))
	path := filepath.Join(os.TempDir(), "hozo-wasmbuild-"+hex.EncodeToString(sum[:8])+".lock")
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > 10*time.Minute {
			_ = os.Remove(path)
			continue
		}
		time.Sleep(50 * time.Millisecond)
	}
}
