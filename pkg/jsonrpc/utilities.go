package jsonrpc

import (
	"fmt"
	"regexp"
	"strings"
)

// ExtractMustaches returns a map containing all keys found inside {mustaches}
// as keys with true values (useful as a set).
func ExtractMustaches(s string) map[string]bool {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(s, -1)

	m := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			key := strings.TrimSpace(match[1])
			if key != "" {
				m[key] = false
			}
		}
	}
	return m
}

func makePath(path string, v ...interface{}) (string, error) {
	result := path
	paramIndex := 0

	for {
		start := -1
		end := -1

		// Find the next parameter enclosed in {}
		for i := 0; i < len(result); i++ {
			if result[i] == '{' {
				start = i
			} else if result[i] == '}' && start != -1 {
				end = i
				break
			}
		}

		// No more parameters found
		if start == -1 || end == -1 {
			break
		}

		// Check if we have enough parameters
		if paramIndex >= len(v) {
			return "", fmt.Errorf("not enough parameters: found %d parameter(s) in path but only %d value(s) provided", paramIndex+1, len(v))
		}

		// Replace the parameter with the value
		result = result[:start] + fmt.Sprintf("%v", v[paramIndex]) + result[end+1:]
		paramIndex++
	}

	return result, nil
}

func makePathKV(path string, kv map[string]interface{}) (string, error) {
	keys := ExtractMustaches(path)

	result := path
	for key := range keys {
		if value, ok := kv[key]; ok {
			keys[key] = true

			// consumed
			delete(kv, key)
			delete(keys, key)
			result = strings.ReplaceAll(result, "{"+key+"}", fmt.Sprintf("%v", value))
		}
	}

	if len(keys) > 0 {
		return result, fmt.Errorf("kv did not provide all required parameters")
	}

	if len(kv) > 0 {
		return result, fmt.Errorf("path did not consume all keys parameters")
	}

	return result, nil
}

func MakePath(path string, v ...interface{}) string {
	path, err := makePath(path, v...)
	if err != nil {
		panic(fmt.Sprintf("Path: %s Err:%s", path, err))
	}
	return path
}

func MakePathKV(path string, kv map[string]interface{}) string {
	result, err := makePathKV(path, kv)
	if err != nil {
		panic(fmt.Sprintf("Path: %s Err:%s", path, err))
	}
	return result
}
