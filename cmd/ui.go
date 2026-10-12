package cmd

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"agentup/internal/operation"
	"agentup/internal/service"
	"agentup/internal/webui"
)

var uiPort int
var uiNoOpen bool

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the local bilingual Web UI",
	Long: `Start a local Web UI for agent status, confirmed upgrades and diagnostics.

The server only listens on 127.0.0.1 and stops when this command exits.
Opening the UI never upgrades tools automatically.`,
	Args: cobra.NoArgs,
	RunE: runUI,
}

func runUI(cmd *cobra.Command, _ []string) error {
	if uiPort < 0 || uiPort > 65535 {
		return fmt.Errorf("--port must be between 0 and 65535")
	}
	release, err := operation.Acquire("ui")
	if err != nil {
		return fmt.Errorf("AgentUp UI is already running or AgentUp is being updated: %w", err)
	}
	defer release()
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", uiPort))
	if err != nil {
		return fmt.Errorf("start local UI: %w", err)
	}
	defer listener.Close()
	ctx := commandContext(cmd)
	s := service.New(ctx, Version, runtime.GOARCH, appRunner, appPlatform, appTools)
	defer s.Close()
	handler, err := webui.New(s, "http://"+listener.Addr().String())
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	_ = s.Refresh(outputFlags.Offline)
	p := printer(cmd)
	p.Heading("AgentUp Web UI", "local session")
	p.Line(handler.URL())
	p.Line("Press Ctrl+C to stop / Ctrl+C 停止服务")
	if !uiNoOpen {
		if err := openBrowser(handler.URL()); err != nil {
			notices(cmd).Notice("Browser could not be opened; use the URL above.")
		}
	}
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		_ = server.Close()
		<-done
		return nil
	}
}

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func init() {
	uiCmd.Flags().IntVar(&uiPort, "port", 0, "local port (0 selects an available port)")
	uiCmd.Flags().BoolVar(&uiNoOpen, "no-open", false, "do not open a browser automatically")
}
