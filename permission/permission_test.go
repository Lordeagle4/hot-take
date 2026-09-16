package permission_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Lordeagle4/hot-take/permission"
	"github.com/Lordeagle4/hot-take/tool"
)

func TestPolicyAuthorizesByMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     permission.Mode
		approved bool
		wantErr  bool
	}{
		{name: "allow", mode: permission.Allow},
		{name: "deny", mode: permission.Deny, wantErr: true},
		{name: "approved", mode: permission.Ask, approved: true},
		{name: "rejected", mode: permission.Ask, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			policy, err := permission.NewPolicy(permission.Deny, []permission.Rule{{Tool: "remote_search", Mode: test.mode}}, permission.ApproveFunc(func(context.Context, permission.Request) (bool, error) {
				return test.approved, nil
			}))
			if err != nil {
				t.Fatalf("NewPolicy() error = %v", err)
			}
			err = policy.Authorize(context.Background(), permission.Request{Call: tool.Call{Name: "remote_search"}})
			if (err != nil) != test.wantErr {
				t.Fatalf("Authorize() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr && !errors.Is(err, permission.ErrDenied) {
				t.Fatalf("Authorize() error = %v, want ErrDenied", err)
			}
		})
	}
}

func TestNewPolicyRejectsAskWithoutApprover(t *testing.T) {
	t.Parallel()

	if _, err := permission.NewPolicy(permission.Ask, nil, nil); err == nil {
		t.Fatal("NewPolicy() error = nil, want error")
	}
}
