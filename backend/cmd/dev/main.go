// Command dev runs the local API and restarts it when the backend's code changes, so it never
// needs restarting by hand.
//
//	go run ./cmd/dev
//
// It builds ./cmd/api to one fixed file (bin/api, bin/api.exe on Windows) rather than a new
// temporary one per run like `go run`, so a firewall or antivirus prompt about it comes once, not
// on every restart. It watches the module's .go and .sql files (tests and testdata aside) by
// polling their modification times, which needs no extra dependency and behaves the same on every
// OS. A build that fails is printed and the running API keeps serving until the next change.
//
// The API reads its config from the environment as usual (`task dev:api` loads .env). Local
// development only: it is never deployed.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	pkg := flag.String("pkg", "./cmd/api", "package to build and run")
	every := flag.Duration("poll", 500*time.Millisecond, "how often to look for changes")
	flag.Parse()

	out := filepath.Join("bin", "api")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *pkg, out, *every); err != nil {
		fmt.Fprintln(os.Stderr, "dev:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, pkg, out string, every time.Duration) error {
	var api *proc
	restart := func() {
		if err := build(pkg, out); err != nil {
			logf("build failed, the running API is unchanged:\n%v", err)
			return
		}
		stopAPI(api)
		p, err := startAPI(out)
		if err != nil {
			logf("start: %v", err)
			api = nil
			return
		}
		api = p
	}

	last, err := snapshot(".")
	if err != nil {
		return err
	}
	restart()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			stopAPI(api)
			return nil
		case <-tick.C:
		}
		now, err := snapshot(".")
		if err != nil {
			logf("watch: %v", err)
			continue
		}
		changed := diff(last, now)
		if len(changed) == 0 {
			continue
		}
		// Let a burst of writes (an editor's save, a code generator) settle first.
		for {
			time.Sleep(every)
			settled, err := snapshot(".")
			if err != nil || len(diff(now, settled)) == 0 {
				break
			}
			now = settled
		}
		last = now
		logf("%s changed, rebuilding", describe(changed))
		restart()
	}
}

// fileState is what a change is noticed by.
type fileState struct {
	mod  time.Time
	size int64
}

// snapshot lists the watched files under root: .go and .sql files, tests, testdata and build
// output aside.
func snapshot(root string) (map[string]fileState, error) {
	files := map[string]fileState{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == "bin" || name == "dist" || name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		watched := (strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")) ||
			strings.HasSuffix(name, ".sql") || name == "go.mod" || name == "go.sum"
		if !watched {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files[path] = fileState{info.ModTime(), info.Size()}
		return nil
	})
	return files, err
}

// diff lists the files added, changed or removed between two snapshots.
func diff(before, after map[string]fileState) []string {
	var out []string
	for p, s := range after {
		if b, ok := before[p]; !ok || b != s {
			out = append(out, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, p)
		}
	}
	return out
}

func describe(files []string) string {
	if len(files) == 1 {
		return filepath.ToSlash(files[0])
	}
	return fmt.Sprintf("%d files", len(files))
}

// build compiles pkg next to out, then moves it over out once the old process is gone (Windows
// can't replace a running executable).
func build(pkg, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", out+".new", pkg)
	cmd.Stderr, cmd.Stdout = os.Stderr, os.Stdout
	return cmd.Run()
}

// proc is the running API.
type proc struct {
	cmd      *exec.Cmd
	done     chan struct{} // closed once it has exited
	stopping atomic.Bool   // set before stopAPI ends it, so its exit isn't reported as a crash
}

func startAPI(out string) (*proc, error) {
	if err := os.Rename(out+".new", out); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	p := &proc{cmd: exec.Command(abs), done: make(chan struct{})}
	p.cmd.Stdout, p.cmd.Stderr, p.cmd.Env = os.Stdout, os.Stderr, os.Environ()
	if err := p.cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		err := p.cmd.Wait()
		if !p.stopping.Load() {
			logf("api exited: %v (waiting for a change)", err)
		}
		close(p.done)
	}()
	return p, nil
}

// stopAPI ends the running API: an interrupt first so it shuts down cleanly (not on Windows,
// which has none to send), then a kill after a few seconds.
func stopAPI(p *proc) {
	if p == nil {
		return
	}
	p.stopping.Store(true)
	select {
	case <-p.done:
		return
	default:
	}
	if runtime.GOOS != "windows" {
		_ = p.cmd.Process.Signal(os.Interrupt)
		select {
		case <-p.done:
			return
		case <-time.After(5 * time.Second):
		}
	}
	_ = p.cmd.Process.Kill()
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		logf("the old API didn't exit; the new one may fail to bind")
	}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "dev: "+format+"\n", args...)
}
