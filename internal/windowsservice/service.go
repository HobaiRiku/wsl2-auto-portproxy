package windowsservice

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/app"
	kservice "github.com/kardianos/service"
)

const Name = "wslpp"

type Deployment struct {
	Home       string `json:"home"`
	Listen     string `json:"listen"`
	Account    string `json:"account"`
	OwnerSID   string `json:"ownerSID"`
	Executable string `json:"executable"`
}
type InstallOptions struct {
	Account      string
	OwnerSID     string
	ImportConfig string
	Listen       string
}
type program struct {
	options app.Options
	cancel  context.CancelFunc
	done    chan struct{}
	failure error
}

func (p *program) Start(s kservice.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		p.failure = app.Run(ctx, p.options)
		if p.failure != nil {
			if logger, err := s.SystemLogger(nil); err == nil {
				logger.Error(p.failure)
			}
			fmt.Fprintln(os.Stderr, p.failure)
			os.Exit(1)
		}
	}()
	return nil
}
func (p *program) Stop(kservice.Service) error {
	if p.cancel == nil {
		return nil
	}
	p.cancel()
	select {
	case <-p.done:
		return p.failure
	case <-time.After(20 * time.Second):
		return fmt.Errorf("service shutdown exceeded 20 seconds")
	}
}
func Interactive() bool { return kservice.Interactive() }
func Run(options app.Options) error {
	p := &program{options: options}
	s, err := kservice.New(p, &kservice.Config{Name: Name, DisplayName: "WSL Port Proxy"})
	if err != nil {
		return err
	}
	return s.Run()
}
