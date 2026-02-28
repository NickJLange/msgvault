package encryption

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ExecProvider runs an external command and reads a base64-encoded key from stdout.
type ExecProvider struct {
	command string
}

// NewExecProvider returns a provider that runs the given shell command to obtain a key.
func NewExecProvider(command string) *ExecProvider {
	return &ExecProvider{command: command}
}

// GetKey runs the configured command and decodes the key from its stdout.
func (p *ExecProvider) GetKey(ctx context.Context) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", p.command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running key command %q: %w (stderr: %s)", p.command, err, strings.TrimSpace(stderr.String()))
	}

	raw := strings.TrimSpace(stdout.String())
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding output of key command %q: %w", p.command, err)
	}
	if err := ValidateKey(key); err != nil {
		return nil, fmt.Errorf("key command %q: %w", p.command, err)
	}
	return key, nil
}

// Name returns the provider name.
func (p *ExecProvider) Name() string { return "exec" }
