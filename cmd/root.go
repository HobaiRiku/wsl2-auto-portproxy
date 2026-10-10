package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/app"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/paths"
	buildinfo "github.com/HobaiRiku/wsl2-auto-portproxy/internal/version"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/windowsservice"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/config"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/service"
	"github.com/spf13/cobra"
)

type options struct {
	home    string
	listen  string
	devUI   bool
	pause   bool
	version string
}

func Execute(version string) error {
	o := &options{version: version}
	var showVersion bool
	root := &cobra.Command{Use: "wslpp", Short: "Windows to WSL TCP/UDP port proxy", SilenceUsage: true, SilenceErrors: true, Args: cobra.NoArgs}
	root.PersistentFlags().StringVar(&o.home, "home", "", "data root (or WSLPP_HOME)")
	root.PersistentFlags().StringVar(&o.listen, "listen", "127.0.0.1:47831", "loopback management address")
	root.PersistentFlags().BoolVar(&o.devUI, "ui-dev", false, "allow the loopback Vite development origin")
	root.PersistentFlags().BoolVar(&o.pause, "pause-on-exit", false, "wait for Enter before exiting (set for elevated windows)")
	root.PersistentFlags().MarkHidden("pause-on-exit")
	root.Flags().BoolVarP(&showVersion, "version", "v", false, "print version")
	run := func() error {
		options := app.Options{Home: o.home, Listen: o.listen, Version: version, DevUI: o.devUI}
		if !windowsservice.Interactive() {
			return windowsservice.Run(options)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return app.Run(ctx, options)
	}
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		}
		return run()
	}
	root.AddCommand(&cobra.Command{Use: "run", Short: "Run in the foreground", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { return run() }})
	root.AddCommand(&cobra.Command{Use: "version", Args: cobra.NoArgs, Run: func(cmd *cobra.Command, args []string) { fmt.Fprintln(cmd.OutOrStdout(), buildinfo.String()) }})
	root.AddCommand(&cobra.Command{Use: "status", Short: "Read live service and proxy status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := o.connect(cmd)
		if err != nil {
			return err
		}
		var body json.RawMessage
		if err := c.request("GET", "/status", nil, &body); err != nil {
			return err
		}
		return printJSON(cmd.OutOrStdout(), body)
	}})
	root.AddCommand(&cobra.Command{Use: "ui", Short: "Open the local web UI", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := o.connect(cmd)
		if err != nil {
			return err
		}
		link := c.endpoint + "/"
		if o.devUI {
			link = "http://127.0.0.1:5173/"
		}
		fmt.Fprintln(cmd.OutOrStdout(), link)
		return windowsservice.OpenBrowser(link)
	}})
	configCmd := &cobra.Command{Use: "config", Short: "Manage config through the running service"}
	configCmd.AddCommand(&cobra.Command{Use: "get", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		c, err := o.connect(cmd)
		if err != nil {
			return err
		}
		var doc struct {
			Config   json.RawMessage `json:"config"`
			Rejected *struct {
				Error string `json:"error"`
			} `json:"rejected"`
		}
		if err := c.request("GET", "/config", nil, &doc); err != nil {
			return err
		}
		if doc.Rejected != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: the config file is invalid and was not applied: %s\n", doc.Rejected.Error)
		}
		return printJSON(cmd.OutOrStdout(), doc.Config)
	}})
	configCmd.AddCommand(&cobra.Command{Use: "set <file|->", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, err := o.connect(cmd)
		if err != nil {
			return err
		}
		var doc struct {
			Revision string          `json:"revision"`
			Config   json.RawMessage `json:"config"`
		}
		if err := c.request("GET", "/config", nil, &doc); err != nil {
			return err
		}
		reader := cmd.InOrStdin()
		if args[0] != "-" {
			file, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer file.Close()
			reader = file
		}
		data, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		if err != nil {
			return err
		}
		if len(data) > 1<<20 {
			return errors.New("configuration exceeds 1 MiB")
		}
		parsed, err := config.Parse(data)
		if err != nil {
			return err
		}
		doc.Config, err = json.Marshal(parsed)
		if err != nil {
			return err
		}
		var saved json.RawMessage
		if err := c.request("PUT", "/config", doc, &saved); err != nil {
			return err
		}
		return printJSON(cmd.OutOrStdout(), saved)
	}})
	root.AddCommand(configCmd)
	var doctorDistro string
	doctor := &cobra.Command{Use: "doctor", Short: "Check the current Windows account and WSL visibility", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		current, err := user.Current()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		scanner := service.Scanner{}
		distros, listErr := scanner.Distros(ctx)
		state, scanErr := scanner.Scan(ctx, doctorDistro)
		if scanErr != nil {
			state.Error = scanErr.Error()
		} else if listErr != nil {
			state.Error = listErr.Error()
		}
		ports := make([]string, 0, len(state.Ports))
		for _, p := range state.Ports {
			ports = append(ports, fmt.Sprintf("%s/%d", p.Type, p.Port))
		}
		body, err := json.Marshal(struct {
			Account string           `json:"account"`
			SID     string           `json:"sid"`
			Distros []service.Distro `json:"distros"`
			WSL     service.Snapshot `json:"wsl"`
			Ports   []string         `json:"ports"`
		}{current.Username, current.Uid, distros, state, ports})
		if err != nil {
			return err
		}
		return printJSON(cmd.OutOrStdout(), body)
	}}
	doctor.Flags().StringVar(&doctorDistro, "distro", "", "WSL distribution to check (default: the WSL default distribution)")
	root.AddCommand(doctor)
	var account, ownerSID, importConfig string
	install := &cobra.Command{Use: "install", Short: "Install the Windows SCM service as the WSL owner account", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		current, err := user.Current()
		if err != nil {
			return err
		}
		if account == "" {
			account = current.Username
		}
		if ownerSID == "" {
			owner, err := user.Lookup(account)
			if err != nil {
				return err
			}
			ownerSID = owner.Uid
		}
		if importConfig == "" {
			home, err := paths.Resolve(o.home)
			if err != nil {
				return err
			}
			candidate := filepath.Join(home, "config.json")
			if _, err := os.Stat(candidate); err == nil {
				importConfig = candidate
			}
		}
		elevatedArgs := append([]string{}, os.Args[1:]...)
		elevatedArgs = append(elevatedArgs, "--account", account, "--owner-sid", ownerSID)
		if importConfig != "" {
			elevatedArgs = append(elevatedArgs, "--import-config", importConfig)
		}
		launched, err := windowsservice.Elevate(elevatedArgs)
		if err != nil || launched {
			return err
		}
		if err := windowsservice.Install(windowsservice.InstallOptions{Account: account, OwnerSID: ownerSID, ImportConfig: importConfig, Listen: o.listen}); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Installed. Run wslpp start, then verify wslpp status and wslpp doctor under the service account.")
		return nil
	}}
	install.Flags().StringVar(&account, "account", "", "Windows account that owns the WSL distribution")
	install.Flags().StringVar(&ownerSID, "owner-sid", "", "expected Windows owner SID (preserved across UAC)")
	install.Flags().MarkHidden("owner-sid")
	install.Flags().StringVar(&importConfig, "import-config", "", "import an existing config if the service has none")
	root.AddCommand(install)
	for _, action := range []string{"start", "stop", "restart", "uninstall", "update"} {
		root.AddCommand(&cobra.Command{Use: action, Short: action + " the Windows SCM service", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			launched, err := windowsservice.Elevate(os.Args[1:])
			if err != nil || launched {
				return err
			}
			return windowsservice.Control(cmd.Name())
		}})
	}
	err := root.Execute()
	if o.pause {
		if err != nil {
			fmt.Fprintln(os.Stderr, "\nFailed:", err)
		} else {
			fmt.Fprintln(os.Stderr, "\nDone.")
		}
		fmt.Fprint(os.Stderr, "Press Enter to close this window.")
		bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return err
}

type client struct {
	endpoint string
	http     *http.Client
}

// connect finds the running instance through the endpoint file in its data
// root, or uses --listen directly when it was given.
func (o *options) connect(cmd *cobra.Command) (*client, error) {
	if cmd.Flags().Changed("listen") {
		c, err := dial("http://" + o.listen)
		if err != nil {
			return nil, fmt.Errorf("no reachable wslpp at %s: %w", o.listen, err)
		}
		return c, nil
	}
	return connect(o.home)
}
func dial(address string) (*client, error) {
	if err := validateEndpoint(address); err != nil {
		return nil, err
	}
	c := &client{endpoint: address, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	var health json.RawMessage
	if err := c.request("GET", "/health", nil, &health); err != nil {
		return nil, err
	}
	return c, nil
}
func connect(home string) (*client, error) {
	resolved, err := paths.Resolve(home)
	if err != nil {
		return nil, err
	}
	candidates := []string{resolved}
	if home == "" && os.Getenv("WSLPP_HOME") == "" {
		if system := windowsservice.SystemHome(); system != "" {
			candidates = append(candidates, system)
		}
	}
	for _, candidate := range candidates {
		endpoint, err := os.ReadFile(filepath.Join(candidate, "endpoint"))
		if err != nil {
			continue
		}
		address := strings.TrimSpace(string(endpoint))
		if err := validateEndpoint(address); err != nil {
			return nil, err
		}
		if c, err := dial(address); err == nil {
			return c, nil
		}
	}
	return nil, errors.New("no reachable wslpp service; run wslpp start or wslpp run")
}
func validateEndpoint(address string) error {
	u, err := url.Parse(address)
	if err != nil {
		return err
	}
	host, port, err := net.SplitHostPort(u.Host)
	ip := net.ParseIP(host)
	number, e := strconv.Atoi(port)
	if err != nil || e != nil || number <= 0 || number > 65535 || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || ip == nil || !ip.IsLoopback() {
		return errors.New("invalid local endpoint")
	}
	return nil
}
func (c *client) request(method, path string, body, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequest(method, c.endpoint+"/api"+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		var problem struct {
			Error string `json:"error"`
		}
		json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&problem)
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, problem.Error)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out)
}
func printJSON(out io.Writer, data json.RawMessage) error {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, formatted.String())
	return err
}
