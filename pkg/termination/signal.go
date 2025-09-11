package termination

import (
	"os"
	"os/signal"
	"syscall"
)

func Notify(sig ...os.Signal) <-chan os.Signal {
	if len(sig) == 0 {
		sig = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sig...)
	return ch
}
