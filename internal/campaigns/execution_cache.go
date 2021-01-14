package campaigns

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

func UserCacheDir() (string, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userCacheDir, "sourcegraph-src"), nil
}

type ExecutionCacheKey struct {
	*Task
}

// Key converts the key into a string form that can be used to uniquely identify
// the cache key in a more concise form than the entire Task.
func (key ExecutionCacheKey) Key() (string, error) {
	// We have to resolve the step environments and include them in the cache
	// key to ensure that the cache is properly invalidated when an environment
	// variable changes.
	//
	// Note that we don't base the cache key on the entire global environment:
	// if an unrelated environment variable changes, that's fine. We're only
	// interested in the ones that actually make it into the step container.
	global := os.Environ()
	envs := make([]map[string]string, len(key.Task.Steps))
	for i, step := range key.Task.Steps {
		env, err := step.Env.Resolve(global)
		if err != nil {
			return "", errors.Wrapf(err, "resolving environment for step %d", i)
		}
		envs[i] = env
	}

	raw, err := json.Marshal(struct {
		*Task
		Environments []map[string]string
	}{
		Task:         key.Task,
		Environments: envs,
	})
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(hash[:16]), nil
}

type ExecutionCache interface {
	Get(ctx context.Context, key ExecutionCacheKey) (diff string, outputs map[string]interface{}, found bool, err error)
	Set(ctx context.Context, key ExecutionCacheKey, diff string, outputs map[string]interface{}) error
	Clear(ctx context.Context, key ExecutionCacheKey) error
}

type ExecutionDiskCache struct {
	Dir string
}

func (c ExecutionDiskCache) cacheFilePath(key ExecutionCacheKey) (string, error) {
	keyString, err := key.Key()
	if err != nil {
		return "", errors.Wrap(err, "calculating execution cache key")
	}

	return filepath.Join(c.Dir, keyString+".v3.json"), nil
}

type executionResult struct {
	Diff    string                 `json:"diff"`
	Outputs map[string]interface{} `json:"outputs"`
}

func (c ExecutionDiskCache) Get(ctx context.Context, key ExecutionCacheKey) (string, map[string]interface{}, bool, error) {
	outputs := map[string]interface{}{}

	path, err := c.cacheFilePath(key)
	if err != nil {
		return "", outputs, false, err
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil // treat as not-found
		}
		return "", outputs, false, err
	}

	// There are now three different cache versions out in the wild and to be
	// backwards compatible we read all of them.
	// 2-step plan for taming this:
	//  1) February 2021: deprecate old caches by deleting the files when
	//                    detected and reporting as cache-miss.
	//  2) May 2021 (two releases later): remove handling of these files from
	//                                    this function
	switch {
	case strings.HasSuffix(path, ".v3.json"):
		// v3 of the cache: we cache the diff and the outputs produced by the step.
		var result executionResult
		if err := json.Unmarshal(data, &result); err != nil {
			// Delete the invalid data to avoid causing an error for next time.
			if err := os.Remove(path); err != nil {
				return "", outputs, false, errors.Wrap(err, "while deleting cache file with invalid JSON")
			}
			return "", outputs, false, errors.Wrapf(err, "reading cache file %s", path)
		}
		return result.Diff, result.Outputs, true, nil

	case strings.HasSuffix(path, ".diff"):
		// v2 of the cache: we only cached the diff, since that's the
		// only bit of data we were interested in.
		return string(data), outputs, true, nil

	case strings.HasSuffix(path, ".json"):
		// v1 of the cache: we cached the complete ChangesetSpec instead of just the diffs.
		var result ChangesetSpec
		if err := json.Unmarshal(data, &result); err != nil {
			// Delete the invalid data to avoid causing an error for next time.
			if err := os.Remove(path); err != nil {
				return "", outputs, false, errors.Wrap(err, "while deleting cache file with invalid JSON")
			}
			return "", outputs, false, errors.Wrapf(err, "reading cache file %s", path)
		}
		if len(result.Commits) != 1 {
			return "", outputs, false, errors.New("cached result has no commits")
		}
		return result.Commits[0].Diff, outputs, true, nil
	}

	return "", outputs, false, fmt.Errorf("unknown file format for cache file %q", path)
}

func (c ExecutionDiskCache) Set(ctx context.Context, key ExecutionCacheKey, diff string, outputs map[string]interface{}) error {
	path, err := c.cacheFilePath(key)
	if err != nil {
		return err
	}

	res := &executionResult{Diff: diff, Outputs: outputs}
	raw, err := json.Marshal(&res)
	if err != nil {
		return errors.Wrap(err, "serializing execution result to JSON")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	return ioutil.WriteFile(path, raw, 0600)
}

func (c ExecutionDiskCache) Clear(ctx context.Context, key ExecutionCacheKey) error {
	path, err := c.cacheFilePath(key)
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	return os.Remove(path)
}

// ExecutionNoOpCache is an implementation of actionExecutionCache that does not store or
// retrieve cache entries.
type ExecutionNoOpCache struct{}

func (ExecutionNoOpCache) Get(ctx context.Context, key ExecutionCacheKey) (diff string, outputs map[string]interface{}, found bool, err error) {
	return "", map[string]interface{}{}, false, nil
}

func (ExecutionNoOpCache) Set(ctx context.Context, key ExecutionCacheKey, diff string, outputs map[string]interface{}) error {
	return nil
}

func (ExecutionNoOpCache) Clear(ctx context.Context, key ExecutionCacheKey) error {
	return nil
}
