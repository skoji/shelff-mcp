package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/skoji/shelff-go/internal/mcpserver"
)

func TestResolveRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		envRoot string
		want    string
		wantErr error
	}{
		{
			name: "flag",
			args: []string{"--root", "/library"},
			want: "/library",
		},
		{
			name:    "env fallback",
			envRoot: "/env-library",
			want:    "/env-library",
		},
		{
			name:    "flag wins over env",
			args:    []string{"--root", "/flag-library"},
			envRoot: "/env-library",
			want:    "/flag-library",
		},
		{
			name:    "missing root",
			wantErr: mcpserver.ErrRootNotProvided,
		},
		{
			name:    "unexpected positional arg",
			args:    []string{"extra"},
			wantErr: errors.New("unexpected arguments"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, err := resolveRoot(tt.args, func(string) string { return tt.envRoot })
			if tt.wantErr != nil {
				if errors.Is(tt.wantErr, mcpserver.ErrRootNotProvided) {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("resolveRoot error = %v, want %v", err, tt.wantErr)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Fatalf("resolveRoot error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRoot error = %v", err)
			}
			if root != tt.want {
				t.Fatalf("resolveRoot = %q, want %q", root, tt.want)
			}
		})
	}
}
