//go:build unix

package wasmbuild

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"syscall"
)

// lock は、out のビルドの鍵を取る（OS の一時ディレクトリの、鍵のファイルの flock。プロセスが終われば、外れる）。
func lock(out string) (unlock func(), err error) {
	sum := sha256.Sum256([]byte(out))
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "hozo-wasmbuild-"+hex.EncodeToString(sum[:8])+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
