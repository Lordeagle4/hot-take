package mcp_test

import (
	"testing"

	"github.com/Lordeagle4/hot-take/mcp"
)

func TestLocalToolName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		namespace string
		remote    string
		want      string
		wantErr   bool
	}{
		{name: "plain", namespace: "github", remote: "issues_search", want: "github__issues_search"},
		{name: "normalised", namespace: "google-drive", remote: "files.search", want: "google_drive__files_search"},
		{name: "bad namespace", namespace: "Bad", remote: "search", wantErr: true},
		{name: "bad remote", namespace: "good", remote: "not valid", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := mcp.LocalToolName(test.namespace, test.remote)
			if (err != nil) != test.wantErr {
				t.Fatalf("LocalToolName() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("LocalToolName() = %q, want %q", got, test.want)
			}
		})
	}
}
