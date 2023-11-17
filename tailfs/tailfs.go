package tailfs

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"
)

type Server struct {
	opts        *Opts
	smbConfPath string
	cmd         *exec.Cmd
	// Port is the port on 127.0.0.1 on which the Server is listening.
	Port int
}

type Opts struct {
	// StateDir is the base directory where TailFS will store SMB state.
	StateDir string
	// SMBDCommand is the full path to the smbd binary that TailFS will use to
	// server SMB shares.
	SMBDCommand string
}

func Start(opts *Opts) (*Server, error) {
	if err := os.MkdirAll(opts.StateDir, 0755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	s := &Server{
		opts:        opts,
		smbConfPath: filepath.Join(opts.StateDir, "smb.conf"),
	}

	if err := s.initSMBConfIfNecessary(); err != nil {
		return nil, fmt.Errorf("init smb.conf: %w", err)
	}

	err := s.start()
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	return s, nil
}

func (s *Server) initSMBConfIfNecessary() error {
	_, err := os.Stat(s.smbConfPath)
	if err == nil {
		// file exists, nothing to do
		return nil
	}
	if !os.IsNotExist(err) {
		// couldn't stat file for some other reason
		return fmt.Errorf("check %v exists: %w", s.smbConfPath, err)
	}

	// Need to create config
	file, err := os.OpenFile(s.smbConfPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("create %v: %w", s.smbConfPath, err)
	}
	defer file.Close()

	tmpl, err := template.New("smb.conf").Parse(smbConfTemplate)
	if err != nil {
		return fmt.Errorf("parse smb.conf template: %w", err)
	}

	ds := directorySettings()
	for _, d := range ds {
		d.AbsolutePath = filepath.Join(s.opts.StateDir, d.path)
		err = os.MkdirAll(d.AbsolutePath, d.perm)
		if err != nil {
			return fmt.Errorf("create %v: %w", d.AbsolutePath, err)
		}
	}

	if err := tmpl.Execute(file, ds); err != nil {
		return fmt.Errorf("execute smb.conf template: %w", err)
	}

	return nil
}

func (s *Server) start() error {
	// First find an open port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.Port = l.Addr().(*net.TCPAddr).Port
	l.Close()

	// Set up the command
	s.cmd = exec.Command(
		s.opts.SMBDCommand,
		fmt.Sprintf("--configfile=%s", s.smbConfPath),
		fmt.Sprintf("--port=%d", s.Port),
		"--foreground",
		"--no-process-group",
		"--debug-stdout",
	)

	// Redirect stdout and stderr to the current process
	stdOutPipe, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stdErrPipe, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	go func() {
		io.Copy(os.Stdout, stdOutPipe)
		stdOutPipe.Close()
	}()
	go func() {
		io.Copy(os.Stderr, stdErrPipe)
		stdErrPipe.Close()
	}()

	// Run smbd in a supervisor loop
	go func() {
		fmt.Println("Running smbd in a loop")
		for {
			if err := s.cmd.Run(); err != nil {
				fmt.Printf("smbd exited, will restart: %v\n", err)
				// TODO: use exponential backoff
				time.Sleep(1 * time.Second)
			}
		}
	}()

	return nil
}

type directorySetting struct {
	Setting      string
	AbsolutePath string
	path         string
	perm         fs.FileMode
}

func directorySettings() []*directorySetting {
	return []*directorySetting{
		{Setting: "state directory", path: "", perm: 0755},
		{Setting: "log file", path: "log", perm: 0755},
		{Setting: "pid directory", path: "pid", perm: 0755},
		{Setting: "lock directory", path: "private", perm: 0755},
		{Setting: "private dir", path: "private", perm: 0755},
		{Setting: "binddns dir", path: "bind-dns", perm: 0755},
		{Setting: "cache directory", path: "cache", perm: 0755},
		{Setting: "ncalrpc dir", path: "ncalrpc", perm: 0755},
		{Setting: "ntp signed socket directory", path: "ntp_signd", perm: 0755},
		{Setting: "usershare path", path: "usershares", perm: 0755},
		{Setting: "winbdd socket directory", path: "winbindd", perm: 0755},
	}
}

const smbConfTemplate = `
[global]
        server role     = standalone server
        interfaces      = 127.0.0.1
        registry shares = no
        config backend  = file
		log level       = 5
        {{ range . }}
        {{ .Setting }} = {{ .AbsolutePath }}
        {{ end }}
`
