package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/api"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/atomicfile"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/controller"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/logbuffer"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/paths"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/registry"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/web"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/service"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Options struct {
	Home    string
	Listen  string
	Version string
	DevUI   bool
	Scanner controller.Scanner
}

func Run(ctx context.Context, o Options) error {
	home, err := paths.Resolve(o.Home)
	if err != nil {
		return err
	}
	if err := paths.Ensure(home); err != nil {
		return err
	}
	release, err := paths.Lock(home)
	if err != nil {
		if endpoint, readErr := os.ReadFile(filepath.Join(home, "endpoint")); readErr == nil {
			return fmt.Errorf("wslpp is already running for %s; open %s/ (stop it first, or use --home for a separate instance)", home, strings.TrimSpace(string(endpoint)))
		}
		return err
	}
	defer release()
	if o.Listen == "" {
		o.Listen = "127.0.0.1:47831"
	}
	host, portText, err := net.SplitHostPort(o.Listen)
	ip := net.ParseIP(host)
	port, parseErr := strconv.Atoi(portText)
	if err != nil || parseErr != nil || ip == nil || !ip.IsLoopback() || port < 0 || port > 65535 {
		return errors.New("management listen must be a loopback IP and port")
	}
	// Reserve the API before starting any forwarding or touching runtime state.
	listener, err := net.Listen("tcp", o.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	o.Listen = listener.Addr().String()
	_, actualPort, _ := net.SplitHostPort(o.Listen)
	port, _ = strconv.Atoi(actualPort)
	// Earlier builds wrote an API token; the API no longer uses one.
	os.Remove(filepath.Join(home, "token"))
	rotating := &lumberjack.Logger{Filename: filepath.Join(home, "wslpp.log"), MaxSize: 10, MaxBackups: 3, MaxAge: 7}
	defer rotating.Close()
	buffer := &logbuffer.Buffer{}
	// File first: a service has no usable stderr, and MultiWriter stops at the
	// first writer that fails.
	logger := slog.New(logbuffer.Handler{Buffer: buffer, Next: slog.NewJSONHandler(io.MultiWriter(rotating, os.Stderr), nil)})
	r := registry.New(filepath.Join(home, "config.json"))
	scanner := o.Scanner
	var distros func(context.Context) ([]service.Distro, error)
	if scanner == nil {
		real := service.Scanner{}
		scanner, distros = real, real.Distros
	}
	c := controller.New(r, scanner, logger, port)
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); c.Run(runtimeCtx) }()
	defer func() { cancel(); <-done }()
	started := time.Now()
	router := api.New(api.Options{Registry: r, Controller: c, Logs: buffer, Version: o.Version, Started: started, Port: port, DevUI: o.DevUI, Web: web.Handler(), Distros: distros})
	server := &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	if err := atomicfile.Write(filepath.Join(home, "endpoint"), []byte("http://"+o.Listen), 0600); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(home, "endpoint"))
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-runtimeCtx.Done()
		shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := server.Shutdown(shutdownCtx); err != nil {
			server.Close()
		}
	}()
	logger.Info("wslpp running", "ui", "http://"+o.Listen+"/", "version", o.Version)
	err = server.Serve(listener)
	cancel()
	<-shutdownDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
