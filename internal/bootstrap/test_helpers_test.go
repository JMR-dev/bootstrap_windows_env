package bootstrap

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type fakeRunner struct {
	mu         sync.Mutex
	responses  map[string]CommandResult
	calls      []string
	defaultErr bool
}

func (r *fakeRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := commandKey(name, args...)
	r.calls = append(r.calls, key)
	if result, ok := r.responses[key]; ok {
		result.Command = strings.ReplaceAll(key, "\x00", " ")
		return result
	}
	if r.defaultErr {
		return CommandResult{Command: strings.ReplaceAll(key, "\x00", " "), Err: errors.New("unexpected command")}
	}
	return CommandResult{Command: strings.ReplaceAll(key, "\x00", " ")}
}

func (r *fakeRunner) called(name string, args ...string) bool {
	key := commandKey(name, args...)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, call := range r.calls {
		if call == key {
			return true
		}
	}
	return false
}

func commandKey(name string, args ...string) string {
	return name + "\x00" + strings.Join(args, "\x00")
}
