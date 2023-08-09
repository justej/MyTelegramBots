package logger

import "log"

func ForUser(u int64, msg string, err error) {
	log.Printf("user: %d; %s due to %v", u, msg, err)
}
