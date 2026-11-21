package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"homelab-manager/backend/internal/dockerproxy"

	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	listener, err := systemdListener()
	if err != nil {
		logger.Fatal("could not initialize socket-activated listener", zap.Error(err))
	}
	defer listener.Close()

	engineSocket := os.Getenv("HOMELAB_DOCKER_ENGINE_SOCKET")
	if engineSocket == "" {
		engineSocket = "/var/run/docker.sock"
	}
	proxy := dockerproxy.New(engineSocket)
	defer proxy.CloseIdleConnections()
	server := &http.Server{
		Handler:           proxy,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Error("Docker proxy stopped", zap.Error(err))
			stopped <- os.Interrupt
		}
	}()
	logger.Info("restricted Docker proxy listening")
	<-stopped
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Warn("Docker proxy shutdown failed", zap.Error(err))
	}
}

func systemdListener() (net.Listener, error) {
	listenPID := os.Getenv("LISTEN_PID")
	listenFds := os.Getenv("LISTEN_FDS")
	if listenPID == "" || listenFds == "" {
		return nil, fmt.Errorf("expected a systemd socket-activated listener")
	}
	pid, err := strconv.Atoi(listenPID)
	if err != nil || pid != os.Getpid() {
		return nil, fmt.Errorf("invalid systemd LISTEN_PID")
	}
	fds, err := strconv.Atoi(listenFds)
	if err != nil || fds != 1 {
		return nil, fmt.Errorf("expected exactly one systemd listener")
	}
	file := os.NewFile(3, "docker-proxy-listener")
	if file == nil {
		return nil, fmt.Errorf("systemd listener file descriptor is unavailable")
	}
	listener, err := net.FileListener(file)
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		_ = listener.Close()
		return nil, closeErr
	}
	return listener, nil
}
