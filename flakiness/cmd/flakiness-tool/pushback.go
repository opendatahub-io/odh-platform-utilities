package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	commitAuthorName  = "flakiness-tool[bot]"
	commitAuthorEmail = "flakiness-tool[bot]@users.noreply.github.com"
)

func pushBack(ctx context.Context, quarantinePath string) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return errors.New("GITHUB_TOKEN env var is required for --push-back")
	}

	if err := gitAdd(ctx, quarantinePath); err != nil {
		return err
	}

	if clean, err := gitIsClean(ctx, quarantinePath); err != nil {
		return err
	} else if clean {
		_, _ = fmt.Fprintln(os.Stdout, "push-back: quarantine file unchanged, nothing to push")
		return nil
	}

	if err := gitCommit(ctx, quarantinePath); err != nil {
		return err
	}

	if err := gitPush(ctx, token); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(os.Stdout, "push-back: committed and pushed quarantine update")
	return nil
}

func gitAdd(ctx context.Context, path string) error {
	if err := runGit(ctx, "git", "add", "--", path); err != nil {
		return fmt.Errorf("staging %s: %w", path, err)
	}
	return nil
}

func gitIsClean(ctx context.Context, path string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--cached", "--quiet", "--", path)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("checking git status: %w", err)
}

func gitCommit(ctx context.Context, quarantinePath string) error {
	msg := fmt.Sprintf("chore: update %s (flakiness-tool)\n\nAutomated quarantine state update from flakiness-tool periodic run.", quarantinePath)
	if err := runGit(ctx, "git",
		"-c", "user.name="+commitAuthorName,
		"-c", "user.email="+commitAuthorEmail,
		"commit", "--only", "-m", msg, "--", quarantinePath); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	return nil
}

func gitPush(ctx context.Context, token string) error {
	remote, err := gitRemoteURL(ctx)
	if err != nil {
		return err
	}

	pushURL, err := httpsRemote(remote)
	if err != nil {
		return err
	}

	askpass, err := writeAskpass()
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(askpass) }()

	cmd := exec.CommandContext(ctx, "git", "push", pushURL, "HEAD") //nolint:gosec // args are controlled by this package
	cmd.Env = append(os.Environ(),
		"GIT_ASKPASS="+askpass,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PUSH_TOKEN="+token,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	return nil
}

func gitRemoteURL(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting remote URL: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// httpsRemote converts a GitHub remote URL to HTTPS with the username
// x-access-token. The password is supplied via GIT_ASKPASS at runtime,
// keeping the token out of process argv (/proc/*/cmdline).
func httpsRemote(remote string) (string, error) {
	if strings.HasPrefix(remote, "git@github.com:") {
		repo := strings.TrimPrefix(remote, "git@github.com:")
		repo = strings.TrimSuffix(repo, ".git")
		return fmt.Sprintf("https://x-access-token@github.com/%s.git", repo), nil
	}

	if strings.HasPrefix(remote, "https://github.com/") {
		repo := strings.TrimPrefix(remote, "https://github.com/")
		repo = strings.TrimSuffix(repo, ".git")
		return fmt.Sprintf("https://x-access-token@github.com/%s.git", repo), nil
	}

	return "", errors.New("unsupported remote URL format: expected github.com HTTPS or SSH")
}

func writeAskpass() (string, error) {
	f, err := os.CreateTemp("", "git-askpass-*")
	if err != nil {
		return "", fmt.Errorf("creating askpass script: %w", err)
	}

	path := f.Name()
	if _, err := f.WriteString("#!/bin/sh\nprintf '%s\\n' \"$GIT_PUSH_TOKEN\"\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("writing askpass script: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("closing askpass script: %w", err)
	}

	if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // GIT_ASKPASS helper must be executable
		_ = os.Remove(path)
		return "", fmt.Errorf("setting askpass permissions: %w", err)
	}

	return path, nil
}

func runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // args are controlled by this package
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
