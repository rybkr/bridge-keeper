package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Registry) GoVersion(ctx context.Context) (string, error) {
	return r.runSubprocess(ctx, subprocessSpec{name: "go", args: []string{"version"}})
}

func (r *Registry) RustVersion(ctx context.Context) (string, error) {
	return r.runSubprocess(ctx, subprocessSpec{name: "cargo", args: []string{"--version"}})
}

func (r *Registry) PackageList(ctx context.Context, req PackageListArgs) (string, error) {
	manager, dir, err := r.resolvePackageManager(req.Manager, req.Path)
	if err != nil {
		return "", err
	}
	switch manager {
	case "go":
		return r.runSubprocess(ctx, subprocessSpec{name: "go", args: []string{"list", "-m", "all"}, dir: dir})
	case "cargo":
		return r.runSubprocess(ctx, subprocessSpec{name: "cargo", args: []string{"metadata", "--format-version", "1"}, dir: dir})
	default:
		return "", fmt.Errorf("unsupported package manager %q", manager)
	}
}

func (r *Registry) PackageQuery(ctx context.Context, req PackageQueryArgs) (string, error) {
	manager, dir, err := r.resolvePackageManager(req.Manager, req.Path)
	if err != nil {
		return "", err
	}
	pkg := strings.TrimSpace(req.Package)
	if pkg == "" {
		return "", fmt.Errorf("package is required")
	}
	switch manager {
	case "go":
		return r.runSubprocess(ctx, subprocessSpec{name: "go", args: []string{"list", "-m", "-versions", pkg}, dir: dir})
	case "cargo":
		return r.runSubprocess(ctx, subprocessSpec{name: "cargo", args: []string{"search", pkg, "--limit", "10"}, dir: dir})
	default:
		return "", fmt.Errorf("unsupported package manager %q", manager)
	}
}

func (r *Registry) PackageInstall(ctx context.Context, req PackageInstallArgs) (string, error) {
	manager, dir, err := r.resolvePackageManager(req.Manager, req.Path)
	if err != nil {
		return "", err
	}
	pkg := strings.TrimSpace(req.Package)
	if pkg == "" {
		return "", fmt.Errorf("package is required")
	}
	version := strings.TrimSpace(req.Version)

	switch manager {
	case "go":
		target := pkg
		if version != "" {
			target += "@" + version
		}
		return r.runSubprocess(ctx, subprocessSpec{name: "go", args: []string{"get", target}, dir: dir})
	case "cargo":
		args := []string{"add", pkg}
		if version != "" {
			args = append(args, "--vers", version)
		}
		return r.runSubprocess(ctx, subprocessSpec{name: "cargo", args: args, dir: dir})
	default:
		return "", fmt.Errorf("unsupported package manager %q", manager)
	}
}

func (r *Registry) PackageUpdate(ctx context.Context, req PackageUpdateArgs) (string, error) {
	manager, dir, err := r.resolvePackageManager(req.Manager, req.Path)
	if err != nil {
		return "", err
	}
	pkg := strings.TrimSpace(req.Package)

	switch manager {
	case "go":
		args := []string{"get", "-u", "./..."}
		if pkg != "" {
			args = []string{"get", pkg + "@latest"}
		}
		return r.runSubprocess(ctx, subprocessSpec{name: "go", args: args, dir: dir})
	case "cargo":
		args := []string{"update"}
		if pkg != "" {
			args = append(args, "-p", pkg)
		}
		return r.runSubprocess(ctx, subprocessSpec{name: "cargo", args: args, dir: dir})
	default:
		return "", fmt.Errorf("unsupported package manager %q", manager)
	}
}

func (r *Registry) resolvePackageManager(manager string, dir string) (string, string, error) {
	manager = strings.ToLower(strings.TrimSpace(manager))
	if strings.TrimSpace(dir) == "" && r != nil {
		dir = r.WorkspaceRoot
	}
	if strings.TrimSpace(dir) == "" {
		dir = "."
	}

	if manager == "" {
		detected, err := detectPackageManager(dir)
		if err != nil {
			return "", "", err
		}
		manager = detected
	}

	switch manager {
	case "go", "cargo":
		return manager, dir, nil
	default:
		return "", "", fmt.Errorf("unsupported package manager %q", manager)
	}
}

func detectPackageManager(dir string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "go", nil
	}
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return "cargo", nil
	}
	return "", fmt.Errorf("package manager is required when no go.mod or Cargo.toml exists in %s", dir)
}
