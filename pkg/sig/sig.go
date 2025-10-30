package sig

import (
	"os"
	"os/signal"
	"syscall"
)

// WaitForTermination blocks until SIGINT or SIGTERM arrives.
func WaitForTermination() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
