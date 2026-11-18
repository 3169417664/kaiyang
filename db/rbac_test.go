package db

import "testing"

func TestRBACRolePermissionSetsAreValid(t *testing.T) {
	for _, list := range [][]string{operatorPermissions, viewerPermissions} {
		for _, key := range list {
			if _, ok := RBACPermissions[key]; !ok {
				t.Fatalf("角色权限 %q 不在权限目录中", key)
			}
		}
	}
}

func TestOperatorCannotManagePlatform(t *testing.T) {
	set := map[string]bool{}
	for _, k := range operatorPermissions {
		set[k] = true
	}
	for _, forbidden := range []string{
		"rbac:write", "rbac:read",
		"config:write", "config:read",
		"llm:write", "llm:read",
		"agent:write", "agent:read",
		"mcp:write", "mcp:read",
		"tool:write",
		"c2:read", "c2:write",
		"hostenv:read", "hostenv:write",
	} {
		if set[forbidden] {
			t.Fatalf("operator 不应拥有 %q", forbidden)
		}
	}
}

func TestViewerIsReadOnlyAndCannotSeeAuditOrCredentials(t *testing.T) {
	set := map[string]bool{}
	for _, k := range viewerPermissions {
		set[k] = true
	}
	for key := range RBACPermissions {
		if len(key) > 5 && key[len(key)-5:] == ":write" && set[key] {
			t.Fatalf("viewer 不应拥有写权限 %q", key)
		}
	}
	for _, forbidden := range []string{"log:read", "credential:read", "rbac:read", "config:read", "llm:read", "agent:read", "mcp:read", "c2:read", "hostenv:read"} {
		if set[forbidden] {
			t.Fatalf("viewer 不应拥有 %q", forbidden)
		}
	}
}
