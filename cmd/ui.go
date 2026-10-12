package cmd

import (
	"context"
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
	"agentup/internal/uisession"
	"agentup/internal/webui"
)

var uiPort int
var uiNoOpen bool

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the local bilingual Web UI",
	Long: `Start a local Web UI for agent status, confirmed upgrades and diagnostics.

The server only listens on 127.0.0.1 and stops when this command exits.
Each launch starts an independent instance on an available port.
Use "agentup ui status" to list instances and "agentup ui stop" to stop them.
Opening the UI never upgrades tools automatically.`,
	Args: cobra.NoArgs,
	RunE: runUI,
}

func runUI(cmd *cobra.Command, _ []string) error {
	if uiPort < 0 || uiPort > 65535 {
		return fmt.Errorf("--port must be between 0 and 65535")
	}
	release, err := operation.AcquireShared("ui")
	if err != nil {
		if errors.Is(err, operation.ErrBusy) {
			return fmt.Errorf("cannot start UI while AgentUp is being updated/uninstalled or a legacy UI is running; close the older UI with Ctrl+C and retry")
		}
		return err
	}
	var unregister func()
	defer func() {
		release()
		if unregister != nil {
			unregister()
		}
	}()
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", uiPort))
	if err != nil {
		return fmt.Errorf("cannot listen on 127.0.0.1:%d: %w; choose another --port or use agentup ui stop --port %d", uiPort, err, uiPort)
	}
	defer listener.Close()
	ctx := commandContext(cmd)
	s := service.New(ctx, Version, runtime.GOARCH, appRunner, appPlatform, appTools)
	defer s.Close()
	handler, err := webui.New(s, "http://"+listener.Addr().String())
	if err != nil {
		return err
	}
	_, unregister, err = uisession.Register(handler.URL(), Version)
	if err != nil {
		return fmt.Errorf("register local UI: %w", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	_ = s.Refresh(outputFlags.Offline)
	p := printer(cmd)
	p.Heading("AgentUp Web UI", "local session")
	p.Line(handler.URL())
	p.Line("Press Ctrl+C to stop / Ctrl+C 停止服务")
	p.Line(fmt.Sprintf("Stop this instance / 停止此实例: agentup ui stop --port %d", listener.Addr().(*net.TCPAddr).Port))
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
		_ = handler.Stop(true)
	case <-handler.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
	}
	<-done
	return nil
}

func runUIStatus(cmd *cobra.Command, _ []string) error {
	instances, err := uisession.List()
	if err != nil {
		return err
	}
	if outputFlags.JSON {
		return writeJSON(cmd, instances)
	}
	p := printer(cmd)
	p.Heading("AgentUp UI", fmt.Sprintf("%d running / 运行实例", len(instances)))
	for _, instance := range instances {
		p.Line(fmt.Sprintf("Port %d | PID %d | %s | %s", instance.Port, instance.PID, instance.Version, instance.StartedAt.Format("2006-01-02 15:04:05")))
		p.Line(instance.URL())
	}
	if len(instances) == 0 {
		p.Line("No managed UI instances / 没有运行中的 UI 实例")
	}
	return nil
}

func runUIStop(cmd *cobra.Command, port int, all, force bool) error {
	if port < 0 || port > 65535 || (port != 0 && all) {
		return fmt.Errorf("use --port 1..65535 or --all, not both")
	}
	instances, err := uisession.List()
	if err != nil {
		return err
	}
	if port != 0 {
		var target *uisession.Instance
		for i := range instances {
			if instances[i].Port == port {
				target = &instances[i]
				break
			}
		}
		if target == nil {
			return fmt.Errorf("no managed UI instance on port %d; use agentup ui status to list instances", port)
		}
		instances = []uisession.Instance{*target}
	} else if len(instances) > 1 && !all {
		return fmt.Errorf("%d UI instances are running; choose agentup ui stop --port <port> or agentup ui stop --all", len(instances))
	}
	result := struct {
		Stopped []int `json:"stopped"`
		Failed  []struct {
			Port  int    `json:"port"`
			Error string `json:"error"`
		} `json:"failed"`
	}{Stopped: []int{}}
	var failures []error
	for _, instance := range instances {
		if err := uisession.Stop(commandContext(cmd), instance, force); err != nil {
			failures = append(failures, fmt.Errorf("port %d: %w", instance.Port, err))
			result.Failed = append(result.Failed, struct {
				Port  int    `json:"port"`
				Error string `json:"error"`
			}{instance.Port, err.Error()})
		} else {
			result.Stopped = append(result.Stopped, instance.Port)
		}
	}
	if outputFlags.JSON {
		if err := writeJSON(cmd, result); err != nil {
			return err
		}
		if len(failures) != 0 {
			return reportedError{errors.Join(failures...).Error()}
		}
	} else {
		p := printer(cmd)
		for _, port := range result.Stopped {
			p.Line(fmt.Sprintf("Stopped UI on port %d / 服务已停止", port))
		}
		if len(instances) == 0 {
			p.Line("No managed UI instances / 没有运行中的 UI 实例")
		}
	}
	return errors.Join(failures...)
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
	uiCmd.AddCommand(&cobra.Command{
		Use: "status", Short: "List running local UI instances", Args: cobra.NoArgs, RunE: runUIStatus,
	})
	var port int
	var all, force bool
	stop := &cobra.Command{
		Use: "stop", Short: "Stop a local UI instance or all instances", Args: cobra.NoArgs,
		Long: `Stop the only running instance, a selected port, or all local UI instances.

An active upgrade prevents shutdown unless --force is supplied.
Forced shutdown cancels running commands; it does not roll back package changes.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("port") && port == 0 {
				return fmt.Errorf("--port must be between 1 and 65535")
			}
			return runUIStop(cmd, port, all, force)
		},
	}
	stop.Flags().IntVar(&port, "port", 0, "stop the instance listening on this port")
	stop.Flags().BoolVar(&all, "all", false, "stop all managed local UI instances")
	stop.Flags().BoolVar(&force, "force", false, "cancel any active upgrade and stop (no rollback)")
	uiCmd.AddCommand(stop)
}
