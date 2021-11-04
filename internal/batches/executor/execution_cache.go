package executor

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/execution"
	"github.com/sourcegraph/sourcegraph/lib/batches/execution/cache"
)

func resolveStepsEnvironment(steps []batcheslib.Step) ([]map[string]string, error) {
	// We have to resolve the step environments and include them in the cache
	// key to ensure that the cache is properly invalidated when an environment
	// variable changes.
	//
	// Note that we don't base the cache key on the entire global environment:
	// if an unrelated environment variable changes, that's fine. We're only
	// interested in the ones that actually make it into the step container.
	global := os.Environ()
	envs := make([]map[string]string, len(steps))
	for i, step := range steps {
		// TODO: This should also render templates inside env vars.
		env, err := step.Env.Resolve(global)
		if err != nil {
			return nil, errors.Wrapf(err, "resolving environment for step %d", i)
		}
		envs[i] = env
	}
	return envs, nil
}

func marshalHash(t *Task, envs []map[string]string) (string, error) {
	raw, err := json.Marshal(struct {
		*Task
		Environments []map[string]string
	}{
		Task:         t,
		Environments: envs,
	})
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(hash[:16]), nil
}

func NewDiskCache(dir string) cache.Cache {
	if dir == "" {
		return &ExecutionNoOpCache{}
	}

	return &ExecutionDiskCache{dir}
}

type ExecutionDiskCache struct {
	Dir string
}

const cacheFileExt = ".json"

func (c ExecutionDiskCache) cacheFilePath(key cache.Keyer) (string, error) {
	keyString, err := key.Key()
	if err != nil {
		return "", errors.Wrap(err, "calculating execution cache key")
	}

	return filepath.Join(c.Dir, key.Slug(), keyString+cacheFileExt), nil
}

func (c ExecutionDiskCache) Get(ctx context.Context, key cache.Keyer) (execution.Result, bool, error) {
	var result execution.Result

	path, err := c.cacheFilePath(key)
	if err != nil {
		return result, false, err
	}

	found, err := c.readCacheFile(path, &result)

	return result, found, err
}

func (c ExecutionDiskCache) readCacheFile(path string, result interface{}) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal(data, result); err != nil {
		// Delete the invalid data to avoid causing an error for next time.
		if err := os.Remove(path); err != nil {
			return false, errors.Wrap(err, "while deleting cache file with invalid JSON")
		}
		return false, errors.Wrapf(err, "reading cache file %s", path)
	}

	return true, nil
}

func (c ExecutionDiskCache) writeCacheFile(path string, result interface{}) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return errors.Wrap(err, "serializing cache content to JSON")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	return os.WriteFile(path, raw, 0600)
}

func (c ExecutionDiskCache) Set(ctx context.Context, key cache.Keyer, result execution.Result) error {
	path, err := c.cacheFilePath(key)
	if err != nil {
		return err
	}

	return c.writeCacheFile(path, &result)
}

func (c ExecutionDiskCache) Clear(ctx context.Context, key cache.Keyer) error {
	path, err := c.cacheFilePath(key)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	return os.Remove(path)
}

func (c ExecutionDiskCache) GetStepResult(ctx context.Context, key cache.Keyer) (execution.AfterStepResult, bool, error) {
	var result execution.AfterStepResult
	path, err := c.cacheFilePath(key)
	if err != nil {
		return result, false, err
	}

	found, err := c.readCacheFile(path, &result)
	if err != nil {
		return result, false, err
	}

	return result, found, nil
}

func (c ExecutionDiskCache) SetStepResult(ctx context.Context, key cache.Keyer, result execution.AfterStepResult) error {
	path, err := c.cacheFilePath(key)
	if err != nil {
		return err
	}

	return c.writeCacheFile(path, &result)
}

// ExecutionNoOpCache is an implementation of ExecutionCache that does not store or
// retrieve cache entries.
type ExecutionNoOpCache struct{}

func (ExecutionNoOpCache) Get(ctx context.Context, key cache.Keyer) (result execution.Result, found bool, err error) {
	return execution.Result{}, false, nil
}

func (ExecutionNoOpCache) Set(ctx context.Context, key cache.Keyer, result execution.Result) error {
	return nil
}

func (ExecutionNoOpCache) Clear(ctx context.Context, key cache.Keyer) error {
	return nil
}

func (ExecutionNoOpCache) SetStepResult(ctx context.Context, key cache.Keyer, result execution.AfterStepResult) error {
	return nil
}

func (ExecutionNoOpCache) GetStepResult(ctx context.Context, key cache.Keyer) (execution.AfterStepResult, bool, error) {
	return execution.AfterStepResult{}, false, nil
}

type JSONCacheWriter interface {
	WriteExecutionResult(key string, value execution.Result)
	WriteAfterStepResult(key string, value execution.AfterStepResult)
}

type JSONLinesCache struct {
	Writer JSONCacheWriter
}

func (c *JSONLinesCache) Get(ctx context.Context, key cache.Keyer) (result execution.Result, found bool, err error) {
	// noop
	return execution.Result{}, false, nil
}

func (c *JSONLinesCache) Set(ctx context.Context, key cache.Keyer, result execution.Result) error {
	k, err := key.Key()
	if err != nil {
		return err
	}

	c.Writer.WriteExecutionResult(k, result)

	return nil
}

func (c *JSONLinesCache) SetStepResult(ctx context.Context, key cache.Keyer, result execution.AfterStepResult) error {
	k, err := key.Key()
	if err != nil {
		return err
	}

	c.Writer.WriteAfterStepResult(k, result)

	return nil
}

func (c *JSONLinesCache) GetStepResult(ctx context.Context, key cache.Keyer) (result execution.AfterStepResult, found bool, err error) {
	// noop
	return execution.AfterStepResult{}, false, nil
}

func (c *JSONLinesCache) Clear(ctx context.Context, key cache.Keyer) error {
	// noop
	return nil
}
