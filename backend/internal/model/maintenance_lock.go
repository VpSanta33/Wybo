package model

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Every server shares this lock. Recovery requires exclusive ownership.
func DatabaseLock(dsn string, exclusive bool) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(dsn), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(dsn+".maintenance.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	if err = syscall.Flock(int(f.Fd()), mode|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("数据库正在使用，请先停止所有后端进程")
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
