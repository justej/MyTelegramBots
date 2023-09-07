package log

import (
	"fmt"

	"go.uber.org/zap"
)

func Error(usr int64, err error, s string) {
	zap.L().Error(s, zap.Int64("usr", usr), zap.Error(err))
}

func Errorf(usr int64, err error, s string, args ...interface{}) {
	zap.L().Error(fmt.Sprintf(s, args...), zap.Int64("usr", usr), zap.Error(err))
}

func Warn(usr int64, s string) {
	zap.L().Warn(s, zap.Int64("usr", usr))
}

func Warnf(usr int64, s string, args ...interface{}) {
	zap.L().Warn(fmt.Sprintf(s, args...), zap.Int64("usr", usr))
}

func Info(s string) {
	zap.L().Info(s)
}

func Infof(s string, args ...interface{}) {
	zap.L().Info(fmt.Sprintf(s, args...))
}
