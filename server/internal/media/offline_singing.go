package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultHDemucsCheckpoint = "checkpoints/hdemucs/hdemucs_high_trained.pt"

// DefaultSingActionPrompt is written to LiveAct when sing drive has no custom prompt.
const DefaultSingActionPrompt = "一个人坐着面向镜头，半身构图固定，像唱歌一样口型随人声，头颈轻微随节奏即可。"

var singTitlePrefixes = []string{"唱歌-", "sing-", "Sing-"}

// BuildOfflineLiveSingVideoTitle returns 唱歌-{song name} for offline video / live song lists.
func BuildOfflineLiveSingVideoTitle(songName string) string {
	base := strings.TrimSpace(songName)
	if base == "" {
		return ""
	}
	for _, prefix := range singTitlePrefixes {
		if strings.HasPrefix(base, prefix) {
			return base
		}
	}
	return "唱歌-" + base
}

func ResolveHDemucsCheckpoint() (string, error) {
	if env := strings.TrimSpace(os.Getenv("CYBERVERSE_HDEMUCS_CHECKPOINT")); env != "" {
		if _, err := os.Stat(env); err != nil {
			return "", fmt.Errorf("hdemucs checkpoint not found: %s", env)
		}
		return env, nil
	}
	candidates := []string{
		filepath.Join("..", defaultHDemucsCheckpoint),
		defaultHDemucsCheckpoint,
		filepath.Join("/root/autodl-tmp/CyberVerse", defaultHDemucsCheckpoint),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", errors.New("hdemucs checkpoint not found; set CYBERVERSE_HDEMUCS_CHECKPOINT")
}

func ResolveOfflineSingPython() (string, error) {
	if env := strings.TrimSpace(os.Getenv("CYBERVERSE_PYTHON")); env != "" {
		return env, nil
	}
	if env := strings.TrimSpace(os.Getenv("PYTHON")); env != "" {
		return env, nil
	}
	for _, candidate := range []string{
		"/root/autodl-tmp/envs/cyberverse/bin/python",
		"python3",
	} {
		if candidate == "python3" {
			return candidate, nil
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "python3", nil
}

func repoRootFromServer() string {
	if wd, err := os.Getwd(); err == nil {
		if filepath.Base(wd) == "server" {
			return filepath.Dir(wd)
		}
		return wd
	}
	return "/root/autodl-tmp/CyberVerse"
}

func SeparateVocalsForSing(ctx context.Context, inputWav, jobDir string) (vocalsWav, backingWav string, err error) {
	checkpoint, err := ResolveHDemucsCheckpoint()
	if err != nil {
		return "", "", err
	}
	python, err := ResolveOfflineSingPython()
	if err != nil {
		return "", "", err
	}
	script := filepath.Join(repoRootFromServer(), "scripts", "offline_singing_vocal_separate.py")
	if _, err := os.Stat(script); err != nil {
		return "", "", fmt.Errorf("vocal separate script missing: %w", err)
	}
	vocalsWav = filepath.Join(jobDir, "vocals.wav")
	backingWav = filepath.Join(jobDir, "backing.wav")
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(
		runCtx,
		python,
		script,
		"--input", inputWav,
		"--checkpoint", checkpoint,
		"--vocals-out", vocalsWav,
		"--backing-out", backingWav,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		return "", "", fmt.Errorf("vocal separation failed: %w: %s", runErr, msg)
	}
	if err := requireNonEmptyFile(vocalsWav, "vocals output"); err != nil {
		return "", "", err
	}
	if err := requireNonEmptyFile(backingWav, "backing output"); err != nil {
		return "", "", err
	}
	return vocalsWav, backingWav, nil
}

func ConvertMediaToWAV(ctx context.Context, inputPath, outputPath string) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("ffmpeg is required for singing drive")
	}
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(
		runCtx,
		ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", inputPath,
		"-ac", "2",
		"-ar", "44100",
		outputPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg convert to wav: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return requireNonEmptyFile(outputPath, "converted wav")
}

func PCM16MonoFromWAV(wavPath string, targetSampleRate int) ([]byte, int, error) {
	data, err := os.ReadFile(wavPath)
	if err != nil {
		return nil, 0, err
	}
	pcm, sampleRate, err := DecodePCM16WAV(data)
	if err != nil {
		return nil, 0, err
	}
	if targetSampleRate <= 0 {
		targetSampleRate = 16000
	}
	if sampleRate != targetSampleRate {
		pcm, err = ResamplePCM16Mono(pcm, sampleRate, targetSampleRate)
		if err != nil {
			return nil, 0, err
		}
		sampleRate = targetSampleRate
	}
	if len(pcm)%2 != 0 {
		pcm = pcm[:len(pcm)-1]
	}
	return pcm, sampleRate, nil
}

func MixVideoWithBacking(ctx context.Context, videoPath, backingWav, outputPath string) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return errors.New("ffmpeg is required for singing drive")
	}
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	tmpOut := outputPath + ".sing-mux.tmp.mp4"
	cmd := exec.CommandContext(
		runCtx,
		ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", videoPath,
		"-i", backingWav,
		"-filter_complex", "[0:a][1:a]amix=inputs=2:duration=first:dropout_transition=0[aout]",
		"-map", "0:v:0",
		"-map", "[aout]",
		"-c:v", "copy",
		"-c:a", "aac",
		"-b:a", "192k",
		"-shortest",
		tmpOut,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg mix backing: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := os.Rename(tmpOut, outputPath); err != nil {
		_ = os.Remove(tmpOut)
		return err
	}
	return nil
}

func requireNonEmptyFile(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s is empty", label)
	}
	return nil
}
