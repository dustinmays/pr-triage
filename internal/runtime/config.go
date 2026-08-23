package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigFileName is the standard configuration file name for pr-triage.
const ConfigFileName = "config.yaml"

// Resolution describes the runtime or model selected after applying the fallback
// hierarchy.
type Resolution struct {
	Value  string
	Source string
}

// Name returns the resolved value (convenience alias for Value).
func (r Resolution) Name() string {
	return r.Value
}

type configFile struct {
	Runtime string `yaml:"runtime"`
	Model   string `yaml:"model"`
}

// Resolve applies the configuration hierarchy:
// explicit flag -> repo config -> user config -> built-in default.
// The resolved Resolution contains the Value and its Source ("flag", "repo", "user", "default").
func Resolve(explicit, repoDir string) (Resolution, error) {
	if explicit != "" {
		return validateResolution(explicit, "flag")
	}
	if repoDir != "" {
		name, ok, err := readRuntimeConfigFromDir(repoDir)
		if err != nil {
			return Resolution{}, err
		}
		if ok {
			return validateResolution(name, "repo")
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		name, ok, err := readRuntimeConfigFromHome(home)
		if err != nil {
			return Resolution{}, err
		}
		if ok {
			return validateResolution(name, "user")
		}
	}
	return validateResolution(DefaultName, "default")
}

// ResolveModel applies the configuration hierarchy for model selection:
// explicit flag -> repo config -> user config -> default model.
func ResolveModel(explicit, repoDir string) (Resolution, error) {
	if explicit != "" {
		return Resolution{Value: explicit, Source: "flag"}, nil
	}
	if repoDir != "" {
		model, ok, err := readModelConfigFromDir(repoDir)
		if err != nil {
			return Resolution{}, err
		}
		if ok {
			return Resolution{Value: model, Source: "repo"}, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		model, ok, err := readModelConfigFromHome(home)
		if err != nil {
			return Resolution{}, err
		}
		if ok {
			return Resolution{Value: model, Source: "user"}, nil
		}
	}
	return Resolution{Value: DefaultModel, Source: "default"}, nil
}

func validateResolution(name, source string) (Resolution, error) {
	if err := Validate(name); err != nil {
		return Resolution{}, fmt.Errorf("%s runtime config: %w", source, err)
	}
	return Resolution{Value: name, Source: source}, nil
}

func readRuntimeConfigFromDir(dir string) (string, bool, error) {
	candidates := []string{
		filepath.Join(dir, ".pr-triage", ConfigFileName),
		filepath.Join(dir, ".pr-triage.yaml"),
		filepath.Join(dir, ".pr-triage.yml"),
		filepath.Join(dir, ConfigFileName),
	}
	for _, path := range candidates {
		cfg, ok, err := readConfigFile(path)
		if err != nil {
			return "", false, err
		}
		if ok {
			if cfg.Runtime != "" {
				return cfg.Runtime, true, nil
			}
			if cfg.Model != "" {
				return cfg.Model, true, nil
			}
		}
	}
	return "", false, nil
}

func readRuntimeConfigFromHome(home string) (string, bool, error) {
	candidates := []string{
		filepath.Join(home, ".pr-triage", ConfigFileName),
		filepath.Join(home, ".pr-triage.yaml"),
		filepath.Join(home, ".pr-triage.yml"),
		filepath.Join(home, ".config", "pr-triage", ConfigFileName),
	}
	for _, path := range candidates {
		cfg, ok, err := readConfigFile(path)
		if err != nil {
			return "", false, err
		}
		if ok {
			if cfg.Runtime != "" {
				return cfg.Runtime, true, nil
			}
			if cfg.Model != "" {
				return cfg.Model, true, nil
			}
		}
	}
	return "", false, nil
}

func readModelConfigFromDir(dir string) (string, bool, error) {
	candidates := []string{
		filepath.Join(dir, ".pr-triage", ConfigFileName),
		filepath.Join(dir, ".pr-triage.yaml"),
		filepath.Join(dir, ".pr-triage.yml"),
		filepath.Join(dir, ConfigFileName),
	}
	for _, path := range candidates {
		cfg, ok, err := readConfigFile(path)
		if err != nil {
			return "", false, err
		}
		if ok && cfg.Model != "" {
			return cfg.Model, true, nil
		}
	}
	return "", false, nil
}

func readModelConfigFromHome(home string) (string, bool, error) {
	candidates := []string{
		filepath.Join(home, ".pr-triage", ConfigFileName),
		filepath.Join(home, ".pr-triage.yaml"),
		filepath.Join(home, ".pr-triage.yml"),
		filepath.Join(home, ".config", "pr-triage", ConfigFileName),
	}
	for _, path := range candidates {
		cfg, ok, err := readConfigFile(path)
		if err != nil {
			return "", false, err
		}
		if ok && cfg.Model != "" {
			return cfg.Model, true, nil
		}
	}
	return "", false, nil
}

func readConfigFile(path string) (*configFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read runtime config %s: %w", path, err)
	}
	var cfg configFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, false, fmt.Errorf("parse runtime config %s: %w", path, err)
	}
	return &cfg, true, nil
}
