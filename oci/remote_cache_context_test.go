package oci

import (
	"context"
	"strings"
	"testing"
)

func TestNewRemoteCacheFromContextDisabledWithoutValues(t *testing.T) {
	ctx := context.Background()

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextDisabledWithoutValues")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: not configured")
	t.Logf("  remote cache repository: not configured")
	t.Logf("  remote cache insecure: not configured")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when registry and repository are not configured")
	}
}

func TestNewRemoteCacheFromContextDisabledWithEmptyValues(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "")
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "")
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, false)

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextDisabledWithEmptyValues")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: empty string")
	t.Logf("  remote cache repository: empty string")
	t.Logf("  remote cache insecure: false")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when registry and repository are empty")
	}
}

func TestNewRemoteCacheFromContextDisabledWithOnlyRegistry(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "localhost:5000")

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextDisabledWithOnlyRegistry")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: localhost:5000")
	t.Logf("  remote cache repository: not configured")
	t.Logf("  remote cache insecure: not configured")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when repository is not configured")
	}
}

func TestNewRemoteCacheFromContextDisabledWithOnlyRepository(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "2dfs/cache")

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextDisabledWithOnlyRepository")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: not configured")
	t.Logf("  remote cache repository: 2dfs/cache")
	t.Logf("  remote cache insecure: not configured")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when registry is not configured")
	}
}

func TestNewRemoteCacheFromContextEnabled(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "localhost:5000")
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "2dfs/cache")
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, true)

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextEnabled")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: localhost:5000")
	t.Logf("  remote cache repository: 2dfs/cache")
	t.Logf("  remote cache insecure: true")
	t.Logf("EXPECTED:")
	t.Logf("  error: nil")
	t.Logf("  remote cache: non-nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is non-nil: %v", remoteCache != nil)

	if err != nil {
		t.Fatalf("newRemoteCacheFromContext() returned error: %v", err)
	}

	if remoteCache == nil {
		t.Fatal("expected remote cache to be created when registry and repository are configured")
	}
}

func TestNewRemoteCacheFromContextRejectsInvalidInsecureValue(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "localhost:5000")
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "2dfs/cache")
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, "true")

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextRejectsInvalidInsecureValue")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: localhost:5000")
	t.Logf("  remote cache repository: 2dfs/cache")
	t.Logf("  remote cache insecure: string(\"true\")")
	t.Logf("EXPECTED:")
	t.Logf("  error: contains \"invalid remote cache insecure value\"")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err == nil {
		t.Fatal("expected error for non-bool remote cache insecure value, got nil")
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when insecure value is invalid")
	}

	if !strings.Contains(err.Error(), "invalid remote cache insecure value") {
		t.Fatalf("expected invalid insecure value error, got: %v", err)
	}
}

func TestNewRemoteCacheFromContextRejectsInvalidRegistryValue(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, 123)
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, "2dfs/cache")
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, false)

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextRejectsInvalidRegistryValue")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: int(123)")
	t.Logf("  remote cache repository: 2dfs/cache")
	t.Logf("  remote cache insecure: false")
	t.Logf("EXPECTED:")
	t.Logf("  error: contains \"invalid remote cache registry or repository value\"")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err == nil {
		t.Fatal("expected error for non-string remote cache registry value, got nil")
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when registry value is invalid")
	}

	if !strings.Contains(err.Error(), "invalid remote cache registry or repository value") {
		t.Fatalf("expected invalid registry/repository value error, got: %v", err)
	}
}

func TestNewRemoteCacheFromContextRejectsInvalidRepositoryValue(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RemoteCacheRegistryContextKey, "localhost:5000")
	ctx = context.WithValue(ctx, RemoteCacheRepositoryContextKey, 456)
	ctx = context.WithValue(ctx, RemoteCacheInsecureContextKey, false)

	remoteCache, err := newRemoteCacheFromContext(ctx)

	t.Logf("TEST: NewRemoteCacheFromContextRejectsInvalidRepositoryValue")
	t.Logf("INPUT:")
	t.Logf("  remote cache registry: localhost:5000")
	t.Logf("  remote cache repository: int(456)")
	t.Logf("  remote cache insecure: false")
	t.Logf("EXPECTED:")
	t.Logf("  error: contains \"invalid remote cache registry or repository value\"")
	t.Logf("  remote cache: nil")
	t.Logf("OUTPUT:")
	t.Logf("  error: %v", err)
	t.Logf("  remote cache is nil: %v", remoteCache == nil)

	if err == nil {
		t.Fatal("expected error for non-string remote cache repository value, got nil")
	}

	if remoteCache != nil {
		t.Fatal("expected remote cache to be nil when repository value is invalid")
	}

	if !strings.Contains(err.Error(), "invalid remote cache registry or repository value") {
		t.Fatalf("expected invalid registry/repository value error, got: %v", err)
	}
}
