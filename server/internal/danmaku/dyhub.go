package danmaku

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	dyhubMu      sync.Mutex
	dyhubProc    *exec.Cmd
	dyhubRepo    string
	dyhubHTTPCli     = &http.Client{Timeout: 5 * time.Second}
	dyhubDownloadCli = &http.Client{Timeout: 3 * time.Minute}
)

const defaultDyhubRelay = "http://127.0.0.1:8757"

func dyhubInstallDir(repoRoot string) string {
	if dir := strings.TrimSpace(os.Getenv("CYBERVERSE_DYHUB_DIR")); dir != "" {
		return dir
	}
	return filepath.Join(repoRoot, "runtime", "dyhub")
}

func relayStatsURL(relayBase string) (string, error) {
	base := strings.TrimSpace(relayBase)
	if base == "" {
		base = defaultDyhubRelay
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		u, err = url.Parse("http://" + base)
		if err != nil {
			return "", err
		}
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/api/stats"
	return u.String(), nil
}

func relayPort(relayBase string) (string, error) {
	base := strings.TrimSpace(relayBase)
	if base == "" {
		base = defaultDyhubRelay
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		u, err = url.Parse("http://" + base)
		if err != nil {
			return "", err
		}
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "8757"
		}
	}
	return port, nil
}

func dyhubHealthy(ctx context.Context, relayBase string) bool {
	statsURL, err := relayStatsURL(relayBase)
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statsURL, nil)
	if err != nil {
		return false
	}
	resp, err := dyhubHTTPCli.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func waitDyhubHealthy(ctx context.Context, relayBase string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if dyhubHealthy(ctx, relayBase) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("dyhub did not become ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func runCommand(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "npm_config fund=false", "npm_config audit=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dyhubTarballURLs() []string {
	if u := strings.TrimSpace(os.Getenv("CYBERVERSE_DYHUB_TARBALL_URL")); u != "" {
		return []string{u}
	}
	return []string{
		"https://ghfast.top/https://github.com/ymstar/dyhub/archive/refs/heads/main.tar.gz",
		"https://github.com/ymstar/dyhub/archive/refs/heads/main.tar.gz",
	}
}

func downloadDyhubTarball(ctx context.Context, dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tarPath := filepath.Join(parent, "dyhub-main.tar.gz")
	var lastErr error
	for _, rawURL := range dyhubTarballURLs() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := dyhubDownloadCli.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
			continue
		}
		f, err := os.Create(tarPath)
		if err != nil {
			resp.Body.Close()
			return err
		}
		_, copyErr := io.Copy(f, resp.Body)
		_ = resp.Body.Close()
		_ = f.Close()
		if copyErr != nil {
			lastErr = copyErr
			continue
		}
		if err := extractDyhubTarball(tarPath, dir); err != nil {
			lastErr = err
			continue
		}
		_ = os.Remove(tarPath)
		log.Printf("dyhub: installed from tarball %s", rawURL)
		return nil
	}
	return fmt.Errorf("dyhub tarball download failed: %w", lastErr)
}

func extractDyhubTarball(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := hdr.Name
		if i := strings.Index(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(destDir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			_ = out.Close()
		}
	}
	if _, err := os.Stat(filepath.Join(destDir, "package.json")); err != nil {
		return fmt.Errorf("dyhub tarball missing package.json")
	}
	return nil
}

func bootstrapDyhub(ctx context.Context, dir string) error {
	pkg := filepath.Join(dir, "package.json")
	if _, err := os.Stat(pkg); err != nil {
		if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
			return err
		}
		_ = os.RemoveAll(dir)
		gitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		log.Printf("dyhub: cloning into %s", dir)
		cloneErr := runCommand(gitCtx, filepath.Dir(dir), "git", "clone", "--depth", "1", "https://github.com/ymstar/dyhub.git", dir)
		cancel()
		if cloneErr != nil {
			log.Printf("dyhub: git clone failed, trying tarball mirror: %v", cloneErr)
			if err := downloadDyhubTarball(ctx, dir); err != nil {
				return fmt.Errorf("git clone and tarball both failed: git=%v tarball=%w", cloneErr, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err != nil {
		log.Printf("dyhub: npm install in %s (first run may take a few minutes)", dir)
		if err := runCommand(ctx, dir, "npm", "install"); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "dist")); err != nil {
		log.Printf("dyhub: npm run build in %s", dir)
		if err := runCommand(ctx, dir, "npm", "run", "build"); err != nil {
			return err
		}
	}
	return nil
}

func startDyhubProcess(dir, port string) (*exec.Cmd, error) {
	logPath := filepath.Join(filepath.Dir(dir), "dyhub.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	collector := strings.TrimSpace(os.Getenv("CYBERVERSE_DYHUB_COLLECTOR"))
	if collector == "" {
		collector = "lightweight"
	}

	cmd := exec.Command("npm", "start")
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"DYHUB_PORT="+port,
		"DYHUB_HOST=127.0.0.1",
		"DYHUB_COLLECTOR="+collector,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
	}()
	return cmd, nil
}

// EnsureDyhubRelay starts a local DyHub instance when the relay URL is unreachable.
func EnsureDyhubRelay(ctx context.Context, relayBase, repoRoot string) error {
	if dyhubHealthy(ctx, relayBase) {
		return nil
	}

	port, err := relayPort(relayBase)
	if err != nil {
		return err
	}
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum <= 0 || portNum > 65535 {
		return fmt.Errorf("invalid dyhub relay port %q", port)
	}
	// Only auto-start when targeting loopback.
	relayForHost := strings.TrimSpace(relayBase)
	if relayForHost == "" {
		relayForHost = defaultDyhubRelay
	}
	u, err := url.Parse(relayForHost)
	if err != nil || u.Host == "" {
		u, err = url.Parse("http://" + relayForHost)
	}
	if err != nil {
		return err
	}
	h := strings.ToLower(u.Hostname())
	if h != "" && h != "127.0.0.1" && h != "localhost" && h != "::1" {
		return fmt.Errorf("dyhub relay %s is not reachable; remote relays must be started manually", relayBase)
	}

	dir := dyhubInstallDir(repoRoot)

	dyhubMu.Lock()
	defer dyhubMu.Unlock()

	if dyhubHealthy(ctx, relayBase) {
		return nil
	}
	if dyhubProc != nil && dyhubProc.Process != nil && dyhubRepo == dir {
		waitCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		if err := waitDyhubHealthy(waitCtx, relayBase); err == nil {
			return nil
		}
	}

	bootstrapCtx, cancelBootstrap := context.WithTimeout(ctx, 12*time.Minute)
	defer cancelBootstrap()
	if err := bootstrapDyhub(bootstrapCtx, dir); err != nil {
		return fmt.Errorf("prepare dyhub: %w", err)
	}

	cmd, err := startDyhubProcess(dir, port)
	if err != nil {
		return fmt.Errorf("start dyhub: %w", err)
	}
	dyhubProc = cmd
	dyhubRepo = dir

	readyCtx, cancelReady := context.WithTimeout(ctx, 90*time.Second)
	defer cancelReady()
	if err := waitDyhubHealthy(readyCtx, relayBase); err != nil {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		dyhubProc = nil
		dyhubRepo = ""
		return err
	}
	log.Printf("dyhub relay ready at %s (log: %s)", relayBase, filepath.Join(filepath.Dir(dir), "dyhub.log"))
	return nil
}
