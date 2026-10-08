package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	Home      string
	Listen    string
	Version   string
	LegacyNAT bool
	DevUI     bool
	Scanner   controller.Scanner
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
	token, err := loadToken(filepath.Join(home, "token"))
	if err != nil {
		return err
	}
	rotating := &lumberjack.Logger{Filename: filepath.Join(home, "wslpp.log"), MaxSize: 10, MaxBackups: 3, MaxAge: 7}
	defer rotating.Close()
	buffer := &logbuffer.Buffer{}
	logger := slog.New(logbuffer.Handler{Buffer: buffer, Next: slog.NewJSONHandler(io.MultiWriter(os.Stderr, rotating), nil)})
	r := registry.New(filepath.Join(home, "config.json"))
	scanner := o.Scanner
	if scanner == nil {
		scanner = service.Scanner{LegacyNAT: o.LegacyNAT}
	}
	c := controller.New(r, scanner, logger, port)
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); c.Run(runtimeCtx) }()
	defer func() { cancel(); <-done }()
	started := time.Now()
	router := api.New(api.Options{Registry: r, Controller: c, Logs: buffer, Token: token, Version: o.Version, Started: started, Port: port, DevUI: o.DevUI, Web: web.Handler()})
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
	logger.Info("wslpp running", "listen", o.Listen, "version", o.Version)
	err = server.Serve(listener)
	cancel()
	<-shutdownDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
func loadToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		token := strings.TrimSpace(string(data))
		decoded, e := hex.DecodeString(token)
		if e != nil || len(decoded) != 32 {
			return "", fmt.Errorf("invalid token file %s", path)
		}
		return token, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b[:])
	if err := atomicfile.Write(path, []byte(token), 0600); err != nil {
		return "", err
	}
	return token, nil
}
